package transport

import (
	"context"
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

// GRPCTransport implements Transport using gRPC-like HTTP/2 streaming
// This looks exactly like a normal gRPC service call to DPI
// Advantages:
// - gRPC is extremely common (Google, AWS, Azure all use it)
// - Bidirectional streaming is native to gRPC
// - Works through most CDNs (Cloudflare supports gRPC)
// - Very hard to distinguish from legitimate API traffic
type GRPCTransport struct {
	cfg      *config.Config
	log      *logger.Logger
	listener net.Listener
	server   *http.Server
}

// NewGRPC creates a new gRPC transport
func NewGRPC(cfg *config.Config, log *logger.Logger) *GRPCTransport {
	return &GRPCTransport{
		cfg: cfg,
		log: log,
	}
}

// Name returns the transport name
func (g *GRPCTransport) Name() string {
	return "grpc"
}

// Dial connects to the server using gRPC-like HTTP/2 streaming
func (g *GRPCTransport) Dial() (net.Conn, error) {
	// Create HTTP/2 TLS transport
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2"},
	}

	h2Transport := &http2.Transport{
		TLSClientConfig:    tlsConfig,
		DisableCompression: true,
	}

	// Create bidirectional pipe
	clientSide, tunnelSide := net.Pipe()

	go func() {
		defer tunnelSide.Close()

		// Create streaming request body
		pr, pw := io.Pipe()

		// Build gRPC-like request
		url := fmt.Sprintf("https://%s/grpc.tunnel.v1.TunnelService/Stream", g.cfg.RemoteAddr)
		req, err := http.NewRequest("POST", url, pr)
		if err != nil {
			g.log.Error("gRPC: request creation failed: %v", err)
			return
		}

		// Set gRPC headers (makes it look like real gRPC)
		req.Header.Set("Content-Type", "application/grpc")
		req.Header.Set("TE", "trailers")
		req.Header.Set("User-Agent", "grpc-go/1.62.0")
		req.Header.Set("Grpc-Accept-Encoding", "identity,deflate,gzip")

		client := &http.Client{
			Transport: h2Transport,
			Timeout:   0, // No timeout for streaming
		}

		resp, err := client.Do(req)
		if err != nil {
			g.log.Error("gRPC: request failed: %v", err)
			pw.Close()
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			g.log.Error("gRPC: server returned %d", resp.StatusCode)
			pw.Close()
			return
		}

		// Bidirectional relay
		var wg sync.WaitGroup
		wg.Add(2)

		// tunnelSide → gRPC request body (upload)
		go func() {
			defer wg.Done()
			defer pw.Close()
			buf := make([]byte, 32768)
			for {
				n, err := tunnelSide.Read(buf)
				if n > 0 {
					// Write gRPC frame: [compressed(1)] [length(4)] [data(n)]
					frame := makeGRPCFrame(buf[:n])
					if _, werr := pw.Write(frame); werr != nil {
						return
					}
				}
				if err != nil {
					return
				}
			}
		}()

		// gRPC response body → tunnelSide (download)
		go func() {
			defer wg.Done()
			for {
				data, err := readGRPCFrame(resp.Body)
				if err != nil {
					return
				}
				if _, werr := tunnelSide.Write(data); werr != nil {
					return
				}
			}
		}()

		wg.Wait()
	}()

	g.log.Debug("gRPC connected to %s", g.cfg.RemoteAddr)
	return clientSide, nil
}

