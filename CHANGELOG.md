# Changelog

All notable changes to iPShadowT will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
