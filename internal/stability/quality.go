package stability

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// QualityMonitor tracks connection quality metrics in real-time
// and triggers actions when quality degrades
type QualityMonitor struct {
	log        *logger.Logger
	cfg        QualityConfig
	sessions   sync.Map // sessionID → *QualityMetrics
	done       chan struct{}
	wg         sync.WaitGroup
	onDegrade  func(sessionID int, reason string)
}

// QualityConfig configures the quality monitor
type QualityConfig struct {
	CheckInterval    time.Duration // How often to evaluate (default: 5s)
	MaxLatency       time.Duration // Max acceptable latency (default: 500ms)
	MaxJitter        time.Duration // Max acceptable jitter (default: 200ms)
	MinThroughput    int64         // Min acceptable throughput bytes/sec (default: 10KB/s)
	MaxPacketLoss    float64       // Max acceptable loss ratio 0.0-1.0 (default: 0.1)
	DegradeCallback  func(sessionID int, reason string)
}

// DefaultQualityConfig returns sensible defaults
func DefaultQualityConfig() QualityConfig {
	return QualityConfig{
		CheckInterval: 5 * time.Second,
		MaxLatency:    500 * time.Millisecond,
		MaxJitter:     200 * time.Millisecond,
		MinThroughput: 10240, // 10 KB/s
		MaxPacketLoss: 0.1,   // 10%
	}
}

// QualityMetrics holds real-time quality metrics for a session
type QualityMetrics struct {
	// Latency
	LastRTT    atomic.Int64 // nanoseconds
	AvgRTT     atomic.Int64 // nanoseconds (EMA)
	MinRTT     atomic.Int64
	MaxRTT     atomic.Int64
	Jitter     atomic.Int64 // nanoseconds

	// Throughput
	BytesSent  atomic.Int64
	BytesRecv  atomic.Int64
	lastCheck  int64 // unix nano of last throughput calculation
	lastSent   int64
	lastRecv   int64
	SendRate   atomic.Int64 // bytes/sec
	RecvRate   atomic.Int64 // bytes/sec

	// Loss
	PingsSent  atomic.Int64
	PongsRecv  atomic.Int64

	// State
	Score      atomic.Int64 // 0-100 quality score
	Degraded   atomic.Bool
}

// NewQualityMonitor creates a new quality monitor
func NewQualityMonitor(cfg QualityConfig, log *logger.Logger) *QualityMonitor {
	return &QualityMonitor{
		log:       log,
		cfg:       cfg,
		done:      make(chan struct{}),
		onDegrade: cfg.DegradeCallback,
	}
}

// Register registers a session for quality monitoring
func (qm *QualityMonitor) Register(sessionID int) *QualityMetrics {
	m := &QualityMetrics{}
	m.Score.Store(100)
	m.MinRTT.Store(int64(time.Hour)) // Initialize to high value
	m.lastCheck = time.Now().UnixNano()
	qm.sessions.Store(sessionID, m)
	return m
}

// Unregister removes a session from monitoring
func (qm *QualityMonitor) Unregister(sessionID int) {
	qm.sessions.Delete(sessionID)
}

// RecordRTT records a new RTT measurement
func (qm *QualityMonitor) RecordRTT(sessionID int, rtt time.Duration) {
	val, ok := qm.sessions.Load(sessionID)
	if !ok {
		return
	}
	m := val.(*QualityMetrics)

	rttNano := int64(rtt)
	m.LastRTT.Store(rttNano)

	// Update min/max
	if rttNano < m.MinRTT.Load() {
		m.MinRTT.Store(rttNano)
	}
	if rttNano > m.MaxRTT.Load() {
		m.MaxRTT.Store(rttNano)
	}

	// EMA (alpha = 0.3)
	oldAvg := m.AvgRTT.Load()
	if oldAvg == 0 {
		m.AvgRTT.Store(rttNano)
	} else {
		newAvg := int64(float64(oldAvg)*0.7 + float64(rttNano)*0.3)
		m.AvgRTT.Store(newAvg)

		// Jitter = |current - average|
		jitter := rttNano - newAvg
		if jitter < 0 {
			jitter = -jitter
		}
		m.Jitter.Store(jitter)
	}
}

// RecordBytes records bytes sent/received
func (qm *QualityMonitor) RecordBytes(sessionID int, sent, recv int64) {
	val, ok := qm.sessions.Load(sessionID)
	if !ok {
		return
	}
	m := val.(*QualityMetrics)
	m.BytesSent.Add(sent)
	m.BytesRecv.Add(recv)
}

