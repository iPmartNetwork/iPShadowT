package stealth

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// ProtocolMorph transforms tunnel traffic to look like other protocols
// DPI identifies protocols by their "magic bytes" and packet patterns
// We make our traffic look like allowed protocols
//
// Supported morphs:
// - HTTP/1.1 (looks like web browsing)
// - HTTP/2 (looks like modern web)
// - TLS 1.3 Application Data (looks like encrypted web)
// - DNS over TCP (looks like DNS queries)
type ProtocolMorph struct {
	morphType string
	log       *logger.Logger
}

// MorphType constants
const (
	MorphHTTP1  = "http1"
	MorphHTTP2  = "http2"
	MorphTLS13  = "tls13"
	MorphDNSTCP = "dnstcp"
)

// NewProtocolMorph creates a new protocol morpher
func NewProtocolMorph(morphType string, log *logger.Logger) *ProtocolMorph {
	return &ProtocolMorph{
		morphType: morphType,
		log:       log,
	}
}

// WrapConn wraps a connection with protocol morphing
func (pm *ProtocolMorph) WrapConn(conn net.Conn) net.Conn {
	return &MorphedConn{
		Conn:      conn,
		morphType: pm.morphType,
		isFirst:   true,
	}
}

// MorphedConn wraps a connection with protocol morphing
type MorphedConn struct {
	net.Conn
	morphType string
	isFirst   bool
}

// Write adds protocol-specific framing to outgoing data
func (mc *MorphedConn) Write(p []byte) (int, error) {
	var frame []byte

	switch mc.morphType {
	case MorphHTTP1:
		frame = wrapAsHTTP1(p, mc.isFirst)
	case MorphHTTP2:
		frame = wrapAsHTTP2(p)
	case MorphTLS13:
		frame = wrapAsTLS13AppData(p)
	case MorphDNSTCP:
		frame = wrapAsDNSTCP(p)
	default:
		frame = p
	}

	mc.isFirst = false
	_, err := mc.Conn.Write(frame)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// wrapAsHTTP1 makes data look like HTTP/1.1 chunked response
func wrapAsHTTP1(data []byte, isFirst bool) []byte {
	if isFirst {
		header := "HTTP/1.1 200 OK\r\n" +
			"Content-Type: application/octet-stream\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Server: nginx/1.24.0\r\n" +
			"Date: " + time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT") + "\r\n" +
			"\r\n"
		chunk := fmt.Sprintf("%x\r\n", len(data))
		result := make([]byte, 0, len(header)+len(chunk)+len(data)+2)
		result = append(result, []byte(header)...)
		result = append(result, []byte(chunk)...)
		result = append(result, data...)
		result = append(result, '\r', '\n')
		return result
	}
	chunk := fmt.Sprintf("%x\r\n", len(data))
	result := make([]byte, 0, len(chunk)+len(data)+2)
	result = append(result, []byte(chunk)...)
	result = append(result, data...)
	result = append(result, '\r', '\n')
	return result
}

// wrapAsHTTP2 makes data look like HTTP/2 DATA frame
func wrapAsHTTP2(data []byte) []byte {
	frame := make([]byte, 9+len(data))
	frame[0] = byte(len(data) >> 16)
	frame[1] = byte(len(data) >> 8)
	frame[2] = byte(len(data))
	frame[3] = 0x00 // Type: DATA
	frame[4] = 0x00 // Flags: none
	binary.BigEndian.PutUint32(frame[5:9], 1) // Stream ID: 1
	copy(frame[9:], data)
	return frame
}

// wrapAsTLS13AppData makes data look like TLS 1.3 Application Data record
func wrapAsTLS13AppData(data []byte) []byte {
	record := make([]byte, 5+len(data))
	record[0] = 0x17 // Application Data
	record[1] = 0x03 // TLS 1.2 record version (TLS 1.3 uses this)
	record[2] = 0x03
	binary.BigEndian.PutUint16(record[3:5], uint16(len(data)))
	copy(record[5:], data)
	return record
}

// wrapAsDNSTCP makes data look like DNS over TCP response
func wrapAsDNSTCP(data []byte) []byte {
	msg := make([]byte, 2+12+len(data))
	binary.BigEndian.PutUint16(msg[0:2], uint16(12+len(data)))
	// Fake DNS response header
	randID := make([]byte, 2)
	rand.Read(randID)
	copy(msg[2:4], randID)
	msg[4] = 0x81
	msg[5] = 0x80
	msg[6] = 0x00
	msg[7] = 0x01
	msg[8] = 0x00
	msg[9] = 0x01
	copy(msg[14:], data)
	return msg
}
