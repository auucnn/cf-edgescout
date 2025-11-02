package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/example/cf-edgescout/store"
)

// Server exposes a data API backed by a store.Store.
type Server struct {
	Store          store.Store
	CacheTTL       time.Duration
	DefaultFilters FilterOptions
	AllowedOrigins []string

	now   func() time.Time
	cache *responseCache
}

// FilterOptions captures filter and pagination parameters supplied by clients.
type FilterOptions struct {
	Sources  []string
	Regions  []string
	MinScore *float64
	MaxScore *float64
	Limit    int
	Offset   int
}

type responseCache struct {
	ttl time.Duration
	mu  sync.RWMutex
	m   map[string]cacheEntry
}

type cacheEntry struct {
	data      []byte
	status    int
	header    http.Header
	expiresAt time.Time
}

func (c *responseCache) get(key string, now time.Time) (cacheEntry, bool) {
	if c == nil {
		return cacheEntry{}, false
	}
	c.mu.RLock()
	entry, ok := c.m[key]
	c.mu.RUnlock()
	if !ok {
		return cacheEntry{}, false
	}
	if now.After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.m, key)
		c.mu.Unlock()
		return cacheEntry{}, false
	}
	return entry, true
}

func (c *responseCache) set(key string, entry cacheEntry) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.m == nil {
		c.m = make(map[string]cacheEntry)
	}
	c.m[key] = entry
	c.mu.Unlock()
}

// Handler constructs the HTTP handler tree, wiring middleware and caching.
func (s *Server) Handler() http.Handler {
	if s.now == nil {
		s.now = time.Now
	}
	if s.CacheTTL > 0 && s.cache == nil {
		s.cache = &responseCache{ttl: s.CacheTTL}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.wrap(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	mux.HandleFunc("/results", s.wrap(s.handleResults))
	mux.HandleFunc("/results/summary", s.wrap(s.handleSummary))
	mux.HandleFunc("/results/timeseries", s.wrap(s.handleTimeseries))
	mux.HandleFunc("/results/", s.wrap(s.handleSource))
	return s.withCORS(mux)
}

func (s *Server) wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			next(w, r)
			return
		}
		if s.CacheTTL <= 0 {
			next(w, r)
			return
		}
		key := r.URL.Path + "?" + r.URL.RawQuery
		if entry, ok := s.cache.get(key, s.now()); ok {
			copyHeaders(w.Header(), entry.header)
			w.WriteHeader(entry.status)
			_, _ = w.Write(entry.data)
			return
		}
		rec := newResponseRecorder(w)
		next(rec, r)
		if rec.status >= 200 && rec.status < 400 {
			s.cache.set(key, cacheEntry{
				data:      []byte(rec.body.String()),
				status:    rec.status,
				header:    rec.Header().Clone(),
				expiresAt: s.now().Add(s.CacheTTL),
			})
		}
	}
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if allowOrigin(origin, s.AllowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			} else if allowOrigin("*", s.AllowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowOrigin(origin string, allowed []string) bool {
	if len(allowed) == 0 {
		return origin == "*"
	}
	for _, candidate := range allowed {
		if candidate == "*" || strings.EqualFold(candidate, origin) {
			return true
		}
	}
	return false
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
}

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
	if size := r.URL.Query().Get("bucket"); size != "" {
		if d, err := time.ParseDuration(size); err == nil && d > 0 {
			bucket = d
		} else if size != "" {
			http.Error(w, "invalid bucket duration", http.StatusBadRequest)
			return
		}
	}
	timeseries := buildTimeseries(filtered, bucket)
	respondJSON(w, timeseries)
}