// Listen starts accepting gRPC connections
func (g *GRPCTransport) Listen() (net.Listener, error) {
	addr := g.cfg.BindAddr

	grpcListener := &GRPCListener{
		connCh: make(chan net.Conn, 256),
		done:   make(chan struct{}),
		addr:   addr,
	}

	mux := http.NewServeMux()

	// gRPC tunnel endpoint
	mux.HandleFunc("/grpc.tunnel.v1.TunnelService/Stream", func(w http.ResponseWriter, r *http.Request) {
		// Verify it looks like gRPC
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/grpc" && contentType != "application/grpc+proto" {
			// Not gRPC - serve fake response
			http.Error(w, "Unsupported Media Type", 415)
			return
		}

		// Set gRPC response headers
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Grpc-Accept-Encoding", "identity")
		w.WriteHeader(200)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		flusher.Flush()

		// Create bidirectional pipe
		clientSide, tunnelSide := net.Pipe()

		// Pass tunnel side to listener
		select {
		case grpcListener.connCh <- tunnelSide:
		case <-grpcListener.done:
			clientSide.Close()
			tunnelSide.Close()
			return
		}

		// Bidirectional relay
		var wg sync.WaitGroup
		wg.Add(2)

		// Request body → clientSide (upload from client)
		go func() {
			defer wg.Done()
			for {
				data, err := readGRPCFrame(r.Body)
				if err != nil {
					clientSide.Close()
					return
				}
				if _, werr := clientSide.Write(data); werr != nil {
					return
				}
			}
		}()

		// clientSide → Response body (download to client)
		go func() {
			defer wg.Done()
			buf := make([]byte, 32768)
			for {
				n, err := clientSide.Read(buf)
				if n > 0 {
					frame := makeGRPCFrame(buf[:n])
					w.Write(frame)
					flusher.Flush()
				}
				if err != nil {
					return
				}
			}
		}()

		wg.Wait()

		// Send gRPC trailers
		w.Header().Set("Grpc-Status", "0")
		w.Header().Set("Grpc-Message", "")
	})

	// gRPC reflection (makes it look like a real gRPC service)
	mux.HandleFunc("/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Grpc-Status", "12") // UNIMPLEMENTED
		w.Header().Set("Grpc-Message", "Method not found")
		w.WriteHeader(200)
	})

	// Health check (standard gRPC health)
	mux.HandleFunc("/grpc.health.v1.Health/Check", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.WriteHeader(200)
		// Return SERVING status
		w.Write(makeGRPCFrame([]byte{0x08, 0x01})) // HealthCheckResponse{status: SERVING}
	})

	// Default handler (probe resistance)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
		w.Write([]byte(`<!DOCTYPE html><html><body><h1>Service Running</h1></body></html>`))
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Configure HTTP/2
	http2.ConfigureServer(server, &http2.Server{
		MaxConcurrentStreams: 1000,
	})

	if g.cfg.TLSCert != "" && g.cfg.TLSKey != "" {
		go func() {
			if err := server.ListenAndServeTLS(g.cfg.TLSCert, g.cfg.TLSKey); err != nil && err != http.ErrServerClosed {
				g.log.Error("gRPC server error: %v", err)
			}
		}()
	} else {
		go func() {
			// For development: use self-signed cert
			g.log.Warn("gRPC transport requires TLS certificate for production use")
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				g.log.Error("gRPC server error: %v", err)
			}
		}()
	}

	grpcListener.server = server
	g.listener = grpcListener
	g.log.Info("gRPC transport listening on %s", addr)
	return grpcListener, nil
}

// Close shuts down the transport
func (g *GRPCTransport) Close() error {
	if g.listener != nil {
		return g.listener.Close()
	}
	return nil
}

// GRPCListener implements net.Listener for gRPC
type GRPCListener struct {
	connCh chan net.Conn
	done   chan struct{}
	addr   string
	server *http.Server
}

func (l *GRPCListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connCh:
		return conn, nil
	case <-l.done:
		return nil, fmt.Errorf("listener closed")
	}
}

func (l *GRPCListener) Close() error {
	close(l.done)
	if l.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return l.server.Shutdown(ctx)
	}
	return nil
}

func (l *GRPCListener) Addr() net.Addr {
	addr, _ := net.ResolveTCPAddr("tcp", l.addr)
	return addr
}

// makeGRPCFrame wraps data in a gRPC frame
// Format: [compressed(1 byte)] [length(4 bytes big-endian)] [data]
func makeGRPCFrame(data []byte) []byte {
	frame := make([]byte, 5+len(data))
	frame[0] = 0 // not compressed
	frame[1] = byte(len(data) >> 24)
	frame[2] = byte(len(data) >> 16)
	frame[3] = byte(len(data) >> 8)
	frame[4] = byte(len(data))
	copy(frame[5:], data)
	return frame
}

// readGRPCFrame reads a single gRPC frame from a reader
func readGRPCFrame(r io.Reader) ([]byte, error) {
	// Read header (5 bytes)
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	// Parse length
	length := int(header[1])<<24 | int(header[2])<<16 | int(header[3])<<8 | int(header[4])
	if length > 4*1024*1024 { // 4MB max
		return nil, fmt.Errorf("gRPC frame too large: %d", length)
	}

	if length == 0 {
		return []byte{}, nil
	}

	// Read data
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	return data, nil
}
