package prober

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/example/cf-edgescout/geo"
)

// Measurement captures the outcome of probing a single IP.
type Measurement struct {
	IP                  net.IP
	Domain              string
	RequestHost         string
	TCPDuration         time.Duration
	TLSDuration         time.Duration
	HTTPDuration        time.Duration
	Success             bool
	Error               string
	ALPN                string
	TLSVersion          string
	SNI                 string
	Throughput          float64
	CFRay               string
	CFColo              string
	Geo                 geo.Info
	DataSource          string
	CertificateCN       string
	CertificateDNSNames []string
	OriginHost          string
	HTTPFingerprint     HTTPFingerprint
	Validation          ValidationResult
	Timestamp           time.Time
}

// HTTPFingerprint records the high level HTTP characteristics observed.
type HTTPFingerprint struct {
	StatusCode    int               `json:"status_code"`
	Headers       map[string]string `json:"headers,omitempty"`
	ContentLength int64             `json:"content_length"`
}

// ValidationResult captures the outcome of additional safety checks.
type ValidationResult struct {
	SNI              string   `json:"sni"`
	CertificateCN    string   `json:"certificate_cn"`
	CertificateMatch bool     `json:"certificate_match"`
	ExpectedCNs      []string `json:"expected_cns,omitempty"`
	OriginHost       string   `json:"origin_host,omitempty"`
	ExpectedOrigin   string   `json:"expected_origin,omitempty"`
	OriginMatch      bool     `json:"origin_match"`
	Failures         []string `json:"failures,omitempty"`
}

// Prober executes network measurements against Cloudflare edge IPs.
type Prober struct {
	Dialer     *net.Dialer
	TLSConfig  *tls.Config
	HTTPClient *http.Client
	HTTPMethod string
	HTTPPath   string
	Port       string
}

// New creates a Prober with sensible defaults for TLS and HTTP probing.
func New(domain string) *Prober {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	tlsConfig := &tls.Config{ServerName: domain, NextProtos: []string{"h2", "http/1.1"}}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		TLSClientConfig:       tlsConfig.Clone(),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
		DisableCompression:    true,
		ExpectContinueTimeout: 2 * time.Second,
	}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	return &Prober{
		Dialer:     dialer,
		TLSConfig:  tlsConfig,
		HTTPClient: client,
		HTTPMethod: http.MethodGet,
		HTTPPath:   "/",
		Port:       "443",
	}
}

// Probe executes TCP, TLS and HTTP measurements for the given IP.
func (p *Prober) Probe(ctx context.Context, ip net.IP, domain string) (*Measurement, error) {
	if ip == nil {
		return nil, errors.New("ip is nil")
	}
	m := &Measurement{IP: ip, Domain: domain, Timestamp: time.Now()}
	address := net.JoinHostPort(ip.String(), p.port())

	tcpStart := time.Now()
	conn, err := p.Dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		m.Error = fmt.Sprintf("tcp dial: %v", err)
		return m, nil
	}
	m.TCPDuration = time.Since(tcpStart)
	_ = conn.Close()

	tlsStart := time.Now()
	tlsConn, err := tls.DialWithDialer(p.Dialer, "tcp", address, p.TLSConfig)
	if err != nil {
		m.Error = fmt.Sprintf("tls dial: %v", err)
		return m, nil
	}
	if state := tlsConn.ConnectionState(); state.HandshakeComplete {
		m.ALPN = state.NegotiatedProtocol
		m.TLSVersion = tlsVersionString(state.Version)
		m.SNI = state.ServerName
		if len(state.PeerCertificates) > 0 {
			cert := state.PeerCertificates[0]
			m.CertificateCN = cert.Subject.CommonName
			if len(cert.DNSNames) > 0 {
				m.CertificateDNSNames = append([]string(nil), cert.DNSNames...)
			}
		}
	}
	m.TLSDuration = time.Since(tlsStart)
	_ = tlsConn.Close()

	httpStart := time.Now()
	transport := p.cloneTransportForIP(ip)
	client := *p.HTTPClient
	client.Transport = transport

	req, err := http.NewRequestWithContext(ctx, p.HTTPMethod, "https://"+domain+p.HTTPPath, nil)
	if err != nil {
		return nil, err
	}
	req.Host = domain
	m.RequestHost = req.Host
	resp, err := client.Do(req)
	if err != nil {
		m.Error = fmt.Sprintf("http: %v", err)
		return m, nil
	}
	bodyStart := time.Now()
	bytesRead, readErr := io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	duration := time.Since(bodyStart)
	if duration > 0 {
		m.Throughput = float64(bytesRead) * 8 / duration.Seconds()
	}
	m.HTTPFingerprint.StatusCode = resp.StatusCode
	m.HTTPFingerprint.ContentLength = bytesRead
	headers := map[string]string{}
	headerKeys := []string{"Server", "CF-RAY", "CF-Cache-Status", "Content-Type"}
	for _, key := range headerKeys {
		if value := resp.Header.Get(key); value != "" {
			headers[strings.ToLower(key)] = value
		}
	}
	if len(headers) > 0 {
		m.HTTPFingerprint.Headers = headers
	}
	if origin := resp.Header.Get("CF-Worker-Upstream"); origin != "" {
		m.OriginHost = origin
	} else if origin := resp.Header.Get("X-Backend-Host"); origin != "" {
		m.OriginHost = origin
	}
	if resp.TLS != nil {
		if m.ALPN == "" {
			m.ALPN = resp.TLS.NegotiatedProtocol
		}
		if m.TLSVersion == "" {
			m.TLSVersion = tlsVersionString(resp.TLS.Version)
		}
	}
	m.HTTPDuration = time.Since(httpStart)
	m.CFRay = resp.Header.Get("CF-RAY")
	if m.CFRay != "" {
		parts := strings.Split(m.CFRay, "-")
		if len(parts) > 1 {
			m.CFColo = strings.ToUpper(parts[len(parts)-1])
		}
	}
	if colo := resp.Header.Get("CF-ORIGIN-COL"); colo != "" && m.CFColo == "" {
		m.CFColo = strings.ToUpper(colo)
	}
	if m.SNI == "" {
		m.SNI = p.TLSConfig.ServerName
	}
	if info, ok := geo.LookupColo(m.CFColo); ok {
		m.Geo = info
	}
	m.Success = readErr == nil
	if readErr != nil {
		m.Error = fmt.Sprintf("read body: %v", readErr)
	}
	return m, nil
}

