package fetcher

import (
    "context"
    "encoding/json"
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
            _, _ = w.Write([]byte(ipv4))
        case "/ips-v6":
            _, _ = w.Write([]byte(ipv6))
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

func TestFetchAllProviders(t *testing.T) {
    apiData := map[string]any{"data": []string{"2.2.2.0/24"}}
    payload, _ := json.Marshal(apiData)
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/official-v4":
            _, _ = w.Write([]byte("3.3.3.0/24"))
        case "/official-v6":
            _, _ = w.Write([]byte("2001:db8::/48"))
        case "/third.json":
            _, _ = w.Write(payload)
        default:
            http.NotFound(w, r)
        }
    }))
    defer server.Close()

    providers := []ProviderSpec{
        {
            Name:  "official",
            Kind:  SourceKindOfficial,
            Weight: 1,
            IPv4: EndpointSpec{URL: server.URL + "/official-v4", Format: FormatPlainCIDR},
            IPv6: EndpointSpec{URL: server.URL + "/official-v6", Format: FormatPlainCIDR},
            Enabled: true,
        },
        {
            Name:  "mirror",
            Kind:  SourceKindThirdParty,
            Weight: 0.5,
            IPv4: EndpointSpec{URL: server.URL + "/third.json", Format: FormatJSONArray, JSONPath: []string{"data"}},
            Enabled: true,
        },
    }
    f := &Fetcher{Client: server.Client()}
    sources, err := f.FetchAll(context.Background(), providers)
    if len(sources) != 2 {
        t.Fatalf("expected 2 sources got %d", len(sources))
    }
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}

func TestFilterProviders(t *testing.T) {
    providers := []ProviderSpec{{Name: "official", Enabled: true}, {Name: "third", Enabled: false}}
    filtered, err := FilterProviders(providers, []string{"official"})
    if err != nil {
        t.Fatalf("filter err: %v", err)
    }
    if len(filtered) != 1 {
        t.Fatalf("expected single provider")
    }
}
