package antidpi

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// HalfDuplex implements half-duplex communication
// Instead of sending upload and download on the same connection,
// it uses TWO separate connections:
// - Connection 1: Client → Server (upload only)
// - Connection 2: Server → Client (download only)
//
// Why this helps against DPI:
// 1. DPI looks for bidirectional patterns (request/response)
// 2. With half-duplex, each connection looks like a one-way stream
// 3. Harder to correlate the two connections as belonging to same session
// 4. Traffic pattern looks more like CDN content delivery or log streaming
//
// Inspired by L2-Tunnel-Installer's HalfDuplex approach

// HalfDuplexConfig holds half-duplex configuration
type HalfDuplexConfig struct {
	Enabled     bool
	UploadAddr  string // Address for upload connection
	DownloadAddr string // Address for download connection
}

// HalfDuplexClient manages two connections for half-duplex communication
type HalfDuplexClient struct {
	uploadConn   net.Conn
	downloadConn net.Conn
	log          *logger.Logger
	mu           sync.Mutex
	closed       bool
}

// NewHalfDuplexClient creates a half-duplex client from two connections
func NewHalfDuplexClient(uploadConn, downloadConn net.Conn, log *logger.Logger) *HalfDuplexClient {
	return &HalfDuplexClient{
		uploadConn:   uploadConn,
		downloadConn: downloadConn,
		log:          log,
	}
}

// AsNetConn returns a net.Conn interface that uses half-duplex internally
func (h *HalfDuplexClient) AsNetConn() net.Conn {
	return &halfDuplexConn{
		upload:   h.uploadConn,
		download: h.downloadConn,
	}
}

// Close closes both connections
func (h *HalfDuplexClient) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return nil
	}
	h.closed = true

	var err1, err2 error
	if h.uploadConn != nil {
		err1 = h.uploadConn.Close()
	}
	if h.downloadConn != nil {
		err2 = h.downloadConn.Close()
	}

	if err1 != nil {
		return err1
	}
	return err2
}

// halfDuplexConn implements net.Conn using separate upload/download connections
type halfDuplexConn struct {
	upload   net.Conn // Write goes here
	download net.Conn // Read comes from here
}

func (c *halfDuplexConn) Read(p []byte) (int, error) {
	return c.download.Read(p)
}

func (c *halfDuplexConn) Write(p []byte) (int, error) {
	return c.upload.Write(p)
}

func (c *halfDuplexConn) Close() error {
	c.upload.Close()
	return c.download.Close()
}

func (c *halfDuplexConn) LocalAddr() net.Addr {
	return c.upload.LocalAddr()
}

func (c *halfDuplexConn) RemoteAddr() net.Addr {
	return c.upload.RemoteAddr()
}

func (c *halfDuplexConn) SetDeadline(t time.Time) error {
	c.upload.SetDeadline(t)
	return c.download.SetDeadline(t)
}

func (c *halfDuplexConn) SetReadDeadline(t time.Time) error {
	return c.download.SetReadDeadline(t)
}

func (c *halfDuplexConn) SetWriteDeadline(t time.Time) error {
	return c.upload.SetWriteDeadline(t)
}

// HalfDuplexServer manages half-duplex connections on the server side
type HalfDuplexServer struct {
	log           *logger.Logger
	uploadConns   chan net.Conn
	downloadConns chan net.Conn
	paired        chan net.Conn // Paired connections ready for use
	done          chan struct{}
}

// NewHalfDuplexServer creates a new half-duplex server
func NewHalfDuplexServer(log *logger.Logger) *HalfDuplexServer {
	return &HalfDuplexServer{
		log:           log,
		uploadConns:   make(chan net.Conn, 64),
		downloadConns: make(chan net.Conn, 64),
		paired:        make(chan net.Conn, 32),
		done:          make(chan struct{}),
	}
}

// Start begins pairing upload and download connections
func (s *HalfDuplexServer) Start() {
	go s.pairLoop()
}

// RegisterUpload registers an upload connection
func (s *HalfDuplexServer) RegisterUpload(conn net.Conn) {
	select {
	case s.uploadConns <- conn:
	case <-s.done:
		conn.Close()
	}
}

// RegisterDownload registers a download connection
func (s *HalfDuplexServer) RegisterDownload(conn net.Conn) {
	select {
	case s.downloadConns <- conn:
	case <-s.done:
		conn.Close()
	}
}

// Accept returns the next paired half-duplex connection
func (s *HalfDuplexServer) Accept() (net.Conn, error) {
	select {
	case conn := <-s.paired:
		return conn, nil
	case <-s.done:
		return nil, io.ErrClosedPipe
	}
}

// pairLoop pairs upload and download connections
func (s *HalfDuplexServer) pairLoop() {
	for {
		select {
		case <-s.done:
			return
		case upload := <-s.uploadConns:
			// Wait for matching download connection
			select {
			case download := <-s.downloadConns:
				// Pair them
				paired := &halfDuplexConn{
					upload:   download, // Server writes to client's download
					download: upload,   // Server reads from client's upload
				}
				select {
				case s.paired <- paired:
					s.log.Debug("HalfDuplex: paired connections")
				case <-s.done:
					upload.Close()
					download.Close()
					return
				}
			case <-time.After(30 * time.Second):
				s.log.Debug("HalfDuplex: timeout waiting for download pair")
				upload.Close()
			case <-s.done:
				upload.Close()
				return
			}
		}
	}
}

// Close shuts down the half-duplex server
func (s *HalfDuplexServer) Close() {
	close(s.done)
}
