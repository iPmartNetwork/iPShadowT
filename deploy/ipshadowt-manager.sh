#!/bin/bash
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  iPShadowT Manager v2.0.0
#  Anti-DPI Multi-Transport Tunnel Engine
#  iPmart Network (Ali Hassanzadeh)
#  https://github.com/iPmartNetwork/iPShadowT
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# ─── Constants ────────────────────────────────────
VERSION="2.0.0"
GITHUB_REPO="iPmartNetwork/iPShadowT"
BINARY_NAME="ipshadowt"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/ipshadowt"
BACKUP_DIR="/etc/ipshadowt/backups"
SERVICE_NAME="ipshadowt"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
SYSCTL_FILE="/etc/sysctl.d/99-ipshadowt.conf"
LOG_FILE="/var/log/ipshadowt-manager.log"

# ─── Colors ───────────────────────────────────────
R='\033[0;31m'
G='\033[0;32m'
Y='\033[1;33m'
B='\033[0;34m'
M='\033[0;35m'
C='\033[0;36m'
W='\033[1;37m'
D='\033[0;90m'
N='\033[0m'
BOLD='\033[1m'

# ─── UI Helpers ───────────────────────────────────
print_line()  { echo -e "${B}─────────────────────────────────────────────────────────${N}"; }
print_dline() { echo -e "${B}═════════════════════════════════════════════════════════${N}"; }
msg_ok()      { echo -e "  ${G}✓${N} $1"; }
msg_err()     { echo -e "  ${R}✗${N} $1"; }
msg_warn()    { echo -e "  ${Y}⚠${N} $1"; }
msg_info()    { echo -e "  ${C}➤${N} $1"; }
msg_ask()     { echo -ne "  ${W}?${N} $1"; }
msg_step()    { echo -e "  ${M}●${N} $1"; }

print_banner() {
    clear
    echo ""
    print_dline
    echo -e "${C}   _ ____  ____  _               _          _____ ${N}"
    echo -e "${C}  (_)  _ \\/ ___|| |__   __ _  __| | _____  |_   _|${N}"
    echo -e "${C}  | | |_) \\___ \\| '_ \\ / _\` |/ _\` |/ _ \\ \\ /\\ / /| |  ${N}"
    echo -e "${C}  | |  __/ ___) | | | | (_| | (_| | (_) \\ V  V / | |  ${N}"
    echo -e "${C}  |_|_|   |____/|_| |_|\\__,_|\\__,_|\\___/ \\_/\\_/  |_|  ${N}"
    echo ""
    echo -e "  ${BOLD}Anti-DPI Multi-Transport Tunnel Engine${N}"
    echo -e "  ${D}iPmart Network • v${VERSION} • github.com/iPmartNetwork${N}"
    print_dline
    # Server info line
    local ip=$(curl -s4 --max-time 2 ifconfig.me 2>/dev/null || echo "N/A")
    local geo=$(curl -s --max-time 2 "http://ip-api.com/line/${ip}?fields=country,city" 2>/dev/null)
    local country=$(echo "$geo" | sed -n '1p')
    local city=$(echo "$geo" | sed -n '2p')
    echo -e "  ${D}IP: ${W}${ip}${D}  •  ${city}, ${country}${N}"
    print_dline
    echo ""
}

press_enter() {
    echo ""
    msg_ask "Press Enter to continue..."; read -r
}

# ─── System Checks ────────────────────────────────
check_root() {
    if [ "$EUID" -ne 0 ]; then
        echo -e "${R}Error: Run as root (sudo bash $0)${N}"
        exit 1
    fi
}

detect_arch() {
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)  ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        armv7l)  ARCH="armv7" ;;
        armv6l)  ARCH="armv6" ;;
        mips)    ARCH="mips" ;;
        mipsel)  ARCH="mipsle" ;;
        *) msg_err "Unsupported architecture: $ARCH"; exit 1 ;;
    esac
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
}

is_installed() { [ -f "${INSTALL_DIR}/${BINARY_NAME}" ]; }
is_running()   { systemctl is-active --quiet ${SERVICE_NAME} 2>/dev/null; }
get_version()  { is_installed && (${INSTALL_DIR}/${BINARY_NAME} -v 2>/dev/null | head -1 | awk '{print $NF}' || echo "?") || echo "N/A"; }
gen_pass()     { tr -dc 'a-zA-Z0-9' </dev/urandom | fold -w 32 | head -n 1; }

validate_addr() {
    local addr=$1
    [[ $addr =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]] && return 0
    [[ $addr =~ ^[a-zA-Z0-9]([a-zA-Z0-9\.\-]*[a-zA-Z0-9])?$ ]] && return 0
    return 1
}

count_tunnels() {
    local count=0
    for f in ${CONFIG_DIR}/config.toml ${CONFIG_DIR}/tunnel-*.toml; do
        [ -f "$f" ] && count=$((count+1))
    done
    echo $count
}

