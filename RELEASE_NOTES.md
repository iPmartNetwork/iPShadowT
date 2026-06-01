# iPShadowT v2.2.1 — Bug Fix Release

**Release date:** 2026-06-01  
**Repository:** https://github.com/iPmartNetwork/iPShadowT

## Summary

Fixes slow upload speed on **tcpmux** tunnels (Iran client → foreign server). Improvements apply on **any port** — no per-port configuration required.

## Fixed

- Apply `SO_SNDBUF` / `SO_RCVBUF` from config on all TCP connections
- Tune accepted server connections automatically
- Use 256KB relay buffers (was 32KB) for upload and download paths
- Wire `kernel_tuning = true` to startup (BBR + TCP window tuning on Linux)
- Implement missing `upload_boost` sysctl profile
- Auto upload-friendly buffer defaults for client and server
- Manager script: default to `upload_boost`, disable fragment/padding for speed profile
- CLI version and GitHub auto-updater URL corrected

## Upgrade

```bash
# One-line update (existing installs)
curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/iPShadowT/master/deploy/ipshadowt-manager.sh | sudo bash

# Or download binary
wget https://github.com/iPmartNetwork/iPShadowT/releases/download/v2.2.1/ipshadowt-linux-amd64
chmod +x ipshadowt-linux-amd64
sudo systemctl restart ipshadowt
```

Both **client (Iran)** and **server (abroad)** should be updated for best upload performance.

## Recommended config

```toml
[performance]
buffer_profile = "upload_boost"
kernel_tuning = true
nodelay = true
```

See [CHANGELOG.md](CHANGELOG.md) for full details.
