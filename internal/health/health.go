package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Status represents the health status
type Status struct {
	Status      string    `json:"status"`
	Uptime      string    `json:"uptime"`
	Connections int64     `json:"connections"`
	Streams     int64     `json:"streams"`
	BytesIn     int64     `json:"bytes_in"`
	BytesOut    int64     `json:"bytes_out"`
	StartTime   time.Time `json:"start_time"`
}

// Service provides a health check HTTP endpoint
type Service struct {
	log         *logger.Logger
	addr        string
	startTime   time.Time
	connections atomic.Int64
	streams     atomic.Int64
	bytesIn     atomic.Int64
	bytesOut    atomic.Int64
	server      *http.Server
}

// NewService creates a new health check service
func NewService(addr string, log *logger.Logger) *Service {
	return &Service{
		log:       log,
		addr:      addr,
		startTime: time.Now(),
	}
}

// Start begins the health check HTTP server
func (s *Service) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/stats", s.handleStats)

	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error("Health service error: %v", err)
		}
	}()

	s.log.Info("Health service listening on %s", s.addr)
	return nil
}

// handleHealth returns basic health status
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"uptime": time.Since(s.startTime).String(),
	})
}

// handleStats returns detailed statistics
func (s *Service) handleStats(w http.ResponseWriter, r *http.Request) {
	status := Status{
		Status:      "ok",
		Uptime:      time.Since(s.startTime).String(),
		Connections: s.connections.Load(),
		Streams:     s.streams.Load(),
		BytesIn:     s.bytesIn.Load(),
		BytesOut:    s.bytesOut.Load(),
		StartTime:   s.startTime,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}

// AddConnection increments the connection counter
func (s *Service) AddConnection() {
	s.connections.Add(1)
}

// RemoveConnection decrements the connection counter
func (s *Service) RemoveConnection() {
	s.connections.Add(-1)
}

// AddStream increments the stream counter
func (s *Service) AddStream() {
	s.streams.Add(1)
}

// RemoveStream decrements the stream counter
func (s *Service) RemoveStream() {
	s.streams.Add(-1)
}

// AddBytesIn adds to the incoming bytes counter
func (s *Service) AddBytesIn(n int64) {
	s.bytesIn.Add(n)
}

// AddBytesOut adds to the outgoing bytes counter
func (s *Service) AddBytesOut(n int64) {
	s.bytesOut.Add(n)
}

// Stop shuts down the health service
func (s *Service) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// FormatBytes formats bytes into human-readable string
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
