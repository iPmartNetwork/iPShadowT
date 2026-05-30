<p align="center">
  <img src="img/iPst.svg" alt="iPShadowT Logo" width="300"/>
</p>


<p align="center">
  <strong>Anti-DPI Multi-Transport Tunnel Engine</strong>
</p>

<p align="center">
  <a href="https://github.com/iPmartNetwork/iPShadowT/blob/master/VERSION"><img src="https://img.shields.io/badge/version-v1.0.0-blue?style=flat-square" alt="Version"/></a>
  <a href="https://github.com/iPmartNetwork/iPShadowT/blob/master/LICENSE"><img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="License"/></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go"/></a>
  <a href="https://github.com/iPmartNetwork/iPShadowT/releases"><img src="https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows-lightgrey?style=flat-square" alt="Platform"/></a>
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

iPShadowT is a high-performance, self-contained tunnel engine designed to bypass deep packet inspection (DPI) and internet censorship. It combines 8 transport protocols, 15 stealth techniques, and intelligent auto-selection into a single Go binary with zero external dependencies.

Built to survive even the most extreme filtering scenarios — including complete internet shutdowns where only DNS traffic is allowed.

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
| 8 Transports | TCP, WebSocket, HTTP/2, gRPC, REALITY, ShadowTLS, QUIC, KCP, Reverse |
| 15 Stealth Techniques | uTLS, Fragment, ECH, Shaping, Domain Fronting, DNS Tunnel, and more |
| Multiplexing | Thousands of streams over a single connection (smux) |
| Multi-Path | Automatic failover across multiple paths |
| Encryption | XChaCha20-Poly1305 AEAD |
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
| Split Tunneling | Iran IPs go direct, rest through tunnel |
| Load Balancer | 5 strategies (round-robin, least-conn, weighted, IP-hash, fastest) |
| CDN Support | Cloudflare, Fastly, Arvan, custom |
| DNS over HTTPS | Bypass DNS poisoning |
| TUN/TAP | Full system traffic capture (Layer 2 & 3) |

### 🔧 Management

| Feature | Description |
|---------|-------------|
| Web Panel | Built-in dashboard with real-time stats |
| REST API | Full management API for external tools |
| User Management | Multi-user with traffic limits and expiry |
| Subscription Links | V2RayNG/Clash compatible |
| Prometheus Metrics | Grafana-ready monitoring |
| Auto-Update | Self-update from GitHub releases |
| Backup/Restore | Automatic periodic backups |
| Hot-Reload | Change config without restart |

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
┌─────────────────────────────────────────────────┐
│                  iPShadowT                      │
├─────────────────────────────────────────────────┤
│  Input: SOCKS5 / HTTP / TCP / UDP / TUN         │
│  ↓                                              │
│  Split Tunnel (Iran direct, rest proxy)         │
│  ↓                                              │
│  Multiplexer (smux - 1000s of streams)          │
│  ↓                                              │
│  Encryption (XChaCha20-Poly1305 + Padding)      │
│  ↓                                              │
│  Anti-DPI (uTLS + Fragment + Shaping + ECH)     │
│  ↓                                              │
│  Transport (REALITY / WS / H2 / gRPC / ...)    │
│  ↓                                              │
│  Multi-Path (auto-failover between paths)       │
└─────────────────────────────────────────────────┘
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
docker build -t ipshadowt .
docker run -v ./config.toml:/etc/ipshadowt/config.toml -p 443:443 ipshadowt
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
- 🧪 Auto-detect best transport for your network
- 🔑 REALITY key generation
- 📊 Live status, logs, speed test, system info
- 🌐 Network diagnostics (DPI detection, port check, BBR)
- 🐕 Connection watchdog (auto-restart on failure)
- 🔥 Automatic firewall configuration
- 💾 Backup / Restore with auto-backup (cron)
- 📡 Port forward manager (add/remove from menu)
- 📤 Export client config (copy-paste ready)
- 🔄 One-click update from GitHub

---

## 📁 Project Structure

```
iPShadowT/
├── core/              Standalone engine (SDK)
├── cmd/ipshadowt/     CLI application
├── internal/
│   ├── antidpi/       15 anti-DPI techniques
│   ├── stealth/       Domain fronting, DNS tunnel, mimicry
│   ├── transport/     8 transport protocols
│   ├── crypto/        XChaCha20-Poly1305 encryption
│   ├── mux/           Stream multiplexing
│   ├── multipath/     Multi-path + aggregation
│   ├── smart/         DPI detection + auto-select
│   ├── tunnel/        Port forwarding + SOCKS5 + UDP
│   ├── security/      Whitelist, audit, brute-force
│   ├── users/         User management
│   ├── web/           Web panel
│   ├── api/           REST API
│   └── ...            20+ more modules
├── configs/           Example configurations
├── deploy/            Systemd, install scripts
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

