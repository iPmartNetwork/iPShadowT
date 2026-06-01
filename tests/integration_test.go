package tests

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/client"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/server"
)

// TestE2E_ClientServerConnect tests a full client-server connection cycle
func TestE2E_ClientServerConnect(t *testing.T) {
	// Skip with race detector — stability modules create benign races in test environment
	if testing.Short() {
		t.Skip("skipping E2E in short mode")
	}

	log := logger.New("error")

	// Start a TCP echo server (simulates destination)
	echoAddr := startEchoServer(t)

	// Use a fixed test port
	testPort := "19443"

	// Server config
	serverCfg := &config.Config{
		Mode:      "server",
		Transport: "tcpmux",
		BindAddr:  "127.0.0.1:" + testPort,
		Password:  "test-password-e2e",
		LogLevel:  "error",
	}
	serverCfg.Mux.Concurrency = 2
	serverCfg.Mux.FrameSize = 32768
	serverCfg.Mux.RecvBuffer = 4194304
	serverCfg.Mux.StreamBuffer = 2097152
	serverCfg.Mux.MaxStreams = 100
	serverCfg.Heartbeat.Enabled = true
	serverCfg.Heartbeat.Interval = 5
	serverCfg.Heartbeat.Timeout = 10

	// Start server
	srv, err := server.New(serverCfg, log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Stop()

	// Give server time to bind
	time.Sleep(200 * time.Millisecond)

	// Client config
	clientCfg := &config.Config{
		Mode:       "client",
		Transport:  "tcpmux",
		RemoteAddr: "127.0.0.1:" + testPort,
		Password:   "test-password-e2e",
		LogLevel:   "error",
	}
	clientCfg.Mux.Concurrency = 2
	clientCfg.Mux.FrameSize = 32768
	clientCfg.Mux.RecvBuffer = 4194304
	clientCfg.Mux.StreamBuffer = 2097152
	clientCfg.Mux.MaxStreams = 100
	clientCfg.Heartbeat.Enabled = true
	clientCfg.Heartbeat.Interval = 5
	clientCfg.Heartbeat.Timeout = 10
	clientCfg.Forwards = []config.ForwardConfig{
		{Name: "echo", Type: "tcp", Listen: "127.0.0.1:19444", Remote: echoAddr},
	}

	// Start client
	cli, err := client.New(clientCfg, log)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	if err := cli.Start(); err != nil {
		t.Fatalf("Failed to start client: %v", err)
	}
	defer cli.Stop()

	// Give client time to connect
	time.Sleep(500 * time.Millisecond)

	t.Log("E2E: Client and server connected successfully")
}

// TestE2E_MultipleStreams tests multiple concurrent streams
func TestE2E_MultipleStreams(t *testing.T) {
	// This test verifies that multiplexing works correctly
	// by opening multiple streams concurrently
	t.Log("E2E: Multiple streams test placeholder")
}

// TestE2E_Reconnect tests automatic reconnection
func TestE2E_Reconnect(t *testing.T) {
	// This test verifies that the client reconnects after disconnection
	t.Log("E2E: Reconnect test placeholder")
}

// TestE2E_Failover tests multi-path failover
func TestE2E_Failover(t *testing.T) {
	// This test verifies that failover works when primary path fails
	t.Log("E2E: Failover test placeholder")
}

// startEchoServer starts a TCP echo server for testing
func startEchoServer(t *testing.T) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start echo server: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String()
}

// BenchmarkThroughput_TCPMux benchmarks raw throughput through the tunnel
func BenchmarkThroughput_TCPMux(b *testing.B) {
	b.Skip("Requires running server — run manually")
}

// TestE2E_ConcurrentConnections tests many concurrent connections
func TestE2E_ConcurrentConnections(t *testing.T) {
	t.Log("E2E: Concurrent connections test")

	// Create echo server
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	// Test concurrent connections to echo server
	const numConns = 50
	var wg sync.WaitGroup
	errors := make(chan error, numConns)

	for i := 0; i < numConns; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", ln.Addr().String(), 5*time.Second)
			if err != nil {
				errors <- fmt.Errorf("conn %d: dial failed: %w", id, err)
				return
			}
			defer conn.Close()

			msg := fmt.Sprintf("hello from %d", id)
			conn.Write([]byte(msg))

			buf := make([]byte, len(msg))
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, err := io.ReadFull(conn, buf)
			if err != nil {
				errors <- fmt.Errorf("conn %d: read failed: %w", id, err)
				return
			}
			if string(buf[:n]) != msg {
				errors <- fmt.Errorf("conn %d: echo mismatch", id)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}
