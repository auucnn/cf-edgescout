package sampler

import (
	"net"
	"testing"

	"github.com/example/cf-edgescout/fetcher"
)

func mustCIDR(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("ParseCIDR(%s) error = %v", cidr, err)
	}
	return network
}

func TestSampleSources(t *testing.T) {
	sampler := New(nil)
	sources := []fetcher.SourceRange{
		{
			Provider: fetcher.ProviderSpec{Name: "official", Weight: 1},
			RangeSet: fetcher.RangeSet{IPv4: []*net.IPNet{mustCIDR(t, "1.1.1.0/30")}},
		},
		{
			Provider: fetcher.ProviderSpec{Name: "mirror", Weight: 0.5},
			RangeSet: fetcher.RangeSet{IPv4: []*net.IPNet{mustCIDR(t, "2.2.2.0/30")}},
		},
	}
	candidates, err := sampler.SampleSources(sources, 4)
	if err != nil {
		t.Fatalf("SampleSources error = %v", err)
	}
	if len(candidates) != 4 {
		t.Fatalf("expected 4 candidates, got %d", len(candidates))
	}
}

func TestSample(t *testing.T) {
	sampler := New(nil)
	rs := fetcher.RangeSet{IPv4: []*net.IPNet{mustCIDR(t, "1.1.1.0/30")}}
	candidates, err := sampler.Sample(rs, 2)
	if err != nil {
		t.Fatalf("Sample error = %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
}
