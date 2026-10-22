#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo $0 <domain> [email]" >&2
  exit 1
fi

if [[ $# -lt 1 ]]; then
  echo "Usage: sudo $0 <domain> [email]" >&2
  echo "Example: sudo $0 aalang.org admin@aalang.org" >&2
  echo "The domain must already point at this server's public IP." >&2
  exit 1
fi

DOMAIN="$1"
EMAIL="${2:-}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y nginx certbot python3-certbot-nginx

if ! curl -sf http://127.0.0.1:8080/healthz >/dev/null; then
  echo "Go app is not responding on 127.0.0.1:8080." >&2
  echo "Run first: sudo $SCRIPT_DIR/install.sh" >&2
  exit 1
fi

sed "s/DOMAIN_PLACEHOLDER/${DOMAIN}/g" "$SCRIPT_DIR/nginx.conf.template" \
  > /etc/nginx/sites-available/ipyintervu
ln -sf /etc/nginx/sites-available/ipyintervu /etc/nginx/sites-enabled/ipyintervu
rm -f /etc/nginx/sites-enabled/default

nginx -t
systemctl enable nginx
systemctl restart nginx

CERTBOT_ARGS=(--nginx -d "${DOMAIN}" --agree-tos --no-eff-email --redirect)
if [[ -n "${EMAIL}" ]]; then
  CERTBOT_ARGS+=(--email "${EMAIL}")
else
  CERTBOT_ARGS+=(--register-unsafely-without-email)
fi

certbot "${CERTBOT_ARGS[@]}"

nginx -t
systemctl reload nginx

echo "HTTPS is enabled for https://${DOMAIN}/"
echo "Health check: curl https://${DOMAIN}/healthz"
