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
	if len(result.Components) != 3 {
		t.Fatalf("expected 3 components")
	}
}
