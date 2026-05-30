#!/bin/bash
# iPShadowT Manager v1.0.0
# iPmart Network (Ali Hassanzadeh)
# https://github.com/iPmartNetwork/iPShadowT

set -e

# ─── Constants ────────────────────────────────────
VERSION="1.0.0"
GITHUB_REPO="iPmartNetwork/iPShadowT"
BINARY_NAME="ipshadowt"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/ipshadowt"
BACKUP_DIR="/etc/ipshadowt/backups"
SERVICE_NAME="ipshadowt"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
SYSCTL_FILE="/etc/sysctl.d/99-ipshadowt.conf"

# ─── Colors ───────────────────────────────────────
R='\033[0;31m'
G='\033[0;32m'
Y='\033[1;33m'
B='\033[0;34m'
C='\033[0;36m'
W='\033[1;37m'
D='\033[0;90m'
N='\033[0m'

# ─── UI Helpers ───────────────────────────────────
print_line()  { echo -e "${B}────────────────────────────────────────────────────${N}"; }
print_dline() { echo -e "${B}════════════════════════════════════════════════════${N}"; }
msg_ok()      { echo -e "  ${G}[OK]${N} $1"; }
msg_err()     { echo -e "  ${R}[!!]${N} $1"; }
msg_warn()    { echo -e "  ${Y}[**]${N} $1"; }
msg_info()    { echo -e "  ${C}[>>]${N} $1"; }
msg_ask()     { echo -ne "  ${W}[?]${N} $1"; }

print_banner() {
    clear
    echo ""
    print_dline
    echo -e "${W}   _ ____  ____  _               _          _____ ${N}"
    echo -e "${W}  (_)  _ \\/ ___|| |__   __ _  __| | _____  |_   _|${N}"
    echo -e "${W}  | | |_) \\___ \\| '_ \\ / _\` |/ _\` |/ _ \\ \\ /\\ / /| |  ${N}"
    echo -e "${W}  | |  __/ ___) | | | | (_| | (_| | (_) \\ V  V / | |  ${N}"
    echo -e "${W}  |_|_|   |____/|_| |_|\\__,_|\\__,_|\\___/ \\_/\\_/  |_|  ${N}"
    echo ""
    echo -e "  ${D}Anti-DPI Multi-Transport Tunnel Engine${N}"
    echo -e "  ${D}iPmart Network (Ali Hassanzadeh) - v${VERSION}${N}"
    print_dline
    # Server info
    local ip=$(curl -s4 --max-time 2 ifconfig.me 2>/dev/null || echo "N/A")
    local geo=$(curl -s --max-time 2 "http://ip-api.com/line/${ip}?fields=country,city,isp" 2>/dev/null)
    local country=$(echo "$geo" | sed -n '1p')
    local city=$(echo "$geo" | sed -n '2p')
    local isp=$(echo "$geo" | sed -n '3p')
    echo -e "  ${D}IP: ${W}${ip}${D}  |  ${city}, ${country}  |  ${isp}${N}"
    print_dline
    echo ""
}

press_enter() {
    echo ""
    msg_ask "Press [Enter] to return to menu..."
    read -r
}

# ─── System Checks ────────────────────────────────
check_root() {
    if [ "$EUID" -ne 0 ]; then
        msg_err "This script must be run as root."
        echo "    Run: sudo bash $0"
        exit 1
    fi
}

detect_arch() {
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)  ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        armv7l)  ARCH="arm" ;;
        *) msg_err "Unsupported architecture: $ARCH"; exit 1 ;;
    esac
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
}

detect_os_type() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS_NAME=$ID
        OS_VERSION=$VERSION_ID
    else
        OS_NAME="unknown"
        OS_VERSION="0"
    fi
}

is_installed() { [ -f "${INSTALL_DIR}/${BINARY_NAME}" ]; }
is_running()   { systemctl is-active --quiet ${SERVICE_NAME} 2>/dev/null; }

