package antidpi

import (
	"crypto/rand"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// TrafficShaper modifies traffic patterns to look like normal web browsing
// DPI systems use statistical analysis (packet sizes, timing, ratios) to detect tunnels
// This shaper makes tunnel traffic look like regular HTTPS browsing
type TrafficShaper struct {
	enabled    bool
	log        *logger.Logger
	profile    ShapingProfile
}

// ShapingProfile defines how traffic should be shaped
type ShapingProfile struct {
	// Packet size distribution (mimic HTTP response sizes)
	MinPacketSize int
	MaxPacketSize int

	// Timing (mimic human browsing patterns)
	MinDelay time.Duration
	MaxDelay time.Duration

	// Burst pattern (mimic page loads)
	BurstSize    int           // packets per burst
	BurstDelay   time.Duration // delay between bursts
	InterBurst   time.Duration // delay between burst groups

	// Padding to normalize packet sizes
	PadToMultiple int // pad packets to multiples of this size
}

// DefaultBrowsingProfile returns a profile that mimics web browsing
func DefaultBrowsingProfile() ShapingProfile {
	return ShapingProfile{
		MinPacketSize: 64,
		MaxPacketSize: 1460, // typical MSS
		MinDelay:      0,
		MaxDelay:      5 * time.Millisecond,
		BurstSize:     10,
		BurstDelay:    1 * time.Millisecond,
		InterBurst:    50 * time.Millisecond,
		PadToMultiple: 64,
	}
}

// StreamingProfile returns a profile that mimics video streaming
func StreamingProfile() ShapingProfile {
	return ShapingProfile{
		MinPacketSize: 1000,
		MaxPacketSize: 1460,
		MinDelay:      0,
		MaxDelay:      2 * time.Millisecond,
		BurstSize:     50,
		BurstDelay:    0,
		InterBurst:    20 * time.Millisecond,
		PadToMultiple: 188, // MPEG-TS packet size
	}
}

// NewTrafficShaper creates a new traffic shaper
func NewTrafficShaper(enabled bool, log *logger.Logger) *TrafficShaper {
	return &TrafficShaper{
		enabled: enabled,
		log:     log,
		profile: DefaultBrowsingProfile(),
	}
}

// SetProfile sets the shaping profile
func (ts *TrafficShaper) SetProfile(profile ShapingProfile) {
	ts.profile = profile
}

// WrapConn wraps a connection with traffic shaping
func (ts *TrafficShaper) WrapConn(conn net.Conn) net.Conn {
	if !ts.enabled {
		return conn
	}

	return &ShapedConn{
		Conn:    conn,
		shaper:  ts,
		packets: 0,
	}
}

// ShapedConn wraps a net.Conn with traffic shaping
type ShapedConn struct {
	net.Conn
	shaper     *TrafficShaper
	packets    int
	mu         sync.Mutex
}

// Write shapes outgoing traffic
func (sc *ShapedConn) Write(p []byte) (int, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	profile := sc.shaper.profile

	// Pad data to multiple of PadToMultiple
	data := sc.padData(p)

	// If data is larger than max packet size, split into chunks
	if len(data) > profile.MaxPacketSize {
		return sc.writeChunked(data)
	}

	// Add inter-packet delay
	if sc.packets > 0 && profile.MaxDelay > 0 {
		delay := sc.randomDelay(profile.MinDelay, profile.MaxDelay)
		time.Sleep(delay)
	}

	// Add burst pattern delay
	if profile.BurstSize > 0 && sc.packets > 0 && sc.packets%profile.BurstSize == 0 {
		time.Sleep(profile.InterBurst)
	}

	n, err := sc.Conn.Write(data)
	sc.packets++

	// Return original data length (not padded)
	if n >= len(p) {
		return len(p), err
	}
	return n, err
}

// writeChunked writes data in chunks that mimic normal packet sizes
func (sc *ShapedConn) writeChunked(data []byte) (int, error) {
	profile := sc.shaper.profile
	totalWritten := 0
	offset := 0

	for offset < len(data) {
		// Random chunk size
		chunkSize := sc.randomInt(profile.MinPacketSize, profile.MaxPacketSize)
		if chunkSize > len(data)-offset {
			chunkSize = len(data) - offset
		}

		// Write chunk
		n, err := sc.Conn.Write(data[offset : offset+chunkSize])
		totalWritten += n
		offset += chunkSize

		if err != nil {
			return totalWritten, err
		}

		sc.packets++

		// Inter-packet delay
		if offset < len(data) {
			delay := sc.randomDelay(profile.MinDelay, profile.MaxDelay)
			time.Sleep(delay)

			// Burst pattern
			if profile.BurstSize > 0 && sc.packets%profile.BurstSize == 0 {
				time.Sleep(profile.InterBurst)
			}
		}
	}

	return totalWritten, nil
}

// padData pads data to a multiple of PadToMultiple
func (sc *ShapedConn) padData(data []byte) []byte {
	multiple := sc.shaper.profile.PadToMultiple
	if multiple <= 0 {
		return data
	}

	remainder := len(data) % multiple
	if remainder == 0 {
		return data
	}

	padSize := multiple - remainder
	padded := make([]byte, len(data)+padSize)
	copy(padded, data)
	// Fill padding with random bytes
	rand.Read(padded[len(data):])

	return padded
}

// randomDelay generates a random delay between min and max
func (sc *ShapedConn) randomDelay(min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}

	rangeNs := int64(max - min)
	n, err := rand.Int(rand.Reader, big.NewInt(rangeNs))
	if err != nil {
		return min
	}

	return min + time.Duration(n.Int64())
}

// randomInt generates a random int between min and max
func (sc *ShapedConn) randomInt(min, max int) int {
	if max <= min {
		return min
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min)))
	if err != nil {
		return min
	}

	return min + int(n.Int64())
}
