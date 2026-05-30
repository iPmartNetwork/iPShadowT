// Package core is the standalone iPShadowT tunnel engine.
//
// It provides a clean, simple API for embedding the tunnel in any Go application.
// The engine handles all complexity internally: transport selection, encryption,
// multiplexing, anti-DPI techniques, and connection management.
//
// # Quick Start (Config File)
//
//	engine, err := core.New(core.WithConfigFile("/etc/ipshadowt/config.toml"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	engine.Start()
//	defer engine.Stop()
//
// # Programmatic Client
//
//	engine, err := core.New(
//	    core.WithMode(core.ModeClient),
//	    core.WithTransport(core.TransportReality),
//	    core.WithRemoteAddr("my-server.com:443"),
//	    core.WithPassword("super-secret"),
//	    core.WithReality("www.google.com", "PUBLIC_KEY_HEX", "SHORT_ID"),
//	    core.WithSOCKS5("127.0.0.1:1080"),
//	    core.WithAntiDPI(true),
//	    core.WithUTLS("chrome"),
//	    core.WithFragment(true, "40-80"),
//	)
//
// # Programmatic Server
//
//	engine, err := core.New(
//	    core.WithMode(core.ModeServer),
//	    core.WithTransport(core.TransportReality),
//	    core.WithBindAddr("0.0.0.0:443"),
//	    core.WithPassword("super-secret"),
//	    core.WithRealityServer("www.google.com", "PRIVATE_KEY_HEX", "SHORT_ID", "www.google.com:443"),
//	    core.WithKernelTuning(true),
//	)
//
// # Events
//
//	engine.OnEvent(core.EventConnected, func(data interface{}) {
//	    fmt.Println("Connected!")
//	})
//	engine.OnEvent(core.EventError, func(data interface{}) {
//	    fmt.Printf("Error: %v\n", data)
//	})
//
// # Available Transports
//
//   - TransportTCPMux: Raw TCP with multiplexing (fastest, least DPI resistance)
//   - TransportWSMux: WebSocket with multiplexing (CDN-compatible)
//   - TransportH2Mux: HTTP/2 multiplexed (looks like normal web traffic)
//   - TransportGRPC: gRPC streaming (looks like API calls)
//   - TransportReality: REALITY protocol (maximum DPI resistance)
//   - TransportShadowTLS: ShadowTLS v3 (real TLS handshake, no cert needed)
//   - TransportQUIC: QUIC/UDP (fast, but may be blocked)
//   - TransportReverse: Reverse tunnel (for servers behind NAT)
//
// # Anti-DPI Features
//
//   - uTLS: Mimics browser TLS fingerprints (Chrome, Firefox, Safari)
//   - TLS Fragmentation: Splits ClientHello to hide SNI
//   - ECH: Encrypted Client Hello
//   - Traffic Shaping: Makes traffic look like normal browsing
//   - Probe Resistance: Server shows real website to scanners
//   - HalfDuplex: Separate upload/download channels
//   - Random Padding: Defeats traffic analysis
//
// # Integration Examples
//
// The core package can be used in:
//   - CLI applications (like the included cmd/ipshadowt)
//   - Desktop GUI apps (with Go + Wails/Fyne)
//   - Mobile apps (with gomobile)
//   - Web services (embedded in HTTP servers)
//   - Microservices (as a sidecar proxy)
//   - IoT devices (cross-compiled)
package core