get_version() {
    if is_installed; then
        ${INSTALL_DIR}/${BINARY_NAME} -v 2>/dev/null | head -1 | awk '{print $2}' || echo "unknown"
    else
        echo "not installed"
    fi
}

gen_pass() { cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 32 | head -n 1; }

validate_ip() {
    local ip=$1
    [[ $ip =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]] && return 0
    [[ $ip =~ ^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)*$ ]] && return 0
    return 1
}

# ─── Install ──────────────────────────────────────
do_install() {
    print_banner
    echo -e "  ${W}[ INSTALL ]${N}"
    echo ""

    if is_installed; then
        msg_warn "iPShadowT already installed ($(get_version))"
        msg_ask "Reinstall? [y/N]: "; read -r ans
        [[ "$ans" != "y" ]] && return
    fi

    detect_arch
    msg_info "System: ${OS}/${ARCH}"

    # Prerequisites
    msg_info "Installing prerequisites..."
    apt-get update -qq >/dev/null 2>&1 || true
    for pkg in curl wget jq iptables tar; do
        command -v $pkg &>/dev/null || apt-get install -y -qq $pkg >/dev/null 2>&1 || true
    done
    msg_ok "Prerequisites ready"

    # Download binary
    msg_info "Downloading binary..."
    local url="https://github.com/${GITHUB_REPO}/releases/latest/download/${BINARY_NAME}-${OS}-${ARCH}"
    local tmp="/tmp/${BINARY_NAME}-dl"

    if curl -fSL --progress-bar -o "$tmp" "$url" 2>/dev/null; then
        chmod +x "$tmp"
        mv "$tmp" "${INSTALL_DIR}/${BINARY_NAME}"
        msg_ok "Binary installed"
    elif [ -f "./${BINARY_NAME}-${OS}-${ARCH}" ]; then
        cp "./${BINARY_NAME}-${OS}-${ARCH}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
        msg_ok "Binary installed (local)"
    elif [ -f "./${BINARY_NAME}" ]; then
        cp "./${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
        msg_ok "Binary installed (local)"
    else
        msg_err "Download failed. Place binary in current dir and retry."
        return 1
    fi

    # Config dir
    mkdir -p "${CONFIG_DIR}"

    # Systemd service
    cat > "${SERVICE_FILE}" << EOF
[Unit]
Description=iPShadowT Tunnel
After=network.target network-online.target
Wants=network-online.target
[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BINARY_NAME} -c ${CONFIG_DIR}/config.toml
Restart=always
RestartSec=5
LimitNOFILE=65535
[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable ${SERVICE_NAME} >/dev/null 2>&1
    msg_ok "Systemd service created"

    # Kernel tuning
    cat > "${SYSCTL_FILE}" << 'EOF'
net.ipv4.ip_forward=1
net.core.rmem_max=16777216
net.core.wmem_max=16777216
net.ipv4.tcp_rmem=4096 524288 16777216
net.ipv4.tcp_wmem=4096 524288 16777216
net.ipv4.tcp_fastopen=3
net.ipv4.tcp_slow_start_after_idle=0
net.core.somaxconn=65535
net.core.default_qdisc=fq
net.ipv4.tcp_congestion_control=bbr
EOF
    sysctl -p "${SYSCTL_FILE}" >/dev/null 2>&1 || true
    msg_ok "Kernel tuned + BBR enabled"

    # Firewall
    if command -v ufw &>/dev/null; then
        ufw allow 443/tcp >/dev/null 2>&1
        ufw allow 443/udp >/dev/null 2>&1
    fi
    msg_ok "Firewall configured"

    echo ""
    print_line
    msg_ok "Installation complete!"
    print_line
    echo ""
    msg_ask "Configure tunnel now? [Y/n]: "; read -r ans
    [[ "${ans:-y}" == "y" ]] && do_configure
}

# ─── Configure ────────────────────────────────────
do_configure() {
    print_banner
    echo -e "  ${W}[ CONFIGURE TUNNEL ]${N}"
    echo ""
    echo -e "  ${C}1)${N} Setup as ${W}Iran Server${N} (Client - behind filter)"
    echo -e "  ${C}2)${N} Setup as ${W}Foreign Server${N} (Server - abroad)"
    echo -e "  ${C}3)${N} Edit config manually"
    echo -e "  ${C}4)${N} Show current config"
    echo -e "  ${C}5)${N} Auto-detect best transport"
    echo -e "  ${C}0)${N} Back"
    echo ""
    msg_ask "Choice: "; read -r choice

    case $choice in
        1) setup_iran ;;
        2) setup_foreign ;;
        3) nano "${CONFIG_DIR}/config.toml" 2>/dev/null || vi "${CONFIG_DIR}/config.toml" ;;
        4) [ -f "${CONFIG_DIR}/config.toml" ] && cat "${CONFIG_DIR}/config.toml" || msg_err "No config" ;;
        5) msg_ask "Server IP: "; read -r ip; test_transport "$ip" ;;
        0) return ;;
    esac
    press_enter
}

