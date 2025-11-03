package fetcher

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SourceKind string

const (
	SourceKindOfficial   SourceKind = "official"
	SourceKindThirdParty SourceKind = "third-party"
)

type ResponseFormat string

const (
	FormatPlainCIDR ResponseFormat = "plain_cidr"
	FormatJSONArray ResponseFormat = "json_array"
)

type EndpointSpec struct {
	URL      string
	Format   ResponseFormat
	JSONPath []string
}

type ProviderSpec struct {
	Name        string
	DisplayName string
	Kind        SourceKind
	Description string
	Weight      float64
	IPv4        EndpointSpec
	IPv6        EndpointSpec
	Enabled     bool
}

type SourceRange struct {
	Provider ProviderSpec
	RangeSet RangeSet
}

func DefaultProviders() []ProviderSpec {
	return []ProviderSpec{
		{
			Name:        "official",
			DisplayName: "Cloudflare 官方发布",
			Kind:        SourceKindOfficial,
			Description: "Cloudflare 官方公布的 IPv4/IPv6 网段",
			Weight:      1.0,
			IPv4:        EndpointSpec{URL: "https://www.cloudflare.com/ips-v4", Format: FormatPlainCIDR},
			IPv6:        EndpointSpec{URL: "https://www.cloudflare.com/ips-v6", Format: FormatPlainCIDR},
			Enabled:     true,
		},
		{
			Name:        "bestip",
			DisplayName: "BestIP 社区镜像",
			Kind:        SourceKindThirdParty,
			Description: "来自 bestip.one 的 Cloudflare 节点数据",
			Weight:      0.8,
			IPv4: EndpointSpec{
				URL:      "https://api.bestip.one/cloudflare/ipv4",
				Format:   FormatJSONArray,
				JSONPath: []string{"data"},
			},
			IPv6: EndpointSpec{
				URL:      "https://api.bestip.one/cloudflare/ipv6",
				Format:   FormatJSONArray,
				JSONPath: []string{"data"},
			},
			Enabled: true,
		},
		{
			Name:        "uouin",
			DisplayName: "UOUIN 优选节点",
			Kind:        SourceKindThirdParty,
			Description: "参考 api.uouin.com 提供的 Cloudflare 节点列表",
			Weight:      0.7,
			IPv4: EndpointSpec{
				URL:      "https://api.uouin.com/cloudflare/ipv4",
				Format:   FormatJSONArray,
				JSONPath: []string{"data", "ipv4"},
			},
			IPv6: EndpointSpec{
				URL:      "https://api.uouin.com/cloudflare/ipv6",
				Format:   FormatJSONArray,
				JSONPath: []string{"data", "ipv6"},
			},
			Enabled: true,
		},
	}
}

func FilterProviders(providers []ProviderSpec, names []string) ([]ProviderSpec, error) {
	if len(names) == 0 {
		out := make([]ProviderSpec, 0, len(providers))
		for _, p := range providers {
			if p.Enabled {
				out = append(out, p)
			}
		}
		if len(out) == 0 {
			return nil, errors.New("没有可用的节点提供方")
		}
		return out, nil
	}
	normalized := make([]string, 0, len(names))
	for _, name := range names {
		if trimmed := strings.TrimSpace(strings.ToLower(name)); trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	if len(normalized) == 0 {
		return FilterProviders(providers, nil)
	}
	out := make([]ProviderSpec, 0, len(normalized))
	for _, name := range normalized {
		if name == "all" {
			return FilterProviders(providers, nil)
		}
		found := false
		for _, provider := range providers {
			if strings.ToLower(provider.Name) == name {
				if provider.Enabled {
					out = append(out, provider)
				}
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("未知的节点提供方: %s", name)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("所选节点提供方均被禁用")
	}
	return out, nil
}

func parsePlainCIDR(r io.Reader) ([]*net.IPNet, error) {
	scanner := bufio.NewScanner(r)
	var networks []*net.IPNet
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		network, err := parseNetwork(line)
		if err != nil {
			return nil, err
		}
		networks = append(networks, network)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return networks, nil
}

func parseJSONArray(r io.Reader, path []string) ([]*net.IPNet, error) {
	var payload any
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return nil, err
	}
	target := payload
	for _, key := range path {
		asMap, ok := target.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("JSON 路径 %v 不存在", path)
		}
		target = asMap[key]
	}
	rawList, ok := target.([]any)
	if !ok {
		return nil, errors.New("目标字段不是数组")
	}
	networks := make([]*net.IPNet, 0, len(rawList))
	for _, item := range rawList {
		str, ok := item.(string)
		if !ok {
			continue
		}
		network, err := parseNetwork(str)
		if err != nil {
			return nil, err
		}
		networks = append(networks, network)
	}
	return networks, nil
}

func parseNetwork(value string) (*net.IPNet, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, errors.New("空的网段条目")
	}
	if strings.Contains(trimmed, "/") {
		_, network, err := net.ParseCIDR(trimmed)
		if err != nil {
			return nil, fmt.Errorf("解析 CIDR %q 失败: %w", trimmed, err)
		}
		return network, nil
	}
	ip := net.ParseIP(trimmed)
	if ip == nil {
		return nil, fmt.Errorf("解析 IP %q 失败", trimmed)
	}
	ipCopy := make(net.IP, len(ip))
	copy(ipCopy, ip)
	if v4 := ipCopy.To4(); v4 != nil {
		return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}, nil
	}
	return &net.IPNet{IP: ipCopy, Mask: net.CIDRMask(128, 128)}, nil
}

func deduplicateRanges(rs RangeSet) RangeSet {
	seen4 := make(map[string]*net.IPNet)
	seen6 := make(map[string]*net.IPNet)
	for _, n := range rs.IPv4 {
		if n == nil {
			continue
		}
		seen4[n.String()] = cloneIPNet(n)
	}
	for _, n := range rs.IPv6 {
		if n == nil {
			continue
		}
		if n.IP.To4() != nil {
			seen4[n.String()] = cloneIPNet(n)
		} else {
			seen6[n.String()] = cloneIPNet(n)
		}
	}
	ipv4 := make([]*net.IPNet, 0, len(seen4))
	for _, network := range seen4 {
		ipv4 = append(ipv4, network)
	}
	ipv6 := make([]*net.IPNet, 0, len(seen6))
	for _, network := range seen6 {
		ipv6 = append(ipv6, network)
	}
	return RangeSet{IPv4: ipv4, IPv6: ipv6}
}

// SourceConfig describes the behaviour of a range provider used by the legacy
// aggregator implementation.
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

// DefaultSources returns the built-in range providers for the aggregator.
func DefaultSources() []SourceConfig {
	return []SourceConfig{
		CloudflareSource(),
		BestIPSource(),
		UouinSource(),
	}
}

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

// Signer allows a source to modify a request before it is sent.
type Signer func(*http.Request)

// Parser parses the HTTP response into a list of IP networks.
type Parser func(context.Context, *http.Response) ([]*net.IPNet, error)

func addDefaultUserAgent(req *http.Request) {
	req.Header.Set("User-Agent", "cf-edgescout/1.0")
}

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

func NewProviderFactory(client *http.Client) *ProviderFactory {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if client.Timeout == 0 {
		client.Timeout = 30 * time.Second
	}
	return &ProviderFactory{client: client}
}

func (f *ProviderFactory) Build(cfg SourceConfig) (*Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Provider{config: cfg, client: f.client}, nil
}

type Provider struct {
	config SourceConfig
	client *http.Client
	mu     sync.Mutex
	last   time.Time
}

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
