#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo $0 [path-to-private-key.pem]" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PRIVATE_KEY_SRC="${1:-$ROOT_DIR/env/ipyintervu-key.pem}"

ENV_FILE="/etc/openrouter-app/env"
PRIVATE_KEY_DST="/etc/openrouter-app/auth-private-key.pem"

mkdir -p /etc/openrouter-app

if ! id openrouter >/dev/null 2>&1; then
  useradd --system --home /var/lib/openrouter --shell /usr/sbin/nologin openrouter
  mkdir -p /var/lib/openrouter
  chown openrouter:openrouter /var/lib/openrouter
fi

if [[ -f "$ENV_FILE" ]]; then
  echo "Found existing $ENV_FILE (will not overwrite OPENROUTER_API_KEY)."
else
  if [[ -z "${OPENROUTER_API_KEY:-}" ]]; then
    read -r -s -p "Enter OPENROUTER_API_KEY: " OPENROUTER_API_KEY
    echo
  fi
  if [[ -z "${OPENROUTER_API_KEY}" ]]; then
    echo "OPENROUTER_API_KEY is required." >&2
    exit 1
  fi
  printf 'OPENROUTER_API_KEY=%s\n' "$OPENROUTER_API_KEY" > "$ENV_FILE"
  echo "Wrote $ENV_FILE"
fi

if [[ -f "$PRIVATE_KEY_DST" ]]; then
  echo "Found existing $PRIVATE_KEY_DST (will not overwrite private key)."
elif [[ ! -f "$PRIVATE_KEY_SRC" ]]; then
  echo "Private key not found: $PRIVATE_KEY_SRC" >&2
  echo "Generate one with:" >&2
  echo "  openssl genpkey -algorithm RSA -out $ROOT_DIR/env/ipyintervu-key.pem -pkeyopt rsa_keygen_bits:2048" >&2
  echo "  openssl rsa -in $ROOT_DIR/env/ipyintervu-key.pem -pubout -out $ROOT_DIR/env/ipyintervu-pub.pem" >&2
  exit 1
else
  install -m 600 -o root -g root "$PRIVATE_KEY_SRC" "$PRIVATE_KEY_DST"
  echo "Installed $PRIVATE_KEY_DST from $PRIVATE_KEY_SRC"
fi

chown root:root "$ENV_FILE"
chmod 600 "$ENV_FILE"
chown root:openrouter "$PRIVATE_KEY_DST"
chmod 640 "$PRIVATE_KEY_DST"

echo "Secrets installed."
echo "Distribute the matching public key to users out-of-band."