setup_iran() {
    echo ""
    print_line
    echo -e "  ${W}IRAN SERVER (CLIENT) SETUP${N}"
    print_line
    echo ""

    msg_ask "Foreign server IP/domain: "; read -r remote_ip
    validate_ip "$remote_ip" || { msg_err "Invalid address"; return; }

    msg_ask "Foreign server port [443]: "; read -r port
    port=${port:-443}

    local password=$(gen_pass)
    msg_ask "Password [auto-generate]: "; read -r up
    [ -n "$up" ] && password="$up"

    echo ""
    echo -e "  ${W}Select Transport:${N}"
    echo -e "    ${C}1)${N} reality     ${D}(recommended - max stealth)${N}"
    echo -e "    ${C}2)${N} kcp         ${D}(works on filtered IPs)${N}"
    echo -e "    ${C}3)${N} wsmux       ${D}(CDN compatible)${N}"
    echo -e "    ${C}4)${N} shadowtls   ${D}(no cert needed)${N}"
    echo -e "    ${C}5)${N} tcpmux      ${D}(simple & fast)${N}"
    echo -e "    ${C}6)${N} h2mux       ${D}(looks like web traffic)${N}"
    echo -e "    ${C}7)${N} grpc        ${D}(looks like API)${N}"
    msg_ask "Choice [1]: "; read -r tc
    local transport="reality"
    case $tc in 2) transport="kcp";; 3) transport="wsmux";; 4) transport="shadowtls";; 5) transport="tcpmux";; 6) transport="h2mux";; 7) transport="grpc";; esac

    msg_ask "SOCKS5 port [1080]: "; read -r sp
    sp=${sp:-1080}

    mkdir -p "${CONFIG_DIR}"
    cat > "${CONFIG_DIR}/config.toml" << EOF
# iPShadowT Client - Generated by Manager
mode = "client"
log_level = "info"
transport = "${transport}"
remote_addr = "${remote_ip}:${port}"
password = "${password}"

[mux]
concurrency = 4
frame_size = 32768

[heartbeat]
enabled = true
interval = 20
timeout = 40

[performance]
nodelay = true
keepalive = 15
buffer_profile = "balanced"

[anti_dpi]
enabled = true
utls_fingerprint = "chrome"
fragment = true
fragment_size = "40-80"
padding = true

[[forwards]]
name = "socks5"
type = "socks5"
listen = "0.0.0.0:${sp}"
EOF

    echo ""
    print_line
    echo -e "  ${G}CONFIG SAVED${N}"
    print_line
    echo -e "  Remote:    ${W}${remote_ip}:${port}${N}"
    echo -e "  Transport: ${W}${transport}${N}"
    echo -e "  Password:  ${W}${password}${N}"
    echo -e "  SOCKS5:    ${W}0.0.0.0:${sp}${N}"
    print_line

    msg_ask "Start service now? [Y/n]: "; read -r ans
    if [[ "${ans:-y}" == "y" ]]; then
        systemctl restart ${SERVICE_NAME} && sleep 2
        is_running && msg_ok "Service running!" || msg_err "Failed - check: journalctl -u ${SERVICE_NAME}"
    fi
}

