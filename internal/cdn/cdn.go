package cdn

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

// Provider represents a CDN provider
type Provider string

const (
	ProviderCloudflare Provider = "cloudflare"
	ProviderFastly     Provider = "fastly"
	ProviderGcore      Provider = "gcore"
	ProviderArvan      Provider = "arvan"
	ProviderCustom     Provider = "custom"
)

// CDNConfig holds CDN-specific configuration
type CDNConfig struct {
	Provider    Provider
	Domain      string // CDN domain (e.g., "your-worker.workers.dev")
	Host        string // Host header to send
	Path        string // WebSocket path (e.g., "/ws")
	TLS         bool   // Use TLS
	EarlyData   bool   // Enable 0-RTT early data
	Headers     map[string]string // Custom headers
}

// Connector handles CDN-based connections
type Connector struct {
	config CDNConfig
	log    *logger.Logger
}

// NewConnector creates a new CDN connector
func NewConnector(cfg CDNConfig, log *logger.Logger) *Connector {
	if cfg.Path == "" {
		cfg.Path = "/ws"
	}
	if cfg.Headers == nil {
		cfg.Headers = make(map[string]string)
	}

	return &Connector{
		config: cfg,
		log:    log,
	}
}

// Connect establishes a WebSocket connection through the CDN
func (c *Connector) Connect() (net.Conn, error) {
	scheme := "ws"
	if c.config.TLS {
		scheme = "wss"
	}

	url := fmt.Sprintf("%s://%s%s", scheme, c.config.Domain, c.config.Path)

	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
		ReadBufferSize:   65536,
		WriteBufferSize:  65536,
	}

	if c.config.TLS {
		dialer.TLSClientConfig = &tls.Config{
			ServerName: c.config.Domain,
		}
	}

	// Set headers
	headers := http.Header{}
	headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	if c.config.Host != "" {
		headers.Set("Host", c.config.Host)
	}

	// CDN-specific headers
	switch c.config.Provider {
	case ProviderCloudflare:
		// Cloudflare WebSocket requires proper Origin
		headers.Set("Origin", fmt.Sprintf("https://%s", c.config.Domain))
		if c.config.EarlyData {
			headers.Set("Sec-WebSocket-Protocol", "binary")
		}
	case ProviderFastly:
		headers.Set("Fastly-Debug", "0")
	}

	// Custom headers
	for k, v := range c.config.Headers {
		headers.Set(k, v)
	}

	wsConn, resp, err := dialer.Dial(url, headers)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("CDN WebSocket dial failed (status %d): %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("CDN WebSocket dial failed: %w", err)
	}

	c.log.Debug("CDN connected via %s (%s)", c.config.Provider, c.config.Domain)
	return newWSNetConn(wsConn), nil
}

// GetProviderInfo returns information about CDN setup requirements
func GetProviderInfo(provider Provider) string {
	switch provider {
	case ProviderCloudflare:
		return `Cloudflare Setup:
1. Add your domain to Cloudflare
2. Create a DNS A record pointing to your server IP
3. Set SSL/TLS to "Full" or "Full (strict)"
4. Enable WebSocket in Network settings
5. Use the domain in client config

Alternative (Workers):
1. Create a Cloudflare Worker
2. Deploy WebSocket proxy worker
3. Use worker URL as CDN domain`

	case ProviderFastly:
		return `Fastly Setup:
1. Create a Fastly service
2. Add your server as origin
3. Enable WebSocket passthrough
4. Use Fastly domain in client config`

	case ProviderGcore:
		return `Gcore Setup:
1. Create CDN resource
2. Set origin to your server
3. Enable WebSocket support
4. Use CDN domain in client config`

	default:
		return "Custom CDN: Configure WebSocket proxy to your server"
	}
}

// wsNetConn wraps websocket.Conn as net.Conn (reused from wsmux)
type wsNetConn struct {
	ws     *websocket.Conn
	reader wsNetReader
}

type wsNetReader struct {
	buf []byte
	pos int
}

func newWSNetConn(ws *websocket.Conn) *wsNetConn {
	return &wsNetConn{ws: ws}
}

func (c *wsNetConn) Read(p []byte) (int, error) {
	if c.reader.pos < len(c.reader.buf) {
		n := copy(p, c.reader.buf[c.reader.pos:])
		c.reader.pos += n
		return n, nil
	}

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

func (c *wsNetConn) Write(p []byte) (int, error) {
	err := c.ws.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *wsNetConn) Close() error                       { return c.ws.Close() }
func (c *wsNetConn) LocalAddr() net.Addr                { return c.ws.LocalAddr() }
func (c *wsNetConn) RemoteAddr() net.Addr               { return c.ws.RemoteAddr() }
func (c *wsNetConn) SetDeadline(t time.Time) error      { c.ws.SetReadDeadline(t); return c.ws.SetWriteDeadline(t) }
func (c *wsNetConn) SetReadDeadline(t time.Time) error  { return c.ws.SetReadDeadline(t) }
func (c *wsNetConn) SetWriteDeadline(t time.Time) error { return c.ws.SetWriteDeadline(t) }

// CloudflareWorkerScript returns a sample Cloudflare Worker script
// that proxies WebSocket connections to the origin server
func CloudflareWorkerScript() string {
	return `// Cloudflare Worker - iPShadowT WebSocket Proxy
// Deploy this as a Cloudflare Worker to route traffic through Cloudflare CDN

export default {
  async fetch(request, env) {
    // Only handle WebSocket upgrades
    const upgradeHeader = request.headers.get("Upgrade");
    if (upgradeHeader !== "websocket") {
      return new Response("Expected WebSocket", { status: 426 });
    }

    // Connect to origin server
    const originURL = "wss://YOUR_SERVER_IP:443/tunnel";
    
    const originResponse = await fetch(originURL, {
      headers: request.headers,
    });

    return originResponse;
  }
};`
}

// NewCDNConfigFromMain creates CDN config from main config
func NewCDNConfigFromMain(cfg *config.Config) *CDNConfig {
	return &CDNConfig{
		Provider: ProviderCloudflare,
		Domain:   cfg.RemoteAddr,
		Path:     "/tunnel",
		TLS:      true,
	}
}
