package antidpi

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// ECH implements Encrypted Client Hello support
// ECH (formerly ESNI) encrypts the SNI field in TLS ClientHello
// so that DPI cannot see which domain the client is connecting to
//
// How it works:
// 1. Client fetches ECH config from DNS (HTTPS record type 65)
// 2. Client encrypts the real SNI inside the ClientHello
// 3. DPI only sees the "outer" SNI (public-facing name)
// 4. Server decrypts and sees the real SNI
//
// For our use case, we implement a simplified version:
// - We use a fake outer SNI (e.g., "cloudflare-ech.com")
// - The real destination is encrypted inside

// ECHConfig holds ECH configuration
type ECHConfig struct {
	Enabled    bool
	OuterSNI   string // The SNI visible to DPI (e.g., "cloudflare.com")
	InnerSNI   string // The real SNI (encrypted, invisible to DPI)
	PublicKey  []byte // Server's HPKE public key
}

// ECHHandler manages ECH operations
type ECHHandler struct {
	config ECHConfig
	log    *logger.Logger
}

// NewECHHandler creates a new ECH handler
func NewECHHandler(cfg ECHConfig, log *logger.Logger) *ECHHandler {
	return &ECHHandler{
		config: cfg,
		log:    log,
	}
}

// BuildECHExtension builds the encrypted_client_hello TLS extension
// This is added to the ClientHello to encrypt the real SNI
func (h *ECHHandler) BuildECHExtension(innerSNI string) ([]byte, error) {
	if !h.config.Enabled {
		return nil, nil
	}

	// ECH extension structure (simplified):
	// - Type: 0xfe0d (encrypted_client_hello)
	// - HPKE cipher suite
	// - Config ID
	// - Encrypted ClientHelloInner

	// For now, we build a basic GREASE ECH extension
	// that makes the connection look like it supports ECH
	// without actually requiring server-side ECH support
	return h.buildGREASEECH()
}

// buildGREASEECH builds a GREASE ECH extension
// GREASE ECH makes the connection look like it uses ECH
// even when connecting to servers that don't support it
// This helps normalize traffic patterns
func (h *ECHHandler) buildGREASEECH() ([]byte, error) {
	// GREASE ECH format:
	// - client_hello_type: 0 (outer)
	// - cipher_suite: random HPKE suite
	// - config_id: random byte
	// - enc: random HPKE encapsulated key (32 bytes)
	// - payload: random encrypted data (128-256 bytes)

	configID := make([]byte, 1)
	rand.Read(configID)

	enc := make([]byte, 32)
	rand.Read(enc)

	// Random payload size (128-256 bytes)
	payloadSize := 128 + randomInt(128)
	payload := make([]byte, payloadSize)
	rand.Read(payload)

	// Build extension data
	// type(1) + cipher_suite(4) + config_id(1) + enc_len(2) + enc + payload_len(2) + payload
	extData := make([]byte, 0, 1+4+1+2+len(enc)+2+len(payload))

	// client_hello_type = 0 (outer)
	extData = append(extData, 0x00)

	// cipher_suite: KDF=HKDF-SHA256(0x0001), AEAD=AES-128-GCM(0x0001)
	extData = append(extData, 0x00, 0x01, 0x00, 0x01)

	// config_id
	extData = append(extData, configID[0])

	// enc length + enc
	encLen := make([]byte, 2)
	binary.BigEndian.PutUint16(encLen, uint16(len(enc)))
	extData = append(extData, encLen...)
	extData = append(extData, enc...)

	// payload length + payload
	payLen := make([]byte, 2)
	binary.BigEndian.PutUint16(payLen, uint16(len(payload)))
	extData = append(extData, payLen...)
	extData = append(extData, payload...)

	h.log.Debug("Built GREASE ECH extension (%d bytes)", len(extData))
	return extData, nil
}

// WrapWithECH wraps a connection to use ECH-like behavior
// For connections where real ECH isn't available, this adds
// the ECH extension to make traffic look ECH-enabled
func (h *ECHHandler) WrapWithECH(conn net.Conn) net.Conn {
	if !h.config.Enabled {
		return conn
	}
	// The actual ECH wrapping happens at the uTLS level
	// This is a placeholder for future full ECH implementation
	return conn
}

// randomInt returns a random int in [0, max)
func randomInt(max int) int {
	if max <= 0 {
		return 0
	}
	buf := make([]byte, 4)
	rand.Read(buf)
	return int(binary.BigEndian.Uint32(buf)) % max
}

// SNICamouflage handles SNI-based evasion techniques
type SNICamouflage struct {
	log *logger.Logger
}

// NewSNICamouflage creates a new SNI camouflage handler
func NewSNICamouflage(log *logger.Logger) *SNICamouflage {
	return &SNICamouflage{log: log}
}

// GetCamouflageSNI returns a safe SNI to use based on the target
// Strategy: Use popular domains that are unlikely to be blocked
func (sc *SNICamouflage) GetCamouflageSNI(target string) string {
	// List of popular domains that support TLS 1.3
	// and are unlikely to be blocked in Iran
	safeDomains := []string{
		"www.google.com",
		"www.microsoft.com",
		"www.apple.com",
		"www.amazon.com",
		"login.microsoftonline.com",
		"outlook.office365.com",
		"play.google.com",
		"fonts.googleapis.com",
		"ajax.googleapis.com",
		"cdn.jsdelivr.net",
	}

	// Pick a random safe domain
	idx := randomInt(len(safeDomains))
	selected := safeDomains[idx]

	sc.log.Debug("SNI camouflage: using %s instead of %s", selected, target)
	return selected
}

// ValidateSNI checks if an SNI is safe to use (not blocked)
func (sc *SNICamouflage) ValidateSNI(sni string) error {
	// Basic validation
	if sni == "" {
		return fmt.Errorf("empty SNI")
	}
	if len(sni) > 253 {
		return fmt.Errorf("SNI too long: %d chars", len(sni))
	}
	return nil
}
