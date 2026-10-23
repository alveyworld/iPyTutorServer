#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo $0 [path-to-binary]" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BINARY_SRC="${1:-$ROOT_DIR/ipytutor}"
ENV_FILE="/etc/ipytutor/env"
PRIVATE_KEY="/etc/ipytutor/auth-private-key.pem"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing $ENV_FILE. Run: sudo $SCRIPT_DIR/install-secrets.sh" >&2
  exit 1
fi

if [[ ! -f "$PRIVATE_KEY" ]]; then
  echo "Missing $PRIVATE_KEY. Run: sudo $SCRIPT_DIR/install-secrets.sh" >&2
  exit 1
fi

if ! id ipytutor >/dev/null 2>&1; then
  useradd --system --home /var/lib/ipytutor --shell /usr/sbin/nologin ipytutor
fi

cd "$ROOT_DIR"
"$SCRIPT_DIR/build.sh" "$BINARY_SRC"

install -m 755 "$BINARY_SRC" /usr/local/bin/ipytutor
install -m 644 "$SCRIPT_DIR/ipytutor.service" /etc/systemd/system/ipytutor.service

chown root:root "$ENV_FILE"
chmod 600 "$ENV_FILE"
chown root:ipytutor "$PRIVATE_KEY"
chmod 640 "$PRIVATE_KEY"

systemctl daemon-reload
systemctl enable ipytutor
systemctl restart ipytutor
systemctl --no-pager status ipytutor

echo "iPyTutor is running on 127.0.0.1:8090"
echo "Health check: curl http://127.0.0.1:8090/healthz"
echo "For HTTPS on its own domain: sudo $SCRIPT_DIR/setup-https.sh <your-domain> [email]"
echo "Or at a path on a site nginx already serves: sudo $SCRIPT_DIR/add-tutor-path.sh <domain> [/tutor]"