setup_foreign() {
    echo ""
    print_line
    echo -e "  ${W}FOREIGN SERVER SETUP${N}"
    print_line
    echo ""

    msg_ask "Listen port [443]: "; read -r port
    port=${port:-443}

    local password=$(gen_pass)
    msg_ask "Password [auto-generate]: "; read -r up
    [ -n "$up" ] && password="$up"

    echo ""
    echo -e "  ${W}Select Transport:${N}"
    echo -e "    ${C}1)${N} reality     ${D}(recommended)${N}"
    echo -e "    ${C}2)${N} kcp         ${D}(filtered IPs)${N}"
    echo -e "    ${C}3)${N} wsmux       ${D}(CDN)${N}"
    echo -e "    ${C}4)${N} shadowtls   ${D}(no cert)${N}"
    echo -e "    ${C}5)${N} tcpmux      ${D}(simple)${N}"
    msg_ask "Choice [1]: "; read -r tc
    local transport="reality"
    case $tc in 2) transport="kcp";; 3) transport="wsmux";; 4) transport="shadowtls";; 5) transport="tcpmux";; esac

    mkdir -p "${CONFIG_DIR}"
    cat > "${CONFIG_DIR}/config.toml" << EOF
# iPShadowT Server - Generated by Manager
mode = "server"
log_level = "info"
transport = "${transport}"
bind_addr = "0.0.0.0:${port}"
password = "${password}"

[mux]
concurrency = 8
frame_size = 32768

[heartbeat]
enabled = true
interval = 20
timeout = 40

[performance]
nodelay = true
keepalive = 15
buffer_profile = "high_throughput"
kernel_tuning = true
EOF

    local server_ip=$(curl -s4 --max-time 5 ifconfig.me 2>/dev/null || echo "YOUR_IP")

    echo ""
    print_line
    echo -e "  ${G}SERVER READY${N}"
    print_line
    echo -e "  ${W}Give this to your Iran client:${N}"
    echo ""
    echo -e "  Server IP:   ${G}${server_ip}${N}"
    echo -e "  Port:        ${G}${port}${N}"
    echo -e "  Transport:   ${G}${transport}${N}"
    echo -e "  Password:    ${G}${password}${N}"
    print_line

    msg_ask "Start service now? [Y/n]: "; read -r ans
    if [[ "${ans:-y}" == "y" ]]; then
        systemctl restart ${SERVICE_NAME} && sleep 2
        is_running && msg_ok "Service running!" || msg_err "Failed - check: journalctl -u ${SERVICE_NAME}"
    fi
}

test_transport() {
    local ip=$1
    msg_info "Testing connectivity to ${ip}..."
    if timeout 5 bash -c "echo >/dev/tcp/${ip}/443" 2>/dev/null; then
        msg_ok "TCP/443: OK"
        if timeout 3 bash -c "echo >/dev/udp/${ip}/443" 2>/dev/null; then
            msg_ok "UDP: OK -> Recommend: kcp or quic"
        else
            msg_warn "UDP: Blocked -> Recommend: reality or wsmux"
        fi
    else
        msg_err "TCP/443: Failed -> Recommend: wsmux (via CDN)"
    fi
}

# ─── Service Control ──────────────────────────────
do_service() {
    print_banner
    echo -e "  ${W}[ SERVICE CONTROL ]${N}"
    echo ""
    echo -e "  ${C}1)${N} Start"
    echo -e "  ${C}2)${N} Stop"
    echo -e "  ${C}3)${N} Restart"
    echo -e "  ${C}4)${N} Enable (auto-start on boot)"
    echo -e "  ${C}5)${N} Disable"
    echo -e "  ${C}0)${N} Back"
    echo ""
    msg_ask "Choice: "; read -r c
    case $c in
        1) systemctl start ${SERVICE_NAME} && msg_ok "Started" || msg_err "Failed" ;;
        2) systemctl stop ${SERVICE_NAME} && msg_ok "Stopped" || msg_err "Failed" ;;
        3) systemctl restart ${SERVICE_NAME} && msg_ok "Restarted" || msg_err "Failed" ;;
        4) systemctl enable ${SERVICE_NAME} >/dev/null 2>&1 && msg_ok "Enabled" ;;
        5) systemctl disable ${SERVICE_NAME} >/dev/null 2>&1 && msg_ok "Disabled" ;;
        0) return ;;
    esac
    press_enter
}

