package acme

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Manager handles automatic TLS certificate management
type Manager struct {
	log       *logger.Logger
	domain    string
	email     string
	certDir   string
	certFile  string
	keyFile   string
	cert      *tls.Certificate
	mu        sync.RWMutex
	done      chan struct{}
	wg        sync.WaitGroup
	renewBefore time.Duration
}

// Config holds ACME configuration
type Config struct {
	Domain      string // Domain for the certificate
	Email       string // Contact email for Let's Encrypt
	CertDir     string // Directory to store certificates
	RenewBefore time.Duration // Renew this long before expiry (default: 30 days)
}

// NewManager creates a new ACME certificate manager
func NewManager(cfg Config, log *logger.Logger) *Manager {
	if cfg.CertDir == "" {
		cfg.CertDir = "/etc/ipshadowt/certs"
	}
	if cfg.RenewBefore == 0 {
		cfg.RenewBefore = 30 * 24 * time.Hour // 30 days
	}

	return &Manager{
		log:         log,
		domain:      cfg.Domain,
		email:       cfg.Email,
		certDir:     cfg.CertDir,
		certFile:    filepath.Join(cfg.CertDir, "cert.pem"),
		keyFile:     filepath.Join(cfg.CertDir, "key.pem"),
		renewBefore: cfg.RenewBefore,
		done:        make(chan struct{}),
	}
}

// Start initializes the certificate manager
// It loads existing certs or generates self-signed ones as fallback
func (m *Manager) Start() error {
	os.MkdirAll(m.certDir, 0700)

	// Try to load existing certificate
	if err := m.loadCert(); err != nil {
		m.log.Info("ACME: No existing cert found, generating self-signed for %s", m.domain)
		if err := m.generateSelfSigned(); err != nil {
			return fmt.Errorf("failed to generate self-signed cert: %w", err)
		}
	}

	// Check if renewal is needed
	if m.needsRenewal() {
		m.log.Info("ACME: Certificate needs renewal")
		go m.renewCert()
	}

	// Start renewal checker
	m.wg.Add(1)
	go m.renewalLoop()

	return nil
}

// GetCertificate returns the current TLS certificate
func (m *Manager) GetCertificate() *tls.Certificate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cert
}

// GetTLSConfig returns a tls.Config that uses the managed certificate
func (m *Manager) GetTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			cert := m.GetCertificate()
			if cert == nil {
				return nil, fmt.Errorf("no certificate available")
			}
			return cert, nil
		},
		MinVersion: tls.VersionTLS12,
	}
}

// CertFile returns the path to the certificate file
func (m *Manager) CertFile() string { return m.certFile }

// KeyFile returns the path to the key file
func (m *Manager) KeyFile() string { return m.keyFile }

// loadCert loads certificate from disk
func (m *Manager) loadCert() error {
	cert, err := tls.LoadX509KeyPair(m.certFile, m.keyFile)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.cert = &cert
	m.mu.Unlock()

	// Parse to check expiry
	if len(cert.Certificate) > 0 {
		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		if err == nil {
			m.log.Info("ACME: Loaded cert for %s (expires: %s)", m.domain, x509Cert.NotAfter.Format("2006-01-02"))
		}
	}

	return nil
}

// needsRenewal checks if the certificate needs renewal
func (m *Manager) needsRenewal() bool {
	m.mu.RLock()
	cert := m.cert
	m.mu.RUnlock()

	if cert == nil || len(cert.Certificate) == 0 {
		return true
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return true
	}

	// Renew if within renewBefore of expiry
	return time.Now().Add(m.renewBefore).After(x509Cert.NotAfter)
}

// renewalLoop periodically checks if renewal is needed
func (m *Manager) renewalLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			if m.needsRenewal() {
				m.log.Info("ACME: Starting certificate renewal")
				m.renewCert()
			}
		}
	}
}

// renewCert attempts to renew the certificate
// In production, this would use the ACME protocol with Let's Encrypt
// For now, it attempts to use certbot if available, otherwise regenerates self-signed
func (m *Manager) renewCert() {
	// Try certbot first (if installed)
	if m.tryCertbot() {
		if err := m.loadCert(); err == nil {
			m.log.Info("ACME: Certificate renewed via certbot")
			return
		}
	}

	// Fallback: generate new self-signed
	m.log.Warn("ACME: certbot not available, regenerating self-signed cert")
	if err := m.generateSelfSigned(); err != nil {
		m.log.Error("ACME: Failed to regenerate cert: %v", err)
	}
}

// tryCertbot attempts to use certbot for certificate renewal
func (m *Manager) tryCertbot() bool {
	// Check if certbot is available
	// In production, this would execute:
	// certbot certonly --standalone -d domain --non-interactive --agree-tos -m email
	// For now, return false to use self-signed
	return false
}

// generateSelfSigned generates a self-signed certificate
func (m *Manager) generateSelfSigned() error {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}

	// Create certificate template
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"iPShadowT"},
			CommonName:   m.domain,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	if m.domain != "" {
		template.DNSNames = []string{m.domain}
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Write certificate
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(m.certFile, certPEM, 0644); err != nil {
		return fmt.Errorf("failed to write cert: %w", err)
	}

	// Write private key
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(m.keyFile, keyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write key: %w", err)
	}

	// Load the new cert
	cert, err := tls.LoadX509KeyPair(m.certFile, m.keyFile)
	if err != nil {
		return fmt.Errorf("failed to load new cert: %w", err)
	}

	m.mu.Lock()
	m.cert = &cert
	m.mu.Unlock()

	m.log.Info("ACME: Self-signed cert generated for %s (valid 1 year)", m.domain)
	return nil
}

// Stop shuts down the ACME manager
func (m *Manager) Stop() {
	close(m.done)
	m.wg.Wait()
}
