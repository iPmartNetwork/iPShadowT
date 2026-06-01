package antidpi

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

const (
	// RealityAuthOK is sent by the server after successful REALITY authentication.
	RealityAuthOK = "REALITYOK\n"
)

// parseClientHelloAuth extracts the REALITY session ID and X25519 key share
// from a TLS ClientHello record (5-byte record header + handshake payload).
func parseClientHelloAuth(rawData []byte) (sessionID []byte, ephPublicKey []byte, err error) {
	if len(rawData) < 5 {
		return nil, nil, fmt.Errorf("record too short")
	}
	if rawData[0] != 0x16 {
		return nil, nil, fmt.Errorf("not a TLS handshake record")
	}

	recordLen := int(rawData[3])<<8 | int(rawData[4])
	if recordLen+5 > len(rawData) {
		return nil, nil, fmt.Errorf("incomplete TLS record")
	}

	payload := rawData[5 : 5+recordLen]
	if len(payload) < 4 || payload[0] != 0x01 {
		return nil, nil, fmt.Errorf("not a ClientHello")
	}

	pos := 4 // skip handshake type + 3-byte length
	if len(payload) < pos+2+32+1 {
		return nil, nil, fmt.Errorf("ClientHello truncated")
	}

	pos += 2 // legacy version
	pos += 32 // random

	sessLen := int(payload[pos])
	pos++
	if len(payload) < pos+sessLen {
		return nil, nil, fmt.Errorf("session ID truncated")
	}
	sessionID = make([]byte, sessLen)
	copy(sessionID, payload[pos:pos+sessLen])
	pos += sessLen

	if len(payload) < pos+2 {
		return nil, nil, fmt.Errorf("cipher suites truncated")
	}
	cipherLen := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
	pos += 2 + cipherLen

	if len(payload) < pos+1 {
		return nil, nil, fmt.Errorf("compression methods truncated")
	}
	compLen := int(payload[pos])
	pos += 1 + compLen

	if len(payload) < pos+2 {
		return nil, nil, fmt.Errorf("no extensions")
	}
	extTotalLen := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
	pos += 2
	extEnd := pos + extTotalLen
	if extEnd > len(payload) {
		return nil, nil, fmt.Errorf("extensions truncated")
	}

	for pos+4 <= extEnd {
		extType := binary.BigEndian.Uint16(payload[pos : pos+2])
		extLen := int(binary.BigEndian.Uint16(payload[pos+2 : pos+4]))
		pos += 4
		if pos+extLen > extEnd {
			break
		}
		extData := payload[pos : pos+extLen]
		pos += extLen

		// key_share extension (TLS 1.3)
		if extType == 0x0033 {
			key, kerr := parseKeyShareX25519(extData)
			if kerr == nil {
				ephPublicKey = key
			}
		}
	}

	if len(sessionID) != AuthTokenSize {
		return nil, nil, fmt.Errorf("invalid session ID length: %d", len(sessionID))
	}
	if len(ephPublicKey) != 32 {
		return nil, nil, fmt.Errorf("missing X25519 key share")
	}

	return sessionID, ephPublicKey, nil
}

func parseKeyShareX25519(extData []byte) ([]byte, error) {
	if len(extData) < 2 {
		return nil, fmt.Errorf("key_share too short")
	}
	pos := 2 // skip list length
	for pos+4 <= len(extData) {
		group := binary.BigEndian.Uint16(extData[pos : pos+2])
		keyLen := int(binary.BigEndian.Uint16(extData[pos+2 : pos+4]))
		pos += 4
		if pos+keyLen > len(extData) {
			break
		}
		key := extData[pos : pos+keyLen]
		pos += keyLen
		// X25519 (29) or legacy 0x001d
		if group == 0x001d && len(key) == 32 {
			return key, nil
		}
	}
	return nil, fmt.Errorf("X25519 key share not found")
}

// normalizeShortIDBytes decodes hex short IDs or pads ASCII to 8 bytes.
func normalizeShortIDBytes(shortID string) []byte {
	out := make([]byte, 8)
	if b, err := decodeHexFlexible(shortID); err == nil && len(b) > 0 {
		if len(b) > 8 {
			b = b[:8]
		}
		copy(out, b)
		return out
	}
	s := []byte(shortID)
	if len(s) > 8 {
		s = s[:8]
	}
	copy(out, s)
	return out
}

func decodeHexFlexible(s string) ([]byte, error) {
	if len(s)%2 == 1 {
		s = "0" + s
	}
	return hex.DecodeString(s)
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
