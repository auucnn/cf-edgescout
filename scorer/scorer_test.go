package scorer

import (
    "testing"
    "time"

    "github.com/example/cf-edgescout/prober"
)

func TestScorerScore(t *testing.T) {
	s := New()
	measurement := prober.Measurement{Success: true, TCPDuration: 10 * time.Millisecond, TLSDuration: 20 * time.Millisecond, HTTPDuration: 30 * time.Millisecond, Throughput: 100 * 1024 * 1024}
	result := s.Score(measurement)
	if result.Score <= 0 {
		t.Fatalf("expected positive score")
	}
	if len(result.Components) != 4 {
		t.Fatalf("expected 4 components got %d", len(result.Components))
	}
	for _, key := range []string{"latency", "stability", "integrity", "trust"} {
		if _, ok := result.Components[key]; !ok {
			t.Fatalf("missing component %s", key)
		}
	}
    s := New()
    measurement := prober.Measurement{Success: true, Source: "official", TCPDuration: 10 * time.Millisecond, TLSDuration: 20 * time.Millisecond, HTTPDuration: 30 * time.Millisecond, Throughput: 100 * 1024 * 1024}
    result := s.Score(measurement)
    if result.Score <= 0 {
        t.Fatalf("expected positive score")
    }
    if len(result.Components) < 5 {
        t.Fatalf("expected at least 5 components got %d", len(result.Components))
    }
}
