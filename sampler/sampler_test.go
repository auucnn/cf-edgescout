package sampler

import (
    "net"
    "testing"

    "github.com/example/cf-edgescout/fetcher"
)

func mustCIDR(t *testing.T, cidr string) *net.IPNet {
    t.Helper()
    _, n, err := net.ParseCIDR(cidr)
    if err != nil {
        t.Fatalf("parse cidr: %v", err)
    }
    return n
}

func TestSamplerSample(t *testing.T) {
    rs := fetcher.RangeSet{IPv4: []*net.IPNet{mustCIDR(t, "1.1.1.0/30")}, IPv6: []*net.IPNet{mustCIDR(t, "2001:db8::/126")}}
    s := New(nil)
    candidates, err := s.Sample(rs, 4)
    if err != nil {
        t.Fatalf("Sample error = %v", err)
    }
    if len(candidates) == 0 {
        t.Fatalf("expected candidates")
    }
    for _, c := range candidates {
        if c.Source == "" {
            t.Fatalf("expected source metadata")
        }
    }
}

func TestSamplerSampleSources(t *testing.T) {
    sources := []fetcher.SourceRange{
        {
            Provider: fetcher.ProviderSpec{Name: "official", DisplayName: "Cloudflare 官方发布", Kind: fetcher.SourceKindOfficial, Weight: 1},
            RangeSet: fetcher.RangeSet{IPv4: []*net.IPNet{mustCIDR(t, "1.1.1.0/30")}},
        },
        {
            Provider: fetcher.ProviderSpec{Name: "mirror", DisplayName: "社区镜像", Kind: fetcher.SourceKindThirdParty, Weight: 0.5},
            RangeSet: fetcher.RangeSet{IPv4: []*net.IPNet{mustCIDR(t, "2.2.2.0/30")}},
        },
    }
    s := New(nil)
    candidates, err := s.SampleSources(sources, 4)
    if err != nil {
        t.Fatalf("SampleSources error = %v", err)
    }
    if len(candidates) != 4 {
        t.Fatalf("expected 4 candidates got %d", len(candidates))
    }
    for _, c := range candidates {
        if c.Provider == "" {
            t.Fatalf("expected provider label")
        }
    }
}

func TestSamplerHistory(t *testing.T) {
    rs := fetcher.RangeSet{IPv4: []*net.IPNet{mustCIDR(t, "1.1.1.0/30")}}
    ip := net.ParseIP("1.1.1.1")
    s := New([]net.IP{ip})
    candidates, err := s.Sample(rs, 2)
    if err != nil {
        t.Fatalf("Sample error = %v", err)
    }
    for _, c := range candidates {
        if c.IP.Equal(ip) {
            t.Fatalf("history ip should be skipped")
        }
    }
}
