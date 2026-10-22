#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOMAIN="${1:-aalang.org}"

if [[ -f /etc/nginx/sites-available/aalang ]]; then
  cp /etc/nginx/sites-available/aalang "/etc/nginx/sites-available/aalang.bak.$(date +%s)"
fi

install -m 644 "$SCRIPT_DIR/nginx-aalang.conf" /etc/nginx/sites-available/aalang
ln -sf /etc/nginx/sites-available/aalang /etc/nginx/sites-enabled/aalang
rm -f /etc/nginx/sites-enabled/ipyintervu

nginx -t
systemctl reload nginx

echo "nginx now proxies https://${DOMAIN}/ to 127.0.0.1:8080"
echo "Test: curl https://${DOMAIN}/healthz"