# ─── Status ───────────────────────────────────────
do_status() {
    print_banner
    echo -e "  ${W}[ STATUS ]${N}"
    echo ""

    if ! is_installed; then
        msg_err "iPShadowT is not installed"
        press_enter; return
    fi

    local ver=$(get_version)
    local status="${R}STOPPED${N}"
    is_running && status="${G}RUNNING${N}"

    echo -e "  Version:    ${W}${ver}${N}"
    echo -e "  Status:     ${status}"

    if [ -f "${CONFIG_DIR}/config.toml" ]; then
        local mode=$(grep '^mode' ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)
        local transport=$(grep '^transport' ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)
        local remote=$(grep '^remote_addr' ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)
        local bind=$(grep '^bind_addr' ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)
        echo -e "  Mode:       ${W}${mode}${N}"
        echo -e "  Transport:  ${W}${transport}${N}"
        [ -n "$remote" ] && echo -e "  Remote:     ${W}${remote}${N}"
        [ -n "$bind" ] && echo -e "  Bind:       ${W}${bind}${N}"
    fi

    # System info
    echo ""
    print_line
    local pub_ip=$(curl -s4 --max-time 3 ifconfig.me 2>/dev/null || echo "N/A")
    local mem=$(free -m 2>/dev/null | awk '/Mem:/{printf "%d/%dMB (%d%%)", $3, $2, $3*100/$2}')
    local cpu=$(nproc 2>/dev/null || echo "?")
    local load=$(cat /proc/loadavg 2>/dev/null | awk '{print $1}')
    local disk=$(df -h / 2>/dev/null | awk 'NR==2{print $3"/"$2" ("$5")"}')

    echo -e "  Public IP:  ${W}${pub_ip}${N}"
    echo -e "  Memory:     ${mem}"
    echo -e "  CPU:        ${cpu} cores (load: ${load})"
    echo -e "  Disk:       ${disk}"
    print_line

    echo ""
    echo -e "  ${C}1)${N} View logs (last 30)"
    echo -e "  ${C}2)${N} Live logs (Ctrl+C to exit)"
    echo -e "  ${C}3)${N} Speed test"
    echo -e "  ${C}0)${N} Back"
    msg_ask "Choice: "; read -r c
    case $c in
        1) journalctl -u ${SERVICE_NAME} --no-pager -n 30 ;;
        2) journalctl -u ${SERVICE_NAME} -f ;;
        3) msg_info "Testing..."; local s=$(curl -so /dev/null -w '%{speed_download}' http://speedtest.tele2.net/1MB.zip 2>/dev/null); echo -e "  Download: $(echo $s | awk '{printf "%.2f Mbps", $1/131072}')"; ;;
        0) return ;;
    esac
    press_enter
}

# ─── Keys ─────────────────────────────────────────
do_keys() {
    print_banner
    echo -e "  ${W}[ KEY MANAGEMENT ]${N}"
    echo ""
    echo -e "  ${C}1)${N} Generate REALITY keys"
    echo -e "  ${C}2)${N} Generate random password"
    echo -e "  ${C}3)${N} Export client config"
    echo -e "  ${C}0)${N} Back"
    echo ""
    msg_ask "Choice: "; read -r c
    case $c in
        1) is_installed && ${INSTALL_DIR}/${BINARY_NAME} --gen-reality-keys || msg_err "Not installed" ;;
        2) echo ""; msg_ok "Password: $(gen_pass)"; echo "" ;;
        3) export_config ;;
        0) return ;;
    esac
    press_enter
}

