package antidpi

import (
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"
)

// RealityAuth handles the complete REALITY authentication flow
// The auth token is embedded in the TLS ClientHello's session ID field (32 bytes)
//
// Protocol:
// 1. Client generates ephemeral X25519 key pair
// 2. Client computes shared secret: ECDH(ephemeral_private, server_public)
// 3. Client builds auth token: HMAC-SHA256(shared_secret, timestamp + short_id)
// 4. Client places: [ephemeral_public_key(32)] in key_share extension
//    and [auth_token(16) + short_id(8) + timestamp(4) + padding(4)] in session_id
// 5. Server extracts ephemeral public key from key_share
// 6. Server computes same shared secret: ECDH(server_private, ephemeral_public)
// 7. Server verifies auth token with same HMAC computation
// 8. If valid → tunnel mode; if invalid → proxy to fallback

// AuthToken represents a REALITY authentication token
type AuthToken struct {
	Token     [16]byte // HMAC-SHA256 truncated to 16 bytes
	ShortID   [8]byte  // Client identifier
	Timestamp uint32   // Unix timestamp (for replay protection)
	Padding   [4]byte  // Random padding
}

// AuthTokenSize is the total size of the auth token (fits in session_id: 32 bytes)
const AuthTokenSize = 32

// BuildAuthToken creates an authentication token for the ClientHello session_id
func BuildAuthToken(serverPublicKey *ecdh.PublicKey, shortID string, timeWindow int) (*AuthToken, *ecdh.PrivateKey, error) {
	// Generate ephemeral key pair
	ephemeralKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	// Compute shared secret
	sharedSecret, err := ephemeralKey.ECDH(serverPublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("ECDH failed: %w", err)
	}

	// Current timestamp (truncated to 4 bytes)
	now := uint32(time.Now().Unix())

	// Build auth data: timestamp + short_id
	authData := make([]byte, 0, 12)
	tsBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(tsBuf, now)
	authData = append(authData, tsBuf...)
	authData = append(authData, []byte(shortID)...)

	// Compute HMAC
	mac := hmac.New(sha256.New, sharedSecret)
	mac.Write([]byte("iPShadowT-REALITY-v1"))
	mac.Write(authData)
	fullMAC := mac.Sum(nil)

	// Build token
	token := &AuthToken{
		Timestamp: now,
	}
	copy(token.Token[:], fullMAC[:16])

	// Copy short ID (pad or truncate to 8 bytes)
	shortIDBytes := []byte(shortID)
	if len(shortIDBytes) > 8 {
		shortIDBytes = shortIDBytes[:8]
	}
	copy(token.ShortID[:], shortIDBytes)

	// Random padding
	rand.Read(token.Padding[:])

	return token, ephemeralKey, nil
}

// VerifyAuthToken verifies a received auth token on the server side
func VerifyAuthToken(tokenBytes []byte, ephemeralPublicKeyBytes []byte, serverPrivateKey *ecdh.PrivateKey, allowedShortIDs []string, timeWindow int) (bool, string) {
	if len(tokenBytes) != AuthTokenSize {
		return false, ""
	}

	if len(ephemeralPublicKeyBytes) != 32 {
		return false, ""
	}

	// Parse ephemeral public key
	ephemeralPublicKey, err := ecdh.X25519().NewPublicKey(ephemeralPublicKeyBytes)
	if err != nil {
		return false, ""
	}

	// Compute shared secret
	sharedSecret, err := serverPrivateKey.ECDH(ephemeralPublicKey)
	if err != nil {
		return false, ""
	}

	// Parse token
	var token AuthToken
	copy(token.Token[:], tokenBytes[:16])
	copy(token.ShortID[:], tokenBytes[16:24])
	token.Timestamp = binary.BigEndian.Uint32(tokenBytes[24:28])

	// Check timestamp (allow timeWindow seconds of drift)
	now := uint32(time.Now().Unix())
	if timeWindow <= 0 {
		timeWindow = 120 // Default: 2 minutes
	}
	diff := int64(now) - int64(token.Timestamp)
	if diff < 0 {
		diff = -diff
	}
	if diff > int64(timeWindow) {
		return false, "" // Token expired or from future
	}

	// Check short ID
	shortID := string(trimNull(token.ShortID[:]))
	found := false
	for _, allowed := range allowedShortIDs {
		if shortID == allowed {
			found = true
			break
		}
	}
	if !found {
		return false, ""
	}

	// Verify HMAC
	authData := make([]byte, 0, 12)
	tsBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(tsBuf, token.Timestamp)
	authData = append(authData, tsBuf...)
	authData = append(authData, []byte(shortID)...)

	mac := hmac.New(sha256.New, sharedSecret)
	mac.Write([]byte("iPShadowT-REALITY-v1"))
	mac.Write(authData)
	expectedMAC := mac.Sum(nil)

	// Constant-time comparison
	if !hmac.Equal(token.Token[:], expectedMAC[:16]) {
		return false, ""
	}

	return true, shortID
}

// SerializeAuthToken serializes an auth token to bytes (for session_id field)
func SerializeAuthToken(token *AuthToken) []byte {
	buf := make([]byte, AuthTokenSize)
	copy(buf[:16], token.Token[:])
	copy(buf[16:24], token.ShortID[:])
	binary.BigEndian.PutUint32(buf[24:28], token.Timestamp)
	copy(buf[28:32], token.Padding[:])
	return buf
}

// trimNull removes null bytes from the end of a byte slice
func trimNull(b []byte) []byte {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] != 0 {
			return b[:i+1]
		}
	}
	return b[:0]
}

// ReplayProtection prevents replay attacks by tracking seen tokens
type ReplayProtection struct {
	seen    map[string]time.Time
	window  time.Duration
	maxSize int
}

// NewReplayProtection creates a new replay protection instance
func NewReplayProtection(window time.Duration) *ReplayProtection {
	return &ReplayProtection{
		seen:    make(map[string]time.Time),
		window:  window,
		maxSize: 10000,
	}
}

// Check returns true if the token has NOT been seen before (is fresh)
func (rp *ReplayProtection) Check(tokenBytes []byte) bool {
	key := string(tokenBytes)

	// Check if seen
	if _, exists := rp.seen[key]; exists {
		return false // Replay detected
	}

	// Add to seen
	rp.seen[key] = time.Now()

	// Cleanup old entries periodically
	if len(rp.seen) > rp.maxSize {
		rp.cleanup()
	}

	return true
}

// cleanup removes expired entries
func (rp *ReplayProtection) cleanup() {
	cutoff := time.Now().Add(-rp.window)
	for k, t := range rp.seen {
		if t.Before(cutoff) {
			delete(rp.seen, k)
		}
	}
}
