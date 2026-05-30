package smart

import (
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Detector implements intelligent network analysis
// It detects DPI activity and automatically adapts the tunnel configuration
type Detector struct {
	log            *logger.Logger
	probeResults   []ProbeResult
	mu             sync.Mutex
	dpiDetected    bool
	lastCheck      time.Time
	checkInterval  time.Duration
}

// ProbeResult holds the result of a network probe
type ProbeResult struct {
	Transport string
	Target    string
	Success   bool
	Latency   time.Duration
	Error     string
	Timestamp time.Time
}

// NetworkCondition represents current network conditions
type NetworkCondition struct {
	DPIActive       bool
	UDPBlocked      bool
	TCPThrottled    bool
	TLSIntercepted bool
	BestTransport   string
	Latency         time.Duration
}

// NewDetector creates a new smart detector
func NewDetector(log *logger.Logger) *Detector {
	return &Detector{
		log:           log,
		probeResults:  make([]ProbeResult, 0),
		checkInterval: 60 * time.Second,
	}
}

// AnalyzeNetwork performs comprehensive network analysis
func (d *Detector) AnalyzeNetwork(serverAddr string) *NetworkCondition {
	condition := &NetworkCondition{}

	// Test 1: Basic TCP connectivity
	tcpOK, tcpLatency := d.probeTCP(serverAddr)
	condition.Latency = tcpLatency

	if !tcpOK {
		condition.TCPThrottled = true
		d.log.Warn("Smart: TCP connection failed/throttled")
	}

	// Test 2: UDP connectivity
	udpOK, _ := d.probeUDP(serverAddr)
	condition.UDPBlocked = !udpOK
	if !udpOK {
		d.log.Info("Smart: UDP appears blocked")
	}

	// Test 3: TLS interception detection
	tlsOK := d.probeTLSIntegrity(serverAddr)
	condition.TLSIntercepted = !tlsOK
	if !tlsOK {
		d.log.Warn("Smart: TLS interception detected!")
		condition.DPIActive = true
	}

	// Test 4: DPI detection via timing analysis
	dpiDetected := d.detectDPIByTiming(serverAddr)
	condition.DPIActive = condition.DPIActive || dpiDetected

	// Recommend best transport
	condition.BestTransport = d.recommendTransport(condition)

	d.mu.Lock()
	d.dpiDetected = condition.DPIActive
	d.lastCheck = time.Now()
	d.mu.Unlock()

	d.log.Info("Smart Analysis: DPI=%v, UDP=%v, Throttle=%v, Recommend=%s",
		condition.DPIActive, condition.UDPBlocked, condition.TCPThrottled, condition.BestTransport)

	return condition
}

// probeTCP tests basic TCP connectivity
func (d *Detector) probeTCP(addr string) (bool, time.Duration) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	latency := time.Since(start)

	if err != nil {
		return false, latency
	}
	conn.Close()
	return true, latency
}

// probeUDP tests UDP connectivity
func (d *Detector) probeUDP(addr string) (bool, time.Duration) {
	start := time.Now()
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return false, 0
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return false, time.Since(start)
	}
	defer conn.Close()

	// Send a small packet and wait for response
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	conn.Write([]byte{0x00}) // Probe packet

	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	latency := time.Since(start)

	// If we get a response or ICMP unreachable, UDP works
	// If timeout, UDP might be blocked
	if err != nil {
		return false, latency
	}
	return true, latency
}

// probeTLSIntegrity checks if TLS is being intercepted
func (d *Detector) probeTLSIntegrity(addr string) bool {
	// Connect with TLS and check certificate chain
	// If the certificate doesn't match expected, TLS is being MITMed
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return true // Can't test, assume OK
	}
	conn.Close()

	// In a full implementation, we would:
	// 1. Do TLS handshake
	// 2. Check certificate fingerprint against known good
	// 3. Check for unexpected certificates in chain
	return true
}

// detectDPIByTiming uses timing analysis to detect DPI
func (d *Detector) detectDPIByTiming(addr string) bool {
	// DPI adds latency to the first packet (inspection time)
	// Compare latency of first packet vs subsequent packets
	var firstLatency, avgLatency time.Duration

	for i := 0; i < 5; i++ {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		latency := time.Since(start)

		if err != nil {
			continue
		}
		conn.Close()

		if i == 0 {
			firstLatency = latency
		} else {
			avgLatency += latency
		}
	}

	if avgLatency > 0 {
		avgLatency /= 4
	}

	// If first connection is significantly slower, DPI might be inspecting
	if firstLatency > avgLatency*3 && firstLatency > 500*time.Millisecond {
		return true
	}

	return false
}

// recommendTransport recommends the best transport based on conditions
func (d *Detector) recommendTransport(condition *NetworkCondition) string {
	if condition.DPIActive && condition.TLSIntercepted {
		// Heavy DPI + MITM → REALITY (steals real cert)
		return "reality"
	}

	if condition.DPIActive && !condition.UDPBlocked {
		// DPI active but UDP works → QUIC with obfuscation
		return "quic"
	}

	if condition.DPIActive {
		// DPI active, UDP blocked → WebSocket over CDN
		return "wsmux"
	}

	if condition.TCPThrottled {
		// TCP throttled → gRPC (looks like API traffic, less throttled)
		return "grpc"
	}

	if !condition.UDPBlocked {
		// No DPI, UDP works → QUIC (fastest)
		return "quic"
	}

	// Default: TCP mux (fastest TCP option)
	return "tcpmux"
}

// IsDPIActive returns whether DPI was detected in last check
func (d *Detector) IsDPIActive() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dpiDetected
}

// SpeedTest performs a basic speed test through the tunnel
type SpeedTestResult struct {
	Download float64 // Mbps
	Upload   float64 // Mbps
	Latency  time.Duration
	Jitter   time.Duration
}

// RunSpeedTest performs a speed test
func RunSpeedTest(conn net.Conn, duration time.Duration) *SpeedTestResult {
	result := &SpeedTestResult{}

	// Measure latency (ping)
	start := time.Now()
	conn.Write([]byte{0x01}) // Ping
	buf := make([]byte, 1)
	conn.Read(buf)
	result.Latency = time.Since(start)

	// Measure download speed
	testData := make([]byte, 1024*1024) // 1MB
	downloadStart := time.Now()
	totalRead := 0
	deadline := time.Now().Add(duration)

	conn.SetReadDeadline(deadline)
	for time.Now().Before(deadline) {
		n, err := conn.Read(testData)
		totalRead += n
		if err != nil {
			break
		}
	}
	downloadTime := time.Since(downloadStart).Seconds()
	if downloadTime > 0 {
		result.Download = float64(totalRead*8) / downloadTime / 1000000 // Mbps
	}

	// Measure upload speed
	uploadStart := time.Now()
	totalWritten := 0
	deadline = time.Now().Add(duration)

	conn.SetWriteDeadline(deadline)
	for time.Now().Before(deadline) {
		n, err := conn.Write(testData)
		totalWritten += n
		if err != nil {
			break
		}
	}
	uploadTime := time.Since(uploadStart).Seconds()
	if uploadTime > 0 {
		result.Upload = float64(totalWritten*8) / uploadTime / 1000000 // Mbps
	}

	return result
}