count_running() {
    local count=0
    for svc in $(systemctl list-units --type=service --all 2>/dev/null | grep ipshadowt | awk '{print $1}'); do
        systemctl is-active --quiet "$svc" 2>/dev/null && count=$((count+1))
    done
    echo $count
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  INSTALL
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
do_install() {
    print_banner
    echo -e "  ${W}[ INSTALL iPShadowT ]${N}"
    echo ""

    if is_installed; then
        msg_warn "Already installed ($(get_version))"
        msg_ask "Reinstall? [y/N]: "; read -r ans
        [[ "$ans" != "y" ]] && return
    fi

    detect_arch
    msg_step "Platform: ${OS}/${ARCH}"

    # Prerequisites
    msg_step "Installing prerequisites..."
    apt-get update -qq >/dev/null 2>&1 || yum update -q -y >/dev/null 2>&1 || true
    for pkg in curl wget jq tar openssl; do
        command -v $pkg &>/dev/null || {
            apt-get install -y -qq $pkg >/dev/null 2>&1 || yum install -y -q $pkg >/dev/null 2>&1 || true
        }
    done
    msg_ok "Prerequisites ready"

    # Download
    msg_step "Downloading iPShadowT..."
    local url="https://github.com/${GITHUB_REPO}/releases/latest/download/${BINARY_NAME}-${OS}-${ARCH}"
    local tmp="/tmp/${BINARY_NAME}-download"

    if curl -fSL --progress-bar -o "$tmp" "$url" 2>/dev/null; then
        chmod +x "$tmp"
        mv "$tmp" "${INSTALL_DIR}/${BINARY_NAME}"
        msg_ok "Binary installed: ${INSTALL_DIR}/${BINARY_NAME}"
    elif [ -f "./${BINARY_NAME}" ]; then
        cp "./${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
        msg_ok "Binary installed (local copy)"
    else
        msg_err "Download failed. Check network or place binary in current dir."
        return 1
    fi

    # Directories
    mkdir -p "${CONFIG_DIR}" "${BACKUP_DIR}"

    # Systemd service
    cat > "${SERVICE_FILE}" << 'SVCEOF'
[Unit]
Description=iPShadowT Anti-DPI Tunnel
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/ipshadowt -c /etc/ipshadowt/config.toml
Restart=always
RestartSec=3
LimitNOFILE=1048576
LimitNPROC=infinity
TasksMax=infinity
WatchdogSec=60

[Install]
WantedBy=multi-user.target
SVCEOF
    systemctl daemon-reload
    systemctl enable ${SERVICE_NAME} >/dev/null 2>&1
    msg_ok "Systemd service created"

    # Kernel tuning
    cat > "${SYSCTL_FILE}" << 'EOF'
# iPShadowT kernel tuning
net.ipv4.ip_forward = 1
net.ipv4.tcp_fastopen = 3
net.ipv4.tcp_slow_start_after_idle = 0
net.ipv4.tcp_congestion_control = bbr
net.core.default_qdisc = fq
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.tcp_rmem = 4096 524288 16777216
net.ipv4.tcp_wmem = 4096 524288 16777216
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 15
EOF
    sysctl -p "${SYSCTL_FILE}" >/dev/null 2>&1 || true
    msg_ok "Kernel optimized (BBR + fast open)"

    # Firewall
    if command -v ufw &>/dev/null; then
        ufw allow 443/tcp >/dev/null 2>&1
        ufw allow 443/udp >/dev/null 2>&1
        msg_ok "Firewall: port 443 opened"
    fi

    echo ""
    print_line
    msg_ok "Installation complete! (v$(get_version))"
    print_line
    echo ""
    msg_ask "Configure tunnel now? [Y/n]: "; read -r ans
    [[ "${ans:-y}" =~ ^[Yy]$ ]] && do_configure
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  CONFIGURE
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
do_configure() {
    print_banner
    echo -e "  ${W}[ CONFIGURE ]${N}"
    echo ""
    echo -e "  ${C}1)${N} Setup as ${W}Iran Server${N}     ${D}(Client - connects to foreign)${N}"
    echo -e "  ${C}2)${N} Setup as ${W}Foreign Server${N}  ${D}(Server - accepts connections)${N}"
    echo -e "  ${C}3)${N} Edit config manually"
    echo -e "  ${C}4)${N} Show current config"
    echo -e "  ${C}5)${N} Test transport to server"
    echo -e "  ${C}6)${N} Add port forward"
    echo -e "  ${C}7)${N} Generate REALITY keys"
    echo -e "  ${C}8)${N} Generate random password"
    echo -e "  ${C}9)${N} Export client config"
    echo ""
    echo -e "  ${C}0)${N} Back"
    echo ""
    msg_ask "Choice: "; read -r choice

    case $choice in
        1) setup_client ;;
        2) setup_server ;;
        3) nano "${CONFIG_DIR}/config.toml" 2>/dev/null || vi "${CONFIG_DIR}/config.toml" ;;
        4) echo ""; [ -f "${CONFIG_DIR}/config.toml" ] && cat "${CONFIG_DIR}/config.toml" || msg_err "No config found" ;;
        5) msg_ask "Server IP/domain: "; read -r ip; test_transport "$ip" ;;
        6) add_port_forward ;;
        7) is_installed && ${INSTALL_DIR}/${BINARY_NAME} --gen-reality-keys || msg_err "Not installed" ;;
        8) echo ""; msg_ok "Password: $(gen_pass)" ;;
        9) export_config ;;
        0) return ;;
        *) msg_err "Invalid option" ;;
    esac
    press_enter
}

# ─── Client Setup (Iran) ─────────────────────────
setup_client() {
    echo ""
    print_line
    echo -e "  ${W}CLIENT SETUP (Iran → Foreign)${N}"
    print_line
    echo ""

    # Remote address
    msg_ask "Foreign server IP/domain: "; read -r remote_addr
    validate_addr "$remote_addr" || { msg_err "Invalid address"; return; }

    msg_ask "Port [443]: "; read -r port
    port=${port:-443}

    # Password
    local password=$(gen_pass)
    msg_ask "Password [auto]: "; read -r user_pass
    [ -n "$user_pass" ] && password="$user_pass"

    # Transport selection
    echo ""
    echo -e "  ${W}Transport:${N}"
    echo -e "    ${C}1)${N} reality      ${D}— recommended, max stealth${N}"
    echo -e "    ${C}2)${N} shadowtls    ${D}— no cert needed, good stealth${N}"
    echo -e "    ${C}3)${N} wsmux        ${D}— WebSocket, CDN compatible${N}"
    echo -e "    ${C}4)${N} h2mux        ${D}— HTTP/2, looks like browsing${N}"
    echo -e "    ${C}5)${N} grpc         ${D}— gRPC, looks like API traffic${N}"
    echo -e "    ${C}6)${N} tcpmux       ${D}— simple TCP, fastest${N}"
    echo -e "    ${C}7)${N} kcp          ${D}— UDP, works when TCP blocked${N}"
    echo -e "    ${C}8)${N} quic         ${D}— QUIC/UDP, 0-RTT fast${N}"
    echo -e "    ${C}9)${N} cdn          ${D}— via Cloudflare CDN (IP hidden)${N}"
    echo ""
    msg_ask "Choice [1]: "; read -r tc
    local transport="reality"
    case $tc in
        2) transport="shadowtls" ;;
        3) transport="wsmux" ;;
        4) transport="h2mux" ;;
        5) transport="grpc" ;;
        6) transport="tcpmux" ;;
        7) transport="kcp" ;;
        8) transport="quic" ;;
        9) transport="wsmux" ;;
    esac

    # CDN mode
    local cdn_section=""
    if [ "$tc" = "9" ]; then
        echo ""
        echo -e "  ${W}CDN Setup:${N}"
        msg_ask "CDN domain (e.g. your-domain.com): "; read -r cdn_domain
        [ -z "$cdn_domain" ] && { msg_err "Domain required for CDN mode"; return; }
        remote_addr="$cdn_domain"
        port="443"
        echo -e "    ${C}1)${N} Cloudflare  ${C}2)${N} Gcore  ${C}3)${N} Arvan  ${C}4)${N} Custom"
        msg_ask "Provider [1]: "; read -r cp
        local cdn_provider="cloudflare"
        case $cp in 2) cdn_provider="gcore";; 3) cdn_provider="arvan";; 4) cdn_provider="custom";; esac
        cdn_section="
[cdn]
enabled = true
provider = \"${cdn_provider}\"
domain = \"${cdn_domain}\"
path = \"/tunnel\"
tls = true
early_data = true"
    fi

    # TLS for tcpmux
    local tls_section=""
    if [ "$transport" = "tcpmux" ] && [ "$tc" != "9" ]; then
        echo ""
        msg_ask "Enable TLS for tcpmux? [y/N]: "; read -r tls_ans
        if [[ "$tls_ans" =~ ^[Yy]$ ]]; then
            msg_ask "Cert path [/etc/ipshadowt/cert.pem]: "; read -r cert_path
            msg_ask "Key path [/etc/ipshadowt/key.pem]: "; read -r key_path
            tls_section="tls_cert = \"${cert_path:-/etc/ipshadowt/cert.pem}\"
