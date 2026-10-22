#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo $0 [path-to-binary]" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BINARY_SRC="${1:-$ROOT_DIR/openrouter-app}"
ENV_FILE="/etc/openrouter-app/env"
PRIVATE_KEY="/etc/openrouter-app/auth-private-key.pem"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing $ENV_FILE. Run: sudo $SCRIPT_DIR/install-secrets.sh" >&2
  exit 1
fi

if [[ ! -f "$PRIVATE_KEY" ]]; then
  echo "Missing $PRIVATE_KEY. Run: sudo $SCRIPT_DIR/install-secrets.sh" >&2
  exit 1
fi

if ! id openrouter >/dev/null 2>&1; then
  useradd --system --home /var/lib/openrouter --shell /usr/sbin/nologin openrouter
fi

mkdir -p /var/lib/openrouter
chown openrouter:openrouter /var/lib/openrouter

cd "$ROOT_DIR"
"$SCRIPT_DIR/build.sh" "$BINARY_SRC"

install -m 755 "$BINARY_SRC" /usr/local/bin/openrouter-app
install -m 644 "$SCRIPT_DIR/openrouter-app.service" /etc/systemd/system/openrouter-app.service

chown root:root "$ENV_FILE"
chmod 600 "$ENV_FILE"
chown root:openrouter "$PRIVATE_KEY"
chmod 640 "$PRIVATE_KEY"

systemctl daemon-reload
systemctl enable openrouter-app
systemctl restart openrouter-app
systemctl --no-pager status openrouter-app

echo "iPyInterVu is running on 127.0.0.1:8080"
echo "Health check: curl http://127.0.0.1:8080/healthz"
echo "For HTTPS: sudo $SCRIPT_DIR/setup-https.sh <your-domain> [email]"
