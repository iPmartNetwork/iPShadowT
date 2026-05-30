#!/bin/bash
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  iPShadowT Manager v1.0.0
#  iPmart Network (Ali Hassanzadeh)
#  Anti-DPI Multi-Transport Tunnel
#  https://github.com/iPmartNetwork/iPShadowT
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

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
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
NC='\033[0m'
BOLD='\033[1m'
DIM='\033[2m'

# ─── Helpers ──────────────────────────────────────
print_banner() {
    clear
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${WHITE}  🛡️  iPShadowT Manager ${DIM}v${VERSION}${NC}"
    echo -e "${DIM}  iPmart Network (Ali Hassanzadeh)${NC}"
    echo -e "${DIM}  Anti-DPI Multi-Transport Tunnel${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

msg_info()    { echo -e "  ${GREEN}[✓]${NC} $1"; }
msg_warn()    { echo -e "  ${YELLOW}[!]${NC} $1"; }
msg_error()   { echo -e "  ${RED}[✗]${NC} $1"; }
msg_step()    { echo -e "  ${CYAN}[→]${NC} $1"; }
msg_ask()     { echo -ne "  ${PURPLE}[?]${NC} $1"; }

check_root() {
    if [ "$EUID" -ne 0 ]; then
        msg_error "Please run as root (sudo bash $0)"
        exit 1
    fi
}

detect_arch() {
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)  ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        armv7l)  ARCH="arm" ;;
        *) msg_error "Unsupported architecture: $ARCH"; exit 1 ;;
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

is_installed() {
    [ -f "${INSTALL_DIR}/${BINARY_NAME}" ]
}

is_running() {
    systemctl is-active --quiet ${SERVICE_NAME} 2>/dev/null
}

get_current_version() {
    if is_installed; then
        ${INSTALL_DIR}/${BINARY_NAME} -v 2>/dev/null | head -1 | awk '{print $2}' || echo "unknown"
    else
        echo "not installed"
    fi
}

generate_password() {
    cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 32 | head -n 1
}

validate_ip() {
    local ip=$1
    if [[ $ip =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
        return 0
    fi
    # Also accept domain names
    if [[ $ip =~ ^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)*$ ]]; then
        return 0
    fi
    return 1
}

validate_port() {
    local port=$1
    if [[ $port =~ ^[0-9]+$ ]] && [ $port -ge 1 ] && [ $port -le 65535 ]; then
        return 0
    fi
    return 1
}

press_enter() {
    echo ""
    msg_ask "Press Enter to continue..."
    read
}

# ─── Install Functions ────────────────────────────
install_prerequisites() {
    msg_step "Installing prerequisites..."
    apt-get update -qq > /dev/null 2>&1 || yum update -q > /dev/null 2>&1 || true
    
    for pkg in curl wget jq iptables tar gzip; do
        if ! command -v $pkg &> /dev/null; then
            apt-get install -y -qq $pkg > /dev/null 2>&1 || yum install -y -q $pkg > /dev/null 2>&1 || true
        fi
    done
    msg_info "Prerequisites installed"
}

download_binary() {
    msg_step "Downloading iPShadowT (${OS}/${ARCH})..."
    
    local url="https://github.com/${GITHUB_REPO}/releases/latest/download/${BINARY_NAME}-${OS}-${ARCH}"
    local tmp="/tmp/${BINARY_NAME}-download"
    
    if curl -fsSL --progress-bar -o "$tmp" "$url" 2>/dev/null; then
        chmod +x "$tmp"
        mv "$tmp" "${INSTALL_DIR}/${BINARY_NAME}"
        msg_info "Binary installed: ${INSTALL_DIR}/${BINARY_NAME}"
        return 0
    fi
    
    # Fallback: check local file
    if [ -f "./${BINARY_NAME}-${OS}-${ARCH}" ]; then
        cp "./${BINARY_NAME}-${OS}-${ARCH}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
        msg_info "Binary installed from local file"
        return 0
    elif [ -f "./${BINARY_NAME}" ]; then
        cp "./${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
        msg_info "Binary installed from local file"
        return 0
    fi
    
    msg_error "Download failed. Place binary in current directory and retry."
    return 1
}

install_service() {
    cat > "${SERVICE_FILE}" << EOF
[Unit]
Description=iPShadowT - Anti-DPI Multi-Transport Tunnel
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=${INSTALL_DIR}/${BINARY_NAME} -c ${CONFIG_DIR}/config.toml
Restart=always
RestartSec=5
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${SERVICE_NAME}

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable ${SERVICE_NAME} > /dev/null 2>&1
    msg_info "Systemd service installed and enabled"
}

apply_kernel_tuning() {
    cat > "${SYSCTL_FILE}" << 'EOF'
net.ipv4.ip_forward = 1
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.tcp_rmem = 4096 524288 16777216
net.ipv4.tcp_wmem = 4096 524288 16777216
net.ipv4.tcp_fastopen = 3
net.ipv4.tcp_slow_start_after_idle = 0
net.ipv4.tcp_no_metrics_save = 1
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.core.netdev_max_backlog = 65535
net.ipv4.tcp_keepalive_time = 60
net.ipv4.tcp_keepalive_intvl = 10
net.ipv4.tcp_keepalive_probes = 6
EOF
    sysctl -p "${SYSCTL_FILE}" > /dev/null 2>&1
    msg_info "Kernel tuning applied"
}

# ─── Configure Functions ──────────────────────────
configure_iran() {
    echo ""
    echo -e "  ${CYAN}┌─ Iran Server (Client) Setup ──────────────┐${NC}"
    echo ""
    
    # Get server IP
    local remote_ip=""
    while true; do
        msg_ask "Foreign server IP/domain: "
        read remote_ip
        if validate_ip "$remote_ip"; then break; fi
        msg_error "Invalid IP or domain"
    done
    
    # Get port
    msg_ask "Foreign server port [443]: "
    read remote_port
    remote_port=${remote_port:-443}
    
    # Password
    local password=$(generate_password)
    msg_ask "Password (Enter for auto-generate): "
    read user_pass
    if [ -n "$user_pass" ]; then password="$user_pass"; fi
    
    # Transport selection
    echo ""
    echo -e "  ${CYAN}Select Transport:${NC}"
    echo "    1) reality     - Maximum stealth (recommended)"
    echo "    2) kcp         - Works on filtered servers"
    echo "    3) wsmux       - CDN compatible"
    echo "    4) shadowtls   - No certificate needed"
    echo "    5) tcpmux      - Simple & fast"
    echo "    6) auto        - Auto-detect best"
    msg_ask "Choice [1]: "
    read transport_choice
    
    local transport="reality"
    case $transport_choice in
        2) transport="kcp" ;;
        3) transport="wsmux" ;;
        4) transport="shadowtls" ;;
        5) transport="tcpmux" ;;
        6) transport=$(auto_detect_transport "$remote_ip" "$remote_port") ;;
        *) transport="reality" ;;
    esac
    
    # Anti-DPI
    msg_ask "Enable Anti-DPI? [Y/n]: "
    read antidpi
    antidpi=${antidpi:-y}
    
    # SOCKS5 port
    msg_ask "SOCKS5 proxy port [1080]: "
    read socks_port
    socks_port=${socks_port:-1080}
    
    # Generate config
    mkdir -p "${CONFIG_DIR}"
    cat > "${CONFIG_DIR}/config.toml" << EOF
# iPShadowT Client Configuration
# Generated by iPShadowT Manager
mode = "client"
log_level = "info"
transport = "${transport}"
remote_addr = "${remote_ip}:${remote_port}"
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
kernel_tuning = true

[anti_dpi]
enabled = $([ "$antidpi" = "y" ] && echo "true" || echo "false")
utls_fingerprint = "chrome"
fragment = true
fragment_size = "40-80"
padding = true

[[forwards]]
name = "socks5-proxy"
type = "socks5"
listen = "0.0.0.0:${socks_port}"
EOF

    msg_info "Client config saved: ${CONFIG_DIR}/config.toml"
    echo ""
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "  ${WHITE}  Connection Details:${NC}"
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "    Remote:    ${remote_ip}:${remote_port}"
    echo -e "    Transport: ${transport}"
    echo -e "    Password:  ${password}"
    echo -e "    SOCKS5:    0.0.0.0:${socks_port}"
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    
    # Start service
    msg_ask "Start service now? [Y/n]: "
    read start_now
    if [ "${start_now:-y}" = "y" ]; then
        systemctl restart ${SERVICE_NAME}
        sleep 2
        if is_running; then
            msg_info "Service started successfully!"
        else
            msg_error "Service failed to start. Check: journalctl -u ${SERVICE_NAME}"
        fi
    fi
}