tls_key = \"${key_path:-/etc/ipshadowt/key.pem}\""
        fi
    fi

    # REALITY config
    local reality_section=""
    if [ "$transport" = "reality" ]; then
        echo ""
        echo -e "  ${W}REALITY Settings:${N}"
        msg_ask "SNI (e.g. www.google.com) [www.google.com]: "; read -r sni
        sni=${sni:-www.google.com}
        msg_ask "Public key (from server --gen-reality-keys): "; read -r pub_key
        msg_ask "Short ID (from server): "; read -r short_id
        reality_section="
[reality]
server_name = \"${sni}\"
public_key = \"${pub_key}\"
short_id = \"${short_id}\""
    fi

    # SOCKS5 port
    msg_ask "SOCKS5 listen port [1080]: "; read -r socks_port
    socks_port=${socks_port:-1080}

    # Health check
    echo ""
    msg_ask "Enable health watchdog? [Y/n]: "; read -r wd
    local health_section=""
    local health_port=9090
    if [[ ! "$wd" =~ ^[Nn]$ ]]; then
        msg_ask "Health port [9090]: "; read -r hp
        health_port=${hp:-9090}
        health_section="
[health]
enabled = true
listen = \"127.0.0.1:${health_port}\""
    fi

    # Write config
    mkdir -p "${CONFIG_DIR}"
    cat > "${CONFIG_DIR}/config.toml" << EOF
# iPShadowT Client Config — Generated by Manager v${VERSION}
mode = "client"
log_level = "info"
transport = "${transport}"
remote_addr = "${remote_addr}:${port}"
password = "${password}"
${tls_section}

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
padding_size = "16-256"
${cdn_section}
${health_section}
${reality_section}

[[forwards]]
name = "socks5"
type = "socks5"
listen = "0.0.0.0:${socks_port}"
EOF

    # Update systemd with watchdog
    if [ -n "$health_section" ]; then
        sed -i "s|WatchdogSec=.*|WatchdogSec=60|" "${SERVICE_FILE}" 2>/dev/null
        systemctl daemon-reload
    fi

    # Summary
    echo ""
    print_line
    echo -e "  ${G}✓ CONFIG SAVED${N}"
    print_line
    echo -e "  Remote:     ${W}${remote_addr}:${port}${N}"
    echo -e "  Transport:  ${W}${transport}${N}"
    echo -e "  Password:   ${W}${password}${N}"
    echo -e "  SOCKS5:     ${W}0.0.0.0:${socks_port}${N}"
    [ -n "$cdn_section" ] && echo -e "  CDN:        ${W}${cdn_provider} (${cdn_domain:-})${N}"
    [ -n "$tls_section" ] && echo -e "  TLS:        ${W}Enabled${N}"
    [ -n "$health_section" ] && echo -e "  Health:     ${W}127.0.0.1:${health_port}${N}"
    print_line
    echo ""

    msg_ask "Start service now? [Y/n]: "; read -r ans
    if [[ "${ans:-y}" =~ ^[Yy]$ ]]; then
        systemctl restart ${SERVICE_NAME} && sleep 2
        is_running && msg_ok "Service running!" || msg_err "Failed — journalctl -u ${SERVICE_NAME} -n 10"
    fi
}

# ─── Server Setup (Foreign) ──────────────────────
setup_server() {
    echo ""
    print_line
    echo -e "  ${W}SERVER SETUP (Foreign — accepts Iran connections)${N}"
    print_line
    echo ""

    msg_ask "Listen port [443]: "; read -r port
    port=${port:-443}

    local password=$(gen_pass)
    msg_ask "Password [auto]: "; read -r user_pass
    [ -n "$user_pass" ] && password="$user_pass"

    # Transport
    echo ""
    echo -e "  ${W}Transport:${N}"
    echo -e "    ${C}1)${N} reality      ${D}— recommended${N}"
    echo -e "    ${C}2)${N} shadowtls    ${D}— no cert${N}"
    echo -e "    ${C}3)${N} wsmux        ${D}— CDN ready${N}"
    echo -e "    ${C}4)${N} h2mux        ${D}— HTTP/2${N}"
    echo -e "    ${C}5)${N} grpc         ${D}— gRPC${N}"
    echo -e "    ${C}6)${N} tcpmux       ${D}— simple${N}"
    echo -e "    ${C}7)${N} kcp          ${D}— UDP${N}"
    echo -e "    ${C}8)${N} quic         ${D}— QUIC/UDP${N}"
    echo ""
    msg_ask "Choice [1]: "; read -r tc
    local transport="reality"
    case $tc in
        2) transport="shadowtls" ;;
        3) transport="wsmux" ;;
        4) transport="h2mux" ;;
        5) transport="grpc" ;;
        6) transport="tcpmux" ;;
        7) transport="kcp" ;;
        8) transport="quic" ;;
    esac

    # TLS for tcpmux
    local tls_section=""
    if [ "$transport" = "tcpmux" ]; then
        msg_ask "Enable TLS? [y/N]: "; read -r tls_ans
        if [[ "$tls_ans" =~ ^[Yy]$ ]]; then
            msg_ask "Cert [/etc/ipshadowt/cert.pem]: "; read -r cp
            msg_ask "Key [/etc/ipshadowt/key.pem]: "; read -r kp
            tls_section="tls_cert = \"${cp:-/etc/ipshadowt/cert.pem}\"
tls_key = \"${kp:-/etc/ipshadowt/key.pem}\""
        fi
    fi

    # REALITY for server
    local reality_section=""
    if [ "$transport" = "reality" ]; then
        echo ""
        echo -e "  ${W}REALITY Settings:${N}"
        msg_ask "SNI to mimic [www.google.com]: "; read -r sni
        sni=${sni:-www.google.com}
        msg_ask "Fallback dest [www.google.com:443]: "; read -r dest
        dest=${dest:-www.google.com:443}
        # Generate keys if binary available
        if is_installed; then
            msg_info "Generating REALITY keys..."
            local keys=$(${INSTALL_DIR}/${BINARY_NAME} --gen-reality-keys 2>/dev/null)
            local priv_key=$(echo "$keys" | grep -i "private" | awk '{print $NF}')
            local pub_key=$(echo "$keys" | grep -i "public" | awk '{print $NF}')
            local short_id=$(echo "$keys" | grep -i "short" | awk '{print $NF}')
            [ -z "$short_id" ] && short_id=$(openssl rand -hex 4)
            [ -z "$priv_key" ] && { msg_ask "Private key: "; read -r priv_key; }
            [ -z "$pub_key" ] && { msg_ask "Public key: "; read -r pub_key; }
        else
            msg_ask "Private key: "; read -r priv_key
            msg_ask "Public key: "; read -r pub_key
            short_id=$(openssl rand -hex 4 2>/dev/null || echo "abcd1234")
        fi
        reality_section="