export_config() {
    [ ! -f "${CONFIG_DIR}/config.toml" ] && { msg_err "No config"; return; }
    local server_ip=$(curl -s4 --max-time 3 ifconfig.me 2>/dev/null || echo "YOUR_IP")
    local port=$(grep '^bind_addr' ${CONFIG_DIR}/config.toml 2>/dev/null | grep -oP ':\K[0-9]+')
    local password=$(grep '^password' ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)
    local transport=$(grep '^transport' ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)
    echo ""
    print_line
    echo -e "  ${W}CLIENT CONFIG (copy to Iran server):${N}"
    print_line
    echo ""
    echo "mode = \"client\""
    echo "transport = \"${transport}\""
    echo "remote_addr = \"${server_ip}:${port}\""
    echo "password = \"${password}\""
    echo ""
    echo "[mux]"
    echo "concurrency = 4"
    echo ""
    echo "[anti_dpi]"
    echo "enabled = true"
    echo "utls_fingerprint = \"chrome\""
    echo "fragment = true"
    echo ""
    echo "[[forwards]]"
    echo "name = \"socks5\""
    echo "type = \"socks5\""
    echo "listen = \"0.0.0.0:1080\""
    print_line
}

# ─── Backup ───────────────────────────────────────
do_backup() {
    print_banner
    echo -e "  ${W}[ BACKUP / RESTORE ]${N}"
    echo ""
    echo -e "  ${C}1)${N} Create backup now"
    echo -e "  ${C}2)${N} List backups"
    echo -e "  ${C}3)${N} Restore from backup"
    echo -e "  ${C}4)${N} Setup auto-backup (cron)"
    echo -e "  ${C}0)${N} Back"
    echo ""
    msg_ask "Choice: "; read -r c
    case $c in
        1)
            mkdir -p "${BACKUP_DIR}"
            local ts=$(date +%Y%m%d-%H%M%S)
            tar -czf "${BACKUP_DIR}/backup-${ts}.tar.gz" -C "${CONFIG_DIR}" --exclude=backups . 2>/dev/null
            msg_ok "Backup: ${BACKUP_DIR}/backup-${ts}.tar.gz"
            ;;
        2)
            echo ""; ls -lh ${BACKUP_DIR}/*.tar.gz 2>/dev/null || msg_warn "No backups"
            ;;
        3)
            msg_ask "Backup file path: "; read -r bf
            [ -f "$bf" ] && { tar -xzf "$bf" -C "${CONFIG_DIR}"; msg_ok "Restored"; } || msg_err "Not found"
            ;;
        4)
            (crontab -l 2>/dev/null | grep -v "ipshadowt"; echo "0 */12 * * * tar -czf ${BACKUP_DIR}/auto-\$(date +\%Y\%m\%d).tar.gz -C ${CONFIG_DIR} --exclude=backups . 2>/dev/null") | crontab -
            msg_ok "Auto-backup: every 12 hours"
            ;;
        0) return ;;
    esac
    press_enter
}

# ─── Multi-Tunnel ─────────────────────────────────
do_multi() {
    print_banner
    echo -e "  ${W}[ MULTI-TUNNEL MANAGER ]${N}"
    echo ""
    echo -e "  ${D}Run multiple tunnels to different servers simultaneously${N}"
    echo ""
    echo -e "  ${C}1)${N} List all tunnels"
    echo -e "  ${C}2)${N} Add new tunnel"
    echo -e "  ${C}3)${N} Remove tunnel"
    echo -e "  ${C}4)${N} Restart all tunnels"
    echo -e "  ${C}0)${N} Back"
    echo ""
    msg_ask "Choice: "; read -r c
    case $c in
        1) list_tunnels ;;
        2) add_tunnel ;;
        3) remove_tunnel ;;
        4) restart_all_tunnels ;;
        0) return ;;
    esac
    press_enter
}

