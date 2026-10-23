#!/usr/bin/env bash
# Run iPyTutor on this machine for development, at http://localhost:8080
#
#   tools/run-local.sh                 real model calls through OpenRouter
#   FAKE_LLM=1 tools/run-local.sh      canned replies from tools/fake-openrouter, no API key needed
#   PORT=8090 tools/run-local.sh       another port
#
# The OpenRouter key comes from OPENROUTER_API_KEY or ~/.openrouter-env. Login uses a
# development RSA keypair kept in ~/.ipytutor-dev (created on first run, separate from
# the production key); paste its public key into the login screen.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEV_DIR="$HOME/.ipytutor-dev"
PRIVATE_KEY="$DEV_DIR/auth-private-key.pem"
PUBLIC_KEY="$DEV_DIR/auth-public-key.pem"
FAKE_ADDR="127.0.0.1:${FAKE_PORT:-8099}"

if [[ ! -f "$PRIVATE_KEY" ]]; then
  mkdir -p "$DEV_DIR"
  chmod 700 "$DEV_DIR"
  openssl genpkey -algorithm RSA -out "$PRIVATE_KEY" -pkeyopt rsa_keygen_bits:2048 2>/dev/null
  openssl rsa -in "$PRIVATE_KEY" -pubout -out "$PUBLIC_KEY" 2>/dev/null
  chmod 600 "$PRIVATE_KEY"
  echo "Created a development login keypair in $DEV_DIR"
fi

if [[ "${FAKE_LLM:-}" == "1" ]]; then
  (cd "$ROOT_DIR/tools/fake-openrouter" && go build -o "$DEV_DIR/fake-openrouter" main.go)
  "$DEV_DIR/fake-openrouter" -addr "$FAKE_ADDR" &
  FAKE_PID=$!
  trap 'kill $FAKE_PID 2>/dev/null || true' EXIT
  export OPENROUTER_URL="http://$FAKE_ADDR"
  export OPENROUTER_API_KEY="fake-key"
  echo "Using canned replies from the fake OpenRouter on $FAKE_ADDR"
else
  if [[ -z "${OPENROUTER_API_KEY:-}" && -f "$HOME/.openrouter-env" ]]; then
    # shellcheck disable=SC1091
    source "$HOME/.openrouter-env"
  fi
  if [[ -z "${OPENROUTER_API_KEY:-}" ]]; then
    echo "OPENROUTER_API_KEY is not set and ~/.openrouter-env does not set it." >&2
    echo "Set it, or run with FAKE_LLM=1 to try the tutor with canned replies." >&2
    exit 1
  fi
fi

export OPENROUTER_API_KEY
export AUTH_PRIVATE_KEY_FILE="$PRIVATE_KEY"
export PORT="${PORT:-8080}"
export LISTEN_ADDR="127.0.0.1"
# Local runs use plain http, so the session cookie can't be Secure.
export SECURE_COOKIES=false

echo "Open http://localhost:$PORT and paste the public key from:"
echo "  $PUBLIC_KEY"
echo "(copy it with: pbcopy < $PUBLIC_KEY)"
echo "Logs appear below; [tutor] lines show each turn and model call. Ctrl-C stops the server."
echo

cd "$ROOT_DIR/openrouter-app"
go run .
