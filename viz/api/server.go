package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/example/cf-edgescout/store"
)

type Server struct {
	Store store.Store
}

type listResponse struct {
	Total int            `json:"total"`
	Items []store.Record `json:"items"`
}

type providerSummary struct {
	Source      string  `json:"source"`
	Provider    string  `json:"provider"`
	Count       int     `json:"count"`
	SuccessRate float64 `json:"successRate"`
	AvgScore    float64 `json:"avgScore"`
	AvgLatency  float64 `json:"avgLatencyMs"`
}

type summaryResponse struct {
	GeneratedAt time.Time         `json:"generatedAt"`
	Providers   []providerSummary `json:"providers"`
}

type timeseriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
	Provider  string    `json:"provider"`
	Score     float64   `json:"score"`
	Latency   float64   `json:"latencyMs"`
	Success   bool      `json:"success"`
}

type timeseriesResponse struct {
	Points []timeseriesPoint `json:"points"`
}

type queryOptions struct {
	source   string
	provider string
	success  *bool
	limit    int
	offset   int
}

func (s *Server) Handler() http.Handler {
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/healthz", s.handleHealth)
	apiMux.HandleFunc("/results", s.handleResults)
	apiMux.HandleFunc("/results/summary", s.handleSummary)
	apiMux.HandleFunc("/results/timeseries", s.handleTimeseries)

	root := http.NewServeMux()
	root.HandleFunc("/healthz", s.handleHealth)
	root.HandleFunc("/results", s.handleResults)
	root.HandleFunc("/results/summary", s.handleSummary)
	root.HandleFunc("/results/timeseries", s.handleTimeseries)
	root.Handle("/api/", http.StripPrefix("/api", apiMux))
	return root
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleResults(w http.ResponseWriter, r *http.Request) {
	records, err := s.Store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	opts, err := parseQueryOptions(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filtered := filterRecords(records, opts)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.After(filtered[j].Timestamp)
	})
	total := len(filtered)
	start := opts.offset
	if start > total {
		start = total
	}
	end := start + opts.limit
	if end > total {
		end = total
	}
	page := filtered[start:end]
	writeJSON(w, listResponse{Total: total, Items: page})
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	records, err := s.Store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	opts, err := parseQueryOptions(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filtered := filterRecords(records, opts)
	stats := map[string]*providerSummary{}
	for _, record := range filtered {
		key := strings.ToLower(record.Measurement.Provider)
		if key == "" {
			key = strings.ToLower(record.Measurement.Source)
		}
		if key == "" {
			key = "unknown"
		}
		summary := stats[key]
		if summary == nil {
			summary = &providerSummary{Source: record.Measurement.Source, Provider: record.Measurement.Provider}
			stats[key] = summary
		}
		summary.Count++
		if record.Measurement.Success {
			summary.SuccessRate += 1
		}
		summary.AvgScore += record.Score
		latency := record.Measurement.TCPDuration + record.Measurement.TLSDuration + record.Measurement.HTTPDuration
		summary.AvgLatency += latency.Seconds() * 1000
	}
	response := summaryResponse{GeneratedAt: time.Now()}
	for _, summary := range stats {
		if summary.Count > 0 {
			summary.SuccessRate = summary.SuccessRate / float64(summary.Count)
			summary.AvgScore = summary.AvgScore / float64(summary.Count)
			summary.AvgLatency = summary.AvgLatency / float64(summary.Count)
		}
		response.Providers = append(response.Providers, *summary)
	}
	sort.Slice(response.Providers, func(i, j int) bool {
		return response.Providers[i].AvgScore > response.Providers[j].AvgScore
	})
	writeJSON(w, response)
}

func (s *Server) handleTimeseries(w http.ResponseWriter, r *http.Request) {
	records, err := s.Store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	opts, err := parseQueryOptions(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filtered := filterRecords(records, opts)
	points := make([]timeseriesPoint, 0, len(filtered))
	for _, record := range filtered {
		latency := record.Measurement.TCPDuration + record.Measurement.TLSDuration + record.Measurement.HTTPDuration
		points = append(points, timeseriesPoint{
			Timestamp: record.Timestamp,
			Source:    record.Measurement.Source,
			Provider:  record.Measurement.Provider,
			Score:     record.Score,
			Latency:   latency.Seconds() * 1000,
			Success:   record.Measurement.Success,
		})
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp.Before(points[j].Timestamp)
	})
	writeJSON(w, timeseriesResponse{Points: points})
}

func parseQueryOptions(r *http.Request) (queryOptions, error) {
	opts := queryOptions{limit: 200}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		v, err := strconv.Atoi(limit)
		if err != nil || v <= 0 {
			return opts, fmt.Errorf("invalid limit")
		}
		opts.limit = v
	}
	if offset := r.URL.Query().Get("offset"); offset != "" {
		v, err := strconv.Atoi(offset)
		if err != nil || v < 0 {
			return opts, fmt.Errorf("invalid offset")
		}
		opts.offset = v
	}
	if source := strings.TrimSpace(r.URL.Query().Get("source")); source != "" {
		opts.source = strings.ToLower(source)
	}
	if provider := strings.TrimSpace(r.URL.Query().Get("provider")); provider != "" {
		opts.provider = strings.ToLower(provider)
	}
	if success := strings.TrimSpace(r.URL.Query().Get("success")); success != "" {
		switch strings.ToLower(success) {
		case "true", "1", "yes":
			value := true
			opts.success = &value
		case "false", "0", "no":
			value := false
			opts.success = &value
		default:
			return opts, fmt.Errorf("invalid success filter")
		}
	}
	return opts, nil
}

func filterRecords(records []store.Record, opts queryOptions) []store.Record {
	result := make([]store.Record, 0, len(records))
	for _, record := range records {
		m := record.Measurement
		if opts.source != "" && strings.ToLower(m.Source) != opts.source {
			continue
		}
		if opts.provider != "" && strings.ToLower(m.Provider) != opts.provider {
			continue
		}
		if opts.success != nil && m.Success != *opts.success {
			continue
		}
		result = append(result, record)
	}
	return result
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(v)
}
