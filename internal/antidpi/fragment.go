package antidpi

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Fragmenter splits TLS ClientHello and TCP packets into smaller fragments
// to bypass DPI that inspects the first packet for SNI or protocol fingerprints
type Fragmenter struct {
	enabled   bool
	minSize   int
	maxSize   int
	delay     time.Duration // delay between fragments
	log       *logger.Logger
}

// NewFragmenter creates a new TLS/TCP fragmenter
// sizeRange format: "min-max" (e.g., "50-100")
func NewFragmenter(sizeRange string, enabled bool, log *logger.Logger) (*Fragmenter, error) {
	if !enabled {
		return &Fragmenter{enabled: false, log: log}, nil
	}

	parts := strings.Split(sizeRange, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid fragment size range: %q", sizeRange)
	}

	minSize, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("invalid min fragment size: %w", err)
	}

	maxSize, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, fmt.Errorf("invalid max fragment size: %w", err)
	}

	if minSize < 1 || maxSize < minSize {
		return nil, fmt.Errorf("invalid fragment range: %d-%d", minSize, maxSize)
	}

	return &Fragmenter{
		enabled: true,
		minSize: minSize,
		maxSize: maxSize,
		delay:   10 * time.Millisecond, // small delay between fragments
		log:     log,
	}, nil
}

// FragmentedWrite writes data in random-sized fragments
// This is particularly effective against DPI that only inspects the first TCP segment
func (f *Fragmenter) FragmentedWrite(conn net.Conn, data []byte) error {
	if !f.enabled || len(data) <= f.minSize {
		_, err := conn.Write(data)
		return err
	}

	offset := 0
	fragmentCount := 0

	for offset < len(data) {
		// Random fragment size
		fragSize, err := f.randomSize()
		if err != nil {
			return err
		}

		// Don't exceed remaining data
		end := offset + fragSize
		if end > len(data) {
			end = len(data)
		}

		// Write fragment
		if _, err := conn.Write(data[offset:end]); err != nil {
			return fmt.Errorf("fragment write failed at offset %d: %w", offset, err)
		}

		offset = end
		fragmentCount++

		// Small delay between fragments (helps bypass some DPI)
		if offset < len(data) && f.delay > 0 {
			time.Sleep(f.delay)
		}
	}

	f.log.Debug("Sent %d bytes in %d fragments", len(data), fragmentCount)
	return nil
}

// FragmentTLSClientHello specifically fragments a TLS ClientHello message
// Strategy: Split the ClientHello so that the SNI field spans multiple TCP segments
func (f *Fragmenter) FragmentTLSClientHello(conn net.Conn, clientHello []byte) error {
	if !f.enabled {
		_, err := conn.Write(clientHello)
		return err
	}

	// TLS Record Header is 5 bytes: ContentType(1) + Version(2) + Length(2)
	// We want to split AFTER the record header but BEFORE the SNI
	// Typical SNI offset is around 40-100 bytes into the ClientHello

	if len(clientHello) < 10 {
		_, err := conn.Write(clientHello)
		return err
	}

	// Strategy 1: Split at random point in the first 100 bytes
	// This ensures the SNI field is split across segments
	splitPoint, err := f.randomSplitPoint(len(clientHello))
	if err != nil {
		_, writeErr := conn.Write(clientHello)
		return writeErr
	}

	// Send first fragment
	if _, err := conn.Write(clientHello[:splitPoint]); err != nil {
		return fmt.Errorf("first fragment failed: %w", err)
	}

	// Delay
	time.Sleep(f.delay)

	// Send remaining data (possibly in multiple fragments)
	return f.FragmentedWrite(conn, clientHello[splitPoint:])
}

// randomSplitPoint generates a random split point for TLS ClientHello
// Targets the area where SNI typically resides (bytes 40-200)
func (f *Fragmenter) randomSplitPoint(dataLen int) (int, error) {
	// SNI is typically between byte 40 and 200 in a ClientHello
	minSplit := 5  // After TLS record header
	maxSplit := 100
	if maxSplit > dataLen/2 {
		maxSplit = dataLen / 2
	}
	if minSplit >= maxSplit {
		minSplit = 1
		maxSplit = dataLen / 2
	}

	rangeSize := maxSplit - minSplit
	if rangeSize <= 0 {
		return dataLen / 2, nil
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(rangeSize)))
	if err != nil {
		return dataLen / 2, nil
	}

	return minSplit + int(n.Int64()), nil
}

// randomSize generates a random fragment size
func (f *Fragmenter) randomSize() (int, error) {
	if f.maxSize == f.minSize {
		return f.minSize, nil
	}

	rangeSize := f.maxSize - f.minSize + 1
	n, err := rand.Int(rand.Reader, big.NewInt(int64(rangeSize)))
	if err != nil {
		return f.minSize, err
	}

	return f.minSize + int(n.Int64()), nil
}

// SetDelay sets the delay between fragments
func (f *Fragmenter) SetDelay(d time.Duration) {
	f.delay = d
}

// FragmentedConn wraps a net.Conn to automatically fragment writes
type FragmentedConn struct {
	net.Conn
	fragmenter  *Fragmenter
	firstWrite  bool // fragment only the first write (TLS ClientHello)
	didFirst    bool
}

// NewFragmentedConn creates a connection that fragments the first write
func NewFragmentedConn(conn net.Conn, fragmenter *Fragmenter) *FragmentedConn {
	return &FragmentedConn{
		Conn:       conn,
		fragmenter: fragmenter,
		firstWrite: true,
	}
}

// Write fragments the first write (ClientHello) and passes through subsequent writes
func (fc *FragmentedConn) Write(p []byte) (int, error) {
	if fc.firstWrite && !fc.didFirst && fc.fragmenter.enabled {
		fc.didFirst = true
		// Check if this looks like a TLS ClientHello
		if len(p) > 5 && p[0] == 0x16 { // TLS Handshake
			err := fc.fragmenter.FragmentTLSClientHello(fc.Conn, p)
			if err != nil {
				return 0, err
			}
			return len(p), nil
		}
	}

	return fc.Conn.Write(p)
}