[reality]
server_name = \"${sni}\"
private_key = \"${priv_key}\"
short_id = \"${short_id}\"
dest = \"${dest}\""
        echo ""
        echo -e "  ${W}Give these to client:${N}"
        echo -e "    Public Key: ${G}${pub_key}${N}"
        echo -e "    Short ID:   ${G}${short_id}${N}"
    fi

    # Write config
    mkdir -p "${CONFIG_DIR}"
    cat > "${CONFIG_DIR}/config.toml" << EOF
# iPShadowT Server Config — Generated by Manager v${VERSION}
mode = "server"
log_level = "info"
transport = "${transport}"
bind_addr = "0.0.0.0:${port}"
password = "${password}"
${tls_section}

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

[health]
enabled = true
listen = "127.0.0.1:9090"
${reality_section}
EOF

    local server_ip=$(curl -s4 --max-time 5 ifconfig.me 2>/dev/null || echo "YOUR_IP")

    echo ""
    print_line
    echo -e "  ${G}✓ SERVER READY${N}"
    print_line
    echo -e ""
    echo -e "  ${W}Share with Iran client:${N}"
    echo -e "  ┌────────────────────────────────────────┐"
    echo -e "  │  IP:        ${G}${server_ip}${N}"
    echo -e "  │  Port:      ${G}${port}${N}"
    echo -e "  │  Transport: ${G}${transport}${N}"
    echo -e "  │  Password:  ${G}${password}${N}"
    [ -n "$tls_section" ] && echo -e "  │  TLS:       ${G}Enabled${N}"
    echo -e "  └────────────────────────────────────────┘"
    echo ""

    msg_ask "Start service now? [Y/n]: "; read -r ans
    if [[ "${ans:-y}" =~ ^[Yy]$ ]]; then
        systemctl restart ${SERVICE_NAME} && sleep 2
        is_running && msg_ok "Service running!" || msg_err "Failed — journalctl -u ${SERVICE_NAME} -n 10"
    fi
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  EXPORT CONFIG
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
export_config() {
    [ ! -f "${CONFIG_DIR}/config.toml" ] && { msg_err "No config found"; return; }
    local mode=$(grep '^mode' ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)

    if [ "$mode" = "server" ]; then
        # Export client config for Iran
        local server_ip=$(curl -s4 --max-time 3 ifconfig.me 2>/dev/null || echo "YOUR_IP")
        local port=$(grep '^bind_addr' ${CONFIG_DIR}/config.toml 2>/dev/null | grep -oP ':\K[0-9]+')
        local password=$(grep '^password' ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)
        local transport=$(grep '^transport' ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)

        echo ""
        print_line
        echo -e "  ${W}CLIENT CONFIG (copy to Iran server):${N}"
        print_line
        echo ""
        echo -e "${G}# iPShadowT Client — copy-paste ready${N}"
        echo "mode = \"client\""
        echo "transport = \"${transport}\""
        echo "remote_addr = \"${server_ip}:${port}\""
        echo "password = \"${password}\""
        echo ""
        echo "[mux]"
        echo "concurrency = 4"
        echo ""
        echo "[heartbeat]"
        echo "enabled = true"
        echo "interval = 20"
        echo "timeout = 40"
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

        # Show REALITY keys if applicable
        if [ "$transport" = "reality" ]; then
            local pub_key=$(grep 'public_key\|PublicKey' ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)
            local short_id=$(grep 'short_id' ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)
            local sni=$(grep 'server_name' ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)
            echo ""
            echo "[reality]"
            echo "server_name = \"${sni:-www.google.com}\""
            echo "public_key = \"${pub_key}\""
            echo "short_id = \"${short_id}\""
        fi
        print_line
    else
        echo ""
        msg_info "This is a client config. Export is for servers."
        echo ""
        cat "${CONFIG_DIR}/config.toml"
    fi
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  TRANSPORT TEST
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
test_transport() {
    local ip=$1
    [ -z "$ip" ] && { msg_err "No IP provided"; return; }
    echo ""
    msg_info "Probing ${ip}..."
    echo ""

    local tcp_ok=false udp_ok=false tls_ok=false h2_ok=false
    local latency="N/A"

    # TCP
    if timeout 5 bash -c "echo >/dev/tcp/${ip}/443" 2>/dev/null; then
        msg_ok "TCP/443: Open"; tcp_ok=true
    else
        msg_err "TCP/443: Blocked"
    fi

    # UDP
    if timeout 3 bash -c "echo >/dev/udp/${ip}/443" 2>/dev/null; then
        msg_ok "UDP/443: Open"; udp_ok=true
    else
        msg_warn "UDP/443: Blocked"
    fi

    # TLS
    if command -v openssl &>/dev/null; then
        if timeout 5 openssl s_client -connect "${ip}:443" </dev/null 2>/dev/null | grep -q "CONNECTED"; then
            msg_ok "TLS: Handshake OK"; tls_ok=true
        else
            msg_warn "TLS: Handshake failed (DPI active?)"
        fi
    fi

    # HTTP/2
    if command -v curl &>/dev/null; then
        if curl -sf --max-time 5 --http2 -o /dev/null "https://${ip}:443" 2>/dev/null; then
            msg_ok "HTTP/2: Supported"; h2_ok=true
        fi
    fi

    # Latency
    if command -v ping &>/dev/null; then
        latency=$(ping -c 3 -W 3 "$ip" 2>/dev/null | tail -1 | awk -F'/' '{print $5}')
        [ -n "$latency" ] && msg_info "Latency: ${latency}ms"
    fi

    # Recommendation
    echo ""
    print_line
    echo -e "  ${W}Recommended transport:${N}"
    echo ""
    if [ "$tcp_ok" = "true" ] && [ "$tls_ok" = "true" ]; then
        echo -e "    ${G}1.${N} reality      ${D}(best — TLS works)${N}"
        echo -e "    ${G}2.${N} shadowtls    ${D}(fallback)${N}"
        [ "$h2_ok" = "true" ] && echo -e "    ${G}3.${N} h2mux        ${D}(HTTP/2 OK)${N}"
    elif [ "$tcp_ok" = "true" ]; then
        echo -e "    ${G}1.${N} shadowtls    ${D}(TLS blocked)${N}"
        echo -e "    ${G}2.${N} tcpmux       ${D}(raw TCP)${N}"
        echo -e "    ${Y}3.${N} cdn (wsmux)  ${D}(if direct fails)${N}"
    elif [ "$udp_ok" = "true" ]; then
        echo -e "    ${G}1.${N} kcp          ${D}(UDP works)${N}"
        echo -e "    ${G}2.${N} quic         ${D}(fast UDP)${N}"
    else
        echo -e "    ${R}All direct paths blocked!${N}"
        echo -e "    ${G}1.${N} cdn (wsmux)  ${D}(via Cloudflare)${N}"
    fi
    print_line
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  SERVICE CONTROL
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
do_service() {
    print_banner
    echo -e "  ${W}[ SERVICE CONTROL ]${N}"
    echo ""

    # List services
    echo -e "  ${W}Tunnels:${N}"
    local svcs=()
    for svc in $(systemctl list-units --type=service --all 2>/dev/null | grep ipshadowt | awk '{print $1}'); do
        local st="${R}●${N}"; systemctl is-active --quiet "$svc" 2>/dev/null && st="${G}●${N}"
        echo -e "    ${st} ${svc}"
        svcs+=("$svc")
    done
    [ ${#svcs[@]} -eq 0 ] && msg_warn "No services found"
    echo ""

    echo -e "  ${C}1)${N} Start all"
    echo -e "  ${C}2)${N} Stop all"
    echo -e "  ${C}3)${N} Restart all"
    echo -e "  ${C}4)${N} Start specific"
    echo -e "  ${C}5)${N} Stop specific"
    echo -e "  ${C}6)${N} Restart specific"
    echo -e "  ${C}7)${N} Enable all (boot)"
    echo -e "  ${C}8)${N} Disable all"
    echo ""
    echo -e "  ${C}0)${N} Back"
    echo ""
    msg_ask "Choice: "; read -r c
    case $c in
        1) for s in "${svcs[@]}"; do systemctl start "$s" 2>/dev/null && msg_ok "Started: $s"; done ;;
        2) for s in "${svcs[@]}"; do systemctl stop "$s" 2>/dev/null && msg_ok "Stopped: $s"; done ;;
        3) for s in "${svcs[@]}"; do systemctl restart "$s" 2>/dev/null && msg_ok "Restarted: $s"; done ;;
        4) msg_ask "Service name: "; read -r sn; systemctl start "$sn" && msg_ok "Started" || msg_err "Failed" ;;
        5) msg_ask "Service name: "; read -r sn; systemctl stop "$sn" && msg_ok "Stopped" || msg_err "Failed" ;;
        6) msg_ask "Service name: "; read -r sn; systemctl restart "$sn" && msg_ok "Restarted" || msg_err "Failed" ;;
        7) for s in "${svcs[@]}"; do systemctl enable "$s" >/dev/null 2>&1 && msg_ok "Enabled: $s"; done ;;
        8) for s in "${svcs[@]}"; do systemctl disable "$s" >/dev/null 2>&1 && msg_ok "Disabled: $s"; done ;;
        0) return ;;
    esac
    press_enter
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  STATUS & MONITORING
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
do_status() {
    print_banner
    echo -e "  ${W}[ STATUS & MONITORING ]${N}"
    echo ""

    if ! is_installed; then
        msg_err "iPShadowT is not installed"
        press_enter; return
    fi

    echo -e "  Version: ${W}$(get_version)${N}"
    echo ""

    # Tunnel list
    print_line
    echo -e "  ${W}TUNNELS${N}"
    echo ""
    local t_count=0 r_count=0
    printf "  ${D}%-4s %-12s %-8s %-10s %-26s %s${N}\n" "ST" "NAME" "MODE" "TRANSPORT" "ADDRESS" "FLAGS"
    echo -e "  ${D}──── ──────────── ──────── ────────── ────────────────────────── ─────${N}"

    for conf in ${CONFIG_DIR}/config.toml ${CONFIG_DIR}/tunnel-*.toml; do
        [ -f "$conf" ] || continue
        t_count=$((t_count+1))
        local name=$(basename "$conf" .toml)
        local svc="${SERVICE_NAME}"
        [ "$name" != "config" ] && svc="${SERVICE_NAME}-${name#tunnel-}"

        local st="${R}OFF${N}"
        if systemctl is-active --quiet "$svc" 2>/dev/null; then
            st="${G} ON${N}"; r_count=$((r_count+1))
        fi

        local mode=$(grep '^mode' "$conf" 2>/dev/null | cut -d'"' -f2)
        local tp=$(grep '^transport' "$conf" 2>/dev/null | cut -d'"' -f2)
        local addr=$(grep -E '^(remote_addr|bind_addr)' "$conf" 2>/dev/null | head -1 | cut -d'"' -f2)

        local flags=""
        grep -q '^\[cdn\]' "$conf" 2>/dev/null && flags="${flags}CDN "
        grep -q '^tls_cert' "$conf" 2>/dev/null && flags="${flags}TLS "
        grep -q '^\[health\]' "$conf" 2>/dev/null && flags="${flags}WD "

        printf "  [${st}] %-12s %-8s %-10s %-26s ${C}%s${N}\n" "$name" "$mode" "$tp" "$addr" "$flags"
    done

    [ $t_count -eq 0 ] && msg_warn "No tunnels configured"
    echo ""
    echo -e "  Total: ${W}${t_count}${N}  Running: ${G}${r_count}${N}  Stopped: ${R}$((t_count-r_count))${N}"
    print_line

    # System info
    echo ""
    local pub_ip=$(curl -s4 --max-time 3 ifconfig.me 2>/dev/null || echo "N/A")
    local mem=$(free -m 2>/dev/null | awk '/Mem:/{printf "%d/%dMB (%d%%)", $3, $2, $3*100/$2}')
    local cpu=$(nproc 2>/dev/null || echo "?")
    local load=$(cat /proc/loadavg 2>/dev/null | awk '{print $1}')
    local disk=$(df -h / 2>/dev/null | awk 'NR==2{print $3"/"$2" ("$5")"}')

    echo -e "  ${D}System:${N}"
    echo -e "    IP:     ${W}${pub_ip}${N}"
    echo -e "    Memory: ${mem}"
    echo -e "    CPU:    ${cpu} cores (load: ${load})"
    echo -e "    Disk:   ${disk}"
    echo ""

    echo -e "  ${C}1)${N} View logs (last 30 lines)"
    echo -e "  ${C}2)${N} View logs (specific tunnel)"
    echo -e "  ${C}3)${N} Live logs (follow)"
    echo -e "  ${C}4)${N} Health check all"
    echo -e "  ${C}5)${N} Speed test"
    echo ""
    echo -e "  ${C}0)${N} Back"
    echo ""
    msg_ask "Choice: "; read -r c
    case $c in
        1) echo ""; journalctl -u ${SERVICE_NAME} --no-pager -n 30 ;;
        2) msg_ask "Tunnel name: "; read -r tn
           local sn="${SERVICE_NAME}"; [ -n "$tn" ] && [ "$tn" != "config" ] && sn="${SERVICE_NAME}-${tn}"
           journalctl -u "$sn" --no-pager -n 30 ;;
        3) msg_ask "Tunnel (empty=main): "; read -r tn
           local sn="${SERVICE_NAME}"; [ -n "$tn" ] && [ "$tn" != "config" ] && sn="${SERVICE_NAME}-${tn}"
           journalctl -u "$sn" -f ;;
        4) echo ""
           for conf in ${CONFIG_DIR}/config.toml ${CONFIG_DIR}/tunnel-*.toml; do
               [ -f "$conf" ] || continue
               local name=$(basename "$conf" .toml)
               local svc="${SERVICE_NAME}"; [ "$name" != "config" ] && svc="${SERVICE_NAME}-${name#tunnel-}"
               if systemctl is-active --quiet "$svc" 2>/dev/null; then
                   msg_ok "${name}: running"
               else
                   msg_err "${name}: stopped"
               fi
           done ;;
        5) msg_info "Testing download speed..."
           local spd=$(curl -so /dev/null -w '%{speed_download}' http://speedtest.tele2.net/1MB.zip 2>/dev/null)
           echo -e "  Download: $(echo $spd | awk '{printf "%.2f Mbps", $1/131072}')" ;;
        0) return ;;
    esac
    press_enter
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  MULTI-TUNNEL
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
do_multi() {
    print_banner
    echo -e "  ${W}[ MULTI-TUNNEL MANAGER ]${N}"
    echo -e "  ${D}Manage multiple tunnels to different servers${N}"
    echo ""
    echo -e "  ${C}1)${N} List all tunnels"
    echo -e "  ${C}2)${N} Add client tunnel  ${D}(Iran → Foreign)${N}"
    echo -e "  ${C}3)${N} Add server tunnel  ${D}(Foreign ← Iran)${N}"
    echo -e "  ${C}4)${N} Remove tunnel"
    echo -e "  ${C}5)${N} Restart all tunnels"
    echo -e "  ${C}6)${N} Stop all tunnels"
    echo ""
    echo -e "  ${C}0)${N} Back"
    echo ""
    msg_ask "Choice: "; read -r c
    case $c in
        1) list_tunnels ;;
        2) add_client_tunnel ;;
        3) add_server_tunnel ;;
        4) remove_tunnel ;;
        5) for svc in $(systemctl list-units --type=service --all 2>/dev/null | grep ipshadowt | awk '{print $1}'); do
               systemctl restart "$svc" 2>/dev/null && msg_ok "Restarted: $svc"
           done ;;
        6) for svc in $(systemctl list-units --type=service --all 2>/dev/null | grep ipshadowt | awk '{print $1}'); do
               systemctl stop "$svc" 2>/dev/null && msg_ok "Stopped: $svc"
           done ;;
        0) return ;;
    esac
    press_enter
}

