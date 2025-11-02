package exporter

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/example/cf-edgescout/store"
)

// ToJSONL writes records to w as JSON Lines.
func ToJSONL(records []store.Record, w io.Writer) error {
	encoder := json.NewEncoder(w)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return nil
}

// ToCSV writes a CSV representation of the records.
func ToCSV(records []store.Record, w io.Writer) error {
	writer := csv.NewWriter(w)
	header := []string{"timestamp", "source", "score", "grade", "status", "failure_reasons", "ip", "domain", "request_host", "sni", "certificate_cn", "origin_host", "success", "latency_ms", "throughput_bps", "colo", "colo_city", "colo_country", "status_code"}
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, record := range records {
		m := record.Measurement
		latency := m.TCPDuration + m.TLSDuration + m.HTTPDuration
		failures := strings.Join(record.FailureReasons, ";")
		row := []string{
			record.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			record.Source,
			fmt.Sprintf("%.4f", record.Score),
			record.Grade,
			record.Status,
			failures,
			m.IP.String(),
			m.Domain,
			m.RequestHost,
			m.SNI,
			m.CertificateCN,
			m.OriginHost,
			fmt.Sprintf("%t", m.Success),
			fmt.Sprintf("%.2f", latency.Seconds()*1000),
			fmt.Sprintf("%.0f", m.Throughput),
			m.CFColo,
			m.Geo.City,
			m.Geo.Country,
			fmt.Sprintf("%d", m.HTTPFingerprint.StatusCode),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
