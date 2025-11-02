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
			Description: "基于 cloudflare.com 公布的官方 IPv4/IPv6 网段",
			Weight:      1.0,
			IPv4:        EndpointSpec{URL: ipv4URL, Format: FormatPlainCIDR},
			IPv6:        EndpointSpec{URL: ipv6URL, Format: FormatPlainCIDR},
			Enabled:     true,
		},
		{
			Name:        "bestip",
			DisplayName: "BestIP 社区镜像",
			Kind:        SourceKindThirdParty,
			Description: "来自 bestip.one 提供的 Cloudflare 加速节点数据",
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("%s 响应异常: %d %s", endpoint.URL, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	switch endpoint.Format {
	case "":
		fallthrough
	case FormatPlainCIDR:
		return parsePlainCIDR(resp.Body)
	case FormatJSONArray:
		return parseJSONArray(resp.Body, endpoint.JSONPath)
	default:
		return nil, fmt.Errorf("不支持的响应格式: %s", endpoint.Format)
	}
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
		seen4[n.String()] = n
	}
	for _, n := range rs.IPv6 {
		if n == nil {
			continue
		}
		if n.IP.To4() != nil {
			seen4[n.String()] = n
		} else {
			seen6[n.String()] = n
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
