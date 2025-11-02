package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/example/cf-edgescout/prober"
)

func TestJSONLStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "records.jsonl")
	s := NewJSONL(path)
	record := Record{Timestamp: time.Now(), Score: 0.9, Components: map[string]float64{"latency": 0.8}, Measurement: prober.Measurement{Success: true}}
	if err := s.Save(context.Background(), record); err != nil {
		t.Fatalf("Save error = %v", err)
	}
	records, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
}

func TestMemoryStore(t *testing.T) {
	s := NewMemory()
	record := Record{Timestamp: time.Now()}
	if err := s.Save(context.Background(), record); err != nil {
		t.Fatalf("Save error = %v", err)
	}
	records, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record")
	}
}

func TestJSONLStoreContextCancel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "records.jsonl")
	s := NewJSONL(path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.Save(ctx, Record{Timestamp: time.Now()})
	if err == nil {
		t.Fatalf("expected context error")
	}
	_, err = s.List(ctx)
	if err == nil {
		t.Fatalf("expected context error on list")
	}
	if _, err := os.Stat(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error: %v", err)
	}
}
