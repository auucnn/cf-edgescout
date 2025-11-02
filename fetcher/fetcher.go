package fetcher

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// RangeSet groups IPv4 and IPv6 networks for downstream consumers.
type RangeSet struct {
	IPv4 []*net.IPNet
	IPv6 []*net.IPNet
}

// Fetcher orchestrates fetching and aggregating networks from multiple providers.
type Fetcher struct {
	factory  *ProviderFactory
	configs  []SourceConfig
	cacheDir string
	mu       sync.RWMutex
}

// New creates a fetcher using the provided HTTP client and default sources.
func New(client *http.Client) *Fetcher {
	factory := NewProviderFactory(client)
	cfgs := DefaultSources()
	return &Fetcher{factory: factory, configs: cfgs}
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

// Sources returns the configured sources (for testing).
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
