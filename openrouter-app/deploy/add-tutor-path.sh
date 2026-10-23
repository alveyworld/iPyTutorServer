#!/usr/bin/env bash
# Serve iPyTutor under a path on a site nginx already serves over HTTPS, for example
# https://intebg.org/tutor/ next to the iPyIntervu interview app.
#
#   sudo ./deploy/add-tutor-path.sh intebg.org           serve it at https://intebg.org/tutor/
#   sudo ./deploy/add-tutor-path.sh intebg.org /study    serve it at another path
#
# Run deploy/install.sh first, so the tutor is running on 127.0.0.1:8090. This script:
#   1. installs deploy/nginx-tutor-location.conf as an nginx snippet;
#   2. finds the domain's HTTPS server block (the one certbot set up) and adds an include
#      line for the snippet, after backing up that config file;
#   3. checks the whole nginx config with nginx -t, and if the check fails, puts the old
#      config back and stops;
#   4. reloads nginx and checks that the tutor answers at the new path.
# Running it again is safe: an include that is already there is left alone.
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo $0 <domain> [path]" >&2
  exit 1
fi
if [[ $# -lt 1 ]]; then
  echo "Usage: sudo $0 <domain> [path]" >&2
  echo "Example: sudo $0 intebg.org" >&2
  exit 1
fi

DOMAIN="$1"
TUTOR_PATH="${2:-/tutor}"
TUTOR_PATH="/${TUTOR_PATH#/}"
TUTOR_PATH="${TUTOR_PATH%/}"
if [[ ! "$TUTOR_PATH" =~ ^/[A-Za-z0-9._-]+$ ]]; then
  echo "The path must be one segment such as /tutor, not \"$TUTOR_PATH\"." >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SNIPPET_NAME="ipytutor-${TUTOR_PATH#/}.conf"
SNIPPET="/etc/nginx/snippets/$SNIPPET_NAME"
INCLUDE_LINE="include snippets/$SNIPPET_NAME;"

command -v nginx >/dev/null || { echo "nginx is not installed." >&2; exit 1; }
if ! curl -sf http://127.0.0.1:8090/healthz >/dev/null; then
  echo "iPyTutor is not responding on 127.0.0.1:8090. Run first: sudo $SCRIPT_DIR/install.sh" >&2
  exit 1
fi

# 1. The snippet, with the chosen path.
mkdir -p /etc/nginx/snippets
sed "s#/tutor#${TUTOR_PATH}#g" "$SCRIPT_DIR/nginx-tutor-location.conf" > "$SNIPPET"
chmod 644 "$SNIPPET"
echo "Installed $SNIPPET"

# 2. The domain's HTTPS server block.
CANDIDATES=()
for f in /etc/nginx/sites-enabled/* /etc/nginx/conf.d/*.conf; do
  [[ -f "$f" ]] && CANDIDATES+=("$(readlink -f "$f")")
done
FOUND=()
if [[ ${#CANDIDATES[@]} -gt 0 ]]; then
  while IFS= read -r line; do FOUND+=("$line"); done \
    < <(python3 "$SCRIPT_DIR/nginx_include.py" find "$DOMAIN" "$INCLUDE_LINE" "${CANDIDATES[@]}")
fi

manual_steps() {
  echo "Add this line inside the server { ... } block for $DOMAIN that has \"listen 443\":" >&2
  echo "    $INCLUDE_LINE" >&2
  echo "then run: sudo nginx -t && sudo systemctl reload nginx" >&2
}

if [[ ${#FOUND[@]} -eq 0 ]]; then
  echo "Found no HTTPS server block for $DOMAIN in /etc/nginx/sites-enabled or /etc/nginx/conf.d." >&2
  echo "If the site has no HTTPS yet, set that up first (for example with certbot)." >&2
  manual_steps
  exit 1
fi
if [[ ${#FOUND[@]} -gt 1 ]]; then
  echo "Found more than one HTTPS server block for $DOMAIN:" >&2
  printf '  %s\n' "${FOUND[@]%%$'\t'*}" >&2
  manual_steps
  exit 1
fi

CONF="${FOUND[0]%%$'\t'*}"
ALREADY="${FOUND[0]##*$'\t'}"
BACKUP=""
if [[ "$ALREADY" == "yes" ]]; then
  echo "$CONF already includes the snippet."
else
  BACKUP="$CONF.bak.$(date +%s)"
  cp -p "$CONF" "$BACKUP"
  python3 "$SCRIPT_DIR/nginx_include.py" edit "$DOMAIN" "$INCLUDE_LINE" "$CONF"
  echo "Added \"$INCLUDE_LINE\" to $CONF (backup: $BACKUP)"
fi

# 3. Check before reloading; undo the edit if nginx rejects it.
if ! nginx -t; then
  if [[ -n "$BACKUP" ]]; then
    cp -p "$BACKUP" "$CONF"
    echo "nginx rejected the change, so $CONF was put back as it was." >&2
  fi
  exit 1
fi

# 4. Reload and check the tutor through nginx.
systemctl reload nginx
sleep 1
if curl -sf --resolve "$DOMAIN:443:127.0.0.1" "https://$DOMAIN$TUTOR_PATH/healthz" >/dev/null; then
  echo "iPyTutor is live at https://$DOMAIN$TUTOR_PATH/"
else
  echo "nginx reloaded, but https://$DOMAIN$TUTOR_PATH/healthz did not answer. Check: sudo journalctl -u ipytutor -n 50" >&2
  exit 1
fi
