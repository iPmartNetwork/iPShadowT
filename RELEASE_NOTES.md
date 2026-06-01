<p align="center">
  <strong>iPShadowT v2.2.3</strong><br/>
  <sub>Direct Mode · REALITY Fix · Production Hardening</sub>
</p>

---

## Overview

**v2.2.3** is the recommended release. It includes everything from v2.2.2 plus a **direct pool shutdown fix** (CI `-race` clean).

| | |
|---|---|
| **Version** | `v2.2.3` |
| **Type** | Patch (bug fix) |
| **Date** | 2026-06-01 |
| **Previous** | [v2.2.2](https://github.com/iPmartNetwork/iPShadowT/releases/tag/v2.2.2) |

> **Note:** Do not use v2.2.2 — its CI tests failed. Always install **v2.2.3**.

---

## Highlights (v2.2.2 + v2.2.3)

| Area | Change |
|------|--------|
| **Direct mode** | Server `mux.enabled = false` — one TCP per flow, matches client upload path |
| **REALITY** | Working auth (ClientHello token + `REALITYOK`) |
| **Shutdown** | Graceful SIGTERM/SIGINT → `Stop()` |
| **v2.2.3 fix** | Direct pool data race on shutdown fixed |
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
wget https://github.com/iPmartNetwork/iPShadowT/releases/download/v2.2.3/ipshadowt-linux-amd64
chmod +x ipshadowt-linux-amd64
sudo mv ipshadowt-linux-amd64 /usr/local/bin/ipshadowt
```

Verify:

```bash
ipshadowt -v                                    # v2.2.3
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

If you intentionally run **smux mode** (`mux.enabled = true` on client), server must also use `mux.enabled = true`. Default `upload_boost` uses **direct mode on both sides**.

---

## Full changelog

- [v2.2.3](./CHANGELOG.md#223---2026-06-01)
- [v2.2.2](./CHANGELOG.md#222---2026-06-01)
