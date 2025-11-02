package scheduler

import (
	"context"
	"errors"
	"net"
	"sort"
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
	Sampler        *sampler.Sampler
	Prober         ProbeRunner
	Scorer         *scorer.Scorer
	Store          store.Store
	RateLimit      time.Duration
	Retries        int
	SourcePolicies map[string]SourcePolicy
}

// SourcePolicy defines how probes from a given source should be scheduled.
type SourcePolicy struct {
	Priority  int
	RateLimit time.Duration
}

// Result captures the stored record for convenience when returning from scans.
type Result struct {
	Record store.Record
}

// Scan performs a one-off scan returning the stored records.
func (s *Scheduler) Scan(ctx context.Context, ranges fetcher.RangeSet, domain string, total int) ([]Result, error) {
	if s.Sampler == nil || s.Prober == nil || s.Scorer == nil || s.Store == nil {
		return nil, errors.New("scheduler is missing components")
	}
	candidates, err := s.Sampler.Sample(ranges, total)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	sourceStates := buildSourceStates(candidates, ranges, s)
	sort.Slice(sourceStates, func(i, j int) bool {
		if sourceStates[i].policy.Priority == sourceStates[j].policy.Priority {
			return sourceStates[i].source < sourceStates[j].source
		}
		return sourceStates[i].policy.Priority > sourceStates[j].policy.Priority
	})
	results := make([]Result, 0, len(candidates))
	processed := 0
	for processed < len(candidates) {
		now := time.Now()
		var waitDuration time.Duration
		waitSet := false
		progress := false
		for idx := range sourceStates {
			state := &sourceStates[idx]
			if len(state.queue) == 0 {
				continue
			}
			if state.nextReady.After(now) {
				wait := state.nextReady.Sub(now)
				if !waitSet || wait < waitDuration {
					waitDuration = wait
					waitSet = true
				}
				continue
			}
			candidate := state.queue[0]
			state.queue = state.queue[1:]
			measurement, err := s.tryProbe(ctx, candidate, domain)
			if err != nil {
				return nil, err
			}
			if candidate.Domain != "" {
				measurement.Domain = candidate.Domain
			}
			measurement.DataSource = candidate.Source
			measurement.ApplyValidation(candidate.ExpectedOrigin, candidate.TrustedCNs)
			score := s.Scorer.Score(*measurement)
			record := store.Record{
				Timestamp:      measurement.Timestamp,
				Source:         measurement.DataSource,
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
			processed++
			progress = true
			effectiveRate := state.policy.RateLimit
			if effectiveRate <= 0 {
				effectiveRate = s.RateLimit
			}
			if effectiveRate > 0 {
				state.nextReady = time.Now().Add(effectiveRate)
			} else {
				state.nextReady = time.Now()
			}
		}
		if processed >= len(candidates) {
			break
		}
		if !progress {
			if !waitSet {
				break
			}
			timer := time.NewTimer(waitDuration)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return results, nil
}

type sourceState struct {
	source    string
	queue     []sampler.Candidate
	policy    SourcePolicy
	nextReady time.Time
}

func buildSourceStates(candidates []sampler.Candidate, ranges fetcher.RangeSet, s *Scheduler) []sourceState {
	lookup := map[string]fetcher.SourceRangeSet{}
	for _, src := range ranges.Sources {
		lookup[src.Name] = src
	}
	states := map[string]*sourceState{}
	for _, candidate := range candidates {
		st, ok := states[candidate.Source]
		if !ok {
			st = &sourceState{source: candidate.Source}
			policy := s.SourcePolicies[candidate.Source]
			if src, ok := lookup[candidate.Source]; ok {
				if policy.Priority == 0 {
					policy.Priority = src.Priority
				}
				if policy.RateLimit == 0 {
					policy.RateLimit = src.RateLimit
				}
			}
			st.policy = policy
			states[candidate.Source] = st
		}
		st.queue = append(st.queue, candidate)
	}
	out := make([]sourceState, 0, len(states))
	for _, st := range states {
		out = append(out, *st)
	}
	return out
}

func (s *Scheduler) tryProbe(ctx context.Context, candidate sampler.Candidate, domain string) (*prober.Measurement, error) {
	var measurement *prober.Measurement
	var err error
	attempts := s.Retries + 1
	targetDomain := domain
	if candidate.Domain != "" {
		targetDomain = candidate.Domain
	}
	for attempt := 0; attempt < attempts; attempt++ {
		measurement, err = s.Prober.Probe(ctx, candidate.IP, targetDomain)
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
func (s *Scheduler) RunDaemon(ctx context.Context, fetch func(context.Context) (fetcher.RangeSet, error), domain string, total int, interval time.Duration) error {
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
