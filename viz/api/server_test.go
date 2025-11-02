package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/cf-edgescout/store"
)

func TestServerHandler(t *testing.T) {
	mem := store.NewMemory()
	if err := mem.Save(context.Background(), store.Record{Timestamp: time.Now()}); err != nil {
		t.Fatalf("Save error = %v", err)
	}
	server := &Server{Store: mem}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/results", nil)
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
}
