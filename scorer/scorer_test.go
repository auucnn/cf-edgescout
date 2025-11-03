package scorer

import (
	"testing"
	"time"

	"github.com/example/cf-edgescout/prober"
)

func TestScorerScore(t *testing.T) {
	s := New()
	measurement := prober.Measurement{Success: true, Source: "official", TCPDuration: 10 * time.Millisecond, TLSDuration: 20 * time.Millisecond, HTTPDuration: 30 * time.Millisecond, Throughput: 100 * 1024 * 1024}
	result := s.Score(measurement)
	if result.Score <= 0 {
		t.Fatalf("expected positive score")
	}
	if result.Status != "pass" {
		t.Fatalf("expected pass status, got %s", result.Status)
	}
	expected := []string{"latency", "success", "throughput", "integrity"}
	for _, key := range expected {
		if _, ok := result.Components[key]; !ok {
			t.Fatalf("missing component %s", key)
		}
	}
}
