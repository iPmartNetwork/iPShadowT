package antidpi

import (
	"crypto/tls"
	"fmt"
	"net"

	utls "github.com/refraction-networking/utls"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// UTLSFingerprint represents a browser TLS fingerprint to mimic
type UTLSFingerprint string

const (
	FingerprintChrome     UTLSFingerprint = "chrome"
	FingerprintFirefox    UTLSFingerprint = "firefox"
	FingerprintSafari     UTLSFingerprint = "safari"
	FingerprintEdge       UTLSFingerprint = "edge"
	FingerprintIOS        UTLSFingerprint = "ios"
	FingerprintAndroid    UTLSFingerprint = "android"
	FingerprintRandom     UTLSFingerprint = "random"
	FingerprintRandomized UTLSFingerprint = "randomized"
)

// UTLSDialer wraps connections with uTLS to mimic browser fingerprints
type UTLSDialer struct {
	fingerprint UTLSFingerprint
	serverName  string
	log         *logger.Logger
}

// NewUTLSDialer creates a new uTLS dialer
func NewUTLSDialer(fingerprint string, serverName string, log *logger.Logger) *UTLSDialer {
	return &UTLSDialer{
		fingerprint: UTLSFingerprint(fingerprint),
		serverName:  serverName,
		log:         log,
	}
}

// WrapConn wraps a plain TCP connection with uTLS
func (d *UTLSDialer) WrapConn(conn net.Conn, sni string) (net.Conn, error) {
	if sni == "" {
		sni = d.serverName
	}

	tlsConfig := &utls.Config{
		ServerName:         sni,
		InsecureSkipVerify: true,
	}

	clientHelloID := d.getClientHelloID()

	utlsConn := utls.UClient(conn, tlsConfig, clientHelloID)

	if err := utlsConn.Handshake(); err != nil {
		return nil, fmt.Errorf("uTLS handshake failed: %w", err)
	}

	d.log.Debug("uTLS handshake complete (fingerprint: %s, SNI: %s)", d.fingerprint, sni)
	return utlsConn, nil
}

// getClientHelloID returns the uTLS ClientHelloID for the configured fingerprint
func (d *UTLSDialer) getClientHelloID() utls.ClientHelloID {
	switch d.fingerprint {
	case FingerprintChrome:
		return utls.HelloChrome_Auto
	case FingerprintFirefox:
		return utls.HelloFirefox_Auto
	case FingerprintSafari:
		return utls.HelloSafari_Auto
	case FingerprintEdge:
		return utls.HelloEdge_Auto
	case FingerprintIOS:
		return utls.HelloIOS_Auto
	case FingerprintAndroid:
		return utls.HelloAndroid_11_OkHttp
	case FingerprintRandom:
		return utls.HelloRandomized
	case FingerprintRandomized:
		return utls.HelloRandomized
	default:
		return utls.HelloChrome_Auto
	}
}

// GetAvailableFingerprints returns list of available fingerprints
func GetAvailableFingerprints() []string {
	return []string{
		"chrome", "firefox", "safari", "edge",
		"ios", "android", "random", "randomized",
	}
}

// UTLSListener wraps a TLS listener that accepts connections
// and presents a valid TLS certificate (for server-side)
type UTLSListener struct {
	inner      net.Listener
	tlsConfig  *tls.Config
	log        *logger.Logger
}

// NewUTLSListener creates a listener with proper TLS config
func NewUTLSListener(inner net.Listener, certFile, keyFile string, log *logger.Logger) (*UTLSListener, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		},
	}

	return &UTLSListener{
		inner:     inner,
		tlsConfig: tlsConfig,
		log:       log,
	}, nil
}
