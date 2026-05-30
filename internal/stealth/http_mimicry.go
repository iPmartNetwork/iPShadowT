package stealth

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// HTTPMimicry disguises tunnel traffic as normal HTTP/HTTPS web browsing
// Instead of a persistent connection, it uses short-lived HTTP requests
// that look exactly like a browser loading a webpage
//
// Techniques:
// 1. Data hidden in HTTP headers (Cookie, Authorization, X-Request-ID)
// 2. Data hidden in POST body (looks like form submission or JSON API)
// 3. Data hidden in URL path (looks like asset requests)
// 4. Timing mimics real browsing (burst of requests, then pause)
// 5. Response looks like HTML/CSS/JS/images
//
// This defeats DPI that looks for:
// - Long-lived connections (tunnel = persistent, browsing = short)
// - Constant data flow (tunnel = steady, browsing = bursty)
// - Binary data patterns (tunnel = encrypted, browsing = text/images)
type HTTPMimicry struct {
	serverAddr string
	log        *logger.Logger
	sessionID  string
	seqNum     int
	client     *http.Client
}

// NewHTTPMimicry creates a new HTTP mimicry transport
func NewHTTPMimicry(serverAddr string, log *logger.Logger) *HTTPMimicry {
	return &HTTPMimicry{
		serverAddr: serverAddr,
		log:        log,
		sessionID:  generateSessionID(),
		client: &http.Client{
			Timeout: 30 * time.Second,
			// Don't follow redirects
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// SendAsGET hides data in a GET request (looks like loading a page)
func (hm *HTTPMimicry) SendAsGET(data []byte) ([]byte, error) {
	// Encode data in URL-safe base64, disguised as a path
	encoded := base64.RawURLEncoding.EncodeToString(data)

	// Make it look like a real asset request
	paths := []string{
		"/assets/js/%s.js",
		"/static/css/%s.css",
		"/images/%s.png",
		"/api/v2/data/%s",
		"/cdn-cgi/scripts/%s",
	}
	path := fmt.Sprintf(paths[rand.Intn(len(paths))], encoded[:minInt(32, len(encoded))])

	url := fmt.Sprintf("https://%s%s", hm.serverAddr, path)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Hide remaining data in headers
	req.Header.Set("Cookie", fmt.Sprintf("_session=%s; _data=%s", hm.sessionID, encoded))
	req.Header.Set("User-Agent", getRandomUA())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("X-Request-ID", fmt.Sprintf("%s-%d", hm.sessionID, hm.seqNum))

	hm.seqNum++

	resp, err := hm.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Extract response data from response headers/body
	return hm.extractResponse(resp)
}

// SendAsPOST hides data in a POST request (looks like form submission)
func (hm *HTTPMimicry) SendAsPOST(data []byte) ([]byte, error) {
	encoded := base64.StdEncoding.EncodeToString(data)

	// Disguise as JSON API call
	jsonBody := fmt.Sprintf(`{"event":"pageview","timestamp":%d,"data":"%s","session":"%s"}`,
		time.Now().Unix(), encoded, hm.sessionID)

	url := fmt.Sprintf("https://%s/api/analytics/collect", hm.serverAddr)
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", getRandomUA())
	req.Header.Set("Origin", fmt.Sprintf("https://%s", hm.serverAddr))
	req.Header.Set("X-Request-ID", fmt.Sprintf("%s-%d", hm.sessionID, hm.seqNum))

	hm.seqNum++

	resp, err := hm.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return hm.extractResponse(resp)
}

// extractResponse extracts tunnel data from HTTP response
func (hm *HTTPMimicry) extractResponse(resp *http.Response) ([]byte, error) {
	// Data can be in:
	// 1. X-Data header (base64)
	// 2. Set-Cookie header
	// 3. Response body (disguised as HTML comment or JSON)

	dataHeader := resp.Header.Get("X-Data")
	if dataHeader != "" {
		return base64.StdEncoding.DecodeString(dataHeader)
	}

	// Check cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "_resp" {
			return base64.RawURLEncoding.DecodeString(cookie.Value)
		}
	}

	return nil, fmt.Errorf("no data in response")
}

// getRandomUA returns a random realistic User-Agent string
func getRandomUA() string {
	uas := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1",
	}
	return uas[rand.Intn(len(uas))]
}

func generateSessionID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 16)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BrowsingSimulator simulates realistic browsing patterns
// to make tunnel traffic indistinguishable from real browsing
type BrowsingSimulator struct {
	log *logger.Logger
}

// NewBrowsingSimulator creates a new browsing simulator
func NewBrowsingSimulator(log *logger.Logger) *BrowsingSimulator {
	return &BrowsingSimulator{log: log}
}

// SimulatePageLoad generates traffic that looks like loading a webpage
// A typical page load: 1 HTML + 5-15 assets (CSS, JS, images) in quick succession
func (bs *BrowsingSimulator) SimulatePageLoad(conn net.Conn, realData []byte) error {
	// Split real data into chunks that look like web assets
	chunkSizes := []int{1460, 2920, 4380, 8760, 14600} // Common TCP segment sizes

	offset := 0
	for offset < len(realData) {
		size := chunkSizes[rand.Intn(len(chunkSizes))]
		if size > len(realData)-offset {
			size = len(realData) - offset
		}

		// Send chunk
		if _, err := conn.Write(realData[offset : offset+size]); err != nil {
			return err
		}
		offset += size

		// Inter-request delay (mimics browser loading assets)
		delay := time.Duration(rand.Intn(50)) * time.Millisecond
		time.Sleep(delay)
	}

	// After "page load", pause like a human reading
	readingPause := time.Duration(2000+rand.Intn(5000)) * time.Millisecond
	time.Sleep(readingPause)

	return nil
}