// ApplyValidation performs post-processing checks based on expected upstream data.
func (m *Measurement) ApplyValidation(expectedOrigin string, trustedCNs []string) {
	m.Validation.SNI = m.SNI
	m.Validation.CertificateCN = m.CertificateCN
	m.Validation.ExpectedOrigin = expectedOrigin
	if len(trustedCNs) > 0 {
		m.Validation.ExpectedCNs = append([]string(nil), trustedCNs...)
	} else {
		m.Validation.ExpectedCNs = nil
	}
	actualOrigin := strings.TrimSpace(m.OriginHost)
	m.Validation.OriginHost = actualOrigin
	var failures []string

	matchCN := true
	if len(trustedCNs) > 0 {
		matchCN = false
		for _, cn := range trustedCNs {
			if strings.EqualFold(cn, m.CertificateCN) {
				matchCN = true
				break
			}
		}
	} else if m.CertificateCN != "" && m.Domain != "" {
		matchCN = strings.EqualFold(m.CertificateCN, m.Domain)
	}
	m.Validation.CertificateMatch = matchCN
	if !matchCN {
		failures = append(failures, "certificate_cn_mismatch")
	}

	if expectedOrigin != "" {
		if actualOrigin == "" {
			m.Validation.OriginMatch = false
			failures = append(failures, "origin_host_missing")
		} else {
			matchOrigin := strings.EqualFold(expectedOrigin, actualOrigin)
			m.Validation.OriginMatch = matchOrigin
			if !matchOrigin {
				failures = append(failures, "origin_host_mismatch")
			}
		}
	} else {
		m.Validation.OriginMatch = true
	}
	if len(failures) > 0 {
		m.Validation.Failures = failures
	} else {
		m.Validation.Failures = nil
	}
}

func (p *Prober) cloneTransportForIP(ip net.IP) *http.Transport {
	base, _ := p.HTTPClient.Transport.(*http.Transport)
	if base == nil {
		base = &http.Transport{DialContext: p.Dialer.DialContext, TLSClientConfig: p.TLSConfig.Clone()}
	}
	clone := base.Clone()
	clone.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		target := net.JoinHostPort(ip.String(), p.port())
		return p.Dialer.DialContext(ctx, "tcp", target)
	}
	clone.TLSClientConfig = base.TLSClientConfig.Clone()
	clone.TLSClientConfig.ServerName = p.TLSConfig.ServerName
	return clone
}

func (p *Prober) port() string {
	if p.Port != "" {
		return p.Port
	}
	return "443"
}

func tlsVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS13:
		return "TLS1.3"
	case tls.VersionTLS12:
		return "TLS1.2"
	case tls.VersionTLS11:
		return "TLS1.1"
	case tls.VersionTLS10:
		return "TLS1.0"
	default:
		return "unknown"
	}
}
