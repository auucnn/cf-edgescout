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
		w.Header().Set("CF-RAY", "12345-XYZ")
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
	if m.CFColo != "XYZ" {
		t.Fatalf("expected colo XYZ got %s", m.CFColo)
	}
}
