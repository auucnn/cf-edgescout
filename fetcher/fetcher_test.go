package fetcher

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestFetcherFetchAggregatedSuccess(t *testing.T) {
	var headerMu sync.Mutex
	var observedHeaders []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ips-v4":
			headerMu.Lock()
			observedHeaders = append(observedHeaders, r.Header.Get("X-Test"))
			headerMu.Unlock()
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
			Signer:      func(req *http.Request) { req.Header.Set("X-Test", "ok") },
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
	found := false
	for _, entry := range aggregated.Entries {
		if entry.Network.String() == "1.1.1.0/24" {
			if len(entry.Metadata) != 2 {
				t.Fatalf("expected metadata from two sources, got %d", len(entry.Metadata))
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("merged entry not found")
	}
	headerMu.Lock()
	defer headerMu.Unlock()
	if len(observedHeaders) == 0 || observedHeaders[0] != "ok" {
		t.Fatalf("expected signer to set header, got %+v", observedHeaders)
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

func TestFetcherFetchAggregatedFormatError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not-a-cidr\n"))
	}))
	defer server.Close()

	cfg := SourceConfig{
		Name:        "invalid",
		Endpoints:   []string{server.URL + "/bad"},
		Parser:      ParseCIDRList,
		Credibility: 0.5,
	}
	f := New(server.Client())
	f.UseSources([]SourceConfig{cfg})
	if _, err := f.FetchAggregated(context.Background()); err == nil {
		t.Fatalf("expected parse error, got nil")
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

func TestAggregatorDedup(t *testing.T) {
	_, ipNet1, _ := net.ParseCIDR("1.1.1.0/24")
	_, ipNet2, _ := net.ParseCIDR("1.1.1.0/24")
	agg := NewAggregator()
	agg.Add([]RangeRecord{{
		Network:  ipNet1,
		Metadata: RangeMetadata{Source: "a", Credibility: 1},
	}})
	agg.Add([]RangeRecord{{
		Network:  ipNet2,
		Metadata: RangeMetadata{Source: "b", Credibility: 0.5},
	}})
	set := agg.Result()
	if len(set.Entries) != 1 {
		t.Fatalf("expected single deduped entry, got %d", len(set.Entries))
	}
	if len(set.Entries[0].Metadata) != 2 {
		t.Fatalf("expected two metadata entries, got %d", len(set.Entries[0].Metadata))
	}
}