list_tunnels() {
    echo ""
    printf "  ${D}%-4s %-15s %-12s %s${N}\n" "ST" "NAME" "TRANSPORT" "ADDRESS"
    echo -e "  ${D}──── ─────────────── ──────────── ──────────────────────────────${N}"
    for conf in ${CONFIG_DIR}/tunnel-*.toml ${CONFIG_DIR}/config.toml; do
        [ -f "$conf" ] || continue
        local name=$(basename "$conf" .toml)
        local svc="${SERVICE_NAME}"; [ "$name" != "config" ] && svc="${SERVICE_NAME}-${name#tunnel-}"
        local st="${R}OFF${N}"; systemctl is-active --quiet "$svc" 2>/dev/null && st="${G} ON${N}"
        local tp=$(grep '^transport' "$conf" 2>/dev/null | cut -d'"' -f2)
        local addr=$(grep -E '^(remote_addr|bind_addr)' "$conf" 2>/dev/null | head -1 | cut -d'"' -f2)
        printf "  [${st}] %-15s %-12s %s\n" "$name" "$tp" "$addr"
    done
}

add_client_tunnel() {
    echo ""
    msg_ask "Tunnel name (e.g. DE, TR, US): "; read -r tname
    [ -z "$tname" ] && return
    tname=$(echo "$tname" | tr -cd 'a-zA-Z0-9_-')

    msg_ask "Foreign server IP: "; read -r ip; [ -z "$ip" ] && return
    msg_ask "Port [443]: "; read -r port; port=${port:-443}
    msg_ask "Password [auto]: "; read -r pass; [ -z "$pass" ] && pass=$(gen_pass)

    echo ""
    echo -e "  ${C}1)${N}reality ${C}2)${N}shadowtls ${C}3)${N}wsmux ${C}4)${N}h2mux ${C}5)${N}grpc ${C}6)${N}tcpmux ${C}7)${N}kcp ${C}8)${N}quic ${C}9)${N}cdn"
    msg_ask "Transport [1]: "; read -r tc
    local transport="reality"
    case $tc in 2) transport="shadowtls";; 3) transport="wsmux";; 4) transport="h2mux";; 5) transport="grpc";; 6) transport="tcpmux";; 7) transport="kcp";; 8) transport="quic";; 9) transport="wsmux";; esac

    # CDN
    local cdn_section=""
    if [ "$tc" = "9" ]; then
        msg_ask "CDN domain: "; read -r cdn_domain
        [ -z "$cdn_domain" ] && { msg_err "Required"; return; }
        ip="$cdn_domain"; port="443"
        cdn_section="
