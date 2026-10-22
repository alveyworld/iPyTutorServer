#!/usr/bin/env bash
# Run iPyInterVu on this machine for development, at http://localhost:8080
#
#   tools/run-local.sh                  D5 engine
#   IPY_ENGINE=old tools/run-local.sh   original engine
#   PORT=8090 tools/run-local.sh        another port
#
# The OpenRouter key comes from OPENROUTER_API_KEY or ~/.openrouter-env. Login uses a
# development RSA keypair kept in ~/.ipyintervu-dev (created on first run, separate from
# the production key); paste its public key into the login screen.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEV_DIR="$HOME/.ipyintervu-dev"
PRIVATE_KEY="$DEV_DIR/auth-private-key.pem"
PUBLIC_KEY="$DEV_DIR/auth-public-key.pem"

if [[ ! -f "$PRIVATE_KEY" ]]; then
  mkdir -p "$DEV_DIR"
  chmod 700 "$DEV_DIR"
  openssl genpkey -algorithm RSA -out "$PRIVATE_KEY" -pkeyopt rsa_keygen_bits:2048 2>/dev/null
  openssl rsa -in "$PRIVATE_KEY" -pubout -out "$PUBLIC_KEY" 2>/dev/null
  chmod 600 "$PRIVATE_KEY"
  echo "Created a development login keypair in $DEV_DIR"
fi

if [[ -z "${OPENROUTER_API_KEY:-}" && -f "$HOME/.openrouter-env" ]]; then
  # shellcheck disable=SC1091
  source "$HOME/.openrouter-env"
fi
if [[ -z "${OPENROUTER_API_KEY:-}" ]]; then
  echo "OPENROUTER_API_KEY is not set and ~/.openrouter-env does not set it." >&2
  exit 1
fi

export OPENROUTER_API_KEY
export AUTH_PRIVATE_KEY_FILE="$PRIVATE_KEY"
export IPY_ENGINE="${IPY_ENGINE:-d5}"
export PORT="${PORT:-8080}"
export LISTEN_ADDR="127.0.0.1"

echo "Engine: $IPY_ENGINE"
echo "Open http://localhost:$PORT and paste the public key from:"
echo "  $PUBLIC_KEY"
echo "(copy it with: pbcopy < $PUBLIC_KEY)"
echo "Logs appear below; [d5] lines show moves, timings and post-check hits. Ctrl-C stops the server."
echo

cd "$ROOT_DIR/openrouter-app"
exec go run .
