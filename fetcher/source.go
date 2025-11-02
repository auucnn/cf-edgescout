package fetcher

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Signer allows a source to modify a request before it is sent.
type Signer func(*http.Request)

// Parser parses the HTTP response into a list of IP networks.
type Parser func(context.Context, *http.Response) ([]*net.IPNet, error)

// SourceConfig describes the behaviour of a range provider.
type SourceConfig struct {
	Name        string
	Endpoints   []string
	Parser      Parser
	Signer      Signer
	RateLimit   time.Duration
	Credibility float64
}

// Validate ensures the source configuration is well formed.
func (c SourceConfig) Validate() error {
	if c.Name == "" {
		return errors.New("source name is required")
	}
	if len(c.Endpoints) == 0 {
		return fmt.Errorf("source %s has no endpoints", c.Name)
	}
	for _, endpoint := range c.Endpoints {
		if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			return fmt.Errorf("source %s endpoint %q must be HTTP or HTTPS", c.Name, endpoint)
		}
	}
	if c.Parser == nil {
		return fmt.Errorf("source %s is missing a parser", c.Name)
	}
	if c.Credibility <= 0 {
		return fmt.Errorf("source %s must declare a positive credibility", c.Name)
	}
	return nil
}

// Clone makes a shallow copy to prevent accidental mutation of the configuration.
func (c SourceConfig) Clone() SourceConfig {
	dup := c
	dup.Endpoints = append([]string{}, c.Endpoints...)
	return dup
}

// DefaultSources returns the built-in range providers.
func DefaultSources() []SourceConfig {
	return []SourceConfig{
		CloudflareSource(),
		BestIPSource(),
		UouinSource(),
	}
}

// CloudflareSource describes the primary Cloudflare published ranges.
func CloudflareSource() SourceConfig {
	return SourceConfig{
		Name:        "cloudflare",
		Endpoints:   []string{"https://www.cloudflare.com/ips-v4", "https://www.cloudflare.com/ips-v6"},
		Parser:      ParseCIDRList,
		Signer:      addDefaultUserAgent,
		RateLimit:   250 * time.Millisecond,
		Credibility: 1.0,
	}
}

// BestIPSource returns the BestIP maintained Cloudflare ranges.
func BestIPSource() SourceConfig {
	return SourceConfig{
		Name:        "bestip",
		Endpoints:   []string{"https://bestip.io/cloudflare/ips"},
		Parser:      ParseCIDRList,
		Signer:      addDefaultUserAgent,
		RateLimit:   500 * time.Millisecond,
		Credibility: 0.8,
	}
}

// UouinSource returns the uouin maintained Cloudflare ranges.
func UouinSource() SourceConfig {
	return SourceConfig{
		Name:        "uouin",
		Endpoints:   []string{"https://cf.17171.net/api/ips"},
		Parser:      ParseCIDRList,
		Signer:      addDefaultUserAgent,
		RateLimit:   500 * time.Millisecond,
		Credibility: 0.75,
	}
}

// NamedSources resolves the provided source names into configurations.
func NamedSources(names []string) ([]SourceConfig, error) {
	if len(names) == 0 {
		return nil, errors.New("no sources requested")
	}
	available := map[string]SourceConfig{}
	for _, cfg := range DefaultSources() {
		available[cfg.Name] = cfg
	}
	configs := make([]SourceConfig, 0, len(names))
	for _, name := range names {
		cfg, ok := available[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			return nil, fmt.Errorf("unknown source %q", name)
		}
		configs = append(configs, cfg.Clone())
	}
	return configs, nil
}

func addDefaultUserAgent(req *http.Request) {
	req.Header.Set("User-Agent", "cf-edgescout/1.0")
}

// ParseCIDRList consumes a newline separated body of CIDRs.
func ParseCIDRList(ctx context.Context, resp *http.Response) ([]*net.IPNet, error) {
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	var networks []*net.IPNet
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		_, network, err := net.ParseCIDR(line)
		if err != nil {
			return nil, fmt.Errorf("parse cidr %q: %w", line, err)
		}
		networks = append(networks, network)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(networks) == 0 {
		return nil, errors.New("no networks parsed")
	}
	return networks, nil
}

// ProviderFactory constructs providers with a shared HTTP client.
type ProviderFactory struct {
	client *http.Client
}

// NewProviderFactory creates a new provider factory with sane defaults.
func NewProviderFactory(client *http.Client) *ProviderFactory {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if client.Timeout == 0 {
		client.Timeout = 30 * time.Second
	}
	return &ProviderFactory{client: client}
}

// Build creates a Provider for the given source configuration.
func (f *ProviderFactory) Build(cfg SourceConfig) (*Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Provider{config: cfg, client: f.client}, nil
}

// Provider represents a concrete data source.
type Provider struct {
	config SourceConfig
	client *http.Client
	mu     sync.Mutex
	last   time.Time
}

// Fetch retrieves the ranges from the provider, using fallback endpoints if required.
func (p *Provider) Fetch(ctx context.Context) ([]RangeRecord, error) {
	var aggregated []RangeRecord
	var errs []error
	for _, endpoint := range p.config.Endpoints {
		if err := p.waitForRateLimit(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if p.config.Signer != nil {
			p.config.Signer(req)
		}
		resp, err := p.client.Do(req)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			errs = append(errs, fmt.Errorf("%s returned %d", p.config.Name, resp.StatusCode))
			continue
		}
		networks, err := p.config.Parser(ctx, resp)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		ts := time.Now().UTC()
		records := make([]RangeRecord, 0, len(networks))
		for _, network := range networks {
			records = append(records, RangeRecord{
				Network: cloneIPNet(network),
				Metadata: RangeMetadata{
					Source:      p.config.Name,
					Endpoint:    endpoint,
					RetrievedAt: ts,
					Credibility: p.config.Credibility,
				},
			})
		}
		aggregated = append(aggregated, records...)
	}
	if len(aggregated) > 0 {
		return aggregated, nil
	}
	if len(errs) == 0 {
		errs = append(errs, fmt.Errorf("all endpoints failed for %s", p.config.Name))
	}
	return nil, errors.Join(errs...)
}

func (p *Provider) waitForRateLimit(ctx context.Context) error {
	if p.config.RateLimit <= 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.last.IsZero() {
		p.last = time.Now()
		return nil
	}
	elapsed := time.Since(p.last)
	if elapsed >= p.config.RateLimit {
		p.last = time.Now()
		return nil
	}
	wait := p.config.RateLimit - elapsed
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		p.last = time.Now()
		return nil
	}
}

func cloneIPNet(n *net.IPNet) *net.IPNet {
	if n == nil {
		return nil
	}
	dup := &net.IPNet{}
	dup.Mask = append([]byte{}, n.Mask...)
	dup.IP = append([]byte{}, n.IP...)
	return dup
}

// RangeMetadata carries provenance information for a CIDR block.
type RangeMetadata struct {
	Source      string    `json:"source"`
	Endpoint    string    `json:"endpoint"`
	RetrievedAt time.Time `json:"retrieved_at"`
	Credibility float64   `json:"credibility"`
}

// RangeRecord is a single network annotated with metadata.
type RangeRecord struct {
	Network  *net.IPNet    `json:"network"`
	Metadata RangeMetadata `json:"metadata"`
}

var _ = cloneIPNet
