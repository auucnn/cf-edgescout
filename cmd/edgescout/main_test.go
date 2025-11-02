package main

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/cf-edgescout/fetcher"
)

func TestParseSourceList(t *testing.T) {
	inputs := " cloudflare , bestip , ,uouin "
	got := parseSourceList(inputs)
	want := []string{"cloudflare", "bestip", "uouin"}
	if len(got) != len(want) {
		t.Fatalf("expected %d elements, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected element %d: %s", i, got[i])
		}
	}
}

func TestConfigureFetcherInvalidSource(t *testing.T) {
	f := fetcher.New(nil)
	if err := configureFetcher(f, "unknown", ""); err == nil {
		t.Fatalf("expected error for unknown source")
	}
}

func TestFetchRangesPartialError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("1.1.1.0/24\n"))
	}))
	defer server.Close()

	good := fetcher.SourceConfig{
		Name:        "good",
		Endpoints:   []string{server.URL + "/ips"},
		Parser:      fetcher.ParseCIDRList,
		Credibility: 1,
	}
	bad := fetcher.SourceConfig{
		Name:        "bad",
		Endpoints:   []string{"http://127.0.0.1:9/unreachable"},
		Parser:      fetcher.ParseCIDRList,
		Credibility: 0.5,
	}
	client := &http.Client{}
	f := fetcher.New(client)
	f.UseSources([]fetcher.SourceConfig{good, bad})

	var buf bytes.Buffer
	original := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(original)

	rs, err := fetchRanges(context.Background(), f)
	if err != nil {
		t.Fatalf("fetchRanges() returned error = %v", err)
	}
	if len(rs.IPv4) != 1 {
		t.Fatalf("expected 1 ipv4 range, got %d", len(rs.IPv4))
	}
	if buf.Len() == 0 {
		t.Fatalf("expected warning to be logged for partial error")
	}
}

func TestFetchRangesError(t *testing.T) {
	bad := fetcher.SourceConfig{
		Name:        "bad",
		Endpoints:   []string{"http://127.0.0.1:9/unreachable"},
		Parser:      fetcher.ParseCIDRList,
		Credibility: 0.5,
	}
	f := fetcher.New(&http.Client{})
	f.UseSources([]fetcher.SourceConfig{bad})
	if _, err := fetchRanges(context.Background(), f); err == nil {
		t.Fatalf("expected error when no sources succeed")
	}
}
