package fetcher

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetcherFetch(t *testing.T) {
	ipv4 := "# comment\n1.2.3.0/24\n"
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
	f := &Fetcher{Client: client, IPv4URL: server.URL + "/ips-v4", IPv6URL: server.URL + "/ips-v6"}
	rs, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(rs.IPv4) != 1 {
		t.Fatalf("expected 1 ipv4 range, got %d", len(rs.IPv4))
	}
	if len(rs.IPv6) != 1 {
		t.Fatalf("expected 1 ipv6 range, got %d", len(rs.IPv6))
	}
	if _, network, _ := net.ParseCIDR("1.2.3.0/24"); network.String() != rs.IPv4[0].String() {
		t.Fatalf("unexpected ipv4 network %s", rs.IPv4[0])
	}
}
