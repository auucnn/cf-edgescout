package scheduler

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/example/cf-edgescout/fetcher"
	"github.com/example/cf-edgescout/prober"
	"github.com/example/cf-edgescout/sampler"
	"github.com/example/cf-edgescout/scorer"
	"github.com/example/cf-edgescout/store"
)

type stubProber struct {
	measurement prober.Measurement
	calls       int
}

func (p *stubProber) Probe(ctx context.Context, ip net.IP, domain string) (*prober.Measurement, error) {
	p.calls++
	m := p.measurement
	m.IP = append(net.IP(nil), ip...)
	m.Domain = domain
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now()
	}
	return &m, nil
}

func TestSchedulerScan(t *testing.T) {
	_, ipv4, _ := net.ParseCIDR("1.1.1.1/32")
	source := fetcher.SourceRange{
		Provider: fetcher.ProviderSpec{Name: "official", Kind: fetcher.SourceKindOfficial, Weight: 1},
		RangeSet: fetcher.RangeSet{IPv4: []*net.IPNet{ipv4}},
	}
	s := &Scheduler{
		Sampler:   sampler.New(nil),
		Prober:    &stubProber{measurement: prober.Measurement{Success: true}},
		Scorer:    scorer.New(),
		Store:     store.NewMemory(),
		RateLimit: 0,
		Retries:   1,
	}
	results, err := s.Scan(context.Background(), []fetcher.SourceRange{source}, "example.com", 1)
	if err != nil {
		t.Fatalf("Scan error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	records, _ := s.Store.List(context.Background())
	if len(records) != 1 {
		t.Fatalf("store should contain 1 record")
	}
	if records[0].Measurement.Source == "" {
		t.Fatalf("expected measurement source metadata")
	}
}

func TestRunDaemonStopsOnContext(t *testing.T) {
	_, ipv4, _ := net.ParseCIDR("1.1.1.1/32")
	fetch := func(ctx context.Context) ([]fetcher.SourceRange, error) {
		return []fetcher.SourceRange{{Provider: fetcher.ProviderSpec{Name: "official"}, RangeSet: fetcher.RangeSet{IPv4: []*net.IPNet{ipv4}}}}, nil
	}
	s := &Scheduler{
		Sampler:   sampler.New(nil),
		Prober:    &stubProber{measurement: prober.Measurement{Success: true}},
		Scorer:    scorer.New(),
		Store:     store.NewMemory(),
		RateLimit: 0,
		Retries:   0,
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	err := s.RunDaemon(ctx, fetch, "example.com", 1, 10*time.Millisecond)
	if err == nil {
		t.Fatalf("expected context cancellation error")
	}
}
