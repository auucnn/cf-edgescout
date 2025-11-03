package store

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/example/cf-edgescout/prober"
)

// Record represents a scored measurement ready to be persisted.
type Record struct {
	Timestamp   time.Time          `json:"timestamp"`
	Score       float64            `json:"score"`
	Components  map[string]float64 `json:"components"`
	Measurement prober.Measurement `json:"measurement"`
	Source      string             `json:"source,omitempty"`
	Region      string             `json:"region,omitempty"`
}

// Store persists and retrieves measurement records.
type Store interface {
	Save(ctx context.Context, record Record) error
	List(ctx context.Context) ([]Record, error)
}

// JSONLStore appends records to a JSON Lines file and can read them back.
type JSONLStore struct {
	path string
	mu   sync.Mutex
}

// NewJSONL creates a JSONLStore writing to the provided path.
func NewJSONL(path string) *JSONLStore {
	return &JSONLStore{path: path}
}

// Save appends the record as a JSON line.
func (s *JSONLStore) Save(ctx context.Context, record Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// List reads all records from the JSONL file.
func (s *JSONLStore) List(ctx context.Context) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.path, os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var records []Record
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record Record
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return records, nil
}

// MemoryStore keeps records in memory, useful for tests and daemon mode.
type MemoryStore struct {
	mu      sync.Mutex
	records []Record
}

// NewMemory creates a MemoryStore.
func NewMemory() *MemoryStore {
	return &MemoryStore{}
}

// Save appends a record in-memory.
func (s *MemoryStore) Save(ctx context.Context, record Record) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	return nil
}

// List returns a snapshot of the records.
func (s *MemoryStore) List(ctx context.Context) ([]Record, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, len(s.records))
	copy(out, s.records)
	return out, nil
}

// ErrNotFound indicates the requested record is missing.
var ErrNotFound = errors.New("record not found")
