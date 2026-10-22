#!/usr/bin/env bash
# Start nginx as the systemd daemon it normally runs as, and confirm it is
# listening on the expected port. Companion to shutdown.sh.
#
# Usage: sudo ./startup.sh [port]
#   port  defaults to 80
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo $0 [port]" >&2
  exit 1
fi

PORT="${1:-80}"
SERVICE="nginx"

# Fail early on a bad config rather than leaving the daemon half-started.
if ! nginx -t >/dev/null 2>&1; then
  echo "nginx configuration test failed:" >&2
  nginx -t
  exit 1
fi

if systemctl is-active --quiet "$SERVICE"; then
  echo "$SERVICE is already running."
else
  echo "Starting $SERVICE..."
  systemctl start "$SERVICE"
fi

# Make sure it comes back on reboot, matching how it was installed.
if ! systemctl is-enabled --quiet "$SERVICE"; then
  echo "Enabling $SERVICE at boot..."
  systemctl enable "$SERVICE" >/dev/null
fi

# Give the daemon a moment to bind.
for _ in 1 2 3 4 5; do
  if ss -ltn "( sport = :$PORT )" | tail -n +2 | grep -q .; then
    break
  fi
  sleep 1
done

LISTENERS="$(ss -ltnp "( sport = :$PORT )" | tail -n +2)"

if [[ -n "$LISTENERS" ]]; then
  echo "$SERVICE is running and listening on port $PORT."
  exit 0
else
  echo "$SERVICE started but nothing is listening on port $PORT." >&2
  systemctl status "$SERVICE" --no-pager || true
  exit 2
fi
