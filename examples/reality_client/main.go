// Example: REALITY client with full anti-DPI
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/iPmart/iPShadowT/core"
)

func main() {
	engine, err := core.New(
		core.WithMode(core.ModeClient),
		core.WithRemoteAddr("your-server.com:443"),
		core.WithPassword("your-secret-password"),

		// REALITY protocol (maximum stealth)
		core.WithReality(
			"www.google.com",       // SNI to mimic
			"SERVER_PUBLIC_KEY_HEX", // Server's public key
			"SHORT_ID_HEX",         // Your short ID
		),

		// Anti-DPI features
		core.WithAntiDPI(true),
		core.WithUTLS("chrome"),           // Mimic Chrome browser
		core.WithFragment(true, "40-80"),  // Fragment TLS ClientHello
		core.WithTrafficShape(false),      // Enable for maximum stealth (slower)

		// Local proxy
		core.WithSOCKS5("127.0.0.1:1080"),

		// Performance
		core.WithMux(4, 32768),
		core.WithNodelay(true),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	engine.OnEvent(core.EventStarted, func(data interface{}) {
		fmt.Println("🛡️  REALITY tunnel active!")
		fmt.Println("   Transport: REALITY + uTLS(Chrome) + Fragment")
		fmt.Println("   SOCKS5: 127.0.0.1:1080")
		fmt.Println("   DPI sees: Normal HTTPS to www.google.com")
	})

	if err := engine.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed: %v\n", err)
		os.Exit(1)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	engine.Stop()
}
