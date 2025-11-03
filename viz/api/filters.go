package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/example/cf-edgescout/store"
)

// FilterOptions captures filter and pagination parameters supplied by clients.
type FilterOptions struct {
	Sources  []string
	Regions  []string
	MinScore *float64
	MaxScore *float64
	Limit    int
	Offset   int
}

func (s *Server) parseFilterOptions(r *http.Request) (FilterOptions, error) {
	q := r.URL.Query()
	minCopy, maxCopy := cloneFloat(s.DefaultFilters.MinScore), cloneFloat(s.DefaultFilters.MaxScore)
	opts := FilterOptions{
		Sources:  append([]string(nil), s.DefaultFilters.Sources...),
		Regions:  append([]string(nil), s.DefaultFilters.Regions...),
		MinScore: minCopy,
		MaxScore: maxCopy,
		Limit:    s.DefaultFilters.Limit,
		Offset:   s.DefaultFilters.Offset,
	}

	if limit := q.Get("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 0 {
			return FilterOptions{}, fmt.Errorf("invalid limit")
		}
		opts.Limit = n
	}
	if opts.Limit == 0 {
		opts.Limit = 50
	}

	if offset := q.Get("offset"); offset != "" {
		n, err := strconv.Atoi(offset)
		if err != nil || n < 0 {
			return FilterOptions{}, fmt.Errorf("invalid offset")
		}
		opts.Offset = n
	}

	if sources := q.Get("source"); sources != "" {
		opts.Sources = splitAndClean(sources)
	}
	if regions := q.Get("region"); regions != "" {
		opts.Regions = splitAndClean(regions)
	}

	if min := q.Get("score_min"); min != "" {
		v, err := strconv.ParseFloat(min, 64)
		if err != nil {
			return FilterOptions{}, fmt.Errorf("invalid score_min")
		}
		opts.MinScore = &v
	}
	if max := q.Get("score_max"); max != "" {
		v, err := strconv.ParseFloat(max, 64)
		if err != nil {
			return FilterOptions{}, fmt.Errorf("invalid score_max")
		}
		opts.MaxScore = &v
	}

	return opts, nil
}

func cloneFloat(src *float64) *float64 {
	if src == nil {
		return nil
	}
	v := *src
	return &v
}

func splitAndClean(value string) []string {
	parts := strings.Split(value, ",")
	cleaned := parts[:0]
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			cleaned = append(cleaned, strings.ToLower(trimmed))
		}
	}
	return append([]string(nil), cleaned...)
}

func filterRecords(records []store.Record, opts FilterOptions) []store.Record {
	if len(records) == 0 {
		return nil
	}

	var out []store.Record
	for _, record := range records {
		if len(opts.Sources) > 0 {
			source := strings.ToLower(sourceOf(record))
			if !contains(opts.Sources, source) {
				continue
			}
		}
		if len(opts.Regions) > 0 {
			region := strings.ToLower(regionOf(record))
			if !contains(opts.Regions, region) {
				continue
			}
		}
		if opts.MinScore != nil && record.Score < *opts.MinScore {
			continue
		}
		if opts.MaxScore != nil && record.Score > *opts.MaxScore {
			continue
		}
		out = append(out, record)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func sourceOf(record store.Record) string {
	if record.Source != "" {
		return record.Source
	}
	if record.Measurement.Domain != "" {
		return record.Measurement.Domain
	}
	if record.Measurement.ALPN != "" {
		return record.Measurement.ALPN
	}
	return "unknown"
}

func regionOf(record store.Record) string {
	if record.Region != "" {
		return record.Region
	}
	if record.Measurement.CFColo != "" {
		return record.Measurement.CFColo
	}
	return ""
}
