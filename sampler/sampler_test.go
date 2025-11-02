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
	seen := map[string]struct{}{}
	for _, c := range candidates {
		if _, ok := seen[c.IP.String()]; ok {
			t.Fatalf("duplicate ip sampled: %s", c.IP)
		}
		seen[c.IP.String()] = struct{}{}
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
