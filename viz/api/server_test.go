package api

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/example/cf-edgescout/prober"
    "github.com/example/cf-edgescout/store"
)

func prepareStore(t *testing.T) store.Store {
    t.Helper()
    mem := store.NewMemory()
    records := []store.Record{
        {
            Timestamp: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
            Score:     0.9,
            Measurement: prober.Measurement{
                Domain:      "example.com",
                Source:      "official",
                Provider:    "Cloudflare 官方发布",
                Success:     true,
                TCPDuration: 10 * time.Millisecond,
                TLSDuration: 15 * time.Millisecond,
                HTTPDuration: 20 * time.Millisecond,
            },
        },
        {
            Timestamp: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
            Score:     0.7,
            Measurement: prober.Measurement{
                Domain:      "example.com",
                Source:      "bestip",
                Provider:    "BestIP 社区镜像",
                Success:     false,
                TCPDuration: 30 * time.Millisecond,
                TLSDuration: 40 * time.Millisecond,
                HTTPDuration: 50 * time.Millisecond,
            },
        },
    }
    for _, record := range records {
        if err := mem.Save(context.Background(), record); err != nil {
            t.Fatalf("save: %v", err)
        }
    }
    return mem
}

func TestResultsEndpoint(t *testing.T) {
    mem := prepareStore(t)
    server := &Server{Store: mem}
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/api/results?source=official&limit=5", nil)
    server.Handler().ServeHTTP(rr, req)
    if rr.Code != http.StatusOK {
        t.Fatalf("expected 200 got %d", rr.Code)
    }
    var resp listResponse
    if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
        t.Fatalf("decode: %v", err)
    }
    if resp.Total != 1 {
        t.Fatalf("expected filtered total 1 got %d", resp.Total)
    }
    if len(resp.Items) != 1 {
        t.Fatalf("expected 1 item got %d", len(resp.Items))
    }
}

func TestSummaryEndpoint(t *testing.T) {
    mem := prepareStore(t)
    server := &Server{Store: mem}
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/api/results/summary", nil)
    server.Handler().ServeHTTP(rr, req)
    if rr.Code != http.StatusOK {
        t.Fatalf("expected 200 got %d", rr.Code)
    }
    var resp summaryResponse
    if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
        t.Fatalf("decode: %v", err)
    }
    if len(resp.Providers) != 2 {
        t.Fatalf("expected 2 providers got %d", len(resp.Providers))
    }
}

func TestTimeseriesEndpoint(t *testing.T) {
    mem := prepareStore(t)
    server := &Server{Store: mem}
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/api/results/timeseries", nil)
    server.Handler().ServeHTTP(rr, req)
    if rr.Code != http.StatusOK {
        t.Fatalf("expected 200 got %d", rr.Code)
    }
    var resp timeseriesResponse
    if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
        t.Fatalf("decode: %v", err)
    }
    if len(resp.Points) != 2 {
        t.Fatalf("expected 2 points got %d", len(resp.Points))
    }
    if resp.Points[0].Timestamp.After(resp.Points[1].Timestamp) {
        t.Fatalf("expected chronological order")
    }
}
