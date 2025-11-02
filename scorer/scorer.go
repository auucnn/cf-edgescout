package scorer

import (
	"math"
	"time"

	"github.com/example/cf-edgescout/prober"
)

// Config defines weights applied to individual metrics when computing the composite score.
type Config struct {
	LatencyWeight    float64
	SuccessWeight    float64
	ThroughputWeight float64
}

// Result contains the final score and the intermediate metric contributions.
type Result struct {
	Score       float64
	Components  map[string]float64
	Measurement prober.Measurement
}

// Scorer normalises measurements and computes a composite score.
type Scorer struct {
	Config Config
}

// New returns a Scorer with sensible default weights.
func New() *Scorer {
	return &Scorer{Config: Config{LatencyWeight: 0.4, SuccessWeight: 0.3, ThroughputWeight: 0.3}}
}

// Score computes the final score for the measurement.
func (s *Scorer) Score(m prober.Measurement) Result {
	components := map[string]float64{}
	latencyNorm := normaliseLatency(m.TCPDuration + m.TLSDuration + m.HTTPDuration)
	components["latency"] = latencyNorm
	successNorm := 0.0
	if m.Success {
		successNorm = 1.0
	}
	components["success"] = successNorm
	throughputNorm := normaliseThroughput(m.Throughput)
	components["throughput"] = throughputNorm

	totalWeight := s.Config.LatencyWeight + s.Config.SuccessWeight + s.Config.ThroughputWeight
	if totalWeight == 0 {
		totalWeight = 1
	}
	score := (latencyNorm*s.Config.LatencyWeight + successNorm*s.Config.SuccessWeight + throughputNorm*s.Config.ThroughputWeight) / totalWeight
	return Result{Score: score, Components: components, Measurement: m}
}

func normaliseLatency(d time.Duration) float64 {
	if d <= 0 {
		return 1
	}
	// 0.5s is considered poor, sub-50ms is excellent.
	max := 500 * time.Millisecond
	value := 1 - float64(d)/float64(max)
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	return value
}

func normaliseThroughput(bitsPerSecond float64) float64 {
	if bitsPerSecond <= 0 {
		return 0
	}
	// Consider 50 Mbps or more as ideal.
	ideal := 50 * 1024 * 1024 * 8
	ratio := bitsPerSecond / float64(ideal)
	if ratio > 1 {
		ratio = 1
	}
	if ratio < 0 {
		ratio = 0
	}
	return math.Sqrt(ratio)
}
