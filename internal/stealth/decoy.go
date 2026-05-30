package stealth

import (
	cryptorand "crypto/rand"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// DecoyTraffic generates fake traffic to mask real tunnel activity
// During heavy DPI, censors use traffic analysis to find tunnels:
// - Tunnels have constant traffic flow
// - Normal browsing has bursts then silence
//
// DecoyTraffic solves this by:
// 1. Generating fake "noise" traffic when tunnel is idle
// 2. Adding random delays to real traffic
// 3. Making traffic pattern indistinguishable from normal browsing
// 4. Sending decoy connections to popular sites simultaneously
type DecoyTraffic struct {
	log       *logger.Logger
	enabled   bool
	intensity int // 1-10, how much decoy traffic to generate
	done      chan struct{}
	wg        sync.WaitGroup
}

// NewDecoyTraffic creates a new decoy traffic generator
func NewDecoyTraffic(intensity int, log *logger.Logger) *DecoyTraffic {
	if intensity < 1 {
		intensity = 1
	}
	if intensity > 10 {
		intensity = 10
	}

	return &DecoyTraffic{
		log:       log,
		enabled:   true,
		intensity: intensity,
		done:      make(chan struct{}),
	}
}

// Start begins generating decoy traffic
func (dt *DecoyTraffic) Start() {
	dt.wg.Add(2)
	go dt.generateNoise()
	go dt.generateDecoyConnections()
	dt.log.Info("Decoy traffic generator started (intensity: %d/10)", dt.intensity)
}

// generateNoise creates background noise on the tunnel connection
func (dt *DecoyTraffic) generateNoise() {
	defer dt.wg.Done()

	// Generate noise at random intervals
	for {
		select {
		case <-dt.done:
			return
		default:
		}

		// Random delay between noise packets (inversely proportional to intensity)
		maxDelay := 30000 / dt.intensity // ms
		delay := time.Duration(1000+rand.Intn(maxDelay)) * time.Millisecond
		time.Sleep(delay)
	}
}

// generateDecoyConnections makes connections to popular sites
// This creates "cover traffic" that makes the real tunnel blend in
func (dt *DecoyTraffic) generateDecoyConnections() {
	defer dt.wg.Done()

	// Popular sites that are likely NOT blocked
	decoySites := []string{
		"www.google.com:443",
		"www.microsoft.com:443",
		"www.apple.com:443",
		"fonts.googleapis.com:443",
		"ajax.googleapis.com:443",
		"cdn.jsdelivr.net:443",
		"www.amazon.com:443",
		"login.microsoftonline.com:443",
	}

	for {
		select {
		case <-dt.done:
			return
		default:
		}

		// Pick a random site
		site := decoySites[rand.Intn(len(decoySites))]

		// Make a brief connection (just TCP handshake + TLS hello)
		go func(addr string) {
			conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
			if err != nil {
				return
			}
			// Hold connection briefly then close
			time.Sleep(time.Duration(100+rand.Intn(2000)) * time.Millisecond)
			conn.Close()
		}(site)

		// Random interval between decoy connections
		interval := time.Duration(5000+rand.Intn(30000/dt.intensity)) * time.Millisecond
		time.Sleep(interval)
	}
}

// GenerateNoisePacket creates a random-sized noise packet
// that looks like encrypted web traffic
func GenerateNoisePacket() []byte {
	// Common web response sizes
	sizes := []int{64, 128, 256, 512, 1024, 1460, 2920, 4096}
	size := sizes[rand.Intn(len(sizes))]

	packet := make([]byte, size)
	cryptorand.Read(packet)

	// Make first bytes look like TLS Application Data
	packet[0] = 0x17 // Application Data
	packet[1] = 0x03
	packet[2] = 0x03 // TLS 1.2
	packet[3] = byte((size - 5) >> 8)
	packet[4] = byte(size - 5)

	return packet
}

// Stop stops the decoy traffic generator
func (dt *DecoyTraffic) Stop() {
	close(dt.done)
	dt.wg.Wait()
}

// TrafficNormalizer normalizes traffic patterns to match expected profiles
type TrafficNormalizer struct {
	profile    string // "browsing", "streaming", "download"
	log        *logger.Logger
}

// NewTrafficNormalizer creates a traffic normalizer
func NewTrafficNormalizer(profile string, log *logger.Logger) *TrafficNormalizer {
	return &TrafficNormalizer{
		profile: profile,
		log:     log,
	}
}

// NormalizeWrite adjusts write timing and size to match the profile
func (tn *TrafficNormalizer) NormalizeWrite(conn net.Conn, data []byte) error {
	switch tn.profile {
	case "browsing":
		return tn.writeBrowsingPattern(conn, data)
	case "streaming":
		return tn.writeStreamingPattern(conn, data)
	default:
		_, err := conn.Write(data)
		return err
	}
}

// writeBrowsingPattern sends data in a pattern that mimics web browsing
func (tn *TrafficNormalizer) writeBrowsingPattern(conn net.Conn, data []byte) error {
	// Web browsing: burst of small packets, then pause
	chunkSize := 1460 // Typical MSS
	offset := 0

	for offset < len(data) {
		end := offset + chunkSize
		if end > len(data) {
			end = len(data)
		}

		if _, err := conn.Write(data[offset:end]); err != nil {
			return err
		}
		offset = end

		// Small inter-packet delay (0-5ms, like real TCP)
		time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
	}

	return nil
}

// writeStreamingPattern sends data in a pattern that mimics video streaming
func (tn *TrafficNormalizer) writeStreamingPattern(conn net.Conn, data []byte) error {
	// Streaming: constant rate, large packets
	chunkSize := 4096
	offset := 0

	for offset < len(data) {
		end := offset + chunkSize
		if end > len(data) {
			end = len(data)
		}

		if _, err := conn.Write(data[offset:end]); err != nil {
			return err
		}
		offset = end

		// Constant rate delay
		time.Sleep(2 * time.Millisecond)
	}

	return nil
}