[cdn]
enabled = true
provider = \"cloudflare\"
domain = \"${cdn_domain}\"
path = \"/tunnel\"
tls = true"
    fi

    # TLS for tcpmux
    local tls_section=""
    if [ "$transport" = "tcpmux" ] && [ "$tc" != "9" ]; then
        msg_ask "Enable TLS for tcpmux? [y/N]: "; read -r tls_ans
        if [[ "$tls_ans" =~ ^[Yy]$ ]]; then
            msg_ask "Cert [/etc/ipshadowt/cert.pem]: "; read -r tc_path
            msg_ask "Key [/etc/ipshadowt/key.pem]: "; read -r tk_path
            tls_section="tls_cert = \"${tc_path:-/etc/ipshadowt/cert.pem}\"
tls_key = \"${tk_path:-/etc/ipshadowt/key.pem}\""
        fi
    fi

    # REALITY for client
    local reality_section=""
    if [ "$transport" = "reality" ]; then
        echo ""
        msg_ask "SNI [www.google.com]: "; read -r sni; sni=${sni:-www.google.com}
        msg_ask "Public key (from server): "; read -r pub_key
        msg_ask "Short ID (from server): "; read -r short_id
        reality_section="
[reality]
server_name = \"${sni}\"
public_key = \"${pub_key}\"
short_id = \"${short_id}\""
    fi

    # SOCKS5 port auto-detect
    local sp=1080
    while ss -tuln 2>/dev/null | grep -q ":${sp} "; do sp=$((sp+1)); done
    msg_ask "SOCKS5 port [${sp}]: "; read -r usp; sp=${usp:-$sp}

    # Port forwards
    msg_ask "Extra port forwards (comma-sep, e.g. 443,8443) or empty: "; read -r ports_input
    local fwd_section=""
    if [ -n "$ports_input" ]; then
        IFS=',' read -ra PORTS <<< "$ports_input"
        for p in "${PORTS[@]}"; do
            p=$(echo "$p" | tr -d ' '); [ -z "$p" ] && continue
            fwd_section="${fwd_section}
[[forwards]]
name = \"fwd-${p}\"
type = \"tcp\"
listen = \"0.0.0.0:${p}\"
remote = \"${p}\"
"
        done
    fi

    local cf="${CONFIG_DIR}/tunnel-${tname}.toml"
    cat > "$cf" << EOF
# iPShadowT Client Tunnel — ${tname}
mode = "client"
log_level = "info"
transport = "${transport}"
remote_addr = "${ip}:${port}"
password = "${pass}"
${tls_section}

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
${cdn_section}
${reality_section}

[health]
enabled = true
listen = "127.0.0.1:0"

