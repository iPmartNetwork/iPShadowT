<p align="center">
  <img src="img/iPst.svg" alt="iPShadowT Logo" width="300"/>
</p>


<p align="center">
  <strong>Anti-DPI Multi-Transport Tunnel Engine</strong>
</p>

<p align="center">
  <a href="https://github.com/iPmartNetwork/iPShadowT/blob/master/VERSION"><img src="https://img.shields.io/badge/version-v2.0.0-blue?style=flat-square" alt="Version"/></a>
  <a href="https://github.com/iPmartNetwork/iPShadowT/blob/master/LICENSE"><img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="License"/></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go"/></a>
  <a href="https://github.com/iPmartNetwork/iPShadowT/releases"><img src="https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows%20%7C%20freebsd-lightgrey?style=flat-square" alt="Platform"/></a>
</p>

<p align="center">
  <a href="https://github.com/iPmartNetwork/iPShadowT/stargazers"><img src="https://img.shields.io/github/stars/iPmartNetwork/iPShadowT?style=flat-square" alt="Stars"/></a>
  <a href="https://github.com/iPmartNetwork/iPShadowT/network/members"><img src="https://img.shields.io/github/forks/iPmartNetwork/iPShadowT?style=flat-square" alt="Forks"/></a>
  <a href="https://github.com/iPmartNetwork/iPShadowT/issues"><img src="https://img.shields.io/github/issues/iPmartNetwork/iPShadowT?style=flat-square" alt="Issues"/></a>
  <a href="https://github.com/iPmartNetwork/iPShadowT/commits/main"><img src="https://img.shields.io/github/last-commit/iPmartNetwork/iPShadowT?style=flat-square" alt="Last Commit"/></a>
</p>

<p align="center">
  <a href="#-quick-start">Quick Start</a> •
  <a href="#-features">Features</a> •
  <a href="#-transports">Transports</a> •
  <a href="#-anti-dpi">Anti-DPI</a> •
  <a href="#-configuration">Configuration</a> •
  <a href="CHANGELOG.md">Changelog</a> •
  <a href="README-FA.md">فارسی</a>
</p>

---

## 📋 Overview

iPShadowT is a high-performance, self-contained tunnel engine designed to bypass deep packet inspection (DPI) and internet censorship. It combines 9 transport protocols, 15 stealth techniques, and intelligent auto-selection into a single Go binary with zero external dependencies.

Built to survive even the most extreme filtering scenarios — including complete internet shutdowns where only DNS traffic is allowed.

### 🆕 What's New in v2.0.0

- ⚡ **Full QUIC transport** (quic-go, 0-RTT, connection migration)
- 🔀 **Auto-Failover** with multi-path (priority/round-robin/latency)
- 🎭 **Traffic Obfuscation** (4 modes: HTTPS mimic, Video, Burst, Constant)
- 🔒 **DNS Leak Protection** (automatic DoH)
- ☁️ **CDN Mode** (Cloudflare/Gcore/Arvan — IP hidden)
- 🧩 **Plugin System** (Transport/Auth/Filter plugins)
- 📈 **Real-time Dashboard** (WebSocket + live charts)
- 📡 **Prometheus + Grafana** monitoring stack
- 🌐 **Cluster Mode** (multi-server with geo-routing)
- 🗺️ **Split Tunneling** (Iran IP bypass, 200+ CIDRs)
- ♻️ **Graceful Upgrade** (zero-downtime binary updates)
- ⚖️ **Per-user Rate Limiting** + Security Manager
- 🔄 **Config Sync** (encrypted push/pull between servers)
- 📜 **ACME/Auto-Cert** (automatic TLS certificate management)

---

## ⚡ Quick Start

```bash
# Download
wget https://github.com/iPmartNetwork/iPShadowT/releases/latest/download/ipshadowt-linux-amd64
chmod +x ipshadowt-linux-amd64

# Generate keys
./ipshadowt-linux-amd64 --gen-reality-keys

# Run server (abroad)
./ipshadowt-linux-amd64 -c server.toml

# Run client (Iran)
./ipshadowt-linux-amd64 -c client.toml
```

