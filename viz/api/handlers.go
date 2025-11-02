package api

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleResults(w http.ResponseWriter, r *http.Request) {
	records, err := s.Store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	opts, err := s.parseFilterOptions(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	filtered := filterRecords(records, opts)
	reverseRecords(filtered)
	total := len(filtered)
	start := opts.Offset
	if start > total {
		start = total
	}
	end := start + opts.Limit
	if opts.Limit == 0 || end > total {
		end = total
	}

	payload := map[string]any{
		"total":   total,
		"offset":  start,
		"limit":   opts.Limit,
		"results": filtered[start:end],
	}
	respondJSON(w, payload)
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	records, err := s.Store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	opts, err := s.parseFilterOptions(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	filtered := filterRecords(records, opts)
	summary := buildSummary(filtered)
	respondJSON(w, summary)
}

func (s *Server) handleTimeseries(w http.ResponseWriter, r *http.Request) {
	records, err := s.Store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	opts, err := s.parseFilterOptions(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	filtered := filterRecords(records, opts)
	bucket := time.Minute
	if raw := r.URL.Query().Get("bucket"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			bucket = parsed
		} else {
			http.Error(w, "invalid bucket duration", http.StatusBadRequest)
			return
		}
	}

	timeseries := buildTimeseries(filtered, bucket)
	respondJSON(w, timeseries)
}

func (s *Server) handleSource(w http.ResponseWriter, r *http.Request) {
	segments := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/results/"), "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		http.NotFound(w, r)
		return
	}
	source := segments[0]

	records, err := s.Store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	opts, err := s.parseFilterOptions(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	opts.Sources = append(opts.Sources, source)
	filtered := filterRecords(records, opts)
	detail := buildSourceDetail(source, filtered)
	respondJSON(w, detail)
}
