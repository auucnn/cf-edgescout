package scheduler

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/example/cf-edgescout/fetcher"
	"github.com/example/cf-edgescout/prober"
	"github.com/example/cf-edgescout/sampler"
	"github.com/example/cf-edgescout/scorer"
	"github.com/example/cf-edgescout/store"
)

// Scheduler coordinates sampling, probing, scoring and persistence.
type ProbeRunner interface {
	Probe(ctx context.Context, ip net.IP, domain string) (*prober.Measurement, error)
}

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
	if s.Sampler == nil || s.Prober == nil || s.Scorer == nil || s.Store == nil {
		return nil, errors.New("scheduler is missing components")
	}
	candidates, err := s.Sampler.SampleSources(sources, total)
	if err != nil {
		return nil, err
	}
	if s.Parallelism <= 1 {
		return s.scanSequential(ctx, candidates, domain)
	}
	return s.scanParallel(ctx, candidates, domain)
}

func (s *Scheduler) scanSequential(ctx context.Context, candidates []sampler.Candidate, domain string) ([]Result, error) {
	results := make([]Result, 0, len(candidates))
	var lastProbe time.Time
	for _, candidate := range candidates {
		if s.RateLimit > 0 && !lastProbe.IsZero() {
			wait := s.RateLimit - time.Since(lastProbe)
			if wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, ctx.Err()
				case <-timer.C:
				}
			}
		}
		measurement, err := s.tryProbe(ctx, candidate.IP, domain)
		if err != nil {
			return nil, err
		}
		measurement.Source = candidate.Source
		measurement.Provider = candidate.Provider
		measurement.SourceType = string(candidate.ProviderKind)
		if candidate.Network != nil {
			measurement.Network = candidate.Network.String()
		}
		measurement.Family = candidate.Family
		measurement.SourceWeight = candidate.Weight
		lastProbe = time.Now()
		score := s.Scorer.Score(*measurement)
		record := store.Record{Timestamp: measurement.Timestamp, Score: score.Score, Components: score.Components, Measurement: score.Measurement}
		if err := s.Store.Save(ctx, record); err != nil {
			return nil, err
		}
		results = append(results, Result{Record: record})
	}
	return results, nil
}

func (s *Scheduler) scanParallel(ctx context.Context, candidates []sampler.Candidate, domain string) ([]Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	parallel := s.Parallelism
	if parallel <= 1 {
		parallel = 1
	}
	sem := make(chan struct{}, parallel)
	type storedResult struct {
		res Result
		err error
	}
	resultCh := make(chan storedResult, len(candidates))
	var wg sync.WaitGroup
	var startCh <-chan time.Time
	var ticker *time.Ticker
	if s.RateLimit > 0 {
		ticker = time.NewTicker(s.RateLimit)
		startCh = ticker.C
	}
	for _, candidate := range candidates {
		select {
		case <-ctx.Done():
			break
		default:
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(candidate sampler.Candidate) {
			defer wg.Done()
			defer func() { <-sem }()
			if startCh != nil {
				select {
				case <-ctx.Done():
					resultCh <- storedResult{err: ctx.Err()}
					return
				case <-startCh:
				}
			}
			measurement, err := s.tryProbe(ctx, candidate.IP, domain)
			if err != nil {
				resultCh <- storedResult{err: err}
				return
			}
			score := s.Scorer.Score(*measurement)
			record := store.Record{Timestamp: measurement.Timestamp, Score: score.Score, Components: score.Components, Measurement: score.Measurement}
			if err := s.Store.Save(ctx, record); err != nil {
				resultCh <- storedResult{err: err}
				return
			}
			resultCh <- storedResult{res: Result{Record: record}}
		}(candidate)
	}
	go func() {
		wg.Wait()
		if ticker != nil {
			ticker.Stop()
		}
		close(resultCh)
	}()
	results := make([]Result, 0, len(candidates))
	for res := range resultCh {
		if res.err != nil {
			cancel()
			return nil, res.err
		}
		results = append(results, res.res)
	}
	return results, nil
}

func (s *Scheduler) tryProbe(ctx context.Context, ip net.IP, domain string) (*prober.Measurement, error) {
	var measurement *prober.Measurement
	var err error
	attempts := s.Retries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		measurement, err = s.Prober.Probe(ctx, ip, domain)
		if err != nil {
			return nil, err
		}
		if measurement.Success || attempt == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return measurement, nil
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
