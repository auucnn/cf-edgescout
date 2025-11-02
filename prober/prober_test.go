package prober

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProberProbe(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("CF-RAY", "12345-SJC")
		w.Header().Set("CF-Worker-Upstream", "origin.example.com")
		w.Header().Set("Server", "test")
		w.Write([]byte("hello"))
	}))
	defer server.Close()

	ipStr, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	ip := net.ParseIP(ipStr)
	if ip == nil {
		t.Fatalf("failed to parse server ip")
	}

	dialer := &net.Dialer{Timeout: time.Second}
	tlsConfig := &tls.Config{ServerName: "example.com", InsecureSkipVerify: true, NextProtos: []string{"http/1.1"}}
	transport := &http.Transport{DialContext: dialer.DialContext, TLSClientConfig: tlsConfig, ForceAttemptHTTP2: false}
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	p := &Prober{Dialer: dialer, TLSConfig: tlsConfig, HTTPClient: client, HTTPMethod: http.MethodGet, HTTPPath: "/", Port: port}

	ctx := context.Background()
	m, err := p.Probe(ctx, ip, "example.com")
	if err != nil {
		t.Fatalf("Probe error = %v", err)
	}
	if !m.Success {
		t.Fatalf("expected success, got %+v", m)
	}
	if m.CFColo != "SJC" {
		t.Fatalf("expected colo SJC got %s", m.CFColo)
	}
	if m.Geo.City != "San Jose" {
		t.Fatalf("expected geo city San Jose got %s", m.Geo.City)
	}
	if m.HTTPFingerprint.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code %d", m.HTTPFingerprint.StatusCode)
	}
	if m.SNI == "" {
		t.Fatalf("expected SNI to be recorded")
	}
	if m.OriginHost != "origin.example.com" {
		t.Fatalf("expected origin host to be recorded, got %s", m.OriginHost)
	}
}

func TestMeasurementApplyValidation(t *testing.T) {
	m := &Measurement{Domain: "example.com", CertificateCN: "example.com", OriginHost: "origin.example.com", SNI: "example.com"}
	m.ApplyValidation("origin.example.com", []string{"example.com"})
	if len(m.Validation.Failures) != 0 {
		t.Fatalf("expected no failures got %v", m.Validation.Failures)
	}
	if !m.Validation.CertificateMatch || !m.Validation.OriginMatch {
		t.Fatalf("expected validation matches %+v", m.Validation)
	}

	m2 := &Measurement{Domain: "example.com", CertificateCN: "bad.com", OriginHost: "wrong.com", SNI: "example.com"}
	m2.ApplyValidation("origin.example.com", []string{"example.com"})
	if len(m2.Validation.Failures) == 0 {
		t.Fatalf("expected failures for mismatched validation")
	}
	hasCertMismatch := false
	hasOriginMismatch := false
	for _, failure := range m2.Validation.Failures {
		if failure == "certificate_cn_mismatch" {
			hasCertMismatch = true
		}
		if failure == "origin_host_mismatch" || failure == "origin_host_missing" {
			hasOriginMismatch = true
		}
	}
	if !hasCertMismatch || !hasOriginMismatch {
		t.Fatalf("expected certificate and origin mismatch failures got %v", m2.Validation.Failures)
	}
}
