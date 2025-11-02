package exporter

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/example/cf-edgescout/prober"
	"github.com/example/cf-edgescout/store"
)

func sampleRecord() store.Record {
	return store.Record{
		Timestamp:  time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		Score:      0.8,
		Components: map[string]float64{"latency": 0.7},
		Measurement: prober.Measurement{
			Domain:       "example.com",
			IP:           []byte{1, 1, 1, 1},
			Success:      true,
			TCPDuration:  10 * time.Millisecond,
			TLSDuration:  20 * time.Millisecond,
			HTTPDuration: 30 * time.Millisecond,
			Throughput:   1000,
			CFColo:       "SJC",
		},
	}
}

func TestToJSONL(t *testing.T) {
	var buf bytes.Buffer
	if err := ToJSONL([]store.Record{sampleRecord()}, &buf); err != nil {
		t.Fatalf("ToJSONL error = %v", err)
	}
	if !strings.Contains(buf.String(), "example.com") {
		t.Fatalf("expected domain in output")
	}
}

func TestToCSV(t *testing.T) {
	var buf bytes.Buffer
	if err := ToCSV([]store.Record{sampleRecord()}, &buf); err != nil {
		t.Fatalf("ToCSV error = %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "example.com") {
		t.Fatalf("expected domain in csv")
	}
	if !strings.Contains(output, "timestamp") {
		t.Fatalf("expected header")
	}
}
