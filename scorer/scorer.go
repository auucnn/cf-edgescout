package scorer

import (
	"math"
	"sort"
	"time"

	"github.com/example/cf-edgescout/prober"
)

// Config defines weights applied to individual metrics when computing the composite score.
type Config struct {
	LatencyWeight   float64
	StabilityWeight float64
	IntegrityWeight float64
	TrustWeight     float64
	GradeBoundaries map[string]float64
	SourceTrust     map[string]float64
	PassThreshold   float64
}

// Result contains the final score and the intermediate metric contributions.
type Result struct {
	Score       float64
	Grade       string
	Status      string
	Failures    []string
	Components  map[string]float64
	Measurement prober.Measurement
}

// Scorer normalises measurements and computes a composite score.
type Scorer struct {
	Config Config
}

// New returns a Scorer with sensible default weights.
func New() *Scorer {
	return &Scorer{Config: Config{
		LatencyWeight:   0.35,
		StabilityWeight: 0.25,
		IntegrityWeight: 0.25,
		TrustWeight:     0.15,
		GradeBoundaries: map[string]float64{"A": 0.85, "B": 0.7, "C": 0.5, "D": 0},
		SourceTrust:     map[string]float64{"official": 1.0},
		PassThreshold:   0.6,
	}}
}

// Score computes the final score for the measurement.
func (s *Scorer) Score(m prober.Measurement) Result {
	components := map[string]float64{}
	latencyNorm := normaliseLatency(m.TCPDuration + m.TLSDuration + m.HTTPDuration)
	components["latency"] = latencyNorm
	throughputNorm := normaliseThroughput(m.Throughput)
	stabilityNorm := 0.0
	if m.Success {
		stabilityNorm = 0.5 + 0.5*throughputNorm
	} else if throughputNorm > 0 {
		stabilityNorm = 0.2 * throughputNorm
	}
	if stabilityNorm > 1 {
		stabilityNorm = 1
	}
	components["stability"] = stabilityNorm
	integrityNorm := normaliseIntegrity(m.Validation)
	components["integrity"] = integrityNorm
	trustNorm := s.trustForSource(m.DataSource)
	components["trust"] = trustNorm

	totalWeight := s.Config.LatencyWeight + s.Config.StabilityWeight + s.Config.IntegrityWeight + s.Config.TrustWeight
	if totalWeight == 0 {
		totalWeight = 1
	}
	score := (latencyNorm*s.Config.LatencyWeight + stabilityNorm*s.Config.StabilityWeight + integrityNorm*s.Config.IntegrityWeight + trustNorm*s.Config.TrustWeight) / totalWeight
	grade := determineGrade(score, s.Config.GradeBoundaries)
	status := "fail"
	if score >= s.Config.PassThreshold && integrityNorm >= 0.5 && len(m.Validation.Failures) == 0 {
		status = "pass"
	}
	failures := append([]string(nil), m.Validation.Failures...)
	if integrityNorm < 1 && len(m.Validation.Failures) == 0 {
		failures = append(failures, "integrity_degraded")
	}
	return Result{Score: score, Grade: grade, Status: status, Failures: failures, Components: components, Measurement: m}
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

func normaliseIntegrity(v prober.ValidationResult) float64 {
	if len(v.Failures) == 0 && v.CertificateMatch && v.OriginMatch {
		return 1
	}
	if len(v.Failures) == 0 {
		return 0.75
	}
	penalty := float64(len(v.Failures)) * 0.25
	score := 1 - penalty
	if score < 0 {
		score = 0
	}
	return score
}

func (s *Scorer) trustForSource(source string) float64 {
	if source == "" {
		return 0.5
	}
	if value, ok := s.Config.SourceTrust[source]; ok {
		return value
	}
	return 0.6
}

func determineGrade(score float64, boundaries map[string]float64) string {
	if len(boundaries) == 0 {
		return "U"
	}
	type pair struct {
		grade string
		min   float64
	}
	grades := make([]pair, 0, len(boundaries))
	for grade, min := range boundaries {
		grades = append(grades, pair{grade: grade, min: min})
	}
	sort.Slice(grades, func(i, j int) bool {
		if grades[i].min == grades[j].min {
			return grades[i].grade < grades[j].grade
		}
		return grades[i].min > grades[j].min
	})
	for _, g := range grades {
		if score >= g.min {
			return g.grade
		}
	}
	return "U"
}
