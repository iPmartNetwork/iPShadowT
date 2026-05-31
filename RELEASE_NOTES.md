# iPShadowT v2.0.0

## 🚀 Major Release — Full Integration & New Features

This release brings 18 new modules fully integrated into the engine, making iPShadowT a complete anti-censorship tunnel platform.

---

### ⚡ New Features

#### Transport
- **Full QUIC Implementation** — Real QUIC via `quic-go` with 0-RTT, connection migration, and UDP multiplexing

#### Anti-DPI & Stealth
- **Traffic Obfuscation** — 4 modes: HTTPS mimic, Video streaming mimic, Random burst, Constant rate
- **DNS Leak Protection** — Automatic DNS-over-HTTPS (DoH) for all hostname resolution

#### Networking
- **Auto-Failover** — Multi-path failover with priority/round-robin/latency strategies and automatic recovery
- **CDN Mode** — Full CDN integration in deploy script (Cloudflare, Gcore, Arvan)
- **Split Tunneling** — Rule-based routing with Iran IP bypass (200+ CIDRs)
- **Adaptive Connection Pool** — Auto-scaling pool with warmup, min/max sizing
- **Cluster Mode** — Multi-server with geographic routing, health checks, state sync
- **Config Sync** — Encrypted config push/pull between servers

#### Security & Management
- **Plugin System** — Transport, Auth, and Filter plugins with hook system
- **Rate Limiting** — Per-user bandwidth control integrated in server
- **Security Manager** — IP blacklist/whitelist, brute-force protection, audit log
- **ACME/Auto-Cert** — Automatic TLS certificate management with renewal

#### Monitoring & Operations
- **Real-time Dashboard** — WebSocket-based with traffic charts, user management
- **Prometheus + Grafana** — Docker Compose stack with alert rules
- **Health/Watchdog** — HTTP health endpoint integrated in systemd
- **Graceful Upgrade** — Zero-downtime binary upgrades

#### Deploy Script
- **CDN mode** in setup wizard
- **TLS option** for tcpmux transport
- **Watchdog** integration with systemd WatchdogSec
- **Smart transport detection** (TCP/UDP/TLS/H2 probing with recommendations)
- **Multi-tunnel status** — Full per-tunnel status display with tags

---

### 🔧 Integration

All modules are now wired into the engine:

| Module | Where |
|--------|-------|
| DNS DoH | Client — resolves before connect |
| Obfuscation | Client — wraps every session |
| Rate Limiter | Server — wraps every connection |
| Security | Server — checks before handshake |
| Plugin Filters | Server — runs after handshake |
| Health/Metrics | Server — tracks connections/streams/bytes |
| Failover | Engine — switches paths on failure |

---

### 📦 Build Targets

| Platform | File |
|----------|------|
| Linux x86_64 | `ipshadowt-linux-amd64` |
| Linux ARM64 | `ipshadowt-linux-arm64` |
| Linux ARM (v7) | `ipshadowt-linux-arm` |
| Windows x64 | `ipshadowt-windows-amd64.exe` |
| macOS Intel | `ipshadowt-darwin-amd64` |
| macOS Apple Silicon | `ipshadowt-darwin-arm64` |

---

### 📋 Quick Install

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/iPShadowT/master/deploy/ipshadowt-manager.sh)
```

---

**Full Changelog**: https://github.com/iPmartNetwork/iPShadowT/compare/v1.0.0...v2.0.0
