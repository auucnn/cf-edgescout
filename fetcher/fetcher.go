package fetcher

import (
        "context"
        "errors"
        "fmt"
        "io"
        "net"
        "net/http"
        "strings"
        "sync"
)

// RangeSet groups IPv4 and IPv6 networks for downstream consumers.
type RangeSet struct {
	IPv4    []*net.IPNet
	IPv6    []*net.IPNet
	Sources []SourceRangeSet
}

// SourceRangeSet groups networks that originate from the same upstream source.
type SourceRangeSet struct {
	Name        string
	Credibility float64
	IPv4        []*net.IPNet
	IPv6        []*net.IPNet
}

// Fetcher orchestrates fetching and aggregating networks from multiple providers.
type Fetcher struct {
	factory  *ProviderFactory
	configs  []SourceConfig
	cacheDir string
	mu       sync.RWMutex
	client   *http.Client
}

// New creates a fetcher using the provided HTTP client and default sources.
func New(client *http.Client) *Fetcher {
	factory := NewProviderFactory(client)
	cfgs := DefaultSources()
	return &Fetcher{factory: factory, configs: cfgs, client: factory.client}
}

// SetCacheDir enables persistence of aggregated results to disk.
func (f *Fetcher) SetCacheDir(dir string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cacheDir = dir
}

// CacheDir returns the configured cache directory.
func (f *Fetcher) CacheDir() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.cacheDir
}

// UseSources replaces the current source list.
func (f *Fetcher) UseSources(configs []SourceConfig) {
	copies := make([]SourceConfig, 0, len(configs))
	for _, cfg := range configs {
		copies = append(copies, cfg.Clone())
	}
	f.mu.Lock()
	f.configs = copies
	f.mu.Unlock()
}

// UseSourceNames resolves names into configurations and replaces the source list.
func (f *Fetcher) UseSourceNames(names []string) error {
	configs, err := NamedSources(names)
	if err != nil {
		return err
	}
	f.UseSources(configs)
	return nil
}

// Sources returns the configured sources (primarily for testing).
func (f *Fetcher) Sources() []SourceConfig {
	f.mu.RLock()
	defer f.mu.RUnlock()
	copies := make([]SourceConfig, 0, len(f.configs))
	for _, cfg := range f.configs {
		copies = append(copies, cfg.Clone())
	}
	return copies
}

// FetchAggregated retrieves ranges from all configured sources in parallel.
func (f *Fetcher) FetchAggregated(ctx context.Context) (AggregatedSet, error) {
	f.mu.RLock()
	configs := make([]SourceConfig, len(f.configs))
	copy(configs, f.configs)
	cacheDir := f.cacheDir
	f.mu.RUnlock()

	if len(configs) == 0 {
		return AggregatedSet{}, errors.New("no sources configured")
	}

	providers := make([]*Provider, 0, len(configs))
	for _, cfg := range configs {
		provider, err := f.factory.Build(cfg)
		if err != nil {
			return AggregatedSet{}, err
		}
		providers = append(providers, provider)
	}

	type result struct {
		records []RangeRecord
		err     error
	}

	results := make(chan result, len(providers))
	var wg sync.WaitGroup
	for _, provider := range providers {
		wg.Add(1)
		go func(p *Provider) {
			defer wg.Done()
			records, err := p.Fetch(ctx)
			results <- result{records: records, err: err}
		}(provider)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	aggregator := NewAggregator()
	var errs []error
	for res := range results {
		if len(res.records) > 0 {
			aggregator.Add(res.records)
		}
		if res.err != nil {
			errs = append(errs, res.err)
		}
	}

	set := aggregator.Result()
	if len(set.Entries) > 0 {
		if err := set.Persist(cacheDir); err != nil {
			errs = append(errs, fmt.Errorf("persist cache: %w", err))
		}
		return set, errors.Join(errs...)
	}

	if cacheDir != "" {
		cached, err := LoadAggregatedFromCache(cacheDir)
		if err == nil {
			return cached, errors.Join(errs...)
		}
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		errs = append(errs, errors.New("no results fetched"))
	}
	return AggregatedSet{}, errors.Join(errs...)
}

// Fetch retrieves the aggregated ranges and returns them as a RangeSet for legacy consumers.
func (f *Fetcher) Fetch(ctx context.Context) (RangeSet, error) {
	aggregated, err := f.FetchAggregated(ctx)
	if err != nil && len(aggregated.Entries) == 0 {
		return RangeSet{}, err
	}
	return aggregated.RangeSet(), err
}

// FetchProvider retrieves ranges for a single provider specification.
func (f *Fetcher) FetchProvider(ctx context.Context, provider ProviderSpec) (SourceRange, error) {
	ipv4, err := f.fetchEndpoint(ctx, provider.IPv4)
	if err != nil {
		return SourceRange{}, fmt.Errorf("%s ipv4: %w", provider.Name, err)
	}
	ipv6, err := f.fetchEndpoint(ctx, provider.IPv6)
	if err != nil {
		return SourceRange{}, fmt.Errorf("%s ipv6: %w", provider.Name, err)
	}
	rs := RangeSet{IPv4: ipv4, IPv6: ipv6}
	return SourceRange{Provider: provider, RangeSet: rs}, nil
}

// FetchAll retrieves ranges for the provided set of providers.
func (f *Fetcher) FetchAll(ctx context.Context, providers []ProviderSpec) ([]SourceRange, error) {
	if len(providers) == 0 {
		return nil, errors.New("没有提供方可供抓取")
	}
	results := make([]SourceRange, 0, len(providers))
	var errs []string
	for _, provider := range providers {
		source, err := f.FetchProvider(ctx, provider)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		cleaned := deduplicateRanges(source.RangeSet)
		source.RangeSet = cleaned
		results = append(results, source)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("全部数据源抓取失败: %s", strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		return results, fmt.Errorf("部分数据源抓取失败: %s", strings.Join(errs, "; "))
	}
	return results, nil
}

func (f *Fetcher) fetchEndpoint(ctx context.Context, endpoint EndpointSpec) ([]*net.IPNet, error) {
	if endpoint.URL == "" {
		return nil, nil
	}
	client := f.client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("%s 响应异常: %d %s", endpoint.URL, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	switch endpoint.Format {
	case "", FormatPlainCIDR:
		return parsePlainCIDR(resp.Body)
	case FormatJSONArray:
		return parseJSONArray(resp.Body, endpoint.JSONPath)
	default:
		return nil, fmt.Errorf("不支持的响应格式: %s", endpoint.Format)
	}
}