configure_foreign() {
    echo ""
    echo -e "  ${CYAN}┌─ Foreign Server Setup ────────────────────┐${NC}"
    echo ""
    
    # Port
    msg_ask "Listen port [443]: "
    read listen_port
    listen_port=${listen_port:-443}
    
    # Password
    local password=$(generate_password)
    msg_ask "Password (Enter for auto-generate): "
    read user_pass
    if [ -n "$user_pass" ]; then password="$user_pass"; fi
    
    # Transport
    echo ""
    echo -e "  ${CYAN}Select Transport:${NC}"
    echo "    1) reality     - Maximum stealth (recommended)"
    echo "    2) kcp         - Works on filtered servers"
    echo "    3) wsmux       - CDN compatible"
    echo "    4) shadowtls   - No certificate needed"
    echo "    5) tcpmux      - Simple & fast"
    msg_ask "Choice [1]: "
    read transport_choice
    
    local transport="reality"
    case $transport_choice in
        2) transport="kcp" ;;
        3) transport="wsmux" ;;
        4) transport="shadowtls" ;;
        5) transport="tcpmux" ;;
        *) transport="reality" ;;
    esac
    
    # REALITY config
    local reality_block=""
    if [ "$transport" = "reality" ]; then
        msg_step "Generating REALITY keys..."
        local keys=$(${INSTALL_DIR}/${BINARY_NAME} --gen-reality-keys 2>/dev/null || true)
        local private_key=$(echo "$keys" | grep "Private Key:" | awk '{print $NF}')
        local public_key=$(echo "$keys" | grep "Public Key:" | awk '{print $NF}')
        local short_id=$(echo "$keys" | grep "Short ID:" | awk '{print $NF}')
        
        # Fallback if keygen fails
        if [ -z "$private_key" ]; then
            private_key=$(cat /dev/urandom | tr -dc 'a-f0-9' | fold -w 64 | head -n 1)
            public_key=$(cat /dev/urandom | tr -dc 'a-f0-9' | fold -w 64 | head -n 1)
            short_id=$(cat /dev/urandom | tr -dc 'a-f0-9' | fold -w 16 | head -n 1)
        fi
        
        msg_ask "Cover website (SNI) [www.google.com]: "
        read sni
        sni=${sni:-www.google.com}
        
        reality_block="
[reality]
server_name = \"${sni}\"
private_key = \"${private_key}\"
short_id = \"${short_id}\"
dest = \"${sni}:443\""
    fi
    
    # Generate config
    mkdir -p "${CONFIG_DIR}"
    cat > "${CONFIG_DIR}/config.toml" << EOF
# iPShadowT Server Configuration
# Generated by iPShadowT Manager
mode = "server"
log_level = "info"
transport = "${transport}"
bind_addr = "0.0.0.0:${listen_port}"
password = "${password}"

[mux]
concurrency = 8
frame_size = 32768
recv_buffer = 4194304
stream_buffer = 2097152

[heartbeat]
enabled = true
interval = 20
timeout = 40

[performance]
nodelay = true
keepalive = 15
buffer_profile = "high_throughput"
kernel_tuning = true
${reality_block}
EOF

    msg_info "Server config saved: ${CONFIG_DIR}/config.toml"
    
    # Show client info
    local server_ip=$(curl -s4 ifconfig.me 2>/dev/null || curl -s4 ip.sb 2>/dev/null || echo "YOUR_SERVER_IP")
    
    echo ""
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "  ${WHITE}  📋 Give this info to your CLIENT:${NC}"
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "    Server IP:   ${server_ip}"
    echo -e "    Port:        ${listen_port}"
    echo -e "    Transport:   ${transport}"
    echo -e "    Password:    ${password}"
    if [ "$transport" = "reality" ]; then
        echo -e "    Public Key:  ${public_key}"
        echo -e "    Short ID:    ${short_id}"
        echo -e "    SNI:         ${sni}"
    fi
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    
    # Start service
    msg_ask "Start service now? [Y/n]: "
    read start_now
    if [ "${start_now:-y}" = "y" ]; then
        systemctl restart ${SERVICE_NAME}
        sleep 2
        if is_running; then
            msg_info "Service started successfully!"
        else
            msg_error "Service failed to start. Check: journalctl -u ${SERVICE_NAME}"
        fi
    fi
}

auto_detect_transport() {
    local ip=$1
    local port=$2
    msg_step "Testing transports to ${ip}:${port}..."
    
    # Test TCP
    if timeout 5 bash -c "echo > /dev/tcp/${ip}/${port}" 2>/dev/null; then
        msg_info "TCP/443: OK"
        # Test if UDP works
        if timeout 3 bash -c "echo > /dev/udp/${ip}/${port}" 2>/dev/null; then
            msg_info "UDP: OK → Recommending: kcp"
            echo "kcp"
        else
            msg_info "UDP: Blocked → Recommending: reality"
            echo "reality"
        fi
    else
        msg_warn "Direct TCP failed → Recommending: wsmux (CDN)"
        echo "wsmux"
    fi
}

# ─── Status & Monitoring ─────────────────────────
show_status() {
    echo ""
    echo -e "  ${CYAN}┌─ iPShadowT Status ───────────────────────┐${NC}"
    echo ""
    
    if ! is_installed; then
        msg_error "iPShadowT is not installed"
        return
    fi
    
    local version=$(get_current_version)
    local status="🔴 Stopped"
    local uptime="-"
    
    if is_running; then
        status="🟢 Running"
        uptime=$(systemctl show ${SERVICE_NAME} --property=ActiveEnterTimestamp --value 2>/dev/null | xargs -I{} date -d {} +%s 2>/dev/null || echo "")
        if [ -n "$uptime" ]; then
            local now=$(date +%s)
            local diff=$((now - uptime))
            local days=$((diff / 86400))
            local hours=$(( (diff % 86400) / 3600 ))
            local mins=$(( (diff % 3600) / 60 ))
            uptime="${days}d ${hours}h ${mins}m"
        fi
    fi
    
    echo -e "    Version:    ${version}"
    echo -e "    Status:     ${status}"
    echo -e "    Uptime:     ${uptime}"
    
    if [ -f "${CONFIG_DIR}/config.toml" ]; then
        local mode=$(grep "^mode" ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)
        local transport=$(grep "^transport" ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)
        local remote=$(grep "^remote_addr" ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)
        local bind=$(grep "^bind_addr" ${CONFIG_DIR}/config.toml 2>/dev/null | cut -d'"' -f2)
        
        echo -e "    Mode:       ${mode}"
        echo -e "    Transport:  ${transport}"
        [ -n "$remote" ] && echo -e "    Remote:     ${remote}"
        [ -n "$bind" ] && echo -e "    Bind:       ${bind}"
    fi
    
    # Memory & CPU
    local pid=$(pgrep -x ${BINARY_NAME} 2>/dev/null)
    if [ -n "$pid" ]; then
        local mem=$(ps -p $pid -o rss= 2>/dev/null | awk '{printf "%.1f MB", $1/1024}')
        local cpu=$(ps -p $pid -o %cpu= 2>/dev/null)
        echo -e "    Memory:     ${mem}"
        echo -e "    CPU:        ${cpu}%"
    fi
    
    echo ""
    echo -e "  ${CYAN}└───────────────────────────────────────────┘${NC}"
}

show_logs() {
    local lines=${1:-50}
    journalctl -u ${SERVICE_NAME} --no-pager -n $lines
}

show_live_logs() {
    journalctl -u ${SERVICE_NAME} -f
}

# ─── Network Tools ────────────────────────────────
test_connection() {
    msg_ask "Target IP/domain: "
    read target
    msg_ask "Port [443]: "
    read port
    port=${port:-443}
    
    echo ""
    msg_step "Testing connection to ${target}:${port}..."
    
    # TCP test
    if timeout 5 bash -c "echo > /dev/tcp/${target}/${port}" 2>/dev/null; then
        msg_info "TCP connection: OK"
    else
        msg_error "TCP connection: FAILED"
    fi
    
    # Ping test
    if ping -c 3 -W 3 "$target" > /dev/null 2>&1; then
        local latency=$(ping -c 3 -W 3 "$target" 2>/dev/null | tail -1 | awk -F'/' '{print $5}')
        msg_info "Ping: OK (${latency}ms avg)"
    else
        msg_warn "Ping: Blocked (ICMP filtered)"
    fi
    
    # DNS test
    if nslookup "$target" > /dev/null 2>&1; then
        msg_info "DNS resolution: OK"
    else
        msg_warn "DNS resolution: Failed (try DoH)"
    fi
}