list_tunnels() {
    echo ""
    print_line
    local found=0
    for conf in ${CONFIG_DIR}/tunnel-*.toml ${CONFIG_DIR}/config.toml; do
        [ -f "$conf" ] || continue
        found=$((found+1))
        local name=$(basename "$conf" .toml)
        local svc="${SERVICE_NAME}"
        [ "$name" != "config" ] && svc="${SERVICE_NAME}-${name#tunnel-}"
        local st="${R}OFF${N}"; systemctl is-active --quiet "$svc" 2>/dev/null && st="${G}ON ${N}"
        local tp=$(grep '^transport' "$conf" 2>/dev/null | cut -d'"' -f2)
        local addr=$(grep -E '^(remote_addr|bind_addr)' "$conf" 2>/dev/null | head -1 | cut -d'"' -f2)
        printf "  [${st}] %-15s %-12s %s\n" "$name" "$tp" "$addr"
    done
    [ $found -eq 0 ] && msg_warn "No tunnels configured"
    print_line
}

add_tunnel() {
    msg_ask "Tunnel name (e.g. server2): "; read -r tname
    [ -z "$tname" ] && return
    tname=$(echo "$tname" | tr -cd 'a-zA-Z0-9_-')

    msg_ask "Remote server IP: "; read -r ip
    msg_ask "Port [443]: "; read -r port; port=${port:-443}
    msg_ask "Password: "; read -r pass; [ -z "$pass" ] && pass=$(gen_pass)

    echo -e "  ${C}1)${N}reality ${C}2)${N}kcp ${C}3)${N}wsmux ${C}4)${N}tcpmux ${C}5)${N}shadowtls"
    msg_ask "Transport [1]: "; read -r tc
    local transport="reality"
    case $tc in 2) transport="kcp";; 3) transport="wsmux";; 4) transport="tcpmux";; 5) transport="shadowtls";; esac

    local count=$(ls ${CONFIG_DIR}/tunnel-*.toml 2>/dev/null | wc -l)
    local sp=$((1080 + count))
    msg_ask "SOCKS5 port [${sp}]: "; read -r usp; sp=${usp:-$sp}

    local cf="${CONFIG_DIR}/tunnel-${tname}.toml"
    cat > "$cf" << EOF
mode = "client"
transport = "${transport}"
remote_addr = "${ip}:${port}"
password = "${pass}"
[mux]
concurrency = 4
[anti_dpi]
enabled = true
utls_fingerprint = "chrome"
fragment = true
[heartbeat]
enabled = true
interval = 20
timeout = 40
[[forwards]]
name = "socks5-${tname}"
type = "socks5"
listen = "0.0.0.0:${sp}"
EOF

    local svc="${SERVICE_NAME}-${tname}"
    cat > "/etc/systemd/system/${svc}.service" << EOF
[Unit]
Description=iPShadowT - ${tname}
After=network.target
[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BINARY_NAME} -c ${cf}
Restart=always
RestartSec=5
LimitNOFILE=65535
[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable "$svc" >/dev/null 2>&1
    systemctl start "$svc"
    sleep 2
    systemctl is-active --quiet "$svc" && msg_ok "Tunnel '${tname}' active (SOCKS5 :${sp})" || msg_err "Failed"
}

remove_tunnel() {
    list_tunnels
    msg_ask "Tunnel name to remove: "; read -r tname
    [ -z "$tname" ] && return
    local svc="${SERVICE_NAME}-${tname}"
    systemctl stop "$svc" 2>/dev/null
    systemctl disable "$svc" 2>/dev/null
    rm -f "/etc/systemd/system/${svc}.service"
    rm -f "${CONFIG_DIR}/tunnel-${tname}.toml"
    systemctl daemon-reload
    msg_ok "Tunnel '${tname}' removed"
}

