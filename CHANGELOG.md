# Changelog

All notable changes to iPShadowT will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [2.2.3] - 2026-06-01

### Fixed
- **Direct pool data race** — `Stop()` no longer closes pool while `refillPool` is sending (CI `-race` failure)

---

## [2.2.2] - 2026-06-01

### Fixed

#### Protocol & upload path
- **Direct mode alignment** — server supports `mux.enabled = false` (one TCP = one flow), matching client direct pool
- **REALITY protocol** — auth token in ClientHello, TLS record wrapping, uTLS 1.8 compatibility
- **Graceful shutdown** — SIGTERM/SIGINT now calls `Stop()` on client/server
- **obfuscatedConn** — closes underlying TCP connection

#### Performance
- **Relay buffers** — `sync.Pool` for 256KB copy buffers (less GC under load)
- **Quality-aware mux** — `GetStream` uses quality scores + round-robin tie-break
- **Heartbeat** — quality/heartbeat modules only start when `heartbeat.enabled = true`
- **TLS verify** — optional `tls_ca` / `tls_insecure_skip_verify` in config

### Added

- **CLI** — `-validate` and `-doctor` flags for config checks
- **Multi-path** — `[[paths]]` wired into client dial (failover across paths)
- **UDP forward** — `type = "udp"` enabled in forwarder
- **Metrics** — configurable `[metrics] listen` (default `127.0.0.1:9091`)
- **JSON logging** — `log_format = "json"` in config
- **E2E tests** — real byte relay through tunnel (direct mode)
- **Config validation** — forwards, REALITY keys, TLS paths

### Changed

- Server `upload_boost` default uses direct mux mode (matches client)
- Install script: `frame_size = 65535`, server configs without client-only `[pool]`

---

## [2.2.1] - 2026-06-01

### Fixed

#### Upload throughput (tcpmux / Iran client → foreign server)
- **TCP socket buffers** — `send_buffer` / `recv_buffer` from config are now applied via `SO_SNDBUF` / `SO_RCVBUF` on every connection (port-independent)
- **Server accept path** — incoming client connections are tuned on accept (including TLS unwrap)
- **Data relay** — mux and server relay use 256KB copy buffers instead of 32KB default `io.Copy`
- **kernel_tuning** — `kernel_tuning = true` in config now actually runs at client/server startup (was defined but never called)
- **upload_boost profile** — sysctl profile implemented (BBR, 16MB windows, `tcp_notsent_lowat`)
- **Config defaults** — client/server auto-apply upload-friendly TCP and smux buffer sizes when not explicitly set
- **Buffer tuner** — initializes from configured buffers instead of 64KB minimum

#### Install script (`ipshadowt-manager.sh`)
- Client setup defaults to **upload_boost** profile with `kernel_tuning = true`
- Anti-DPI fragment/padding disabled when upload_boost is selected
- Multi-tunnel and export configs include upload tuning
- Sysctl install adds `tcp_window_scaling` and `tcp_notsent_lowat`

#### Other
- CLI default version aligned to v2.2.1 (was stale `v1.0.0-alpha1`)
- Auto-updater points to `iPmartNetwork/iPShadowT` on GitHub

---

## [2.2.0] - 2026-06-01

### 🚀 New Features

#### Anti-DPI
- **SNI Spoofing** — Packet-level SNI manipulation (split/replace/double methods)
- **Domain Fronting** — Client-side domain fronting via CDN (no server changes needed)
- **FakeTCP Transport** — UDP payloads over fake TCP (bypass UDP blocking)
- **Pipeline Architecture** — Chain multiple transports (inspired by WaterWall)

#### Networking
- **WireGuard Forward Type** — Use WireGuard client through iPShadowT tunnel
- **WireGuard Key Generation** — Real Curve25519 keys (auto-generate matching pairs)
- **WireGuard Auto-Setup** — Script generates both server + client configs with matching keys
- **Upload Boost Profile** — Optimized config for maximum upload speed
- **KCP Tuning Config** — Fine-grained KCP parameters (mode, window, FEC, buffer)