Or use the one-line installer:

```bash
curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/iPShadowT/master/deploy/ipshadowt-manager.sh -o ipshadowt-manager.sh && sudo bash ipshadowt-manager.sh
```

---

## ✨ Features

### 🚀 Core

| Feature | Description |
|---------|-------------|
| 9 Transports | TCP, WebSocket, HTTP/2, gRPC, REALITY, ShadowTLS, QUIC, KCP, Reverse |
| 15 Stealth Techniques | uTLS, Fragment, ECH, Shaping, Domain Fronting, DNS Tunnel, and more |
| Traffic Obfuscation | 4 modes: HTTPS mimic, Video streaming, Random burst, Constant rate |
| Multiplexing | Thousands of streams over a single connection (smux) |
| Multi-Path + Failover | Automatic failover with priority/round-robin/latency strategies |
| Encryption | XChaCha20-Poly1305 AEAD |
| Plugin System | Extensible Transport, Auth, and Filter plugins |
| Zero Dependencies | Single static binary, no external tools needed |

### 🛡️ Anti-DPI

| Technique | Purpose |
|-----------|---------|
| REALITY | Server shows real website to probes |
| uTLS | Mimics Chrome/Firefox/Safari TLS fingerprint |
| TLS Fragmentation | Splits ClientHello to hide SNI |
| ECH | Encrypted Client Hello |
| Traffic Shaping | Makes traffic look like normal browsing |
| Domain Fronting | Hides real destination behind CDN |
| DNS Tunnel | Last resort — works when only DNS is allowed |
| Protocol Morphing | Disguises traffic as HTTP/2, TLS, DNS |
| Decoy Traffic | Generates noise to mask patterns |
| HalfDuplex | Separate upload/download channels |

### 📡 Networking

| Feature | Description |
|---------|-------------|
| Port Forwarding | TCP, UDP, SOCKS5, HTTP proxy |
| Split Tunneling | Iran IPs go direct, rest through tunnel (200+ CIDRs) |
| Load Balancer | 5 strategies (round-robin, least-conn, weighted, IP-hash, fastest) |
| CDN Support | Cloudflare, Gcore, Arvan, custom |
| DNS over HTTPS | Automatic DoH — prevents DNS poisoning & leaks |
| TUN/TAP | Full system traffic capture (Layer 2 & 3) |
| Adaptive Pool | Auto-scaling connection pool with warmup |
| Cluster Mode | Multi-server with geographic routing & state sync |
| Config Sync | Encrypted config push/pull between nodes |

### 🔧 Management

| Feature | Description |
|---------|-------------|
| Real-time Dashboard | WebSocket-based with live traffic charts |
| REST API | Full management API with CORS & API key auth |
| User Management | Multi-user with traffic limits, expiry, enable/disable |
| Subscription Links | V2RayNG/Clash compatible |
| Prometheus Metrics | Grafana-ready monitoring with alert rules |
| Rate Limiting | Per-user/IP bandwidth control |
| Security Manager | IP blacklist/whitelist, brute-force protection, audit log |
| ACME/Auto-Cert | Automatic TLS certificate management & renewal |
| Auto-Update | Self-update from GitHub releases |
| Backup/Restore | Automatic periodic backups (cron) |
| Hot-Reload | Change config without restart |
| Graceful Upgrade | Zero-downtime binary updates |

### 🧠 Intelligence

| Feature | Description |
|---------|-------------|
| DPI Detection | Automatically detects active DPI |
| Auto Protocol Selection | Picks best transport for current conditions |
| Smart Failover | Switches transport on degradation |
| Speed Test | Built-in throughput measurement |

---

## 🔌 Transports

