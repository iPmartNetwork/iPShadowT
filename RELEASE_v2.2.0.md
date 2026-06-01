<p align="center">
  <img src="https://raw.githubusercontent.com/iPmartNetwork/iPShadowT/master/img/iPst.svg" width="180">
</p>

<h2 align="center">iPShadowT v2.2.0</h2>
<h4 align="center">Anti-DPI Multi-Transport Tunnel Engine</h4>

<p align="center">
  <b>10 Transports • 18 Stealth Techniques • Stability Engine • WireGuard Integration</b>
</p>

---

## ⚡ Highlights

This release focuses on **deeper stealth**, **connection stability**, and **WireGuard integration**.

---

### 🕵️ New Anti-DPI Techniques

| Feature | Description |
|---------|-------------|
| **SNI Spoofing** | Packet-level SNI manipulation — DPI sees `google.com`, traffic goes to your server |
| **Domain Fronting** | Client-side CDN fronting — no server changes needed (Google, Cloudflare, Fastly) |
| **FakeTCP Transport** | Wraps UDP inside fake TCP — bypasses UDP-blocking firewalls |
| **Pipeline** | Chain transports: `faketcp → h2mux` for maximum stealth |

**SNI Spoof Methods:**
- `split` — Splits ClientHello so SNI is in a separate TCP segment
- `replace` — Replaces SNI with whitelisted domain
- `double` — Sends fake ClientHello first, then real one

---

### 🛡️ WireGuard Integration

| Feature | Description |
|---------|-------------|
| **WireGuard Forward** | Use WireGuard client (phone/laptop) through iPShadowT tunnel |
| **Auto Key Generation** | Real Curve25519 keys — matching server + client configs generated together |
| **Script Auto-Setup** | One menu option installs WireGuard + generates configs + starts interface |

```
[Phone] → WG Client → [Iran: iPShadowT :51820] → Tunnel → [Foreign: WG Server] → Internet
```

Config example:
```toml
[[forwards]]
name = "socks5"
type = "socks5"
listen = "0.0.0.0:1080"

[[forwards]]
name = "wireguard"
type = "wireguard"
listen = "0.0.0.0:51820"
remote = "10.66.66.1:51820"
```

---

### 💓 Stability Engine

| Module | What it does |
|--------|-------------|
| **Heartbeat** | Ping/pong every 5s, measures RTT, declares dead after 3 misses |
| **Reconnector** | Exponential backoff (100ms→30s), auto-switches transport after 5 failures |
| **Quality Monitor** | Scores each session 0-100 (latency + jitter + loss) |
| **DPI Detector** | Classifies failure patterns → recommends best transport |
| **Buffer Tuner** | Auto-adjusts buffers based on BDP (Bandwidth × RTT) |
| **Warmup Pool** | Pre-connects sessions for instant failover |
| **Quality LB** | Routes new streams to highest-scoring session |

---

### 📈 Upload Boost

New performance profile for maximum upload speed:

```toml
[performance]
buffer_profile = "upload_boost"

[mux]
concurrency = 8
frame_size = 65536
```

Profiles: `balanced` • `upload_boost` • `high_throughput` • `low_cpu`

---

### 🔧 Script Improvements

- **10 transports** in all menus (added `faketcp`)
- **SNI Spoofing** option during client setup
- **Performance profile** selection (4 profiles)
- **WireGuard forward** option in setup + multi-tunnel
- **WireGuard auto-setup** (menu option 7 in Configure)
- **Port range** support: `443,8443,2000-2010`
- **Port conflict** detection with suggestions

---

## 📦 Downloads

| File | Platform |
|------|----------|
| `ipshadowt-linux-amd64` | Linux x86_64 |
| `ipshadowt-linux-arm64` | Linux ARM64 |
| `ipshadowt-linux-armv7` | Linux ARMv7 |
| `ipshadowt-windows-amd64.exe` | Windows x64 |
| `ipshadowt-darwin-amd64` | macOS Intel |
| `ipshadowt-darwin-arm64` | macOS Apple Silicon |
| `ipshadowt-freebsd-amd64` | FreeBSD |
| `ipshadowt-manager.sh` | Deploy script |

---

## ⚡ Quick Install

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/iPShadowT/master/deploy/ipshadowt-manager.sh)
```

## 📋 Upgrade

```bash
# Via script:
bash ipshadowt-manager.sh → 7) Update → 1) Update binary

# Manual:
systemctl stop ipshadowt
curl -fSL -o /usr/local/bin/ipshadowt https://github.com/iPmartNetwork/iPShadowT/releases/download/v2.2.0/ipshadowt-linux-amd64
chmod +x /usr/local/bin/ipshadowt
systemctl start ipshadowt
```

> Config files are backward compatible. New features activate when you add their config sections.

---

## 🆕 New Config Options

```toml
[anti_dpi]
sni_spoof = true
sni_spoof_domain = "www.google.com"
sni_spoof_method = "split"

[kcp]
mode = "fast3"
snd_wnd = 2048
rcv_wnd = 2048

[[forwards]]
name = "wireguard"
type = "wireguard"
listen = "0.0.0.0:51820"
remote = "10.66.66.1:51820"
```

---

## 📊 Stats

- **18 files changed**, 2,011 insertions
- **6 new modules** added
- **10 transports** supported
- **4 performance profiles**
- **0 breaking changes**

---

**Full Changelog**: https://github.com/iPmartNetwork/iPShadowT/compare/v2.0.1...v2.2.0

<p align="center">
  <sub>© 2026 <a href="https://github.com/iPmartNetwork">iPmart Network</a> (Ali Hassanzadeh)</sub><br>
  <sub>Made with ❤️ for the community</sub>
</p>