func (s *Server) handleSource(w http.ResponseWriter, r *http.Request) {
	segments := strings.Split(strings.TrimPrefix(r.URL.Path, "/results/"), "/")
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

func (s *Server) parseFilterOptions(r *http.Request) (FilterOptions, error) {
	q := r.URL.Query()
	var minCopy, maxCopy *float64
	if s.DefaultFilters.MinScore != nil {
		v := *s.DefaultFilters.MinScore
		minCopy = &v
	}
	if s.DefaultFilters.MaxScore != nil {
		v := *s.DefaultFilters.MaxScore
		maxCopy = &v
	}
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

func splitAndClean(v string) []string {
	parts := strings.Split(v, ",")
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

func contains(values []string, candidate string) bool {
	for _, v := range values {
		if v == candidate {
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

type Summary struct {
	Total      int              `json:"total"`
	UpdatedAt  time.Time        `json:"updated_at"`
	Score      ScoreSummary     `json:"score"`
	Sources    []GroupSummary   `json:"sources"`
	Regions    []GroupSummary   `json:"regions"`
	Components []ComponentScore `json:"components"`
	Recent     []store.Record   `json:"recent"`
}

type ScoreSummary struct {
	Average float64   `json:"average"`
	Min     float64   `json:"min"`
	Max     float64   `json:"max"`
	Median  float64   `json:"median"`
	Latest  time.Time `json:"latest"`
}

type GroupSummary struct {
	Key   string  `json:"key"`
	Count int     `json:"count"`
	Avg   float64 `json:"average"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

type ComponentScore struct {
	Key     string  `json:"key"`
	Average float64 `json:"average"`
}

type TimeseriesPoint struct {
	Timestamp time.Time          `json:"timestamp"`
	Count     int                `json:"count"`
	Average   float64            `json:"average"`
	Regions   map[string]float64 `json:"regions,omitempty"`
}

type SourceDetail struct {
	Source     string           `json:"source"`
	Total      int              `json:"total"`
	Score      ScoreSummary     `json:"score"`
	Regions    []GroupSummary   `json:"regions"`
	Components []ComponentScore `json:"components"`
	Recent     []store.Record   `json:"recent"`
}

func buildSummary(records []store.Record) Summary {
	scoreSummary := summariseScores(records)
	sources := summariseGroups(records, sourceOf)
	regions := summariseGroups(records, regionOf)
	components := summariseComponents(records)
	recent := lastN(records, 10)
	updatedAt := time.Time{}
	if len(records) > 0 {
		updatedAt = records[len(records)-1].Timestamp
	}
	return Summary{
		Total:      len(records),
		UpdatedAt:  updatedAt,
		Score:      scoreSummary,
		Sources:    sources,
		Regions:    regions,
		Components: components,
		Recent:     recent,
	}
}

func buildSourceDetail(source string, records []store.Record) SourceDetail {
	scoreSummary := summariseScores(records)
	regions := summariseGroups(records, regionOf)
	components := summariseComponents(records)
	recent := lastN(records, 10)
	return SourceDetail{
		Source:     source,
		Total:      len(records),
		Score:      scoreSummary,
		Regions:    regions,
		Components: components,
		Recent:     recent,
	}
}

func summariseScores(records []store.Record) ScoreSummary {
	if len(records) == 0 {
		return ScoreSummary{}
	}
	scores := make([]float64, len(records))
	var sum float64
	min := math.MaxFloat64
	max := -math.MaxFloat64
	var latest time.Time
	for i, record := range records {
		scores[i] = record.Score
		sum += record.Score
		if record.Score < min {
			min = record.Score
		}
		if record.Score > max {
			max = record.Score
		}
		if record.Timestamp.After(latest) {
			latest = record.Timestamp
		}
	}
	sort.Float64s(scores)
	median := scores[len(scores)/2]
	if len(scores)%2 == 0 {
		median = (scores[len(scores)/2-1] + scores[len(scores)/2]) / 2
	}
	return ScoreSummary{
		Average: sum / float64(len(records)),
		Min:     min,
		Max:     max,
		Median:  median,
		Latest:  latest,
	}
}

func summariseGroups(records []store.Record, selector func(store.Record) string) []GroupSummary {
	if selector == nil {
		return nil
	}
	groups := map[string][]float64{}
	for _, record := range records {
		key := selector(record)
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			key = "unknown"
		}
		groups[key] = append(groups[key], record.Score)
	}
	out := make([]GroupSummary, 0, len(groups))
	for key, scores := range groups {
		if len(scores) == 0 {
			continue
		}
		sort.Float64s(scores)
		min := scores[0]
		max := scores[len(scores)-1]
		var sum float64
		for _, score := range scores {
			sum += score
		}
		out = append(out, GroupSummary{
			Key:   key,
			Count: len(scores),
			Avg:   sum / float64(len(scores)),
			Min:   min,
			Max:   max,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Key < out[j].Key
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func summariseComponents(records []store.Record) []ComponentScore {
	totals := map[string]struct {
		sum   float64
		count int
	}{}
	for _, record := range records {
		for key, value := range record.Components {
			entry := totals[key]
			entry.sum += value
			entry.count++
			totals[key] = entry
		}
	}
	out := make([]ComponentScore, 0, len(totals))
	for key, entry := range totals {
		if entry.count == 0 {
			continue
		}
		out = append(out, ComponentScore{Key: key, Average: entry.sum / float64(entry.count)})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Key < out[j].Key
	})
	return out
}

func lastN(records []store.Record, n int) []store.Record {
	if n <= 0 || len(records) == 0 {
		return nil
	}
	if len(records) <= n {
		out := append([]store.Record(nil), records...)
		reverseRecords(out)
		return out
	}
	subset := records[len(records)-n:]
	out := make([]store.Record, len(subset))
	for i := range subset {
		out[i] = subset[len(subset)-1-i]
	}
	return out
}

func reverseRecords(records []store.Record) {
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
}

func buildTimeseries(records []store.Record, bucket time.Duration) []TimeseriesPoint {
	if len(records) == 0 {
		return nil
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp.Before(records[j].Timestamp)
	})
	start := records[0].Timestamp.Truncate(bucket)
	buckets := map[time.Time][]store.Record{}
	for _, record := range records {
		key := start.Add(record.Timestamp.Sub(start).Truncate(bucket))
		buckets[key] = append(buckets[key], record)
	}
	keys := make([]time.Time, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })
	points := make([]TimeseriesPoint, 0, len(keys))
	for _, key := range keys {
		group := buckets[key]
		var sum float64
		regions := map[string]struct {
			sum   float64
			count int
		}{}
		for _, record := range group {
			sum += record.Score
			region := strings.ToLower(regionOf(record))
			entry := regions[region]
			entry.sum += record.Score
			entry.count++
			regions[region] = entry
		}
		regionAverages := map[string]float64{}
		for region, entry := range regions {
			if entry.count == 0 {
				continue
			}
			regionAverages[region] = entry.sum / float64(entry.count)
		}
		points = append(points, TimeseriesPoint{
			Timestamp: key,
			Count:     len(group),
			Average:   sum / float64(len(group)),
			Regions:   regionAverages,
		})
	}
	return points
}

func respondJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	body   strings.Builder
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{ResponseWriter: w, status: http.StatusOK}
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(p []byte) (int, error) {
	r.body.WriteString(string(p))
	return r.ResponseWriter.Write(p)
}