| Transport | Port | DPI Resistance | CDN | Speed |
|-----------|------|---------------|-----|-------|
| `tcpmux` | Any | ⭐⭐ | ❌ | ⭐⭐⭐⭐⭐ |
| `wsmux` | 443 | ⭐⭐⭐ | ✅ | ⭐⭐⭐⭐ |
| `h2mux` | 443 | ⭐⭐⭐⭐ | ✅ | ⭐⭐⭐⭐ |
| `grpc` | 443 | ⭐⭐⭐⭐ | ✅ | ⭐⭐⭐⭐ |
| `reality` | 443 | ⭐⭐⭐⭐⭐ | ❌ | ⭐⭐⭐⭐ |
| `shadowtls` | 443 | ⭐⭐⭐⭐ | ❌ | ⭐⭐⭐⭐ |
| `quic` | 443 | ⭐⭐⭐ | ❌ | ⭐⭐⭐⭐⭐ |
| `kcp` | Any | ⭐⭐⭐⭐⭐ | ❌ | ⭐⭐⭐⭐⭐ |
| `reverse` | 443 | ⭐⭐⭐ | ❌ | ⭐⭐⭐⭐ |

---

## ⚙️ Configuration

### Server (server.toml)

```toml
mode = "server"
transport = "reality"
bind_addr = "0.0.0.0:443"
password = "your-secret"

[reality]
server_name = "www.google.com"
private_key = "SERVER_PRIVATE_KEY"
short_id = "SHORT_ID"
dest = "www.google.com:443"

[performance]
nodelay = true
kernel_tuning = true
buffer_profile = "high_throughput"
```

### Client (client.toml)

```toml
mode = "client"
transport = "reality"
remote_addr = "your-server.com:443"
password = "your-secret"

[reality]
server_name = "www.google.com"
public_key = "SERVER_PUBLIC_KEY"
short_id = "SHORT_ID"

[anti_dpi]
enabled = true
utls_fingerprint = "chrome"
fragment = true

[[forwards]]
name = "socks5"
type = "socks5"
listen = "127.0.0.1:1080"
```

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────┐
│                    iPShadowT v2.0                    │
├─────────────────────────────────────────────────────┤
│  Input: SOCKS5 / HTTP / TCP / UDP / TUN             │
│  ↓                                                  │
│  Split Tunnel (Iran direct, rest proxy)             │
│  ↓                                                  │
│  Multiplexer (smux - 1000s of streams)              │
│  ↓                                                  │
│  Encryption (XChaCha20-Poly1305 + Padding)          │
│  ↓                                                  │
│  Obfuscation (HTTPS mimic / Video / Burst)          │
│  ↓                                                  │
│  Anti-DPI (uTLS + Fragment + Shaping + ECH)         │
│  ↓                                                  │
│  Transport (REALITY / WS / H2 / gRPC / QUIC / ...) │
│  ↓                                                  │
│  Multi-Path + Auto-Failover (priority/latency)      │
│  ↓                                                  │
│  DNS-over-HTTPS (leak protection)                   │
└─────────────────────────────────────────────────────┘

Server Side:
┌─────────────────────────────────────────────────────┐
│  Security (IP check) → Rate Limit → Plugin Filters  │
│  → Mux Session → Stream → Destination              │
│  → Metrics + Health + Prometheus + Dashboard        │
└─────────────────────────────────────────────────────┘
```

---

## 📦 Build

```bash
git clone https://github.com/iPmartNetwork/iPShadowT.git
cd iPShadowT
go mod tidy
make build-linux        # Linux AMD64
make build-linux-arm    # Linux ARM64
make build-all          # All platforms
```

---

## 🐳 Docker

```bash
# Build locally
docker build -t ipshadowt .
docker run -v ./config.toml:/etc/ipshadowt/config.toml -p 443:443 -p 443:443/udp ipshadowt