restart_all_tunnels() {
    for conf in ${CONFIG_DIR}/tunnel-*.toml ${CONFIG_DIR}/config.toml; do
        [ -f "$conf" ] || continue
        local name=$(basename "$conf" .toml)
        local svc="${SERVICE_NAME}"
        [ "$name" != "config" ] && svc="${SERVICE_NAME}-${name#tunnel-}"
        systemctl restart "$svc" 2>/dev/null && msg_ok "Restarted: $svc"
    done
}

# ─── Update ───────────────────────────────────────
do_update() {
    local cur=$(get_version)
    msg_info "Current: ${cur}"
    msg_info "Checking GitHub..."
    local latest=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4)
    [ -z "$latest" ] && { msg_err "Cannot reach GitHub"; return; }
    [ "$cur" = "$latest" ] && { msg_ok "Already up to date"; return; }
    msg_ok "New version: ${latest}"
    msg_ask "Update? [Y/n]: "; read -r ans
    if [[ "${ans:-y}" == "y" ]]; then
        systemctl stop ${SERVICE_NAME} 2>/dev/null || true
        detect_arch
        local url="https://github.com/${GITHUB_REPO}/releases/latest/download/${BINARY_NAME}-${OS}-${ARCH}"
        curl -fSL -o "${INSTALL_DIR}/${BINARY_NAME}" "$url" && chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
        systemctl start ${SERVICE_NAME}
        msg_ok "Updated to ${latest}!"
    fi
}

# ─── Uninstall ────────────────────────────────────
do_uninstall() {
    echo ""
    msg_warn "This will completely remove iPShadowT."
    msg_ask "Continue? [y/N]: "; read -r ans
    [ "$ans" != "y" ] && return
    systemctl stop ${SERVICE_NAME} 2>/dev/null || true
    systemctl disable ${SERVICE_NAME} 2>/dev/null || true
    rm -f "${SERVICE_FILE}" "${INSTALL_DIR}/${BINARY_NAME}" "${SYSCTL_FILE}"
    # Remove multi-tunnel services
    for f in /etc/systemd/system/${SERVICE_NAME}-*.service; do
        [ -f "$f" ] && rm -f "$f"
    done
    systemctl daemon-reload
    msg_ask "Remove config? [y/N]: "; read -r rc
    [ "$rc" = "y" ] && rm -rf "${CONFIG_DIR}"
    msg_ok "iPShadowT removed"
}

# ─── Main Menu ────────────────────────────────────
main_menu() {
    while true; do
        print_banner

        # Quick status line
        if is_installed; then
            local st="${R}Stopped${N}"; is_running && st="${G}Running${N}"
            echo -e "  Status: [${st}]  Version: ${W}$(get_version)${N}"
        else
            echo -e "  Status: ${D}Not installed${N}"
        fi
        echo ""
        print_line
        echo ""
        echo -e "  ${C} 1)${N}  Install iPShadowT"
        echo -e "  ${C} 2)${N}  Configure Tunnel"
        echo -e "  ${C} 3)${N}  Start / Stop / Restart"
        echo -e "  ${C} 4)${N}  Status & Monitoring"
        echo -e "  ${C} 5)${N}  Key Management"
        echo -e "  ${C} 6)${N}  Backup / Restore"
        echo -e "  ${C} 7)${N}  Multi-Tunnel Manager"
        echo -e "  ${C} 8)${N}  Update"
        echo -e "  ${C} 9)${N}  Uninstall"
        echo -e "  ${C} 0)${N}  Exit"
        echo ""
        print_line
        echo ""
        msg_ask "Choice: "; read -r choice

        case $choice in
            1) do_install ;;
            2) do_configure ;;
            3) do_service ;;
            4) do_status ;;
            5) do_keys ;;
            6) do_backup ;;
            7) do_multi ;;
            8) do_update; press_enter ;;
            9) do_uninstall; press_enter ;;
            0) echo ""; msg_ok "Goodbye!"; echo ""; exit 0 ;;
            *) msg_err "Invalid option" ;;
        esac
    done
}

# ─── Entry ────────────────────────────────────────
check_root
detect_arch
detect_os_type
main_menu