speed_test() {
    msg_step "Running speed test..."
    if command -v curl &> /dev/null; then
        local speed=$(curl -s -o /dev/null -w '%{speed_download}' http://speedtest.tele2.net/1MB.zip 2>/dev/null)
        if [ -n "$speed" ] && [ "$speed" != "0.000" ]; then
            local mbps=$(echo "$speed" | awk '{printf "%.2f", $1/1048576*8}')
            msg_info "Download speed: ~${mbps} Mbps"
        else
            msg_warn "Speed test failed (no internet or blocked)"
        fi
    else
        msg_error "curl not available"
    fi
}

port_check() {
    msg_ask "Port to check [443]: "
    read port
    port=${port:-443}
    
    if ss -tuln | grep -q ":${port} "; then
        local proc=$(ss -tulnp | grep ":${port} " | awk '{print $NF}')
        msg_info "Port ${port}: IN USE by ${proc}"
    else
        msg_info "Port ${port}: AVAILABLE"
    fi
}

# ─── Key Management ───────────────────────────────
generate_keys() {
    if is_installed; then
        ${INSTALL_DIR}/${BINARY_NAME} --gen-reality-keys
    else
        msg_error "iPShadowT not installed. Install first."
    fi
}

generate_random_password() {
    local pass=$(generate_password)
    echo ""
    msg_info "Generated password: ${pass}"
    echo ""
}

show_current_config() {
    if [ -f "${CONFIG_DIR}/config.toml" ]; then
        echo ""
        echo -e "  ${CYAN}─── Current Config ───${NC}"
        cat "${CONFIG_DIR}/config.toml"
        echo -e "  ${CYAN}──────────────────────${NC}"
    else
        msg_warn "No config file found"
    fi
}

# ─── Backup Functions ─────────────────────────────
create_backup() {
    mkdir -p "${BACKUP_DIR}"
    local timestamp=$(date +%Y%m%d-%H%M%S)
    local backup_file="${BACKUP_DIR}/ipshadowt-backup-${timestamp}.tar.gz"
    
    tar -czf "$backup_file" -C "${CONFIG_DIR}" --exclude="backups" . 2>/dev/null
    msg_info "Backup created: ${backup_file}"
}

list_backups() {
    echo ""
    if [ -d "${BACKUP_DIR}" ] && [ "$(ls -A ${BACKUP_DIR} 2>/dev/null)" ]; then
        echo -e "  ${CYAN}Available Backups:${NC}"
        ls -lh "${BACKUP_DIR}"/*.tar.gz 2>/dev/null | awk '{print "    " $NF " (" $5 ")"}'
    else
        msg_warn "No backups found"
    fi
}

restore_backup() {
    list_backups
    echo ""
    msg_ask "Backup file path: "
    read backup_path
    
    if [ -f "$backup_path" ]; then
        tar -xzf "$backup_path" -C "${CONFIG_DIR}"
        msg_info "Restored from: ${backup_path}"
        msg_warn "Restart service to apply: systemctl restart ${SERVICE_NAME}"
    else
        msg_error "File not found: ${backup_path}"
    fi
}

# ─── Update Function ─────────────────────────────
update_ipshadowt() {
    local current=$(get_current_version)
    msg_step "Current version: ${current}"
    msg_step "Checking for updates..."
    
    local latest=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4)
    
    if [ -z "$latest" ]; then
        msg_error "Failed to check updates (no internet?)"
        return
    fi
    
    if [ "$current" = "$latest" ]; then
        msg_info "Already up to date (${current})"
        return
    fi
    
    msg_info "New version available: ${latest}"
    msg_ask "Update now? [Y/n]: "
    read confirm
    
    if [ "${confirm:-y}" = "y" ]; then
        create_backup
        systemctl stop ${SERVICE_NAME} 2>/dev/null || true
        download_binary
        systemctl start ${SERVICE_NAME}
        msg_info "Updated to ${latest}!"
    fi
}

# ─── Uninstall Function ──────────────────────────
uninstall_ipshadowt() {
    echo ""
    msg_warn "This will remove iPShadowT from this system."
    msg_ask "Are you sure? [y/N]: "
    read confirm
    
    if [ "$confirm" != "y" ]; then
        msg_info "Cancelled"
        return
    fi
    
    systemctl stop ${SERVICE_NAME} 2>/dev/null || true
    systemctl disable ${SERVICE_NAME} 2>/dev/null || true
    rm -f "${SERVICE_FILE}"
    rm -f "${INSTALL_DIR}/${BINARY_NAME}"
    rm -f "${SYSCTL_FILE}"
    systemctl daemon-reload
    sysctl --system > /dev/null 2>&1 || true
    
    msg_ask "Remove config and backups? [y/N]: "
    read remove_config
    if [ "$remove_config" = "y" ]; then
        rm -rf "${CONFIG_DIR}"
        msg_info "Config removed"
    fi
    
    msg_info "iPShadowT uninstalled successfully"
}

# ─── Multi-Tunnel Support ─────────────────────────

# List all tunnel instances
list_tunnels() {
    echo ""
    echo -e "  ${CYAN}┌─ Active Tunnels ──────────────────────────┐${NC}"
    echo ""
    
    local found=0
    for conf in ${CONFIG_DIR}/tunnel-*.toml ${CONFIG_DIR}/config.toml; do
        [ -f "$conf" ] || continue
        found=$((found+1))
        local name=$(basename "$conf" .toml)
        local svc="${SERVICE_NAME}"
        [ "$name" != "config" ] && svc="${SERVICE_NAME}-${name#tunnel-}"
        
        local status_icon="🔴"
        systemctl is-active --quiet "$svc" 2>/dev/null && status_icon="🟢"
        
        local transport=$(grep '^transport' "$conf" 2>/dev/null | cut -d'"' -f2)
        local remote=$(grep '^remote_addr' "$conf" 2>/dev/null | cut -d'"' -f2)
        local bind=$(grep '^bind_addr' "$conf" 2>/dev/null | cut -d'"' -f2)
        local target="${remote:-$bind}"
        
        echo -e "    ${status_icon} ${WHITE}${name}${NC} [${transport}] → ${target} (${svc})"
    done
    
    if [ $found -eq 0 ]; then
        msg_warn "No tunnels configured"
    fi
    echo ""
    echo -e "  ${CYAN}└───────────────────────────────────────────┘${NC}"
}

# Add a new tunnel instance
add_tunnel() {
    echo ""
    msg_ask "Tunnel name (e.g., server2, backup): "
    read tunnel_name
    
    if [ -z "$tunnel_name" ]; then
        msg_error "Name cannot be empty"
        return
    fi
    
    # Sanitize name
    tunnel_name=$(echo "$tunnel_name" | tr -cd 'a-zA-Z0-9_-')
    local conf_file="${CONFIG_DIR}/tunnel-${tunnel_name}.toml"
    local svc_name="${SERVICE_NAME}-${tunnel_name}"
    
    if [ -f "$conf_file" ]; then
        msg_error "Tunnel '${tunnel_name}' already exists"
        return
    fi
    
    # Ask mode
    echo ""
    echo "    1) 🇮🇷 Iran (Client)"
    echo "    2) 🌍 Foreign (Server)"
    msg_ask "Mode [1]: "
    read mode_choice
    
    if [ "${mode_choice:-1}" = "2" ]; then
        # Server mode
        msg_ask "Listen port: "
        read port
        local password=$(generate_password)
        msg_ask "Password (Enter=auto): "
        read user_pass
        [ -n "$user_pass" ] && password="$user_pass"
        
        echo "    1) reality  2) kcp  3) wsmux  4) tcpmux  5) shadowtls"
        msg_ask "Transport [1]: "
        read tc
        local transport="reality"
        case $tc in 2) transport="kcp";; 3) transport="wsmux";; 4) transport="tcpmux";; 5) transport="shadowtls";; esac
        
        cat > "$conf_file" << EOF
mode = "server"
log_level = "info"
transport = "${transport}"
bind_addr = "0.0.0.0:${port}"
password = "${password}"

[mux]
concurrency = 8

[heartbeat]
enabled = true
interval = 20
timeout = 40

[performance]
nodelay = true
keepalive = 15
buffer_profile = "high_throughput"
EOF
    else
        # Client mode
        msg_ask "Foreign server IP: "
        read remote_ip
        msg_ask "Port [443]: "
        read port
        port=${port:-443}
        msg_ask "Password: "
        read password
        
        echo "    1) reality  2) kcp  3) wsmux  4) tcpmux  5) shadowtls"
        msg_ask "Transport [1]: "
        read tc
        local transport="reality"
        case $tc in 2) transport="kcp";; 3) transport="wsmux";; 4) transport="tcpmux";; 5) transport="shadowtls";; esac
        
        msg_ask "SOCKS5 port [0=disabled]: "
        read socks_port
        
        cat > "$conf_file" << EOF
mode = "client"
log_level = "info"
transport = "${transport}"
remote_addr = "${remote_ip}:${port}"
password = "${password}"

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

[performance]
nodelay = true
keepalive = 15
EOF
        if [ -n "$socks_port" ] && [ "$socks_port" != "0" ]; then
            cat >> "$conf_file" << EOF

[[forwards]]
name = "socks5-${tunnel_name}"
type = "socks5"
listen = "0.0.0.0:${socks_port}"
EOF
        fi
    fi
    
    # Create systemd service for this tunnel
    cat > "/etc/systemd/system/${svc_name}.service" << EOF
[Unit]
Description=iPShadowT Tunnel - ${tunnel_name}
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=${INSTALL_DIR}/${BINARY_NAME} -c ${conf_file}
Restart=always
RestartSec=5
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${svc_name}

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable "$svc_name" > /dev/null 2>&1
    
    msg_info "Tunnel '${tunnel_name}' created"
    msg_ask "Start now? [Y/n]: "
    read start
    if [ "${start:-y}" = "y" ]; then
        systemctl start "$svc_name"
        sleep 2
        if systemctl is-active --quiet "$svc_name"; then
            msg_info "Tunnel '${tunnel_name}' started!"
        else
            msg_error "Failed to start. Check: journalctl -u ${svc_name}"
        fi
    fi
}

# Remove a tunnel instance
remove_tunnel() {
    list_tunnels
    echo ""
    msg_ask "Tunnel name to remove: "
    read tunnel_name
    
    if [ -z "$tunnel_name" ]; then return; fi
    
    local conf_file="${CONFIG_DIR}/tunnel-${tunnel_name}.toml"
    local svc_name="${SERVICE_NAME}-${tunnel_name}"
    
    if [ ! -f "$conf_file" ]; then
        msg_error "Tunnel '${tunnel_name}' not found"
        return
    fi
    
    msg_warn "This will remove tunnel '${tunnel_name}'"
    msg_ask "Confirm? [y/N]: "
    read confirm
    if [ "$confirm" != "y" ]; then return; fi
    
    systemctl stop "$svc_name" 2>/dev/null
    systemctl disable "$svc_name" 2>/dev/null
    rm -f "/etc/systemd/system/${svc_name}.service"
    rm -f "$conf_file"
    systemctl daemon-reload
    
    msg_info "Tunnel '${tunnel_name}' removed"
}

# Start/stop/restart a specific tunnel
manage_tunnel() {
    list_tunnels
    echo ""
    msg_ask "Tunnel name (or 'all'): "
    read tunnel_name
    
    if [ -z "$tunnel_name" ]; then return; fi
    
    echo "    1) Start  2) Stop  3) Restart  4) Logs"
    msg_ask "Action [3]: "
    read action
    
    if [ "$tunnel_name" = "all" ]; then
        for conf in ${CONFIG_DIR}/tunnel-*.toml ${CONFIG_DIR}/config.toml; do
            [ -f "$conf" ] || continue
            local name=$(basename "$conf" .toml)
            local svc="${SERVICE_NAME}"
            [ "$name" != "config" ] && svc="${SERVICE_NAME}-${name#tunnel-}"
            
            case ${action:-3} in
                1) systemctl start "$svc" 2>/dev/null && msg_info "Started: $svc" ;;
                2) systemctl stop "$svc" 2>/dev/null && msg_info "Stopped: $svc" ;;
                3) systemctl restart "$svc" 2>/dev/null && msg_info "Restarted: $svc" ;;
            esac
        done
    else
        local svc="${SERVICE_NAME}-${tunnel_name}"
        case ${action:-3} in
            1) systemctl start "$svc" && msg_info "Started" || msg_error "Failed" ;;
            2) systemctl stop "$svc" && msg_info "Stopped" || msg_error "Failed" ;;
            3) systemctl restart "$svc" && msg_info "Restarted" || msg_error "Failed" ;;
            4) journalctl -u "$svc" --no-pager -n 30 ;;
        esac
    fi
}

# ─── Advanced Features ────────────────────────────

# Cronjob auto-backup
setup_auto_backup() {
    echo ""
    msg_ask "Backup interval in hours [24]: "
    read interval
    interval=${interval:-24}
    
    # Create backup script
    cat > /usr/local/bin/ipshadowt-backup << 'BKEOF'
#!/bin/bash
BACKUP_DIR="/etc/ipshadowt/backups"
mkdir -p "$BACKUP_DIR"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
tar -czf "${BACKUP_DIR}/auto-${TIMESTAMP}.tar.gz" -C /etc/ipshadowt --exclude=backups . 2>/dev/null
# Keep only last 10 backups
ls -t ${BACKUP_DIR}/auto-*.tar.gz 2>/dev/null | tail -n +11 | xargs rm -f 2>/dev/null
BKEOF
    chmod +x /usr/local/bin/ipshadowt-backup
    
    # Add cron job
    (crontab -l 2>/dev/null | grep -v "ipshadowt-backup"; echo "0 */${interval} * * * /usr/local/bin/ipshadowt-backup") | crontab -
    msg_info "Auto-backup enabled: every ${interval} hours (max 10 kept)"
}

disable_auto_backup() {
    crontab -l 2>/dev/null | grep -v "ipshadowt-backup" | crontab -
    rm -f /usr/local/bin/ipshadowt-backup
    msg_info "Auto-backup disabled"
}

# Config validator
validate_config() {
    local config_file="${CONFIG_DIR}/config.toml"
    if [ ! -f "$config_file" ]; then
        msg_error "Config file not found"
        return 1
    fi
    
    msg_step "Validating config..."
    local errors=0
    
    # Check required fields
    if ! grep -q '^mode' "$config_file"; then
        msg_error "Missing: mode"; errors=$((errors+1))
    fi
    if ! grep -q '^password' "$config_file"; then
        msg_error "Missing: password"; errors=$((errors+1))
    fi
    if ! grep -q '^transport' "$config_file"; then
        msg_error "Missing: transport"; errors=$((errors+1))
    fi
    
    local mode=$(grep '^mode' "$config_file" | cut -d'"' -f2)
    if [ "$mode" = "server" ] && ! grep -q '^bind_addr' "$config_file"; then
        msg_error "Server mode requires bind_addr"; errors=$((errors+1))
    fi
    if [ "$mode" = "client" ] && ! grep -q '^remote_addr' "$config_file"; then
        msg_error "Client mode requires remote_addr"; errors=$((errors+1))
    fi
    
    # Check password not default
    if grep -q 'CHANGE_THIS_PASSWORD' "$config_file"; then
        msg_warn "Password is still default! Change it."
        errors=$((errors+1))
    fi
    
    if [ $errors -eq 0 ]; then
        msg_info "Config is valid ✓"
        return 0
    else
        msg_error "${errors} error(s) found"
        return 1
    fi
}

# Firewall auto-config
configure_firewall() {
    local port=${1:-443}
    msg_step "Configuring firewall for port ${port}..."
    
    # UFW
    if command -v ufw &> /dev/null; then
        ufw allow ${port}/tcp > /dev/null 2>&1
        ufw allow ${port}/udp > /dev/null 2>&1
        msg_info "UFW: port ${port} opened"
        return
    fi
    
    # Firewalld
    if command -v firewall-cmd &> /dev/null; then
        firewall-cmd --permanent --add-port=${port}/tcp > /dev/null 2>&1
        firewall-cmd --permanent --add-port=${port}/udp > /dev/null 2>&1
        firewall-cmd --reload > /dev/null 2>&1
        msg_info "Firewalld: port ${port} opened"
        return
    fi
    
    # iptables fallback
    iptables -I INPUT -p tcp --dport ${port} -j ACCEPT 2>/dev/null
    iptables -I INPUT -p udp --dport ${port} -j ACCEPT 2>/dev/null
    msg_info "iptables: port ${port} opened"
}

# Connection watchdog
install_watchdog() {
    msg_step "Installing connection watchdog..."
    
    cat > /usr/local/bin/ipshadowt-watchdog << 'WDEOF'
#!/bin/bash
# iPShadowT Connection Watchdog
# Checks if tunnel is alive, restarts if dead

SERVICE="ipshadowt"
CONFIG="/etc/ipshadowt/config.toml"
LOG="/var/log/ipshadowt-watchdog.log"
MAX_FAILURES=3
FAILURES=0

check_tunnel() {
    # Check if service is running
    if ! systemctl is-active --quiet $SERVICE; then
        return 1
    fi
    # Check if SOCKS5 port is responding (if client mode)
    local socks_port=$(grep -A2 'type = "socks5"' $CONFIG 2>/dev/null | grep listen | grep -oP ':\K[0-9]+')
    if [ -n "$socks_port" ]; then
        if ! timeout 5 bash -c "echo > /dev/tcp/127.0.0.1/${socks_port}" 2>/dev/null; then
            return 1
        fi
    fi
    return 0
}

while true; do
    if check_tunnel; then
        FAILURES=0
    else
        FAILURES=$((FAILURES+1))
        echo "$(date): Tunnel check failed (${FAILURES}/${MAX_FAILURES})" >> $LOG
        
        if [ $FAILURES -ge $MAX_FAILURES ]; then
            echo "$(date): Restarting service..." >> $LOG
            systemctl restart $SERVICE
            FAILURES=0
            sleep 10
        fi
    fi
    sleep 30
done
WDEOF
    chmod +x /usr/local/bin/ipshadowt-watchdog
    
    # Create systemd service for watchdog
    cat > /etc/systemd/system/ipshadowt-watchdog.service << 'EOF'
[Unit]
Description=iPShadowT Connection Watchdog
After=ipshadowt.service

[Service]
Type=simple
ExecStart=/usr/local/bin/ipshadowt-watchdog
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable ipshadowt-watchdog > /dev/null 2>&1
    systemctl start ipshadowt-watchdog
    msg_info "Watchdog installed and started"
    msg_info "Log: /var/log/ipshadowt-watchdog.log"
}

remove_watchdog() {
    systemctl stop ipshadowt-watchdog 2>/dev/null
    systemctl disable ipshadowt-watchdog 2>/dev/null
    rm -f /etc/systemd/system/ipshadowt-watchdog.service
    rm -f /usr/local/bin/ipshadowt-watchdog
    systemctl daemon-reload
    msg_info "Watchdog removed"
}

# Export client config
export_client_config() {
    if [ ! -f "${CONFIG_DIR}/config.toml" ]; then
        msg_error "No config found"
        return
    fi
    
    local mode=$(grep '^mode' ${CONFIG_DIR}/config.toml | cut -d'"' -f2)
    if [ "$mode" != "server" ]; then
        msg_warn "This is a client config. Export is for server → client."
        return
    fi
    
    local server_ip=$(curl -s4 ifconfig.me 2>/dev/null || curl -s4 ip.sb 2>/dev/null || echo "YOUR_IP")
    local port=$(grep '^bind_addr' ${CONFIG_DIR}/config.toml | grep -oP ':\K[0-9]+')
    local password=$(grep '^password' ${CONFIG_DIR}/config.toml | cut -d'"' -f2)
    local transport=$(grep '^transport' ${CONFIG_DIR}/config.toml | cut -d'"' -f2)
    local public_key=$(grep 'public_key\|private_key' ${CONFIG_DIR}/config.toml | head -1 | cut -d'"' -f2)
    local short_id=$(grep 'short_id' ${CONFIG_DIR}/config.toml | cut -d'"' -f2)
    local sni=$(grep 'server_name' ${CONFIG_DIR}/config.toml | cut -d'"' -f2)
    
    echo ""
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "  ${WHITE}  📤 Client Config (copy to Iran server):${NC}"
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
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
    if [ "$transport" = "reality" ] && [ -n "$short_id" ]; then
        echo "[reality]"
        echo "server_name = \"${sni}\""
        echo "public_key = \"${public_key}\""
        echo "short_id = \"${short_id}\""
        echo ""
    fi
    echo "[[forwards]]"
    echo "name = \"socks5\""
    echo "type = \"socks5\""
    echo "listen = \"0.0.0.0:1080\""
    echo ""
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    
    # Save to file
    msg_ask "Save to file? [Y/n]: "
    read save
    if [ "${save:-y}" = "y" ]; then
        local outfile="${CONFIG_DIR}/client-export.toml"
        echo "mode = \"client\"" > "$outfile"
        echo "transport = \"${transport}\"" >> "$outfile"
        echo "remote_addr = \"${server_ip}:${port}\"" >> "$outfile"
        echo "password = \"${password}\"" >> "$outfile"
        echo "" >> "$outfile"
        echo "[mux]" >> "$outfile"
        echo "concurrency = 4" >> "$outfile"
        echo "" >> "$outfile"
        echo "[anti_dpi]" >> "$outfile"
        echo "enabled = true" >> "$outfile"
        echo "utls_fingerprint = \"chrome\"" >> "$outfile"
        echo "fragment = true" >> "$outfile"
        echo "" >> "$outfile"
        if [ "$transport" = "reality" ] && [ -n "$short_id" ]; then
            echo "[reality]" >> "$outfile"
            echo "server_name = \"${sni}\"" >> "$outfile"
            echo "public_key = \"${public_key}\"" >> "$outfile"
            echo "short_id = \"${short_id}\"" >> "$outfile"
            echo "" >> "$outfile"
        fi
        echo "[[forwards]]" >> "$outfile"
        echo "name = \"socks5\"" >> "$outfile"
        echo "type = \"socks5\"" >> "$outfile"
        echo "listen = \"0.0.0.0:1080\"" >> "$outfile"
        msg_info "Saved to: ${outfile}"
    fi
}

# BBR enabler
enable_bbr() {
    msg_step "Enabling TCP BBR..."
    
    # Check if already enabled
    if sysctl net.ipv4.tcp_congestion_control 2>/dev/null | grep -q bbr; then
        msg_info "BBR is already enabled"
        return
    fi
    
    # Enable BBR
    echo "net.core.default_qdisc = fq" >> /etc/sysctl.d/99-ipshadowt.conf
    echo "net.ipv4.tcp_congestion_control = bbr" >> /etc/sysctl.d/99-ipshadowt.conf
    sysctl -p /etc/sysctl.d/99-ipshadowt.conf > /dev/null 2>&1
    
    # Verify
    if sysctl net.ipv4.tcp_congestion_control 2>/dev/null | grep -q bbr; then
        msg_info "BBR enabled successfully"
    else
        msg_warn "BBR may not be supported on this kernel"
    fi
}

# System info
show_system_info() {
    echo ""
    echo -e "  ${CYAN}┌─ System Information ─────────────────────┐${NC}"
    echo ""
    
    # OS
    echo -e "    OS:         $(cat /etc/os-release 2>/dev/null | grep PRETTY_NAME | cut -d'"' -f2 || uname -s)"
    echo -e "    Kernel:     $(uname -r)"
    echo -e "    Arch:       $(uname -m)"
    
    # IP
    local pub_ip=$(curl -s4 --max-time 5 ifconfig.me 2>/dev/null || echo "N/A")
    local local_ip=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "N/A")
    echo -e "    Public IP:  ${pub_ip}"
    echo -e "    Local IP:   ${local_ip}"
    
    # Resources
    local mem_total=$(free -m | awk '/Mem:/{print $2}')
    local mem_used=$(free -m | awk '/Mem:/{print $3}')
    local mem_pct=$((mem_used * 100 / mem_total))
    echo -e "    RAM:        ${mem_used}/${mem_total} MB (${mem_pct}%)"
    
    local cpu_cores=$(nproc 2>/dev/null || echo "?")
    local load=$(cat /proc/loadavg 2>/dev/null | awk '{print $1}')
    echo -e "    CPU:        ${cpu_cores} cores (load: ${load})"
    
    local disk_used=$(df -h / | awk 'NR==2{print $3"/"$2" ("$5")"}')
    echo -e "    Disk:       ${disk_used}"
    
    # Network
    local bbr_status="disabled"
    sysctl net.ipv4.tcp_congestion_control 2>/dev/null | grep -q bbr && bbr_status="enabled"
    echo -e "    BBR:        ${bbr_status}"
    
    echo ""
    echo -e "  ${CYAN}└───────────────────────────────────────────┘${NC}"
}