# Or pull from GHCR
docker pull ghcr.io/ipmartnetwork/ipshadowt:2.0.0
docker run -v ./config.toml:/etc/ipshadowt/config.toml ghcr.io/ipmartnetwork/ipshadowt:2.0.0
```

### Monitoring Stack (Prometheus + Grafana)

```bash
cd deploy/
docker-compose -f docker-compose.monitoring.yml up -d
# Grafana: http://localhost:3000 (admin/admin)
# Prometheus: http://localhost:9090
```

---

## 🖥️ Manager Script

Full interactive management with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/iPShadowT/master/deploy/ipshadowt-manager.sh -o ipshadowt-manager.sh && sudo bash ipshadowt-manager.sh
```

Features:
- 🚀 One-click install (auto-download binary + prerequisites)
- ⚙️ Interactive tunnel setup wizard (Iran/Foreign)
- 🔀 Multi-Tunnel: one-to-many, many-to-one, one-to-one
- ☁️ CDN mode setup (Cloudflare, Gcore, Arvan)
- 🔒 TLS option for tcpmux transport
- 🧪 Smart transport detection (TCP/UDP/TLS/H2 probing)
- 🔑 REALITY key generation + auto-config
- 📊 Multi-tunnel status with per-tunnel details
- 🐕 Watchdog integration (systemd WatchdogSec)
- 🔥 Automatic firewall + BBR + kernel tuning
- 💾 Backup / Restore with auto-backup (cron)
- 📡 Port forward manager (add/remove from menu)
- 📤 Export client config (copy-paste ready)
- 🔄 One-click update from GitHub
- 🩺 Health check all tunnels

---

## 📁 Project Structure

```
iPShadowT/
├── core/              Standalone engine (SDK) + Failover
├── cmd/ipshadowt/     CLI application
├── internal/
│   ├── acme/          Auto TLS certificate management
│   ├── antidpi/       15 anti-DPI techniques + obfuscation
│   ├── api/           REST API server
│   ├── cdn/           CDN connector (Cloudflare, Gcore, Arvan)
│   ├── client/        Client with DoH + obfuscation
│   ├── cluster/       Multi-server cluster mode
│   ├── config/        TOML configuration
│   ├── configsync/    Encrypted config sync
│   ├── crypto/        XChaCha20-Poly1305 encryption
│   ├── dns/           DNS-over-HTTPS resolver
│   ├── health/        Health check / watchdog
│   ├── loadbalancer/  5-strategy load balancer
│   ├── metrics/       Prometheus-compatible metrics
│   ├── multipath/     Multi-path + bandwidth aggregation
│   ├── mux/           Stream multiplexing (smux)
│   ├── plugin/        Plugin system (Transport/Auth/Filter)
│   ├── pool/          Adaptive connection pool
│   ├── ratelimit/     Per-user rate limiting
│   ├── security/      IP whitelist, audit, brute-force
│   ├── server/        Server with security + metrics + plugins
│   ├── smart/         DPI detection + auto-select
│   ├── stealth/       Domain fronting, DNS tunnel, mimicry
│   ├── subscription/  V2RayNG/Clash subscription links
│   ├── transport/     9 transport protocols (incl. full QUIC)
│   ├── tun/           TUN device + split tunneling
│   ├── tunnel/        Port forwarding + SOCKS5
│   ├── upgrade/       Graceful zero-downtime upgrade
│   ├── users/         User management
│   └── web/           Real-time dashboard (WebSocket)
├── configs/           Example configurations
├── deploy/            Systemd, scripts, Prometheus, Grafana
└── examples/          SDK usage examples
```

---

## 🔒 Security

- XChaCha20-Poly1305 authenticated encryption
- ECDH X25519 key exchange (REALITY)
- HMAC-SHA256 authentication with replay protection
- Certificate pinning support
- IP whitelist/blacklist
- Brute-force protection
- Full audit logging

---

## 📄 License

[MIT](LICENSE)

---

<p align="center">
  <img src="img/iPst.png" alt="iPShadowT" width="150"/>
  <br/>
  <sub>Made with ❤️ by <a href="https://github.com/iPmartNetwork">iPmart Network</a> (Ali Hassanzadeh)</sub>
  <br/>
  <sub>© 2026 iPmart Network. All rights reserved.</sub>
</p>

