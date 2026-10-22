#!/usr/bin/env bash
# Stop the web server that holds port 80 (nginx) and confirm the port is free.
#
# Usage: sudo ./shutdown.sh [port]
#   port  defaults to 80
#
# The openrouter-app service (localhost:8080) is left running; only nginx,
# which fronts it on ports 80/443, is stopped.
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo $0 [port]" >&2
  exit 1
fi

PORT="${1:-80}"
SERVICE="nginx"

if systemctl is-active --quiet "$SERVICE"; then
  echo "Stopping $SERVICE..."
  systemctl stop "$SERVICE"
else
  echo "$SERVICE is not running."
fi

# Give the kernel a moment to release the socket.
for _ in 1 2 3 4 5; do
  if ! ss -ltn "( sport = :$PORT )" | tail -n +2 | grep -q .; then
    break
  fi
  sleep 1
done

LISTENERS="$(ss -ltnp "( sport = :$PORT )" | tail -n +2)"

if [[ -z "$LISTENERS" ]]; then
  echo "Port $PORT is free."
  exit 0
else
  echo "Port $PORT is NOT free. Still listening:"
  echo "$LISTENERS"
  exit 2
fi
