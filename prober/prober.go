package prober

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/example/cf-edgescout/geo"
)

// IntegrityReport captures TLS and HTTP integrity information.
type IntegrityReport struct {
	TLSServerName   string   `json:"tlsServerName"`
	CertificateCN   string   `json:"certificateCN"`
	CertificateSANs []string `json:"certificateSANs"`
	MatchesSNI      bool     `json:"matchesSni"`
	HTTPStatus      int      `json:"httpStatus"`
	ResponseHash    string   `json:"responseHash"`
}

// LocationInfo describes the colo metadata extracted from headers.
type LocationInfo struct {
	Colo    string `json:"colo"`
	City    string `json:"city"`
	Country string `json:"country"`
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
	Source              string
	SourceType          string
	SourceWeight        float64
	Provider            string
	Network             string
	Family              string
	CertificateCN       string
	CertificateDNSNames []string
	OriginHost          string
	HTTPFingerprint     HTTPFingerprint
	Validation          ValidationResult
	Integrity           IntegrityReport
	BytesRead           int64
	Location            LocationInfo
	Timestamp           time.Time
}

// ApplyValidation evaluates the measurement against the expected origin and trusted CNs.
func (m *Measurement) ApplyValidation(expectedOrigin string, trustedCNs []string) {
	if m == nil {
		return
	}
	m.Validation.SNI = m.Domain
	m.Validation.CertificateCN = m.CertificateCN
	m.Validation.ExpectedCNs = append([]string(nil), trustedCNs...)
	m.Validation.ExpectedOrigin = expectedOrigin

	if len(trustedCNs) == 0 {
		m.Validation.CertificateMatch = true
	} else {
		lower := make([]string, 0, len(trustedCNs))
		for _, cn := range trustedCNs {
			if cn = strings.TrimSpace(strings.ToLower(cn)); cn != "" {
				lower = append(lower, cn)
			}
		}
		match := false
		candidate := strings.ToLower(m.CertificateCN)
		for _, cn := range lower {
			if candidate == cn {
				match = true
				break
			}
			for _, alt := range m.CertificateDNSNames {
				if strings.ToLower(alt) == cn {
					match = true
					break
				}
			}
			if match {
				break
			}
		}
		m.Validation.CertificateMatch = match
		if !match {
			m.Validation.Failures = append(m.Validation.Failures, "certificate_cn_mismatch")
		}
	}

	m.Validation.OriginHost = m.OriginHost
	if expectedOrigin == "" {
		m.Validation.OriginMatch = true
	} else if strings.EqualFold(expectedOrigin, m.OriginHost) {
		m.Validation.OriginMatch = true
	} else {
		m.Validation.Failures = append(m.Validation.Failures, "origin_host_mismatch")
	}
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

func (p *Prober) port() string {
	if p.Port == "" {
		return "443"
	}
	return p.Port
}

func (p *Prober) cloneTransportForIP(ip net.IP, domain string) *http.Transport {
	base, _ := p.HTTPClient.Transport.(*http.Transport)
	if base == nil {
		base = &http.Transport{}
	}
	clone := base.Clone()
	address := net.JoinHostPort(ip.String(), p.port())
	clone.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return p.Dialer.DialContext(ctx, "tcp", address)
	}
	clone.TLSClientConfig = p.TLSConfig.Clone()
	if clone.TLSClientConfig == nil {
		clone.TLSClientConfig = &tls.Config{}
	}
	clone.TLSClientConfig.ServerName = domain
	return clone
}

func (p *Prober) tlsConfigFor(domain string) *tls.Config {
	if p.TLSConfig == nil {
		return &tls.Config{ServerName: domain, NextProtos: []string{"h2", "http/1.1"}}
	}
	cfg := p.TLSConfig.Clone()
	cfg.ServerName = domain
	return cfg
}

// Probe executes TCP, TLS and HTTP measurements for the given IP.
func (p *Prober) Probe(ctx context.Context, ip net.IP, domain string) (*Measurement, error) {
	if ip == nil {
		return nil, errors.New("ip is nil")
	}
	if domain == "" {
		return nil, errors.New("domain is required")
	}
	m := &Measurement{IP: ip, Domain: domain, Timestamp: time.Now()}
	m.Integrity.TLSServerName = domain
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
	tlsConn, err := tls.DialWithDialer(p.Dialer, "tcp", address, p.tlsConfigFor(domain))
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
			m.CertificateDNSNames = append([]string(nil), cert.DNSNames...)
			m.Integrity.CertificateCN = cert.Subject.CommonName
			m.Integrity.CertificateSANs = append([]string(nil), cert.DNSNames...)
			if err := cert.VerifyHostname(domain); err == nil {
				m.Integrity.MatchesSNI = true
			}
		}
	}
	m.TLSDuration = time.Since(tlsStart)
	_ = tlsConn.Close()

	transport := p.cloneTransportForIP(ip, domain)
	client := *p.HTTPClient
	client.Transport = transport

	req, err := http.NewRequestWithContext(ctx, p.HTTPMethod, "https://"+domain+p.HTTPPath, nil)
	if err != nil {
		return nil, err
	}
	req.Host = domain
	m.RequestHost = req.Host

	httpStart := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		m.Error = fmt.Sprintf("http: %v", err)
		return m, nil
	}
	defer resp.Body.Close()
	bodyReader := io.LimitReader(resp.Body, 1<<20)
	hasher := sha256.New()
	bytesRead, readErr := io.Copy(io.Discard, io.TeeReader(bodyReader, hasher))
	if readErr != nil {
		m.Error = fmt.Sprintf("read body: %v", readErr)
	}
	m.BytesRead = bytesRead
	m.HTTPDuration = time.Since(httpStart)
	m.Integrity.HTTPStatus = resp.StatusCode
	m.Integrity.ResponseHash = hex.EncodeToString(hasher.Sum(nil))
	m.HTTPFingerprint.StatusCode = resp.StatusCode
	m.HTTPFingerprint.ContentLength = resp.ContentLength
	m.HTTPFingerprint.Headers = map[string]string{}
	for key, values := range resp.Header {
		if len(values) > 0 {
			m.HTTPFingerprint.Headers[key] = values[0]
		}
	}
	durationSeconds := m.HTTPDuration.Seconds()
	if durationSeconds > 0 {
		m.Throughput = float64(bytesRead*8) / durationSeconds
	}
	originHeaders := []string{"CF-Worker-Upstream", "CF-Worker-Subrequest", "CF-Cache-Status"}
	for _, header := range originHeaders {
		if value := strings.TrimSpace(resp.Header.Get(header)); value != "" {
			m.OriginHost = value
			break
		}
	}
	m.CFRay = resp.Header.Get("CF-Ray")
	if parts := strings.Split(m.CFRay, "-"); len(parts) == 2 {
		m.CFColo = strings.ToUpper(parts[1])
	}
	if info, ok := geo.LookupColo(m.CFColo); ok {
		m.Geo = info
		m.Location = LocationInfo{Colo: info.Code, City: info.City, Country: info.Country}
	} else {
		m.Location.Colo = m.CFColo
	}

	m.Success = resp.StatusCode >= 200 && resp.StatusCode < 400 && m.Error == ""
	return m, nil
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