// RecordPing records a ping sent
func (qm *QualityMonitor) RecordPing(sessionID int) {
	val, ok := qm.sessions.Load(sessionID)
	if !ok {
		return
	}
	m := val.(*QualityMetrics)
	m.PingsSent.Add(1)
}

// RecordPong records a pong received
func (qm *QualityMonitor) RecordPong(sessionID int) {
	val, ok := qm.sessions.Load(sessionID)
	if !ok {
		return
	}
	m := val.(*QualityMetrics)
	m.PongsRecv.Add(1)
}

// Start begins quality monitoring
func (qm *QualityMonitor) Start() {
	qm.wg.Add(1)
	go qm.evaluateLoop()
	qm.log.Info("Quality monitor started (interval=%v)", qm.cfg.CheckInterval)
}

// evaluateLoop periodically evaluates quality
func (qm *QualityMonitor) evaluateLoop() {
	defer qm.wg.Done()

	ticker := time.NewTicker(qm.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-qm.done:
			return
		case <-ticker.C:
			qm.evaluateAll()
		}
	}
}

// evaluateAll evaluates quality for all sessions
func (qm *QualityMonitor) evaluateAll() {
	now := time.Now().UnixNano()

	qm.sessions.Range(func(key, value interface{}) bool {
		sessionID := key.(int)
		m := value.(*QualityMetrics)

		score := 100

		// Latency check
		avgRTT := time.Duration(m.AvgRTT.Load())
		if avgRTT > qm.cfg.MaxLatency {
			score -= 30
			if !m.Degraded.Load() {
				m.Degraded.Store(true)
				qm.log.Warn("Session %d: high latency (%v > %v)", sessionID, avgRTT, qm.cfg.MaxLatency)
				if qm.onDegrade != nil {
					go qm.onDegrade(sessionID, "high_latency")
				}
			}
		}

		// Jitter check
		jitter := time.Duration(m.Jitter.Load())
		if jitter > qm.cfg.MaxJitter {
			score -= 20
		}

		// Throughput check
		elapsed := float64(now-m.lastCheck) / float64(time.Second)
		if elapsed > 0 {
			currentSent := m.BytesSent.Load()
			currentRecv := m.BytesRecv.Load()
			sendRate := int64(float64(currentSent-m.lastSent) / elapsed)
			recvRate := int64(float64(currentRecv-m.lastRecv) / elapsed)
			m.SendRate.Store(sendRate)
			m.RecvRate.Store(recvRate)
			m.lastSent = currentSent
			m.lastRecv = currentRecv
			m.lastCheck = now
		}

		// Packet loss check
		pings := m.PingsSent.Load()
		pongs := m.PongsRecv.Load()
		if pings > 5 {
			lossRatio := 1.0 - float64(pongs)/float64(pings)
			if lossRatio > qm.cfg.MaxPacketLoss {
				score -= 40
				if !m.Degraded.Load() {
					m.Degraded.Store(true)
					qm.log.Warn("Session %d: high packet loss (%.1f%%)", sessionID, lossRatio*100)
					if qm.onDegrade != nil {
						go qm.onDegrade(sessionID, "packet_loss")
					}
				}
			}
		}

		// Clamp score
		if score < 0 {
			score = 0
		}
		m.Score.Store(int64(score))

		// Recovery: if score is back to good, clear degraded flag
		if score >= 70 && m.Degraded.Load() {
			m.Degraded.Store(false)
			qm.log.Info("Session %d: quality recovered (score: %d)", sessionID, score)
		}

		return true
	})
}

// GetScore returns the quality score for a session (0-100)
func (qm *QualityMonitor) GetScore(sessionID int) int {
	val, ok := qm.sessions.Load(sessionID)
	if !ok {
		return 0
	}
	return int(val.(*QualityMetrics).Score.Load())
}

// GetBestSession returns the session ID with the highest quality score
func (qm *QualityMonitor) GetBestSession() int {
	bestID := -1
	bestScore := int64(-1)

	qm.sessions.Range(func(key, value interface{}) bool {
		sessionID := key.(int)
		m := value.(*QualityMetrics)
		score := m.Score.Load()
		if score > bestScore {
			bestScore = score
			bestID = sessionID
		}
		return true
	})

	return bestID
}

// Stop stops the quality monitor
func (qm *QualityMonitor) Stop() {
	close(qm.done)
	qm.wg.Wait()
}
