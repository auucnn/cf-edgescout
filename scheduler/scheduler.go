package scheduler

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/example/cf-edgescout/fetcher"
	"github.com/example/cf-edgescout/prober"
	"github.com/example/cf-edgescout/sampler"
	"github.com/example/cf-edgescout/scorer"
	"github.com/example/cf-edgescout/store"
)

// ProbeRunner describes the subset of the prober used by the scheduler.
type ProbeRunner interface {
	Probe(ctx context.Context, ip net.IP, domain string) (*prober.Measurement, error)
}

// Scheduler coordinates sampling, probing, scoring and persistence.
type Scheduler struct {
	Sampler     *sampler.Sampler
	Prober      ProbeRunner
	Scorer      *scorer.Scorer
	Store       store.Store
	RateLimit   time.Duration
	Retries     int
	Parallelism int
}

// Result captures the stored record for convenience when returning from scans.
type Result struct {
	Record store.Record
}

// Scan performs a one-off scan returning the stored records.
func (s *Scheduler) Scan(ctx context.Context, sources []fetcher.SourceRange, domain string, total int) ([]Result, error) {
	if s == nil {
		return nil, errors.New("scheduler is nil")
	}
	if s.Sampler == nil || s.Prober == nil || s.Scorer == nil || s.Store == nil {
		return nil, errors.New("scheduler is missing components")
	}
	if total <= 0 {
		return nil, errors.New("total must be > 0")
	}
	candidates, err := s.Sampler.SampleSources(sources, total)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	results := make([]Result, 0, len(candidates))
	lastProbe := time.Time{}
	for _, candidate := range candidates {
		if s.RateLimit > 0 && !lastProbe.IsZero() {
			if err := sleepWithContext(ctx, s.RateLimit-time.Since(lastProbe)); err != nil {
				return nil, err
			}
		}
		measurement, err := s.tryProbe(ctx, candidate, domain)
		if err != nil {
			return nil, err
		}
		s.enrichMeasurement(measurement, candidate)
		score := s.Scorer.Score(*measurement)
		record := store.Record{
			Timestamp:      score.Measurement.Timestamp,
			Source:         score.Measurement.Source,
			Score:          score.Score,
			Grade:          score.Grade,
			Status:         score.Status,
			FailureReasons: append([]string(nil), score.Failures...),
			Components:     score.Components,
			Measurement:    score.Measurement,
		}
		if err := s.Store.Save(ctx, record); err != nil {
			return nil, err
		}
		results = append(results, Result{Record: record})
		lastProbe = time.Now()
	}
	return results, nil
}

func (s *Scheduler) tryProbe(ctx context.Context, candidate sampler.Candidate, domain string) (*prober.Measurement, error) {
	attempts := s.Retries + 1
	targetDomain := domain
	if candidate.Domain != "" {
		targetDomain = candidate.Domain
	}
	for attempt := 0; attempt < attempts; attempt++ {
		measurement, err := s.Prober.Probe(ctx, candidate.IP, targetDomain)
		if err != nil {
			return nil, err
		}
		if measurement.Success || attempt == attempts-1 {
			return measurement, nil
		}
		if err := sleepWithContext(ctx, 100*time.Millisecond); err != nil {
			return nil, err
		}
	}
	return nil, errors.New("probe attempts exhausted")
}

func (s *Scheduler) enrichMeasurement(m *prober.Measurement, candidate sampler.Candidate) {
	if m == nil {
		return
	}
	if candidate.Domain != "" {
		m.Domain = candidate.Domain
	}
	m.Source = candidate.Source
	m.Provider = candidate.Provider
	m.SourceType = string(candidate.ProviderKind)
	m.SourceWeight = candidate.Weight
	if candidate.Network != nil {
		m.Network = candidate.Network.String()
	}
	m.Family = candidate.Family
	m.DataSource = candidate.Source
	m.ApplyValidation(candidate.ExpectedOrigin, candidate.TrustedCNs)
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// RunDaemon continuously fetches ranges and scans at the provided interval.
func (s *Scheduler) RunDaemon(ctx context.Context, fetch func(context.Context) ([]fetcher.SourceRange, error), domain string, total int, interval time.Duration) error {
	if fetch == nil {
		return errors.New("fetch function is nil")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		ranges, err := fetch(ctx)
		if err == nil {
			_, err = s.Scan(ctx, ranges, domain, total)
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
