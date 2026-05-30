package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Collector collects and exposes metrics
// Compatible with Prometheus format for Grafana dashboards
type Collector struct {
	log       *logger.Logger
	startTime time.Time

	// Connection metrics
	TotalConnections   atomic.Int64
	ActiveConnections  atomic.Int64
	FailedConnections  atomic.Int64

	// Traffic metrics
	BytesReceived atomic.Int64
	BytesSent     atomic.Int64

	// Stream metrics
	TotalStreams  atomic.Int64
	ActiveStreams atomic.Int64

	// Transport metrics
	TransportDials   atomic.Int64
	TransportErrors  atomic.Int64

	// Anti-DPI metrics
	ProbesDetected   atomic.Int64
	ProbesBlocked    atomic.Int64

	// Path metrics
	PathFailovers    atomic.Int64
}

// NewCollector creates a new metrics collector
func NewCollector(log *logger.Logger) *Collector {
	return &Collector{
		log:       log,
		startTime: time.Now(),
	}
}

// ServePrometheus starts a Prometheus-compatible metrics endpoint
func (c *Collector) ServePrometheus(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", c.handlePrometheus)
	mux.HandleFunc("/metrics/json", c.handleJSON)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.log.Error("Metrics server error: %v", err)
		}
	}()

	c.log.Info("Metrics endpoint on %s/metrics", addr)
	return nil
}

// handlePrometheus returns metrics in Prometheus text format
func (c *Collector) handlePrometheus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	uptime := time.Since(c.startTime).Seconds()

	fmt.Fprintf(w, "# HELP ipshadowt_uptime_seconds Time since start\n")
	fmt.Fprintf(w, "# TYPE ipshadowt_uptime_seconds gauge\n")
	fmt.Fprintf(w, "ipshadowt_uptime_seconds %.2f\n\n", uptime)

	fmt.Fprintf(w, "# HELP ipshadowt_connections_total Total connections\n")
	fmt.Fprintf(w, "# TYPE ipshadowt_connections_total counter\n")
	fmt.Fprintf(w, "ipshadowt_connections_total %d\n\n", c.TotalConnections.Load())

	fmt.Fprintf(w, "# HELP ipshadowt_connections_active Active connections\n")
	fmt.Fprintf(w, "# TYPE ipshadowt_connections_active gauge\n")
	fmt.Fprintf(w, "ipshadowt_connections_active %d\n\n", c.ActiveConnections.Load())

	fmt.Fprintf(w, "# HELP ipshadowt_connections_failed Failed connections\n")
	fmt.Fprintf(w, "# TYPE ipshadowt_connections_failed counter\n")
	fmt.Fprintf(w, "ipshadowt_connections_failed %d\n\n", c.FailedConnections.Load())

	fmt.Fprintf(w, "# HELP ipshadowt_bytes_received_total Bytes received\n")
	fmt.Fprintf(w, "# TYPE ipshadowt_bytes_received_total counter\n")
	fmt.Fprintf(w, "ipshadowt_bytes_received_total %d\n\n", c.BytesReceived.Load())

	fmt.Fprintf(w, "# HELP ipshadowt_bytes_sent_total Bytes sent\n")
	fmt.Fprintf(w, "# TYPE ipshadowt_bytes_sent_total counter\n")
	fmt.Fprintf(w, "ipshadowt_bytes_sent_total %d\n\n", c.BytesSent.Load())

	fmt.Fprintf(w, "# HELP ipshadowt_streams_total Total streams\n")
	fmt.Fprintf(w, "# TYPE ipshadowt_streams_total counter\n")
	fmt.Fprintf(w, "ipshadowt_streams_total %d\n\n", c.TotalStreams.Load())

	fmt.Fprintf(w, "# HELP ipshadowt_streams_active Active streams\n")
	fmt.Fprintf(w, "# TYPE ipshadowt_streams_active gauge\n")
	fmt.Fprintf(w, "ipshadowt_streams_active %d\n\n", c.ActiveStreams.Load())

	fmt.Fprintf(w, "# HELP ipshadowt_probes_detected Probes detected\n")
	fmt.Fprintf(w, "# TYPE ipshadowt_probes_detected counter\n")
	fmt.Fprintf(w, "ipshadowt_probes_detected %d\n\n", c.ProbesDetected.Load())

	fmt.Fprintf(w, "# HELP ipshadowt_path_failovers Path failovers\n")
	fmt.Fprintf(w, "# TYPE ipshadowt_path_failovers counter\n")
	fmt.Fprintf(w, "ipshadowt_path_failovers %d\n\n", c.PathFailovers.Load())
}

// handleJSON returns metrics in JSON format
func (c *Collector) handleJSON(w http.ResponseWriter, r *http.Request) {
	metrics := map[string]interface{}{
		"uptime_seconds":     time.Since(c.startTime).Seconds(),
		"total_connections":  c.TotalConnections.Load(),
		"active_connections": c.ActiveConnections.Load(),
		"failed_connections": c.FailedConnections.Load(),
		"bytes_received":     c.BytesReceived.Load(),
		"bytes_sent":         c.BytesSent.Load(),
		"total_streams":      c.TotalStreams.Load(),
		"active_streams":     c.ActiveStreams.Load(),
		"probes_detected":    c.ProbesDetected.Load(),
		"path_failovers":     c.PathFailovers.Load(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// Snapshot returns a point-in-time snapshot of all metrics
type Snapshot struct {
	Uptime            float64 `json:"uptime_seconds"`
	TotalConnections  int64   `json:"total_connections"`
	ActiveConnections int64   `json:"active_connections"`
	BytesReceived     int64   `json:"bytes_received"`
	BytesSent         int64   `json:"bytes_sent"`
	ActiveStreams     int64   `json:"active_streams"`
}

// GetSnapshot returns current metrics snapshot
func (c *Collector) GetSnapshot() Snapshot {
	return Snapshot{
		Uptime:            time.Since(c.startTime).Seconds(),
		TotalConnections:  c.TotalConnections.Load(),
		ActiveConnections: c.ActiveConnections.Load(),
		BytesReceived:     c.BytesReceived.Load(),
		BytesSent:         c.BytesSent.Load(),
		ActiveStreams:     c.ActiveStreams.Load(),
	}
}