#### Stability (Phase 1-4)
- **Heartbeat Monitor** — Application-level ping/pong with RTT measurement (5s interval)
- **Smart Reconnector** — Exponential backoff (100ms→30s) with auto transport switch
- **Quality Monitor** — Per-session scoring (latency/jitter/loss) with degrade callback
- **DPI Detector** — Passive failure pattern analysis with transport recommendation
- **Buffer Tuner** — Auto-adjust buffers based on BDP (Bandwidth-Delay Product)
- **Warmup Pool** — Pre-connected sessions for instant reconnect
- **Quality-Aware Load Balancing** — Pool routes streams to best-scoring session

#### Script
- **10 transports** in all menus (+ faketcp)
- **SNI Spoofing option** in client setup
- **Performance profile selection** (balanced/upload_boost/high_throughput/low_cpu)
- **WireGuard forward option** in client setup + multi-tunnel
- **WireGuard auto-setup** menu (generates keys + configs + installs interface)
- **Port range support** (e.g., 2000-2010)
- **Port conflict detection** with suggestions

#### Other
- **Port utilities** — validate, conflict check, auto-suggest free port
- Config validator accepts `faketcp` transport
- `AntiDPIConfig` extended with SNI spoof + domain fronting fields

### Fixed
- Heartbeat now actually registers sessions (was no-op before)
- Quality monitor receives RTT data from heartbeat pings
- Pool uses quality score in load balancing (not just stream count)
- Buffer tuner output applied to new sessions
- Reconnector switches transport on DPI recommendation
- Transport menus aligned across all script functions

---

## [2.0.1] - 2026-05-31

### Fixed
- Transport options aligned across multi-tunnel menus
- Added h2mux and grpc to multi-tunnel client/server
- Added TLS option for tcpmux in multi-tunnel
- Added REALITY config in multi-tunnel client/server

---

## [2.0.0] - 2026-05-31

### 🚀 Major Release — Full Integration

#### Added
- **QUIC Transport** — Full implementation with `quic-go` (0-RTT, connection migration, UDP mux)
- **Traffic Obfuscation** — 4 modes: HTTPS mimic, Video mimic, Random burst, Constant rate
- **DNS Leak Protection** — Automatic DoH resolver integrated in client
- **Auto-Failover** — Multi-path with priority/round-robin/latency strategies + auto-recovery
- **CDN Mode in Script** — Full CDN setup wizard (Cloudflare, Gcore, Arvan, Custom)
- **TLS for tcpmux** — Optional TLS encryption for tcpmux transport
- **Watchdog/Health** — Systemd WatchdogSec + HTTP health endpoint
- **Adaptive Connection Pool** — Auto-scaling with warmup and usage-based sizing
- **Split Tunneling** — Rule-based routing with Iran IP bypass (200+ CIDRs)
- **Cluster Mode** — Multi-server with geographic routing and state sync
- **Config Sync** — Encrypted push/pull between servers
- **Plugin System** — Transport/Auth/Filter plugins with hook system
- **Graceful Upgrade** — Zero-downtime binary upgrades
- **Real-time Dashboard** — WebSocket-based with traffic charts
- **Prometheus + Grafana** — Docker Compose monitoring stack with alert rules
- **Smart Transport Detection** — TCP/UDP/TLS/H2 probing with recommendations

#### Integrated (Server)
- Rate limiter (per-user bandwidth control)
- Security manager (IP blacklist, brute-force protection, audit log)
- Plugin filters on every connection
- Metrics tracking (connections, streams, bytes) with Prometheus endpoint
- Health service tracking

#### Integrated (Client)
- DNS-over-HTTPS resolver (prevents DNS poisoning)
- Traffic obfuscation wrapper on sessions
- Failover manager in engine

#### Changed
- Version bumped to v2.0.0
- Go version requirement: 1.25+
- Deploy script version: 2.0.0
- Added macOS and ARM builds to CI
- Docker image now includes health check
- Build workflow uses matrix strategy

