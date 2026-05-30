#!/bin/bash
# iPShadowT - Uninstall Script

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Please run as root${NC}"
    exit 1
fi

echo "Stopping service..."
systemctl stop ipshadowt 2>/dev/null || true
systemctl disable ipshadowt 2>/dev/null || true

echo "Removing files..."
rm -f /usr/local/bin/ipshadowt
rm -f /etc/systemd/system/ipshadowt.service
rm -f /etc/sysctl.d/99-ipshadowt.conf

systemctl daemon-reload
sysctl --system > /dev/null 2>&1

echo ""
read -p "Remove config (/etc/ipshadowt)? [y/N]: " REMOVE_CONFIG
if [ "$REMOVE_CONFIG" = "y" ] || [ "$REMOVE_CONFIG" = "Y" ]; then
    rm -rf /etc/ipshadowt
    echo -e "${GREEN}Config removed${NC}"
fi

echo -e "${GREEN}iPShadowT uninstalled${NC}"