# Port forward manager
manage_port_forwards() {
    echo ""
    echo -e "  ${CYAN}┌─ Port Forward Manager ────────────────────┐${NC}"
    echo ""
    
    # Show current forwards
    if [ -f "${CONFIG_DIR}/config.toml" ]; then
        echo -e "  ${WHITE}  Current forwards:${NC}"
        grep -A3 '^\[\[forwards\]\]' ${CONFIG_DIR}/config.toml 2>/dev/null | grep -E 'name|type|listen|remote' | while read line; do
            echo "    $line"
        done
        echo ""
    fi
    
    echo "    1) Add TCP forward"
    echo "    2) Add UDP forward"
    echo "    3) Add SOCKS5 proxy"
    echo "    4) Remove a forward (edit config)"
    echo "    0) Back"
    echo ""
    msg_ask "Choice: "
    read choice
    
    case $choice in
        1)
            msg_ask "Name: "; read fname
            msg_ask "Listen port: "; read flisten
            msg_ask "Remote address (ip:port): "; read fremote
            echo "" >> ${CONFIG_DIR}/config.toml
            echo "[[forwards]]" >> ${CONFIG_DIR}/config.toml
            echo "name = \"${fname}\"" >> ${CONFIG_DIR}/config.toml
            echo "type = \"tcp\"" >> ${CONFIG_DIR}/config.toml
            echo "listen = \"0.0.0.0:${flisten}\"" >> ${CONFIG_DIR}/config.toml
            echo "remote = \"${fremote}\"" >> ${CONFIG_DIR}/config.toml
            msg_info "TCP forward added: :${flisten} → ${fremote}"
            msg_warn "Restart service to apply"
            ;;
        2)
            msg_ask "Name: "; read fname
            msg_ask "Listen port: "; read flisten
            msg_ask "Remote address (ip:port): "; read fremote
            echo "" >> ${CONFIG_DIR}/config.toml
            echo "[[forwards]]" >> ${CONFIG_DIR}/config.toml
            echo "name = \"${fname}\"" >> ${CONFIG_DIR}/config.toml
            echo "type = \"udp\"" >> ${CONFIG_DIR}/config.toml
            echo "listen = \"0.0.0.0:${flisten}\"" >> ${CONFIG_DIR}/config.toml
            echo "remote = \"${fremote}\"" >> ${CONFIG_DIR}/config.toml
            msg_info "UDP forward added: :${flisten} → ${fremote}"
            msg_warn "Restart service to apply"
            ;;
        3)
            msg_ask "Listen port [1080]: "; read flisten
            flisten=${flisten:-1080}
            echo "" >> ${CONFIG_DIR}/config.toml
            echo "[[forwards]]" >> ${CONFIG_DIR}/config.toml
            echo "name = \"socks5\"" >> ${CONFIG_DIR}/config.toml
            echo "type = \"socks5\"" >> ${CONFIG_DIR}/config.toml
            echo "listen = \"0.0.0.0:${flisten}\"" >> ${CONFIG_DIR}/config.toml
            msg_info "SOCKS5 proxy added on port ${flisten}"
            msg_warn "Restart service to apply"
            ;;
        4)
            nano "${CONFIG_DIR}/config.toml" 2>/dev/null || vi "${CONFIG_DIR}/config.toml"
            ;;
    esac
}

