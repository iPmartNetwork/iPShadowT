package stealth

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// DomainFronting implements domain fronting technique
// Key idea: The SNI (visible to DPI) shows one domain (e.g., allowed.com)
// but the HTTP Host header (encrypted inside TLS) shows the real destination
//
// How it works in Iran's context:
// 1. TLS ClientHello SNI = "fonts.googleapis.com" (never blocked)
// 2. After TLS handshake, HTTP Host = "your-tunnel-server.com"
// 3. CDN routes based on Host header, not SNI
// 4. DPI only sees traffic to Google/Cloudflare (allowed)
//
// Works with: Cloudflare, Google Cloud, Amazon CloudFront, Fastly
type DomainFronting struct {
	frontDomain string // Domain in SNI (e.g., "allowed.google.com")
	realDomain  string // Real domain in Host header
	cdnIP       string // CDN IP to connect to
	log         *logger.Logger
}

// DomainFrontConfig holds domain fronting configuration
type DomainFrontConfig struct {
	FrontDomain string // SNI domain (must be on same CDN as real domain)
	RealDomain  string // Actual tunnel server domain
	CDNIP       string // Optional: specific CDN IP to use
}

// NewDomainFronting creates a new domain fronting handler
func NewDomainFronting(cfg DomainFrontConfig, log *logger.Logger) *DomainFronting {
	return &DomainFronting{
		frontDomain: cfg.FrontDomain,
		realDomain:  cfg.RealDomain,
		cdnIP:       cfg.CDNIP,
		log:         log,
	}
}

// Connect establishes a domain-fronted connection
func (df *DomainFronting) Connect() (net.Conn, error) {
	// Determine target IP
	target := df.cdnIP
	if target == "" {
		// Resolve the front domain to get CDN IP
		ips, err := net.LookupIP(df.frontDomain)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve front domain: %w", err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no IPs for front domain")
		}
		target = ips[0].String()
	}

	// TLS connection with front domain as SNI
	tlsConfig := &tls.Config{
		ServerName: df.frontDomain, // This is what DPI sees
	}

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 15 * time.Second},
		"tcp",
		target+":443",
		tlsConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("TLS dial failed: %w", err)
	}

	df.log.Debug("Domain fronting: SNI=%s, Host=%s, IP=%s", df.frontDomain, df.realDomain, target)
	return conn, nil
}

// MakeRequest makes an HTTP request with domain fronting
func (df *DomainFronting) MakeRequest(method, path string, body []byte) (*http.Response, error) {
	target := df.cdnIP
	if target == "" {
		ips, _ := net.LookupIP(df.frontDomain)
		if len(ips) > 0 {
			target = ips[0].String()
		}
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			ServerName: df.frontDomain, // SNI = front domain
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	url := fmt.Sprintf("https://%s%s", target, path)
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	// Host header = real domain (CDN routes based on this)
	req.Host = df.realDomain
	req.Header.Set("Host", df.realDomain)

	return client.Do(req)
}

// GetFrontableDomains returns domains known to work for fronting
// These are domains hosted on the same CDN as common tunnel servers
func GetFrontableDomains() map[string][]string {
	return map[string][]string{
		"cloudflare": {
			"www.cloudflare.com",
			"cdnjs.cloudflare.com",
			"ajax.cloudflare.com",
		},
		"google": {
			"fonts.googleapis.com",
			"ajax.googleapis.com",
			"www.google.com",
			"maps.googleapis.com",
			"translate.googleapis.com",
		},
		"amazon": {
			"d1.awsstatic.com",
			"images-na.ssl-images-amazon.com",
		},
		"fastly": {
			"cdn.jsdelivr.net",
			"github.githubassets.com",
		},
	}
}
