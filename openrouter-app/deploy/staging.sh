#!/usr/bin/env bash
# Run a staging copy of the app from the current source tree, next to production.
#
#   ./deploy/staging.sh          build, install, and start staging on 127.0.0.1:8081
#   ./deploy/staging.sh stop     stop staging
#   ./deploy/staging.sh logs     show recent staging logs
#
# Staging uses the production secrets (/etc/openrouter-app/env) and runs as the
# openrouter user, but listens only on localhost. Reach it by running this on your
# own computer (not the server), then open http://localhost:8081 there:
#   ssh -N -L 8081:127.0.0.1:8081 manager@aalang.org
# Set STAGING_SSH_HOST to change the host shown in the printed instructions.
# Staging runs the D5 engine by default; IPY_ENGINE=old ./deploy/staging.sh runs the original one.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UNIT="openrouter-staging"
BIN="/usr/local/bin/openrouter-app-staging"
PORT="${STAGING_PORT:-8081}"
SSH_HOST="${STAGING_SSH_HOST:-aalang.org}"
ENGINE="${IPY_ENGINE:-d5}"

case "${1:-start}" in
  stop)
    sudo systemctl stop "$UNIT" 2>/dev/null || true
    echo "Staging stopped."
    exit 0
    ;;
  logs)
    sudo journalctl -u "$UNIT" --no-pager -n 80
    exit 0
    ;;
  start)
    ;;
  *)
    echo "usage: $0 [start|stop|logs]" >&2
    exit 2
    ;;
esac

cd "$ROOT_DIR"
BUILD_OUT="$(mktemp)"
trap 'rm -f "$BUILD_OUT"' EXIT

echo "Branch: $(git rev-parse --abbrev-ref HEAD) ($(git rev-parse --short HEAD)$(git diff --quiet || echo ' + uncommitted changes'))"
echo "Engine: $ENGINE"
go test ./...
./deploy/build.sh "$BUILD_OUT"

sudo systemctl stop "$UNIT" 2>/dev/null || true
sudo systemctl reset-failed "$UNIT" 2>/dev/null || true
sudo install -m 755 "$BUILD_OUT" "$BIN"
sudo systemd-run --unit="$UNIT" --uid=openrouter --gid=openrouter \
  -p EnvironmentFile=/etc/openrouter-app/env \
  -E AUTH_PRIVATE_KEY_FILE=/etc/openrouter-app/auth-private-key.pem \
  -E LISTEN_ADDR=127.0.0.1 -E PORT="$PORT" -E IPY_ENGINE="$ENGINE" \
  "$BIN"

for _ in 1 2 3 4 5 6 7 8 9 10; do
  if curl -fs "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1; then
    echo "Staging is up on 127.0.0.1:$PORT with engine $ENGINE (production on 8080 is untouched)."
    echo
    echo "To use it, run this in a terminal on your own computer (not this server)"
    echo "and leave it open; it prints nothing while the tunnel is up:"
    echo "  ssh -N -L $PORT:127.0.0.1:$PORT $(whoami)@$SSH_HOST"
    echo "Then browse to http://localhost:$PORT"
    echo
    echo "Rerun ./deploy/staging.sh after each change. Logs: ./deploy/staging.sh logs"
    echo "Stop it with: ./deploy/staging.sh stop"
    exit 0
  fi
  sleep 0.5
done

echo "Staging did not start. Recent logs:" >&2
sudo journalctl -u "$UNIT" --no-pager -n 30 >&2
exit 1