---

## [1.0.0] - 2026-05-29

### 🎉 Initial Release

First public release of iPShadowT — Anti-DPI Multi-Transport Tunnel Engine.

### Added

#### Core Engine
- Standalone core engine (`core/`) with functional options pattern
- Event system for state change notifications
- SDK-ready API for embedding in other applications

#### Transports (9)
- `tcpmux` — Raw TCP with multiplexing
- `wsmux` — WebSocket (CDN-compatible)
- `h2mux` — HTTP/2 multiplexed streams
- `grpc` — gRPC bidirectional streaming
- `reality` — REALITY protocol (maximum DPI resistance)
- `shadowtls` — ShadowTLS v3 (real TLS handshake, no cert needed)
- `quic` — QUIC/UDP transport
- `kcp` — KCP over UDP/Raw Socket (works when others are blocked)
- `reverse` — Reverse tunnel for NAT traversal

#### Anti-DPI & Stealth (15 techniques)
- uTLS fingerprint mimicry (Chrome, Firefox, Safari, Edge, iOS, Android)
- TLS ClientHello fragmentation
- Encrypted Client Hello (ECH/GREASE)
- Traffic shaping with browsing simulation
- Active probe resistance with fallback website
- REALITY protocol with ECDH X25519 authentication
- ShadowTLS v3 with real handshake relay
- HalfDuplex mode (separate upload/download channels)
- Domain fronting (hide destination behind CDN)
- DNS tunnel (last resort — works when only UDP/53 is allowed)
- HTTP mimicry (disguise as normal web browsing)
- Protocol morphing (HTTP/1.1, HTTP/2, TLS 1.3, DNS-TCP framing)
- Decoy traffic generation
- Random padding with configurable size
- Replay protection with timestamp validation

#### Networking
- TCP/UDP port forwarding
- SOCKS5 proxy server
- HTTP CONNECT proxy
- Split tunneling with Iran IP list (200+ CIDRs)
- Multi-path with automatic failover (priority, round-robin, fastest)
- Bandwidth aggregation across multiple paths
- Load balancer with 5 strategies
- CDN connector (Cloudflare, Fastly, Arvan, custom)
- DNS over HTTPS resolver
- TUN device (Layer 3)
- TAP device (Layer 2)
- Connection pooling with session management

#### Security
- XChaCha20-Poly1305 AEAD encryption
- ECDH X25519 key exchange
- HMAC-SHA256 authentication
- Certificate pinning
- IP whitelist/blacklist
- Brute-force protection (auto-block after 5 failures)
- Full audit logging
- Rate limiting (per-user bandwidth control)

#### Management
- Web panel with real-time dashboard
- REST API with CORS and API key auth
- User management (traffic limits, expiry, enable/disable)
- Subscription links (V2RayNG/Clash compatible)
- Prometheus-compatible metrics endpoint
- Health check HTTP endpoint
- REALITY key generator (`--gen-reality-keys`)

#### Intelligence
- Automatic DPI detection (TCP, UDP, TLS, timing analysis)
- Auto transport selection based on network conditions
- Continuous monitoring with smart failover
- Built-in speed test

#### Operations
- Config hot-reload (no restart needed)
- Automatic backup/restore with rotation
- Auto-update from GitHub releases
- Interactive CLI setup wizard
- Kernel tuning (sysctl auto-configuration)
- iptables management
- Systemd service file
- Docker support
- One-line install/uninstall scripts

#### Documentation
- English README with full documentation
- Persian (Farsi) README
- Example configurations (simple, REALITY, CDN)
- SDK usage examples (simple client, REALITY client, embedded server)

---

## [Unreleased]

### Planned
- Android/iOS client (gomobile)
- Desktop GUI (Wails)
- Telegram bot for management
- WireGuard integration as inner protocol
- Full E2E test suite

---

<p align="center">
  <sub>iPShadowT © 2026 <a href="https://github.com/iPmartNetwork">iPmart Network</a> (Ali Hassanzadeh)</sub>
</p>
