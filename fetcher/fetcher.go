package fetcher

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	ipv4URL = "https://www.cloudflare.com/ips-v4"
	ipv6URL = "https://www.cloudflare.com/ips-v6"
)

// RangeSet groups the IPv4 and IPv6 networks that Cloudflare publishes for its edge.
type RangeSet struct {
	IPv4    []*net.IPNet
	IPv6    []*net.IPNet
	Sources []SourceRangeSet
}

// SourceRangeSet groups networks that originate from the same upstream source.
type SourceRangeSet struct {
	Name           string
	Priority       int
	Concurrency    int
	RateLimit      time.Duration
	Domain         string
	ExpectedOrigin string
	TrustedCNs     []string
	IPv4           []*net.IPNet
	IPv6           []*net.IPNet
}

// Fetcher downloads Cloudflare network ranges and parses them into structured data.
type Fetcher struct {
	Client  *http.Client
	IPv4URL string
	IPv6URL string
}

// New returns a Fetcher using the provided HTTP client or http.DefaultClient if nil.
func New(client *http.Client) *Fetcher {
	if client == nil {
		client = http.DefaultClient
	}
	client.Timeout = 30 * time.Second
	return &Fetcher{
		Client:  client,
		IPv4URL: ipv4URL,
		IPv6URL: ipv6URL,
	}
}

// Fetch retrieves the IPv4 and IPv6 ranges from Cloudflare and parses them into a RangeSet.
func (f *Fetcher) Fetch(ctx context.Context) (RangeSet, error) {
	ipv4, err := f.fetchRange(ctx, f.IPv4URL)
	if err != nil {
		return RangeSet{}, fmt.Errorf("fetch ipv4 ranges: %w", err)
	}
	ipv6, err := f.fetchRange(ctx, f.IPv6URL)
	if err != nil {
		return RangeSet{}, fmt.Errorf("fetch ipv6 ranges: %w", err)
	}
	official := SourceRangeSet{
		Name:        "official",
		Priority:    100,
		Concurrency: 1,
		RateLimit:   0,
		Domain:      "",
		IPv4:        ipv4,
		IPv6:        ipv6,
	}
	return RangeSet{IPv4: ipv4, IPv6: ipv6, Sources: []SourceRangeSet{official}}, nil
}

func (f *Fetcher) fetchRange(ctx context.Context, url string) ([]*net.IPNet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var networks []*net.IPNet
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
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
	return networks, nil
}
