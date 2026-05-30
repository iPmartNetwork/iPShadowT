package transport

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// WSMux implements Transport using WebSocket (CDN-compatible)
type WSMux struct {
	cfg      *config.Config
	log      *logger.Logger
	listener net.Listener
	upgrader websocket.Upgrader
}

// NewWSMux creates a new WebSocket Mux transport
func NewWSMux(cfg *config.Config, log *logger.Logger) *WSMux {
	return &WSMux{
		cfg: cfg,
		log: log,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  32768,
			WriteBufferSize: 32768,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}
}

// Name returns the transport name
func (w *WSMux) Name() string {
	return "wsmux"
}

// Dial connects to the remote WebSocket server
func (w *WSMux) Dial() (net.Conn, error) {
	scheme := "ws"
	if w.cfg.TLSCert != "" || w.cfg.TLSKey != "" {
		scheme = "wss"
	}

	url := fmt.Sprintf("%s://%s/tunnel", scheme, w.cfg.RemoteAddr)

	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
		ReadBufferSize:   32768,
		WriteBufferSize:  32768,
	}

	// Use TLS if configured
	if scheme == "wss" {
		dialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, // TODO: proper validation
		}
	}

	// Add headers to look like a normal browser
	headers := http.Header{}
	headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	headers.Set("Origin", fmt.Sprintf("https://%s", w.cfg.RemoteAddr))

	wsConn, _, err := dialer.Dial(url, headers)
	if err != nil {
		return nil, fmt.Errorf("websocket dial failed: %w", err)
	}

	w.log.Debug("WebSocket connected to %s", url)
	return NewWSConn(wsConn), nil
}

// Listen starts accepting WebSocket connections
func (w *WSMux) Listen() (net.Listener, error) {
	addr := w.cfg.BindAddr

	wsListener := &WSListener{
		connCh: make(chan net.Conn, 256),
		done:   make(chan struct{}),
		addr:   addr,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/tunnel", func(rw http.ResponseWriter, r *http.Request) {
		conn, err := w.upgrader.Upgrade(rw, r, nil)
		if err != nil {
			w.log.Error("WebSocket upgrade failed: %v", err)
			return
		}
		w.log.Debug("WebSocket client connected from %s", r.RemoteAddr)
		wsListener.connCh <- NewWSConn(conn)
	})

	// Also serve a fake page for probe resistance
	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "text/html")
		rw.WriteHeader(200)
		rw.Write([]byte("<!DOCTYPE html><html><head><title>Welcome</title></head><body><h1>It works!</h1></body></html>"))
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	if w.cfg.TLSCert != "" && w.cfg.TLSKey != "" {
		go func() {
			if err := server.ListenAndServeTLS(w.cfg.TLSCert, w.cfg.TLSKey); err != nil && err != http.ErrServerClosed {
				w.log.Error("WebSocket TLS server error: %v", err)
			}
		}()
	} else {
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				w.log.Error("WebSocket server error: %v", err)
			}
		}()
	}

	w.log.Info("WebSocket transport listening on %s", addr)
	wsListener.server = server
	return wsListener, nil
}

// Close shuts down the transport
func (w *WSMux) Close() error {
	if w.listener != nil {
		return w.listener.Close()
	}
	return nil
}

// WSConn wraps a websocket.Conn to implement net.Conn
type WSConn struct {
	ws     *websocket.Conn
	reader *wsReader
}

type wsReader struct {
	buf []byte
	pos int
}

// NewWSConn wraps a WebSocket connection as net.Conn
func NewWSConn(ws *websocket.Conn) *WSConn {
	return &WSConn{
		ws:     ws,
		reader: &wsReader{},
	}
}

func (c *WSConn) Read(p []byte) (int, error) {
	// If we have buffered data, return it first
	if c.reader.pos < len(c.reader.buf) {
		n := copy(p, c.reader.buf[c.reader.pos:])
		c.reader.pos += n
		return n, nil
	}

	// Read next message
	_, msg, err := c.ws.ReadMessage()
	if err != nil {
		return 0, err
	}

	n := copy(p, msg)
	if n < len(msg) {
		c.reader.buf = msg
		c.reader.pos = n
	} else {
		c.reader.buf = nil
		c.reader.pos = 0
	}
	return n, nil
}

func (c *WSConn) Write(p []byte) (int, error) {
	err := c.ws.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *WSConn) Close() error {
	return c.ws.Close()
}

func (c *WSConn) LocalAddr() net.Addr {
	return c.ws.LocalAddr()
}

func (c *WSConn) RemoteAddr() net.Addr {
	return c.ws.RemoteAddr()
}

func (c *WSConn) SetDeadline(t time.Time) error {
	if err := c.ws.SetReadDeadline(t); err != nil {
		return err
	}
	return c.ws.SetWriteDeadline(t)
}

func (c *WSConn) SetReadDeadline(t time.Time) error {
	return c.ws.SetReadDeadline(t)
}

func (c *WSConn) SetWriteDeadline(t time.Time) error {
	return c.ws.SetWriteDeadline(t)
}

// WSListener implements net.Listener for WebSocket
type WSListener struct {
	connCh chan net.Conn
	done   chan struct{}
	addr   string
	server *http.Server
}

func (l *WSListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connCh:
		return conn, nil
	case <-l.done:
		return nil, fmt.Errorf("listener closed")
	}
}

func (l *WSListener) Close() error {
	close(l.done)
	if l.server != nil {
		return l.server.Close()
	}
	return nil
}

func (l *WSListener) Addr() net.Addr {
	addr, _ := net.ResolveTCPAddr("tcp", l.addr)
	return addr
}