[[forwards]]
name = "socks5-${tname}"
type = "socks5"
listen = "0.0.0.0:${sp}"
${fwd_section}
EOF

    # Service
    local svc="${SERVICE_NAME}-${tname}"
    cat > "/etc/systemd/system/${svc}.service" << EOF
[Unit]
Description=iPShadowT — ${tname} (${ip}:${port})
After=network.target
[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BINARY_NAME} -c ${cf}
Restart=always
RestartSec=3
LimitNOFILE=1048576
WatchdogSec=60
[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable "$svc" >/dev/null 2>&1
    systemctl start "$svc"
    sleep 3
    systemctl is-active --quiet "$svc" && msg_ok "Tunnel '${tname}' active (SOCKS5 :${sp})" || msg_err "Failed — journalctl -u ${svc} -n 5"
}

add_server_tunnel() {
    echo ""
    msg_ask "Tunnel name (e.g. iran1, iran2): "; read -r tname
    [ -z "$tname" ] && return
    tname=$(echo "$tname" | tr -cd 'a-zA-Z0-9_-')

    msg_ask "Listen port: "; read -r port; [ -z "$port" ] && return
    msg_ask "Password [auto]: "; read -r pass; [ -z "$pass" ] && pass=$(gen_pass)

    echo -e "  ${C}1)${N}reality ${C}2)${N}shadowtls ${C}3)${N}wsmux ${C}4)${N}h2mux ${C}5)${N}grpc ${C}6)${N}tcpmux ${C}7)${N}kcp ${C}8)${N}quic"
    msg_ask "Transport [1]: "; read -r tc
    local transport="reality"
    case $tc in 2) transport="shadowtls";; 3) transport="wsmux";; 4) transport="h2mux";; 5) transport="grpc";; 6) transport="tcpmux";; 7) transport="kcp";; 8) transport="quic";; esac

    # TLS for tcpmux
    local tls_section=""
    if [ "$transport" = "tcpmux" ]; then
        msg_ask "Enable TLS? [y/N]: "; read -r tls_ans
        if [[ "$tls_ans" =~ ^[Yy]$ ]]; then
            msg_ask "Cert [/etc/ipshadowt/cert.pem]: "; read -r tc_path
            msg_ask "Key [/etc/ipshadowt/key.pem]: "; read -r tk_path
            tls_section="tls_cert = \"${tc_path:-/etc/ipshadowt/cert.pem}\"
tls_key = \"${tk_path:-/etc/ipshadowt/key.pem}\""
        fi
    fi

    # REALITY for server
    local reality_section=""
    if [ "$transport" = "reality" ]; then
        echo ""
        msg_ask "SNI [www.google.com]: "; read -r sni; sni=${sni:-www.google.com}
        msg_ask "Fallback dest [www.google.com:443]: "; read -r dest; dest=${dest:-www.google.com:443}
        if is_installed; then
            msg_info "Generating REALITY keys..."
            local keys=$(${INSTALL_DIR}/${BINARY_NAME} --gen-reality-keys 2>/dev/null)
            local priv_key=$(echo "$keys" | grep -i "private" | awk '{print $NF}')
            local pub_key=$(echo "$keys" | grep -i "public" | awk '{print $NF}')
            local short_id=$(echo "$keys" | grep -i "short" | awk '{print $NF}')
            [ -z "$short_id" ] && short_id=$(openssl rand -hex 4)
            [ -z "$priv_key" ] && { msg_ask "Private key: "; read -r priv_key; }
            [ -z "$pub_key" ] && { msg_ask "Public key: "; read -r pub_key; }
        else
            msg_ask "Private key: "; read -r priv_key
            msg_ask "Public key: "; read -r pub_key
            short_id=$(openssl rand -hex 4 2>/dev/null || echo "abcd1234")
        fi
        reality_section="
[reality]
server_name = \"${sni}\"
private_key = \"${priv_key}\"
short_id = \"${short_id}\"
dest = \"${dest}\""
    fi

    local cf="${CONFIG_DIR}/tunnel-${tname}.toml"
    cat > "$cf" << EOF
# iPShadowT Server Tunnel — ${tname}
mode = "server"
log_level = "info"
transport = "${transport}"
bind_addr = "0.0.0.0:${port}"
password = "${pass}"
${tls_section}

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

[health]
enabled = true
listen = "127.0.0.1:0"
${reality_section}
EOF

    local svc="${SERVICE_NAME}-${tname}"
    cat > "/etc/systemd/system/${svc}.service" << EOF
[Unit]
Description=iPShadowT Server — ${tname} (:${port})
After=network.target
[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BINARY_NAME} -c ${cf}
Restart=always
RestartSec=3
LimitNOFILE=1048576
WatchdogSec=60
[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable "$svc" >/dev/null 2>&1
    systemctl start "$svc"
    sleep 2
    if systemctl is-active --quiet "$svc"; then
        local sip=$(curl -s4 --max-time 3 ifconfig.me 2>/dev/null || echo "YOUR_IP")
        msg_ok "Server tunnel '${tname}' active on :${port}"
        echo -e "  ${D}Share: IP=${sip} Port=${port} Pass=${pass} Transport=${transport}${N}"
        if [ -n "$pub_key" ]; then
            echo -e "  ${D}REALITY Public Key: ${pub_key}${N}"
            echo -e "  ${D}REALITY Short ID: ${short_id}${N}"
        fi
    else
        msg_err "Failed — journalctl -u ${svc} -n 5"
    fi
}

