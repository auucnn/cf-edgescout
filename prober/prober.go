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

// Measurement captures the outcome of probing a single IP.
type IntegrityReport struct {
	TLSServerName   string   `json:"tlsServerName"`
	CertificateCN   string   `json:"certificateCN"`
	CertificateSANs []string `json:"certificateSANs"`
	MatchesSNI      bool     `json:"matchesSni"`
	HTTPStatus      int      `json:"httpStatus"`
	ResponseHash    string   `json:"responseHash"`
}

type LocationInfo struct {
	Colo    string `json:"colo"`
	City    string `json:"city"`
	Country string `json:"country"`
}

type Measurement struct {
	IP           net.IP          `json:"ip"`
	Domain       string          `json:"domain"`
	TCPDuration  time.Duration   `json:"tcpDuration"`
	TLSDuration  time.Duration   `json:"tlsDuration"`
	HTTPDuration time.Duration   `json:"httpDuration"`
	Success      bool            `json:"success"`
	Error        string          `json:"error"`
	ALPN         string          `json:"alpn"`
	TLSVersion   string          `json:"tlsVersion"`
	Throughput   float64         `json:"throughput"`
	CFRay        string          `json:"cfRay"`
	CFColo       string          `json:"cfColo"`
	Timestamp    time.Time       `json:"timestamp"`
	Source       string          `json:"source"`
	SourceType   string          `json:"sourceType"`
	SourceWeight float64         `json:"sourceWeight"`
	Provider     string          `json:"provider"`
	Network      string          `json:"network"`
	Family       string          `json:"family"`
	BytesRead    int64           `json:"bytesRead"`
	Integrity    IntegrityReport `json:"integrity"`
	Location     LocationInfo    `json:"location"`
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
	tlsConn, err := tls.DialWithDialer(p.Dialer, "tcp", address, p.TLSConfig)
	if err != nil {
		m.Error = fmt.Sprintf("tls dial: %v", err)
		return m, nil
	}
	if state := tlsConn.ConnectionState(); state.HandshakeComplete {
		m.ALPN = state.NegotiatedProtocol
		m.TLSVersion = tlsVersionString(state.Version)
		if len(state.PeerCertificates) > 0 {
			cert := state.PeerCertificates[0]
			m.Integrity.CertificateCN = cert.Subject.CommonName
			m.Integrity.CertificateSANs = append([]string{}, cert.DNSNames...)
			if err := cert.VerifyHostname(domain); err == nil {
				m.Integrity.MatchesSNI = true
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
	resp, err := client.Do(req)
	if err != nil {
		m.Error = fmt.Sprintf("http: %v", err)
		return m, nil
	}
	m.Integrity.HTTPStatus = resp.StatusCode
	bodyStart := time.Now()
	hasher := sha256.New()
	bytesRead, readErr := io.Copy(io.MultiWriter(io.Discard, hasher), resp.Body)
	_ = resp.Body.Close()
	duration := time.Since(bodyStart)
	m.BytesRead = bytesRead
	if duration > 0 {
		m.Throughput = float64(bytesRead) * 8 / duration.Seconds()
	}
	m.Integrity.ResponseHash = hex.EncodeToString(hasher.Sum(nil))
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
	m.Success = readErr == nil && resp.StatusCode < 500
	if readErr != nil {
		m.Error = fmt.Sprintf("read body: %v", readErr)
	}
	if m.CFColo != "" {
		if info, ok := geo.LookupColo(m.CFColo); ok {
			m.Location = LocationInfo{Colo: info.Code, City: info.City, Country: info.Country}
		} else {
			m.Location = LocationInfo{Colo: m.CFColo}
		}
	}
	return m, nil
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