# ─── Menu Functions ───────────────────────────────
menu_install() {
    print_banner
    echo -e "  ${WHITE}🚀 Install iPShadowT${NC}"
    echo ""
    
    if is_installed; then
        msg_warn "iPShadowT is already installed ($(get_current_version))"
        msg_ask "Reinstall? [y/N]: "
        read reinstall
        if [ "$reinstall" != "y" ]; then return; fi
    fi
    
    detect_arch
    msg_info "Detected: ${OS}/${ARCH}"
    
    install_prerequisites
    download_binary
    mkdir -p "${CONFIG_DIR}"
    install_service
    apply_kernel_tuning
    enable_bbr
    
    # Auto-configure firewall for port 443
    configure_firewall 443
    
    echo ""
    msg_info "Installation complete!"
    echo ""
    msg_ask "Configure tunnel now? [Y/n]: "
    read configure
    if [ "${configure:-y}" = "y" ]; then
        menu_configure
    fi
}

menu_configure() {
    print_banner
    echo -e "  ${WHITE}⚙️  Configure Tunnel${NC}"
    echo ""
    echo "    1) 🇮🇷 Setup Iran Server (Client)"
    echo "    2) 🌍 Setup Foreign Server"
    echo "    3) 🔧 Edit Config (nano)"
    echo "    4) 📋 Show Current Config"
    echo "    5) ✅ Validate Config"
    echo "    6) 🧪 Auto-Detect Best Transport"
    echo "    7) 📤 Export Client Config"
    echo "    8) 📡 Manage Port Forwards"
    echo "    0) 🔙 Back"
    echo ""
    msg_ask "Choice: "
    read choice
    
    case $choice in
        1) configure_iran ;;
        2) configure_foreign ;;
        3) nano "${CONFIG_DIR}/config.toml" 2>/dev/null || vi "${CONFIG_DIR}/config.toml" ;;
        4) show_current_config ;;
        5) validate_config ;;
        6) 
            msg_ask "Target server IP: "
            read target_ip
            auto_detect_transport "$target_ip" "443"
            ;;
        7) export_client_config ;;
        8) manage_port_forwards ;;
        0) return ;;
    esac
    press_enter
}

