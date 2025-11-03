package scorer

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/example/cf-edgescout/prober"
)

// Config defines weights applied to individual metrics when computing the composite score.
type Config struct {
	LatencyWeight    float64
	SuccessWeight    float64
	ThroughputWeight float64
	IntegrityWeight  float64
	SourcePreference map[string]float64
	GradeBoundaries  map[string]float64
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
		LatencyWeight:    0.35,
		SuccessWeight:    0.25,
		ThroughputWeight: 0.2,
		IntegrityWeight:  0.2,
		SourcePreference: map[string]float64{"official": 1.05},
		GradeBoundaries:  map[string]float64{"A": 0.85, "B": 0.7, "C": 0.5, "D": 0},
	}}
}

// Score computes the final score for the measurement.
func (s *Scorer) Score(m prober.Measurement) Result {
	components := map[string]float64{}
	latencyNorm := normaliseLatency(m.TCPDuration + m.TLSDuration + m.HTTPDuration)
	components["latency"] = latencyNorm

	successNorm := 0.0
	if m.Success {
		successNorm = 1.0
	} else if m.Error == "" {
		successNorm = 0.5
	}
	components["success"] = successNorm

	throughputNorm := normaliseThroughput(m.Throughput)
	components["throughput"] = throughputNorm

	integrityNorm := normaliseIntegrity(m.Validation, m.Integrity.HTTPStatus)
	components["integrity"] = integrityNorm

	totalWeight := s.Config.LatencyWeight + s.Config.SuccessWeight + s.Config.ThroughputWeight + s.Config.IntegrityWeight
	if totalWeight == 0 {
		totalWeight = 1
	}
	score := (latencyNorm*s.Config.LatencyWeight + successNorm*s.Config.SuccessWeight + throughputNorm*s.Config.ThroughputWeight + integrityNorm*s.Config.IntegrityWeight) / totalWeight

	boost := s.sourceBoost(m)
	components["sourcePreference"] = boost
	score *= boost
	if m.SourceWeight > 0 {
		components["sourceWeight"] = m.SourceWeight
		score *= m.SourceWeight
	}
	if score > 1 {
		score = 1
	}
	if score < 0 {
		score = 0
	}

	grade := determineGrade(score, s.Config.GradeBoundaries)
	status := "fail"
	failures := append([]string(nil), m.Validation.Failures...)
	if score >= 0.6 && len(failures) == 0 {
		status = "pass"
	} else if len(failures) == 0 && integrityNorm < 0.75 {
		failures = append(failures, "integrity_degraded")
	}

	return Result{Score: score, Grade: grade, Status: status, Failures: failures, Components: components, Measurement: m}
}

func (s *Scorer) sourceBoost(m prober.Measurement) float64 {
	boost := 1.0
	candidates := []string{m.Source, m.Provider}
	for _, key := range candidates {
		key = strings.ToLower(key)
		if key == "" {
			continue
		}
		if weight, ok := s.Config.SourcePreference[key]; ok {
			boost = weight
		}
	}
	return boost
}

func normaliseLatency(d time.Duration) float64 {
	if d <= 0 {
		return 1
	}
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

func normaliseIntegrity(v prober.ValidationResult, status int) float64 {
	if len(v.Failures) == 0 && status >= 200 && status < 400 {
		if v.CertificateMatch && v.OriginMatch {
			return 1
		}
		return 0.75
	}
	penalty := float64(len(v.Failures)) * 0.25
	score := 1 - penalty
	if score < 0 {
		score = 0
	}
	if status >= 500 {
		score *= 0.5
	}
	return score
}

func determineGrade(score float64, boundaries map[string]float64) string {
	type pair struct {
		grade string
		cut   float64
	}
	var ordered []pair
	for grade, cut := range boundaries {
		ordered = append(ordered, pair{grade: grade, cut: cut})
	}
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].cut > ordered[j].cut
	})
	for _, entry := range ordered {
		if score >= entry.cut {
			return entry.grade
		}
	}
	return "F"
}
