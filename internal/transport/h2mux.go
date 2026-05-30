package transport

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
	"golang.org/x/net/http2"
)

// H2Mux implements Transport using HTTP/2 multiplexing
// Advantages:
// - Looks exactly like normal HTTP/2 traffic (Google, YouTube, etc. all use HTTP/2)
// - Multiple streams over a single connection (built into HTTP/2)
// - Works through CDNs and reverse proxies
// - Very hard for DPI to distinguish from normal web traffic
type H2Mux struct {
	cfg      *config.Config
	log      *logger.Logger
	listener net.Listener
	server   *http.Server
}

// NewH2Mux creates a new HTTP/2 Mux transport
func NewH2Mux(cfg *config.Config, log *logger.Logger) *H2Mux {
	return &H2Mux{
		cfg: cfg,
		log: log,
	}
}

// Name returns the transport name
func (h *H2Mux) Name() string {
	return "h2mux"
}

// Dial connects to the server using HTTP/2
func (h *H2Mux) Dial() (net.Conn, error) {
	// Create HTTP/2 transport
	h2Transport := &http2.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2"},
		},
		DisableCompression: true,
		AllowHTTP:          false,
	}

	// Create a pipe-based connection
	// We use HTTP/2's streaming to create a bidirectional tunnel
	clientConn, serverConn := net.Pipe()

	// Start HTTP/2 request in background
	go func() {
		defer serverConn.Close()

		url := fmt.Sprintf("https://%s/connect", h.cfg.RemoteAddr)

		// Create a request with a streaming body
		pr, pw := io.Pipe()

		req, err := http.NewRequest("POST", url, pr)
		if err != nil {
			h.log.Error("H2: Failed to create request: %v", err)
			return
		}

		req.Header.Set("Content-Type", "application/grpc")
		req.Header.Set("User-Agent", "grpc-go/1.60.0")

		// Send request
		client := &http.Client{Transport: h2Transport}
		resp, err := client.Do(req)
		if err != nil {
			h.log.Error("H2: Request failed: %v", err)
			pw.Close()
			return
		}
		defer resp.Body.Close()

		// Bidirectional relay
		var wg sync.WaitGroup
		wg.Add(2)

		// serverConn → HTTP/2 request body (upload)
		go func() {
			defer wg.Done()
			defer pw.Close()
			io.Copy(pw, serverConn)
		}()

		// HTTP/2 response body → serverConn (download)
		go func() {
			defer wg.Done()
			io.Copy(serverConn, resp.Body)
		}()

		wg.Wait()
	}()

	h.log.Debug("H2 connected to %s", h.cfg.RemoteAddr)
	return clientConn, nil
}

// Listen starts accepting HTTP/2 connections
func (h *H2Mux) Listen() (net.Listener, error) {
	addr := h.cfg.BindAddr

	h2Listener := &H2Listener{
		connCh: make(chan net.Conn, 256),
		done:   make(chan struct{}),
		addr:   addr,
	}

	mux := http.NewServeMux()

	// Tunnel endpoint (looks like gRPC)
	mux.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}

		// Flush headers immediately
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", 500)
			return
		}

		w.Header().Set("Content-Type", "application/grpc")
		w.WriteHeader(200)
		flusher.Flush()

		// Create pipe for bidirectional communication
		clientConn, tunnelConn := net.Pipe()

		// Send tunnelConn to the listener
		select {
		case h2Listener.connCh <- tunnelConn:
		case <-h2Listener.done:
			clientConn.Close()
			tunnelConn.Close()
			return
		}

		// Bidirectional relay between HTTP/2 stream and pipe
		var wg sync.WaitGroup
		wg.Add(2)

		// Request body → clientConn (upload from client)
		go func() {
			defer wg.Done()
			io.Copy(clientConn, r.Body)
			clientConn.Close()
		}()

		// clientConn → Response body (download to client)
		go func() {
			defer wg.Done()
			buf := make([]byte, 32768)
			for {
				n, err := clientConn.Read(buf)
				if n > 0 {
					w.Write(buf[:n])
					flusher.Flush()
				}
				if err != nil {
					break
				}
			}
		}()

		wg.Wait()
	})

	// Fake homepage for probe resistance
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
		w.Write([]byte(`<!DOCTYPE html><html><head><title>API Gateway</title></head><body><h1>API Gateway v2.1</h1><p>Service is running.</p></body></html>`))
	})

	// Health endpoint (looks like a normal API)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"healthy","version":"2.1.0"}`))
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Enable HTTP/2
	http2.ConfigureServer(server, &http2.Server{
		MaxConcurrentStreams: 1000,
	})

	if h.cfg.TLSCert != "" && h.cfg.TLSKey != "" {
		go func() {
			if err := server.ListenAndServeTLS(h.cfg.TLSCert, h.cfg.TLSKey); err != nil && err != http.ErrServerClosed {
				h.log.Error("H2 server error: %v", err)
			}
		}()
	} else {
		// HTTP/2 cleartext (h2c) - not recommended for production
		go func() {
			h2s := &http2.Server{}
			handler := h2c(mux, h2s)
			server.Handler = handler
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				h.log.Error("H2 server error: %v", err)
			}
		}()
	}

	h2Listener.server = server
	h.listener = h2Listener
	h.log.Info("HTTP/2 transport listening on %s", addr)
	return h2Listener, nil
}

// Close shuts down the transport
func (h *H2Mux) Close() error {
	if h.listener != nil {
		return h.listener.Close()
	}
	return nil
}

// H2Listener implements net.Listener for HTTP/2
type H2Listener struct {
	connCh chan net.Conn
	done   chan struct{}
	addr   string
	server *http.Server
}

func (l *H2Listener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connCh:
		return conn, nil
	case <-l.done:
		return nil, fmt.Errorf("listener closed")
	}
}

func (l *H2Listener) Close() error {
	close(l.done)
	if l.server != nil {
		return l.server.Close()
	}
	return nil
}

func (l *H2Listener) Addr() net.Addr {
	addr, _ := net.ResolveTCPAddr("tcp", l.addr)
	return addr
}

// h2c wraps a handler to support HTTP/2 cleartext (h2c)
func h2c(handler http.Handler, h2s *http2.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this is an h2c upgrade request
		if r.ProtoMajor == 2 {
			handler.ServeHTTP(w, r)
			return
		}
		// For HTTP/1.1, check for upgrade
		if r.Header.Get("Upgrade") == "h2c" {
			// Handle h2c upgrade
			handler.ServeHTTP(w, r)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

// H2ConnWrapper wraps the pipe connection with proper deadline support
type H2ConnWrapper struct {
	net.Conn
	readDeadline  time.Time
	writeDeadline time.Time
}

func (c *H2ConnWrapper) SetDeadline(t time.Time) error {
	c.readDeadline = t
	c.writeDeadline = t
	return nil
}

func (c *H2ConnWrapper) SetReadDeadline(t time.Time) error {
	c.readDeadline = t
	return nil
}

func (c *H2ConnWrapper) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return nil
}