menu_service() {
    print_banner
    echo -e "  ${WHITE}🔄 Service Control${NC}"
    echo ""
    echo "    1) ▶️  Start"
    echo "    2) ⏹️  Stop"
    echo "    3) 🔄 Restart"
    echo "    4) ✅ Enable (start on boot)"
    echo "    5) ❌ Disable"
    echo "    6) 🐕 Install Watchdog (auto-restart on failure)"
    echo "    7) 🐕 Remove Watchdog"
    echo "    0) 🔙 Back"
    echo ""
    msg_ask "Choice: "
    read choice
    
    case $choice in
        1) systemctl start ${SERVICE_NAME} && msg_info "Started" || msg_error "Failed" ;;
        2) systemctl stop ${SERVICE_NAME} && msg_info "Stopped" || msg_error "Failed" ;;
        3) systemctl restart ${SERVICE_NAME} && msg_info "Restarted" || msg_error "Failed" ;;
        4) systemctl enable ${SERVICE_NAME} && msg_info "Enabled" ;;
        5) systemctl disable ${SERVICE_NAME} && msg_info "Disabled" ;;
        6) install_watchdog ;;
        7) remove_watchdog ;;
        0) return ;;
    esac
    press_enter
}

menu_monitoring() {
    print_banner
    echo -e "  ${WHITE}📊 Status & Monitoring${NC}"
    echo ""
    echo "    1) 📊 Service Status"
    echo "    2) 🖥️  System Info"
    echo "    3) 📋 View Logs (last 50)"
    echo "    4) 📋 Live Logs (Ctrl+C to exit)"
    echo "    5) 🌐 Test Connection"
    echo "    6) ⚡ Speed Test"
    echo "    7) 🔍 Port Check"
    echo "    0) 🔙 Back"
    echo ""
    msg_ask "Choice: "
    read choice
    
    case $choice in
        1) show_status ;;
        2) show_system_info ;;
        3) show_logs 50 ;;
        4) show_live_logs ;;
        5) test_connection ;;
        6) speed_test ;;
        7) port_check ;;
        0) return ;;
    esac
    press_enter
}

menu_keys() {
    print_banner
    echo -e "  ${WHITE}🔑 Key Management${NC}"
    echo ""
    echo "    1) 🔑 Generate REALITY Keys"
    echo "    2) 🔐 Generate Random Password"
    echo "    3) 📋 Show Current Config"
    echo "    0) 🔙 Back"
    echo ""
    msg_ask "Choice: "
    read choice
    
    case $choice in
        1) generate_keys ;;
        2) generate_random_password ;;
        3) show_current_config ;;
        0) return ;;
    esac
    press_enter
}

