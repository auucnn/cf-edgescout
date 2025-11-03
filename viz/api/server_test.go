package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/cf-edgescout/store"
)

type mockStore struct {
	records []store.Record
	calls   int
}

func (m *mockStore) Save(ctx context.Context, record store.Record) error {
	m.records = append(m.records, record)
	return nil
}

func (m *mockStore) List(ctx context.Context) ([]store.Record, error) {
	m.calls++
	return append([]store.Record(nil), m.records...), nil
}

func sampleRecords() []store.Record {
	base := time.Date(2023, 11, 10, 12, 0, 0, 0, time.UTC)
	return []store.Record{
		{Timestamp: base.Add(-4 * time.Minute), Score: 0.9, Source: "official", Region: "SJC", Components: map[string]float64{"latency": 0.8}},
		{Timestamp: base.Add(-3 * time.Minute), Score: 0.7, Source: "third-party", Region: "AMS", Components: map[string]float64{"latency": 0.6}},
		{Timestamp: base.Add(-2 * time.Minute), Score: 0.85, Source: "official", Region: "SJC", Components: map[string]float64{"latency": 0.9}},
		{Timestamp: base.Add(-1 * time.Minute), Score: 0.65, Source: "third-party", Region: "HKG", Components: map[string]float64{"latency": 0.5}},
	}
}

func TestResultsHandlerSupportsFilteringAndPagination(t *testing.T) {
	st := &mockStore{records: sampleRecords()}
	srv := &Server{Store: st}
	req := httptest.NewRequest(http.MethodGet, "/results?source=official&limit=2", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	var payload struct {
		Total   int            `json:"total"`
		Results []store.Record `json:"results"`
		Offset  int            `json:"offset"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Total != 2 {
		t.Fatalf("expected 2 official records got %d", payload.Total)
	}
	if len(payload.Results) != 2 {
		t.Fatalf("expected 2 results got %d", len(payload.Results))
	}
	if payload.Offset != 0 {
		t.Fatalf("expected offset 0 got %d", payload.Offset)
	}
	if payload.Results[0].Score != 0.85 {
		t.Fatalf("expected most recent score 0.85 got %.2f", payload.Results[0].Score)
	}
	if payload.Results[0].Timestamp.Before(payload.Results[1].Timestamp) {
		t.Fatalf("expected results sorted newest first")
	}
}

func TestSummaryHandlerAggregatesMetrics(t *testing.T) {
	st := &mockStore{records: sampleRecords()}
	srv := &Server{Store: st}
	req := httptest.NewRequest(http.MethodGet, "/results/summary?region=sjc", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	var summary Summary
	if err := json.Unmarshal(rr.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if summary.Total != 2 {
		t.Fatalf("expected 2 records got %d", summary.Total)
	}
	if summary.Score.Average == 0 {
		t.Fatalf("expected non-zero average")
	}
	if len(summary.Components) == 0 {
		t.Fatalf("expected components summary")
	}
	if len(summary.Recent) > 1 {
		first := summary.Recent[0].Timestamp
		last := summary.Recent[len(summary.Recent)-1].Timestamp
		if !first.After(last) && !first.Equal(last) {
			t.Fatalf("recent should be ordered newest first by timestamp")
		}
	}
}

func TestTimeseriesBucketsByDuration(t *testing.T) {
	st := &mockStore{records: sampleRecords()}
	srv := &Server{Store: st}
	req := httptest.NewRequest(http.MethodGet, "/results/timeseries?bucket=2m", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	var points []TimeseriesPoint
	if err := json.Unmarshal(rr.Body.Bytes(), &points); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 buckets got %d", len(points))
	}
	if points[0].Count == 0 || points[1].Count == 0 {
		t.Fatalf("expected counts per bucket")
	}
}

func TestSourceDetailFiltersByPath(t *testing.T) {
	st := &mockStore{records: sampleRecords()}
	srv := &Server{Store: st}
	req := httptest.NewRequest(http.MethodGet, "/results/official", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	var detail SourceDetail
	if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.Source != "official" {
		t.Fatalf("expected official source got %s", detail.Source)
	}
	if detail.Total != 2 {
		t.Fatalf("expected 2 records got %d", detail.Total)
	}
}

func TestCachingSkipsStoreWithinTTL(t *testing.T) {
	st := &mockStore{records: sampleRecords()}
	srv := &Server{Store: st, CacheTTL: time.Minute}
	srv.now = func() time.Time { return time.Unix(1700000000, 0) }
	handler := srv.Handler()
	req := httptest.NewRequest(http.MethodGet, "/results", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if st.calls != 1 {
		t.Fatalf("expected first call to hit store once")
	}
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)
	if st.calls != 1 {
		t.Fatalf("expected second call to use cache")
	}
}

func TestCachingSeparatesByOrigin(t *testing.T) {
	st := &mockStore{records: sampleRecords()}
	srv := &Server{
		Store:          st,
		CacheTTL:       time.Minute,
		AllowedOrigins: []string{"https://a.example", "https://b.example"},
	}
	srv.now = func() time.Time { return time.Unix(1700000000, 0) }
	handler := srv.Handler()

	reqA1 := httptest.NewRequest(http.MethodGet, "/results", nil)
	reqA1.Header.Set("Origin", "https://a.example")
	rrA1 := httptest.NewRecorder()
	handler.ServeHTTP(rrA1, reqA1)
	if got := rrA1.Header().Get("Access-Control-Allow-Origin"); got != "https://a.example" {
		t.Fatalf("expected origin header for a.example, got %q", got)
	}
	if st.calls != 1 {
		t.Fatalf("expected first call to hit store once, got %d", st.calls)
	}

	reqB := httptest.NewRequest(http.MethodGet, "/results", nil)
	reqB.Header.Set("Origin", "https://b.example")
	rrB := httptest.NewRecorder()
	handler.ServeHTTP(rrB, reqB)
	if got := rrB.Header().Get("Access-Control-Allow-Origin"); got != "https://b.example" {
		t.Fatalf("expected origin header for b.example, got %q", got)
	}
	if st.calls != 2 {
		t.Fatalf("expected second origin to bypass cache, got %d calls", st.calls)
	}

	reqA2 := httptest.NewRequest(http.MethodGet, "/results", nil)
	reqA2.Header.Set("Origin", "https://a.example")
	rrA2 := httptest.NewRecorder()
	handler.ServeHTTP(rrA2, reqA2)
	if got := rrA2.Header().Get("Access-Control-Allow-Origin"); got != "https://a.example" {
		t.Fatalf("expected origin header for a.example, got %q", got)
	}
	if st.calls != 2 {
		t.Fatalf("expected cache hit for repeated origin, got %d calls", st.calls)
	}
}

func TestCORSAllowsConfiguredOrigins(t *testing.T) {
	st := &mockStore{records: sampleRecords()}
	srv := &Server{Store: st, AllowedOrigins: []string{"https://example.com"}}
	req := httptest.NewRequest(http.MethodGet, "/results", nil)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("expected allow origin header, got %q", got)
	}
}

func TestInvalidQueryReturns400(t *testing.T) {
	st := &mockStore{records: sampleRecords()}
	srv := &Server{Store: st}
	req := httptest.NewRequest(http.MethodGet, "/results?limit=abc", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid limit") {
		t.Fatalf("expected error message, got %s", rr.Body.String())
	}
}
