// Example: Simple client using the core engine
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/iPmart/iPShadowT/core"
)

func main() {
	// Create engine with programmatic configuration
	engine, err := core.New(
		core.WithMode(core.ModeClient),
		core.WithTransport(core.TransportTCPMux),
		core.WithRemoteAddr("your-server.com:443"),
		core.WithPassword("your-secret-password"),
		core.WithSOCKS5("127.0.0.1:1080"),
		core.WithNodelay(true),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Register event handlers
	engine.OnEvent(core.EventStarted, func(data interface{}) {
		fmt.Println("✅ Tunnel connected!")
		fmt.Println("   SOCKS5 proxy: 127.0.0.1:1080")
	})

	engine.OnEvent(core.EventError, func(data interface{}) {
		fmt.Printf("❌ Error: %v\n", data)
	})

	engine.OnEvent(core.EventDisconnected, func(data interface{}) {
		fmt.Println("⚠️  Disconnected, reconnecting...")
	})

	// Start
	if err := engine.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start: %v\n", err)
		os.Exit(1)
	}

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down...")
	engine.Stop()
}