menu_network() {
    print_banner
    echo -e "  ${WHITE}🌐 Network Tools${NC}"
    echo ""
    echo "    1) 🌐 Test Connection"
    echo "    2) ⚡ Speed Test"
    echo "    3) 🔍 Port Check"
    echo "    4) 📶 Ping Test"
    echo "    5) 🧪 Auto-Detect Transport"
    echo "    6) 🚀 Enable BBR"
    echo "    7) 🔥 Configure Firewall"
    echo "    0) 🔙 Back"
    echo ""
    msg_ask "Choice: "
    read choice
    
    case $choice in
        1) test_connection ;;
        2) speed_test ;;
        3) port_check ;;
        4) msg_ask "Target: "; read t; ping -c 5 "$t" ;;
        5) msg_ask "Target IP: "; read t; auto_detect_transport "$t" "443" ;;
        6) enable_bbr ;;
        7) msg_ask "Port to open [443]: "; read p; configure_firewall "${p:-443}" ;;
        0) return ;;
    esac
    press_enter
}

menu_backup() {
    print_banner
    echo -e "  ${WHITE}💾 Backup / Restore${NC}"
    echo ""
    echo "    1) 💾 Create Backup"
    echo "    2) 📋 List Backups"
    echo "    3) 🔄 Restore from Backup"
    echo "    4) ⏰ Setup Auto-Backup (cron)"
    echo "    5) ❌ Disable Auto-Backup"
    echo "    0) 🔙 Back"
    echo ""
    msg_ask "Choice: "
    read choice
    
    case $choice in
        1) create_backup ;;
        2) list_backups ;;
        3) restore_backup ;;
        4) setup_auto_backup ;;
        5) disable_auto_backup ;;
        0) return ;;
    esac
    press_enter
}

menu_multi_tunnel() {
    print_banner
    echo -e "  ${WHITE}🔀 Multi-Tunnel Manager${NC}"
    echo ""
    echo "    1) 📋 List All Tunnels"
    echo "    2) ─────────────────────────────────"
    echo "    3) 🌍→🇮🇷  One Foreign → Multiple Iran Servers"
    echo "    4) 🌍🌍→🇮🇷  Multiple Foreign → One Iran Server"
    echo "    5) 🌍↔🇮🇷  One-to-One (simple pair)"
    echo "    6) ─────────────────────────────────"
    echo "    7) ➕ Add Custom Tunnel"
    echo "    8) ➖ Remove Tunnel"
    echo "    9) 🔧 Manage (start/stop/restart/logs)"
    echo "   10) 🔄 Restart All Tunnels"
    echo "    0) 🔙 Back"
    echo ""
    msg_ask "Choice: "
    read choice
    
    case $choice in
        1) list_tunnels ;;
        3) setup_one_foreign_multi_iran ;;
        4) setup_multi_foreign_one_iran ;;
        5) setup_one_to_one ;;
        7) add_tunnel ;;
        8) remove_tunnel ;;
        9) manage_tunnel ;;
        10) manage_tunnel_all_restart ;;
        0) return ;;
    esac
    press_enter
}

# ─── Scenario: One Foreign → Multiple Iran ────────
setup_one_foreign_multi_iran() {
    print_banner
    echo -e "  ${WHITE}🌍→🇮🇷  One Foreign Server → Multiple Iran Servers${NC}"
    echo ""
    echo -e "  ${DIM}Architecture:${NC}"
    echo -e "  ${DIM}  Foreign Server (this) ← Iran-1, Iran-2, Iran-3 connect here${NC}"
    echo -e "  ${DIM}  Each Iran server gets its own SOCKS5 port${NC}"
    echo ""
    echo "    1) 🌍 Setup THIS as Foreign Server (accepts multiple clients)"
    echo "    2) 🇮🇷 Setup THIS as one of the Iran Clients"
    echo "    3) 📋 Show connection info for Iran clients"
    echo "    0) 🔙 Back"
    echo ""
    msg_ask "Choice: "
    read choice
    
    case $choice in
        1) setup_foreign_multi_accept ;;
        2) setup_iran_connect_to_foreign ;;
        3) show_foreign_client_info ;;
        0) return ;;
    esac
}

setup_foreign_multi_accept() {
    echo ""
    msg_info "Setting up Foreign Server (accepts multiple Iran clients)"
    echo ""
    msg_ask "Listen port [443]: "
    read port
    port=${port:-443}
    
    local password=$(generate_password)
    msg_ask "Shared password (all Iran clients use this) [auto]: "
    read user_pass
    [ -n "$user_pass" ] && password="$user_pass"
    
    echo "    1) reality  2) kcp  3) wsmux  4) tcpmux  5) shadowtls"
    msg_ask "Transport [1]: "
    read tc
    local transport="reality"
    case $tc in 2) transport="kcp";; 3) transport="wsmux";; 4) transport="tcpmux";; 5) transport="shadowtls";; esac
    
    # Create main config
    mkdir -p "${CONFIG_DIR}"
    cat > "${CONFIG_DIR}/config.toml" << EOF
# iPShadowT Server - Accepts multiple Iran clients
mode = "server"
log_level = "info"
transport = "${transport}"
bind_addr = "0.0.0.0:${port}"
password = "${password}"

[mux]
concurrency = 16
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

    configure_firewall "$port"
    systemctl restart ${SERVICE_NAME} 2>/dev/null
    
    local server_ip=$(curl -s4 --max-time 5 ifconfig.me 2>/dev/null || echo "YOUR_IP")
    
    echo ""
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "  ${WHITE}  ✅ Foreign Server Ready!${NC}"
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "  ${WHITE}  Give this info to ALL Iran servers:${NC}"
    echo ""
    echo -e "    Server IP:   ${server_ip}"
    echo -e "    Port:        ${port}"
    echo -e "    Transport:   ${transport}"
    echo -e "    Password:    ${password}"
    echo ""
    echo -e "  ${DIM}  Each Iran server runs the manager and selects:${NC}"
    echo -e "  ${DIM}  Multi-Tunnel → One Foreign → Multiple Iran → Setup Iran Client${NC}"
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

setup_iran_connect_to_foreign() {
    echo ""
    msg_info "Setting up Iran Client (connects to Foreign server)"
    echo ""
    
    msg_ask "Tunnel name (e.g., main, backup) [main]: "
    read tname
    tname=${tname:-main}
    
    msg_ask "Foreign server IP: "
    read remote_ip
    if ! validate_ip "$remote_ip"; then msg_error "Invalid IP"; return; fi
    
    msg_ask "Foreign server port [443]: "
    read port
    port=${port:-443}
    
    msg_ask "Password (from foreign server): "
    read password
    if [ -z "$password" ]; then msg_error "Password required"; return; fi
    
    echo "    1) reality  2) kcp  3) wsmux  4) tcpmux  5) shadowtls"
    msg_ask "Transport (must match server) [1]: "
    read tc
    local transport="reality"
    case $tc in 2) transport="kcp";; 3) transport="wsmux";; 4) transport="tcpmux";; 5) transport="shadowtls";; esac
    
    msg_ask "SOCKS5 port [1080]: "
    read socks_port
    socks_port=${socks_port:-1080}
    
    local conf_file="${CONFIG_DIR}/config.toml"
    local svc_name="${SERVICE_NAME}"
    if [ "$tname" != "main" ]; then
        conf_file="${CONFIG_DIR}/tunnel-${tname}.toml"
        svc_name="${SERVICE_NAME}-${tname}"
    fi
    
    mkdir -p "${CONFIG_DIR}"
    cat > "$conf_file" << EOF
# iPShadowT Client - ${tname}
# Connects to Foreign: ${remote_ip}:${port}
mode = "client"
log_level = "info"
transport = "${transport}"
remote_addr = "${remote_ip}:${port}"
password = "${password}"

[mux]
concurrency = 4

[anti_dpi]
enabled = true
utls_fingerprint = "chrome"
fragment = true
fragment_size = "40-80"

[heartbeat]
enabled = true
interval = 20
timeout = 40

[performance]
nodelay = true
keepalive = 15

