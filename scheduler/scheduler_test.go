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
	rs := fetcher.RangeSet{IPv4: []*net.IPNet{ipv4}}
	s := &Scheduler{
		Sampler:   sampler.New(nil),
		Prober:    &stubProber{measurement: prober.Measurement{Success: true}},
		Scorer:    scorer.New(),
		Store:     store.NewMemory(),
		RateLimit: 0,
		Retries:   1,
	}
	results, err := s.Scan(context.Background(), rs, "example.com", 1)
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
	if records[0].Source == "" {
		t.Fatalf("expected record source to be set")
	}
    p.calls++
    m := p.measurement
    m.IP = append(net.IP(nil), ip...)
    m.Domain = domain
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
        return []fetcher.SourceRange{{Provider: fetcher.ProviderSpec{Name: "official", Kind: fetcher.SourceKindOfficial}, RangeSet: fetcher.RangeSet{IPv4: []*net.IPNet{ipv4}}}}, nil
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

func TestSchedulerSourcePolicies(t *testing.T) {
	_, officialNet, _ := net.ParseCIDR("1.1.1.1/32")
	_, thirdNet, _ := net.ParseCIDR("1.0.0.2/32")
	ranges := fetcher.RangeSet{
		Sources: []fetcher.SourceRangeSet{
			{Name: "official", Priority: 10, IPv4: []*net.IPNet{officialNet}},
			{Name: "third-party", Priority: 5, IPv4: []*net.IPNet{thirdNet}, ExpectedOrigin: "origin.third", TrustedCNs: []string{"trusted.third"}},
		},
	}
	s := &Scheduler{
		Sampler: sampler.New(nil),
		Prober: &stubProber{measurement: prober.Measurement{
			Success:       true,
			CertificateCN: "mismatch",
			OriginHost:    "other.origin",
			HTTPDuration:  5 * time.Millisecond,
			TLSDuration:   5 * time.Millisecond,
			TCPDuration:   5 * time.Millisecond,
			Throughput:    1_000_000,
		}},
		Scorer:  scorer.New(),
		Store:   store.NewMemory(),
		Retries: 0,
	}
	ctx := context.Background()
	results, err := s.Scan(ctx, ranges, "example.com", 2)
	if err != nil {
		t.Fatalf("Scan error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results got %d", len(results))
	}
	records, _ := s.Store.List(ctx)
	var thirdRecord *store.Record
	for i := range records {
		rec := records[i]
		if rec.Source == "third-party" {
			thirdRecord = &rec
			break
		}
	}
	if thirdRecord == nil {
		t.Fatalf("expected third-party record present")
	}
	if thirdRecord.Status != "fail" {
		t.Fatalf("expected third-party status fail got %s", thirdRecord.Status)
	}
	hasValidationFailure := false
	for _, failure := range thirdRecord.FailureReasons {
		if failure == "origin_host_mismatch" || failure == "certificate_cn_mismatch" {
			hasValidationFailure = true
		}
	}
	if !hasValidationFailure {
		t.Fatalf("expected validation failure reasons, got %v", thirdRecord.FailureReasons)
	}
}
