<p align="center">
  <strong>iPShadowT v2.2.1</strong><br/>
  <sub>Bug Fix Release · Upload Throughput · Port-Independent</sub>
</p>

---

## Overview

This patch release fixes **asymmetric upload performance** on `tcpmux` tunnels — the common **Iran client → foreign server** setup where download was fine but upload stayed low.

All improvements are **port-agnostic**: they apply whether you run on `443`, `8443`, or any custom port. No per-user port mapping required.

| | |
|---|---|
| **Version** | `v2.2.1` |
| **Type** | Bug fix (semver patch) |
| **Date** | 2026-06-01 |
| **Previous** | [v2.2.0](https://github.com/iPmartNetwork/iPShadowT/releases/tag/v2.2.0) |
| **Repository** | [iPmartNetwork/iPShadowT](https://github.com/iPmartNetwork/iPShadowT) |

---

## Highlights

| Area | Before (v2.2.0) | After (v2.2.1) |
|------|-----------------|----------------|
| TCP send/recv buffers | Config ignored on `tcpmux` | Applied on every connection via `SO_SNDBUF` / `SO_RCVBUF` |
| Relay copy buffer | 32 KB (`io.Copy` default) | **256 KB** on mux + server relay |
| `kernel_tuning = true` | Defined, never executed | Runs at client/server startup (Linux) |
| `upload_boost` profile | Missing in sysctl layer | BBR + 16 MB windows + `tcp_notsent_lowat` |
| Default buffers | 4 MB mux / unset TCP | **16 MB TCP** + **8 MB smux** when unset |
| Manager script | `balanced` + fragment on | **`upload_boost` default**, fragment/padding off for speed |
| Auto-updater | Wrong GitHub org | `iPmartNetwork/iPShadowT` |

---

## What Was Fixed

### Core engine

- **`internal/utils/tcpopt.go`** — Central TCP tuning (TLS unwrap, listener wrapper, large relay buffers)
- **`tcpmux`** — Socket buffers on dial **and** accept
- **Client / Server** — `kernel_tuning` wired; upload buffer logging at startup
- **Config** — Upload-friendly defaults + full `upload_boost` profile support
- **Mux relay** — 256 KB bidirectional copy for upload and download paths

### Install & ops

- **`ipshadowt-manager.sh`** — Client wizard defaults to `upload_boost`, `kernel_tuning = true`
- **Multi-tunnel & export** — Generated configs include upload tuning
- **Sysctl (install)** — `tcp_window_scaling`, `tcp_notsent_lowat` added to base tuning

### Misc

- CLI default version corrected (`v2.2.1`)
- Docker `ARG VERSION=2.2.1`
- Systemd / updater URLs point to `iPmartNetwork/iPShadowT`

---

## Traffic path (upload)

```
App → SOCKS5 (Iran client)
  → smux stream (8 MB buffer)
  → TCP tunnel (SO_SNDBUF 16 MB)
  → Foreign server (SO_RCVBUF 16 MB)
  → Internet
```

---

## Upgrade

> Update **both** sides — client (Iran) and server (abroad) — for best results.

### Option A — Manager script (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/iPShadowT/master/deploy/ipshadowt-manager.sh -o ipshadowt-manager.sh
sudo bash ipshadowt-manager.sh
# Menu → Update binary → Restart service
```

### Option B — Binary only

<details>
<summary><strong>Linux AMD64</strong></summary>

```bash
wget https://github.com/iPmartNetwork/iPShadowT/releases/download/v2.2.1/ipshadowt-linux-amd64
chmod +x ipshadowt-linux-amd64
sudo install -m 755 ipshadowt-linux-amd64 /usr/local/bin/ipshadowt
sudo systemctl restart ipshadowt
```

</details>

<details>
<summary><strong>Linux ARM64</strong></summary>

```bash
wget https://github.com/iPmartNetwork/iPShadowT/releases/download/v2.2.1/ipshadowt-linux-arm64
chmod +x ipshadowt-linux-arm64
sudo install -m 755 ipshadowt-linux-arm64 /usr/local/bin/ipshadowt
sudo systemctl restart ipshadowt
```

</details>

### Option C — Docker

```bash
docker pull ghcr.io/ipmartnetwork/ipshadowt:2.2.1
```

---

## Recommended configuration

Minimal snippet — works with **any port**:

```toml
[performance]
nodelay = true
buffer_profile = "upload_boost"   # balanced | high_throughput | low_cpu
kernel_tuning = true              # Linux + root: BBR & TCP windows
```

For maximum upload on `tcpmux`, keep anti-DPI overhead off:

```toml
[anti_dpi]
enabled = false
# fragment = false
# padding = false
# traffic_shape = false
```

Full examples: [`configs/client-upload-boost.toml`](https://github.com/iPmartNetwork/iPShadowT/blob/master/configs/client-upload-boost.toml)

---

## Verify after upgrade

- [ ] `ipshadowt -v` shows **v2.2.1**
- [ ] Log contains: `Upload tuning: send_buf=16777216 recv_buf=16777216 ...`
- [ ] On Linux with root: `sysctl net.ipv4.tcp_congestion_control` → `bbr`
- [ ] Upload test improved vs v2.2.0 on same port

---

## Assets

| Platform | Binary |
|----------|--------|
| Linux AMD64 | `ipshadowt-linux-amd64` |
| Linux ARM64 | `ipshadowt-linux-arm64` |
| Linux ARMv7 | `ipshadowt-linux-armv7` |
| Windows AMD64 | `ipshadowt-windows-amd64.exe` |
| macOS Intel | `ipshadowt-darwin-amd64` |
| macOS Apple Silicon | `ipshadowt-darwin-arm64` |
| Docker | `ghcr.io/ipmartnetwork/ipshadowt:2.2.1` |

Checksums: `SHA256SUMS.txt` · `MD5SUMS.txt` in release assets.

---

## Full changelog

See [CHANGELOG.md](https://github.com/iPmartNetwork/iPShadowT/blob/master/CHANGELOG.md#221---2026-06-01) for complete commit-level details.

---

<p align="center">
  <sub>Made with ❤️ by <a href="https://github.com/iPmartNetwork">iPmart Network</a></sub>
</p>