remove_tunnel() {
    list_tunnels
    echo ""
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

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  PORT FORWARD
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
add_port_forward() {
    echo ""
    echo -e "  ${W}Select tunnel to add forward:${N}"
    local configs=(); local i=1
    for conf in ${CONFIG_DIR}/config.toml ${CONFIG_DIR}/tunnel-*.toml; do
        [ -f "$conf" ] || continue
        local name=$(basename "$conf" .toml)
        local addr=$(grep -E '^(remote_addr|bind_addr)' "$conf" 2>/dev/null | head -1 | cut -d'"' -f2)
        echo -e "    ${C}${i})${N} ${name} ${D}(${addr})${N}"
        configs+=("$conf"); i=$((i+1))
    done
    [ ${#configs[@]} -eq 0 ] && { msg_err "No tunnels"; return; }

    echo ""
    msg_ask "Choice [1]: "; read -r tc; tc=${tc:-1}
    local target="${configs[$((tc-1))]}"
    [ -z "$target" ] || [ ! -f "$target" ] && { msg_err "Invalid"; return; }

    msg_ask "Forward name: "; read -r fname; [ -z "$fname" ] && return
    echo -e "  ${C}1)${N} TCP  ${C}2)${N} UDP"
    msg_ask "Protocol [1]: "; read -r proto
    local ftype="tcp"; [ "$proto" = "2" ] && ftype="udp"
    msg_ask "Listen port (this server): "; read -r lport; [ -z "$lport" ] && return
    msg_ask "Remote port (foreign): "; read -r rport; [ -z "$rport" ] && return

    cat >> "$target" << EOF

[[forwards]]
name = "${fname}"
type = "${ftype}"
listen = "0.0.0.0:${lport}"
remote = "${rport}"
EOF

    local svc_name="${SERVICE_NAME}"
    local cname=$(basename "$target" .toml)
    [ "$cname" != "config" ] && svc_name="${SERVICE_NAME}-${cname#tunnel-}"

    msg_ok "Added: ${ftype} :${lport} → remote:${rport}"
    msg_ask "Restart ${svc_name}? [Y/n]: "; read -r ans
    [[ "${ans:-y}" =~ ^[Yy]$ ]] && systemctl restart "$svc_name" && sleep 2 && msg_ok "Restarted"
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  BACKUP / RESTORE
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
do_backup() {
    print_banner
    echo -e "  ${W}[ BACKUP & RESTORE ]${N}"
    echo ""
    echo -e "  ${C}1)${N} Create backup now"
    echo -e "  ${C}2)${N} List backups"
    echo -e "  ${C}3)${N} Restore from backup"
    echo -e "  ${C}4)${N} Setup auto-backup (cron)"
    echo ""
    echo -e "  ${C}0)${N} Back"
    echo ""
    msg_ask "Choice: "; read -r c
    case $c in
        1) mkdir -p "${BACKUP_DIR}"
           local ts=$(date +%Y%m%d-%H%M%S)
           tar -czf "${BACKUP_DIR}/backup-${ts}.tar.gz" -C "${CONFIG_DIR}" --exclude=backups . 2>/dev/null
           msg_ok "Backup: ${BACKUP_DIR}/backup-${ts}.tar.gz" ;;
        2) echo ""; ls -lh ${BACKUP_DIR}/*.tar.gz 2>/dev/null || msg_warn "No backups" ;;
        3) msg_ask "Backup file path: "; read -r bf
           [ -f "$bf" ] && { tar -xzf "$bf" -C "${CONFIG_DIR}"; msg_ok "Restored"; } || msg_err "Not found" ;;
        4) (crontab -l 2>/dev/null | grep -v "ipshadowt"; echo "0 */6 * * * tar -czf ${BACKUP_DIR}/auto-\$(date +\%Y\%m\%d-\%H).tar.gz -C ${CONFIG_DIR} --exclude=backups . 2>/dev/null") | crontab -
           msg_ok "Auto-backup: every 6 hours" ;;
        0) return ;;
    esac
    press_enter
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  UPDATE
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
do_update() {
    print_banner
    echo -e "  ${W}[ UPDATE ]${N}"
    echo ""
    echo -e "  ${C}1)${N} Update binary (latest release)"
    echo -e "  ${C}2)${N} Update this script"
    echo -e "  ${C}3)${N} Check for updates"
    echo ""
    echo -e "  ${C}0)${N} Back"
    echo ""
    msg_ask "Choice: "; read -r c
    case $c in
        1) update_binary ;;
        2) msg_info "Downloading latest script..."
           curl -fsSL -o "$0" "https://raw.githubusercontent.com/${GITHUB_REPO}/master/deploy/ipshadowt-manager.sh" && msg_ok "Updated! Re-run: bash $0" || msg_err "Failed" ;;
        3) local cur=$(get_version)
           local latest=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4)
           echo -e "  Current: ${W}${cur}${N}"
           echo -e "  Latest:  ${W}${latest:-unknown}${N}"
           [ "$cur" = "$latest" ] && msg_ok "Up to date!" || msg_warn "Update available" ;;
        0) return ;;
    esac
    press_enter
}

update_binary() {
    local cur=$(get_version)
    msg_info "Current: ${cur}"
    msg_info "Checking GitHub..."
    local latest=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4)
    [ -z "$latest" ] && { msg_err "Cannot reach GitHub"; return; }
    [ "$cur" = "$latest" ] && { msg_ok "Already latest (${cur})"; return; }
    msg_info "New version: ${latest}"
    msg_ask "Update now? [Y/n]: "; read -r ans
    if [[ "${ans:-y}" =~ ^[Yy]$ ]]; then
        # Stop services
        for svc in $(systemctl list-units --type=service --all 2>/dev/null | grep ipshadowt | awk '{print $1}'); do
            systemctl stop "$svc" 2>/dev/null
        done
        # Download
        detect_arch
        local url="https://github.com/${GITHUB_REPO}/releases/download/${latest}/${BINARY_NAME}-${OS}-${ARCH}"
        curl -fSL --progress-bar -o "${INSTALL_DIR}/${BINARY_NAME}" "$url" && chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
        # Start services
        for svc in $(systemctl list-units --type=service --all 2>/dev/null | grep ipshadowt | awk '{print $1}'); do
            systemctl start "$svc" 2>/dev/null
        done
        msg_ok "Updated to ${latest}!"
    fi
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  UNINSTALL
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
do_uninstall() {
    echo ""
    msg_warn "This will completely remove iPShadowT."
    msg_ask "Continue? [y/N]: "; read -r ans
    [ "$ans" != "y" ] && return

    # Stop all
    for svc in $(systemctl list-units --type=service --all 2>/dev/null | grep ipshadowt | awk '{print $1}'); do
        systemctl stop "$svc" 2>/dev/null
        systemctl disable "$svc" 2>/dev/null
    done

    # Remove files
    rm -f "${INSTALL_DIR}/${BINARY_NAME}"
    rm -f "${SERVICE_FILE}"
    rm -f "${SYSCTL_FILE}"
    rm -f /etc/systemd/system/${SERVICE_NAME}-*.service
    systemctl daemon-reload

    msg_ask "Remove configs too? [y/N]: "; read -r rc
    [ "$rc" = "y" ] && rm -rf "${CONFIG_DIR}"

    msg_ok "iPShadowT removed"
    press_enter
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  MAIN MENU
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
main_menu() {
    while true; do
        print_banner

        # Quick status
        if is_installed; then
            local st="${R}Stopped${N}"; is_running && st="${G}Running${N}"
            local tunnels=$(count_tunnels)
            local running=$(count_running)
            echo -e "  Status: [${st}]  Version: ${W}$(get_version)${N}  Tunnels: ${W}${running}/${tunnels}${N}"
        else
            echo -e "  ${D}iPShadowT is not installed${N}"
        fi
        echo ""
        print_line
        echo ""
        echo -e "  ${C} 1)${N}  Install / Reinstall"
        echo -e "  ${C} 2)${N}  Configure Tunnel"
        echo -e "  ${C} 3)${N}  Service Control"
        echo -e "  ${C} 4)${N}  Status & Monitoring"
        echo -e "  ${C} 5)${N}  Multi-Tunnel Manager"
        echo -e "  ${C} 6)${N}  Backup & Restore"
        echo -e "  ${C} 7)${N}  Update"
        echo -e "  ${C} 8)${N}  Uninstall"
        echo ""
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
            5) do_multi ;;
            6) do_backup ;;
            7) do_update ;;
            8) do_uninstall ;;
            0) echo ""; msg_ok "Goodbye!"; echo ""; exit 0 ;;
            *) msg_err "Invalid option" ;;
        esac
    done
}

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  ENTRY POINT
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
check_root
detect_arch
main_menu
