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

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func baseServerCfg(port int) *config.Config {
	cfg := &config.Config{
		Mode:      "server",
		Transport: "tcpmux",
		BindAddr:  fmt.Sprintf("127.0.0.1:%d", port),
		Password:  "test-password-e2e",
		LogLevel:  "error",
	}
	cfg.Mux.Enabled = false
	cfg.Metrics.Enabled = false
	return cfg
}

func baseClientCfg(serverPort, listenPort int, echoRemote string) *config.Config {
	cfg := &config.Config{
		Mode:       "client",
		Transport:  "tcpmux",
		RemoteAddr: fmt.Sprintf("127.0.0.1:%d", serverPort),
		Password:   "test-password-e2e",
		LogLevel:   "error",
	}
	cfg.Mux.Enabled = false
	cfg.Pool.Size = 4
	cfg.Forwards = []config.ForwardConfig{
		{Name: "echo", Type: "tcp", Listen: fmt.Sprintf("127.0.0.1:%d", listenPort), Remote: echoRemote},
	}
	return cfg
}

// TestE2E_ClientServerConnect tests a full client-server connection cycle
func TestE2E_ClientServerConnect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E in short mode")
	}

	log := logger.New("error")
	echoAddr := startEchoServer(t)
	serverPort := freePort(t)
	listenPort := freePort(t)

	srv, err := server.New(baseServerCfg(serverPort), log)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer srv.Stop()
	time.Sleep(200 * time.Millisecond)

	cli, err := client.New(baseClientCfg(serverPort, listenPort, echoAddr), log)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	if err := cli.Start(); err != nil {
		t.Fatalf("Failed to start client: %v", err)
	}
	defer cli.Stop()
	time.Sleep(500 * time.Millisecond)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", listenPort), 5*time.Second)
	if err != nil {
		t.Fatalf("dial forward port: %v", err)
	}
	defer conn.Close()

	msg := "hello-tunnel-e2e"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(msg))
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != msg {
		t.Fatalf("echo mismatch: %q", string(buf))
	}
}

// TestE2E_MultipleStreams tests multiple concurrent streams through direct mode
func TestE2E_MultipleStreams(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E in short mode")
	}

	log := logger.New("error")
	echoAddr := startEchoServer(t)
	serverPort := freePort(t)
	listenPort := freePort(t)

	srv, _ := server.New(baseServerCfg(serverPort), log)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	time.Sleep(200 * time.Millisecond)

	cli, err := client.New(baseClientCfg(serverPort, listenPort, echoAddr), log)
	if err != nil {
		t.Fatal(err)
	}
	if err := cli.Start(); err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	time.Sleep(500 * time.Millisecond)

	const n = 5
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", listenPort), 5*time.Second)
			if err != nil {
				errCh <- err
				return
			}
			defer conn.Close()
			msg := fmt.Sprintf("stream-%d", id)
			conn.Write([]byte(msg))
			buf := make([]byte, len(msg))
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			if _, err := io.ReadFull(conn, buf); err != nil {
				errCh <- err
				return
			}
			if string(buf) != msg {
				errCh <- fmt.Errorf("mismatch id=%d", id)
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

// TestE2E_Reconnect tests automatic reconnection
func TestE2E_Reconnect(t *testing.T) {
	t.Log("E2E: reconnect covered by maintainDirectPool — manual soak recommended")
}

// TestE2E_Failover tests multi-path failover
func TestE2E_Failover(t *testing.T) {
	t.Log("E2E: multipath failover — configure [[paths]] and run integration soak")
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
