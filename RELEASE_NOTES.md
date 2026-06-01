<p align="center">
  <strong>iPShadowT v2.2.2</strong><br/>
  <sub>Production Hardening · Direct Mode · REALITY Fix</sub>
</p>

---

## Overview

Release **v2.2.2** completes the upload path (client **direct mode** ↔ server **direct mode**), fixes **REALITY**, and adds production tooling (`-validate`, `-doctor`, JSON logs, configurable metrics).

| | |
|---|---|
| **Version** | `v2.2.2` |
| **Type** | Patch (bug fix + hardening) |
| **Date** | 2026-06-01 |
| **Previous** | [v2.2.1](https://github.com/iPmartNetwork/iPShadowT/releases/tag/v2.2.1) |

---

## Highlights

| Area | Change |
|------|--------|
| **Direct mode** | Server `mux.enabled = false` — one TCP per flow, matches client upload path |
| **REALITY** | Working auth (ClientHello token + `REALITYOK`) |
| **Shutdown** | Graceful SIGTERM/SIGINT → `Stop()` |
| **Performance** | `sync.Pool` relay buffers, quality-aware mux selection |
| **CLI** | `-validate`, `-doctor` |
| **Multi-path** | `[[paths]]` wired in client |
| **Tests** | E2E byte relay through tunnel |

---

## Upgrade

Update **both** client (Iran) and server (abroad):

```bash
sudo bash ipshadowt-manager.sh   # option: Update
# or
wget https://github.com/iPmartNetwork/iPShadowT/releases/download/v2.2.2/ipshadowt-linux-amd64
```

Verify:

```bash
ipshadowt -v
ipshadowt -validate -c /etc/ipshadowt/config.toml
ipshadowt -doctor -c /etc/ipshadowt/config.toml
```

### Required config (upload path)

**Client & server** should both use direct mode:

```toml
[mux]
enabled = false

[pool]
size = 16

[performance]
buffer_profile = "upload_boost"
kernel_tuning = true
```

---

## Breaking note

If you intentionally run **smux mode** (`mux.enabled = true` on client), server must also use `mux.enabled = true`. Default `upload_boost` now uses **direct mode on both sides**.

---

## Full changelog

See [CHANGELOG.md](./CHANGELOG.md#222---2026-06-01).