[[forwards]]
name = "socks5-${tname}"
type = "socks5"
listen = "0.0.0.0:${socks_port}"
EOF

    # Create service if not main
    if [ "$tname" != "main" ]; then
        cat > "/etc/systemd/system/${svc_name}.service" << EOF
[Unit]
Description=iPShadowT Tunnel - ${tname}
After=network.target
[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BINARY_NAME} -c ${conf_file}
Restart=always
RestartSec=5
LimitNOFILE=65535
[Install]
WantedBy=multi-user.target
EOF
        systemctl daemon-reload
        systemctl enable "$svc_name" > /dev/null 2>&1
    fi
    
    systemctl restart "$svc_name"
    sleep 2
    
    if systemctl is-active --quiet "$svc_name"; then
        msg_info "✅ Tunnel '${tname}' connected! SOCKS5 on port ${socks_port}"
    else
        msg_error "Failed. Check: journalctl -u ${svc_name}"
    fi
}

show_foreign_client_info() {
    if [ ! -f "${CONFIG_DIR}/config.toml" ]; then
        msg_error "No config found. Setup foreign server first."
        return
    fi
    local server_ip=$(curl -s4 --max-time 5 ifconfig.me 2>/dev/null || echo "YOUR_IP")
    local port=$(grep '^bind_addr' ${CONFIG_DIR}/config.toml | grep -oP ':\K[0-9]+')
    local password=$(grep '^password' ${CONFIG_DIR}/config.toml | cut -d'"' -f2)
    local transport=$(grep '^transport' ${CONFIG_DIR}/config.toml | cut -d'"' -f2)
    
    echo ""
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "  ${WHITE}  📋 Info for Iran Clients:${NC}"
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "    Server IP:   ${server_ip}"
    echo -e "    Port:        ${port}"
    echo -e "    Transport:   ${transport}"
    echo -e "    Password:    ${password}"
    echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

# ─── Scenario: Multiple Foreign → One Iran ────────
setup_multi_foreign_one_iran() {
    print_banner
    echo -e "  ${WHITE}🌍🌍→🇮🇷  Multiple Foreign Servers → One Iran Server${NC}"
    echo ""
    echo -e "  ${DIM}Architecture:${NC}"
    echo -e "  ${DIM}  This Iran server connects to multiple Foreign servers${NC}"
    echo -e "  ${DIM}  Each connection gets its own SOCKS5 port${NC}"
    echo -e "  ${DIM}  Use for: load balancing, redundancy, different locations${NC}"
    echo ""
    echo "    1) ➕ Add Foreign Server Connection"
    echo "    2) 📋 List All Connections"
    echo "    3) ➖ Remove Connection"
    echo "    4) 🔄 Restart All"
    echo "    0) 🔙 Back"
    echo ""
    msg_ask "Choice: "
    read choice
    
    case $choice in
        1) add_foreign_connection ;;
        2) list_tunnels ;;
        3) remove_tunnel ;;
        4) manage_tunnel_all_restart ;;
        0) return ;;
    esac
}

add_foreign_connection() {
    echo ""
    
    # Count existing tunnels for auto port assignment
    local count=$(ls ${CONFIG_DIR}/tunnel-*.toml 2>/dev/null | wc -l)
    local next_port=$((1080 + count))
    
    msg_ask "Name for this connection (e.g., germany, usa, nl): "
    read cname
    if [ -z "$cname" ]; then msg_error "Name required"; return; fi
    cname=$(echo "$cname" | tr -cd 'a-zA-Z0-9_-')
    
    msg_ask "Foreign server IP: "
    read remote_ip
    
    msg_ask "Port [443]: "
    read port
    port=${port:-443}
    
    msg_ask "Password: "
    read password
    if [ -z "$password" ]; then msg_error "Password required"; return; fi
    
    echo "    1) reality  2) kcp  3) wsmux  4) tcpmux  5) shadowtls"
    msg_ask "Transport [1]: "
    read tc
    local transport="reality"
    case $tc in 2) transport="kcp";; 3) transport="wsmux";; 4) transport="tcpmux";; 5) transport="shadowtls";; esac
    
    msg_ask "SOCKS5 port [${next_port}]: "
    read socks_port
    socks_port=${socks_port:-$next_port}
    
    local conf_file="${CONFIG_DIR}/tunnel-${cname}.toml"
    local svc_name="${SERVICE_NAME}-${cname}"
    
    mkdir -p "${CONFIG_DIR}"
    cat > "$conf_file" << EOF
# iPShadowT Client - ${cname}
# Foreign: ${remote_ip}:${port}
mode = "client"
log_level = "info"
transport = "${transport}"
remote_addr = "${remote_ip}:${port}"
password = "${password}"

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

[performance]
nodelay = true
keepalive = 15

[[forwards]]
name = "socks5-${cname}"
type = "socks5"
listen = "0.0.0.0:${socks_port}"
EOF

    cat > "/etc/systemd/system/${svc_name}.service" << EOF
[Unit]
Description=iPShadowT - ${cname} (${remote_ip})
After=network.target
[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BINARY_NAME} -c ${conf_file}
Restart=always
RestartSec=5
LimitNOFILE=65535
[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable "$svc_name" > /dev/null 2>&1
    systemctl start "$svc_name"
    sleep 2
    
    if systemctl is-active --quiet "$svc_name"; then
        msg_info "✅ Connection '${cname}' active! SOCKS5 → 0.0.0.0:${socks_port}"
    else
        msg_error "Failed. Check: journalctl -u ${svc_name}"
    fi
}

# ─── Scenario: One-to-One (Simple Pair) ──────────
setup_one_to_one() {
    print_banner
    echo -e "  ${WHITE}🌍↔🇮🇷  One-to-One (Simple Pair)${NC}"
    echo ""
    echo -e "  ${DIM}Architecture:${NC}"
    echo -e "  ${DIM}  One Foreign Server ↔ One Iran Server${NC}"
    echo -e "  ${DIM}  Simplest setup, recommended for beginners${NC}"
    echo ""
    echo "    1) 🌍 Setup THIS as Foreign Server"
    echo "    2) 🇮🇷 Setup THIS as Iran Client"
    echo "    0) 🔙 Back"
    echo ""
    msg_ask "Choice: "
    read choice
    
    case $choice in
        1) configure_foreign ;;
        2) configure_iran ;;
        0) return ;;
    esac
}

manage_tunnel_all_restart() {
    for conf in ${CONFIG_DIR}/tunnel-*.toml ${CONFIG_DIR}/config.toml; do
        [ -f "$conf" ] || continue
        local name=$(basename "$conf" .toml)
        local svc="${SERVICE_NAME}"
        [ "$name" != "config" ] && svc="${SERVICE_NAME}-${name#tunnel-}"
        systemctl restart "$svc" 2>/dev/null && msg_info "Restarted: $svc"
    done
}

# ─── Main Menu ────────────────────────────────────
main_menu() {
    while true; do
        print_banner
        
        # Show quick status
        if is_installed; then
            local status_icon="🔴"
            is_running && status_icon="🟢"
            echo -e "  ${DIM}Status: ${status_icon} $(get_current_version)${NC}"
        else
            echo -e "  ${DIM}Status: ⚪ Not installed${NC}"
        fi
        echo ""
        
        echo "    1) 🚀 Install iPShadowT"
        echo "    2) ⚙️  Configure Tunnel"
        echo "    3) 🔄 Start / Stop / Restart"
        echo "    4) 📊 Status & Monitoring"
        echo "    5) 🔑 Key Management"
        echo "    6) 🌐 Network Tools"
        echo "    7) 💾 Backup / Restore"
        echo "    8) 🔄 Update"
        echo "    9) 🗑️  Uninstall"
        echo "   10) 🔀 Multi-Tunnel Manager"
        echo "    0) ❌ Exit"
        echo ""
        msg_ask "Choice: "
        read choice
        
        case $choice in
            1) menu_install ;;
            2) menu_configure ;;
            3) menu_service ;;
            4) menu_monitoring ;;
            5) menu_keys ;;
            6) menu_network ;;
            7) menu_backup ;;
            8) update_ipshadowt; press_enter ;;
            9) uninstall_ipshadowt; press_enter ;;
            10) menu_multi_tunnel ;;
            0) echo ""; msg_info "Goodbye!"; exit 0 ;;
            *) msg_error "Invalid choice" ;;
        esac
    done
}

# ─── Entry Point ─────────────────────────────────
check_root
detect_arch
detect_os_type
main_menu
