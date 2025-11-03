package fetcher

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetcherFetchAggregatedSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ips-v4":
			w.Write([]byte("1.1.1.0/24\n"))
		case "/ips-v6":
			w.Write([]byte("2400:cb00::/32\n"))
		case "/third":
			w.Write([]byte("1.1.1.0/24\n8.8.8.0/24\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfgs := []SourceConfig{
		{
			Name:        "primary",
			Endpoints:   []string{server.URL + "/ips-v4", server.URL + "/ips-v6"},
			Parser:      ParseCIDRList,
			RateLimit:   0,
			Credibility: 1,
		},
		{
			Name:        "backup",
			Endpoints:   []string{server.URL + "/third"},
			Parser:      ParseCIDRList,
			Credibility: 0.5,
		},
	}
	client := server.Client()
	client.Timeout = time.Second
	f := New(client)
	f.UseSources(cfgs)
	aggregated, err := f.FetchAggregated(context.Background())
	if err != nil {
		t.Fatalf("FetchAggregated() error = %v", err)
	}
	if len(aggregated.Entries) != 3 {
		t.Fatalf("expected 3 aggregated entries, got %d", len(aggregated.Entries))
	}
	rs := aggregated.RangeSet()
	if len(rs.IPv4) != 2 || len(rs.IPv6) != 1 {
		t.Fatalf("unexpected range set sizes: %+v", rs)
	}
}

func TestFetcherFetchAggregatedFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.Write([]byte("10.0.0.0/24\n"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := SourceConfig{
		Name:        "fallback",
		Endpoints:   []string{server.URL + "/fail", server.URL + "/ok"},
		Parser:      ParseCIDRList,
		Credibility: 0.6,
	}
	f := New(server.Client())
	f.UseSources([]SourceConfig{cfg})
	aggregated, err := f.FetchAggregated(context.Background())
	if err != nil {
		t.Fatalf("FetchAggregated() error = %v", err)
	}
	if len(aggregated.Entries) != 1 {
		t.Fatalf("expected 1 entry after fallback, got %d", len(aggregated.Entries))
	}
}

func TestFetcherFetchAggregatedNetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	cfg := SourceConfig{
		Name:        "offline",
		Endpoints:   []string{server.URL + "/down"},
		Parser:      ParseCIDRList,
		Credibility: 0.5,
	}
	f := New(&http.Client{Timeout: time.Second})
	f.UseSources([]SourceConfig{cfg})
	if _, err := f.FetchAggregated(context.Background()); err == nil {
		t.Fatalf("expected network error, got nil")
	}
}

func TestFetcherFetchProvider(t *testing.T) {
	ipv4 := "1.2.3.0/24\n"
	ipv6 := "2001:db8::/32\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ips-v4":
			w.Write([]byte(ipv4))
		case "/ips-v6":
			w.Write([]byte(ipv6))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := server.Client()
	f := New(client)
	provider := ProviderSpec{
		Name: "official",
		Kind: SourceKindOfficial,
		IPv4: EndpointSpec{URL: server.URL + "/ips-v4", Format: FormatPlainCIDR},
		IPv6: EndpointSpec{URL: server.URL + "/ips-v6", Format: FormatPlainCIDR},
	}
	src, err := f.FetchProvider(context.Background(), provider)
	if err != nil {
		t.Fatalf("FetchProvider error = %v", err)
	}
	if len(src.RangeSet.IPv4) != 1 || len(src.RangeSet.IPv6) != 1 {
		t.Fatalf("unexpected range counts: %+v", src.RangeSet)
	}
}

func TestDeduplicateRanges(t *testing.T) {
	_, ipNet1, _ := net.ParseCIDR("1.1.1.0/24")
	_, ipNet2, _ := net.ParseCIDR("1.1.1.0/24")
	set := RangeSet{IPv4: []*net.IPNet{ipNet1, ipNet2}}
	deduped := deduplicateRanges(set)
	if len(deduped.IPv4) != 1 {
		t.Fatalf("expected single entry after dedupe, got %d", len(deduped.IPv4))
	}
}
