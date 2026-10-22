#!/usr/bin/env bash
#
# cycle_keys.sh — rotate the iPyInterVu login keypair.
#
# Generates a fresh RSA pair, proves it works end to end against the running
# service, installs it under the SAME filenames the app already expects (no code
# changes), and only then shreds the retired key material — including stray
# copies left in /tmp.
#
# Order matters: nothing is destroyed until the new key has authenticated
# against the live server. If any step fails the old key is restored and the
# service is put back the way it was.
#
# Files touched (names are fixed, do not rename):
#   <repo>/env/ipyintervu-key.pem          private key, working copy
#   <repo>/env/ipyintervu-pub.pem          public key, hand this to students
#   /etc/openrouter-app/auth-private-key.pem   what systemd actually reads
#
# NOTE ON SHRED: shred(1) overwrites in place. On a journalling filesystem
# (ext4), on copy-on-write filesystems (btrfs/zfs), and on SSDs with wear
# levelling it cannot guarantee every old block is gone. It raises the cost of
# recovery; it is not a guarantee. Treat a leaked key as leaked.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

PRIV_REPO="$ROOT_DIR/env/ipyintervu-key.pem"
PUB_REPO="$ROOT_DIR/env/ipyintervu-pub.pem"
PRIV_DEPLOYED="${PRIV_DEPLOYED:-/etc/openrouter-app/auth-private-key.pem}"
SERVICE="${SERVICE:-openrouter-app}"
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
SERVICE_GROUP="${SERVICE_GROUP:-openrouter}"

KEY_BITS=2048
DRY_RUN=0
ASSUME_YES=0
ALL_TMP_PEM=0
HEALTH_TIMEOUT=30

usage() {
  cat <<'USAGE'
Usage: sudo ./deploy/cycle_keys.sh [options]

  --bits N         RSA key size (default 2048; 4096 also works)
  --all-tmp-pem    Shred EVERY *.pem under /tmp, not just copies of the keys
                   being retired. Blunt instrument — other services keep keys
                   in /tmp too. Read the listing before saying yes.
  --dry-run        Generate and self-test a new pair, list exactly what would
                   be replaced and shredded, then discard it. Changes nothing.
                   Runs without root (the deployed key is skipped).
  --yes            Skip the confirmation prompt.
  -h, --help       This text.

Rotating logs every student out; they must paste the new public key
(env/ipyintervu-pub.pem) to sign back in. Interviews in progress are lost.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bits) KEY_BITS="${2:?--bits needs a value}"; shift 2 ;;
    --all-tmp-pem) ALL_TMP_PEM=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    --yes|-y) ASSUME_YES=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

# The three paths the new keypair is installed to. They are REPLACED, never
# swept: after install they hold the new key, so shredding them by path would
# destroy what was just written.
DEST_PATHS=("$PRIV_REPO" "$PUB_REPO" "$PRIV_DEPLOYED")

is_destination() {
  local p="$1" d
  for d in "${DEST_PATHS[@]}"; do
    [[ "$p" == "$d" ]] && return 0
  done
  return 1
}

say()  { printf '%s\n' "$*"; }
step() { printf '\n== %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

need_root() {
  [[ ${EUID} -eq 0 ]] || die "must run as root (sudo $0). Use --dry-run to rehearse without it."
}

for tool in openssl curl shred install systemctl base64 cmp find stat; do
  command -v "$tool" >/dev/null 2>&1 || die "required tool not found: $tool"
done

[[ -d "$ROOT_DIR/env" ]] || die "no env directory at $ROOT_DIR/env"

# Work and backup dirs are root-only and always cleaned up, even on failure.
WORK="$(mktemp -d)"
BACKUP="$(mktemp -d)"
chmod 700 "$WORK" "$BACKUP"

shred_file() {
  local f="$1"
  [[ -f "$f" ]] || return 0
  if shred -u -z -n 3 "$f" 2>/dev/null; then
    say "  shredded  $f"
  else
    rm -f "$f" 2>/dev/null && warn "shred failed, plain-removed $f" || warn "could not remove $f"
  fi
}

shred_dir() {
  local d="$1"
  [[ -d "$d" ]] || return 0
  find "$d" -type f -print0 2>/dev/null | while IFS= read -r -d '' f; do
    shred -u -z -n 3 "$f" 2>/dev/null || rm -f "$f" 2>/dev/null || true
  done
  rm -rf "$d" 2>/dev/null || true
}

cleanup() { shred_dir "$WORK"; shred_dir "$BACKUP"; }
trap cleanup EXIT

sha() { sha256sum "$1" 2>/dev/null | awk '{print $1}'; }

json_field() { sed -n 's/.*"'"$1"'"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'; }

oaep_encrypt() { # <pubkey> <infile> <outfile>
  openssl pkeyutl -encrypt -pubin -inkey "$1" \
    -pkeyopt rsa_padding_mode:oaep \
    -pkeyopt rsa_oaep_md:sha256 \
    -pkeyopt rsa_mgf1_md:sha256 \
    -in "$2" -out "$3"
}

# ---------------------------------------------------------------- old key set
# Hashes of the key material being retired, so the sweep can identify copies of
# THESE keys rather than blindly deleting anything named *.pem.
step "Cataloguing current keys"
OLD_HASHES=()
for f in "$PRIV_REPO" "$PUB_REPO"; do
  if [[ -f "$f" ]]; then
    OLD_HASHES+=("$(sha "$f")")
    say "  current: $f"
  else
    say "  absent : $f (will be created)"
  fi
done
if [[ -r "$PRIV_DEPLOYED" ]]; then
  OLD_HASHES+=("$(sha "$PRIV_DEPLOYED")")
  say "  current: $PRIV_DEPLOYED"
elif [[ -e "$PRIV_DEPLOYED" ]]; then
  say "  present but unreadable: $PRIV_DEPLOYED (need root to catalogue it)"
else
  say "  absent : $PRIV_DEPLOYED"
fi

hash_is_old() {
  local h="$1" old
  for old in "${OLD_HASHES[@]}"; do
    [[ "$h" == "$old" ]] && return 0
  done
  return 1
}

# ------------------------------------------------------------------ new pair
step "Generating a new ${KEY_BITS}-bit RSA pair"
NEW_PRIV="$WORK/new-key.pem"
NEW_PUB="$WORK/new-pub.pem"
(
  umask 077
  openssl genpkey -algorithm RSA -pkeyopt "rsa_keygen_bits:${KEY_BITS}" -out "$NEW_PRIV" 2>/dev/null
)
openssl rsa -in "$NEW_PRIV" -pubout -out "$NEW_PUB" 2>/dev/null
say "  generated"

step "Self-testing the new pair"
diff <(openssl rsa -in "$NEW_PRIV" -pubout 2>/dev/null) "$NEW_PUB" >/dev/null \
  || die "generated public key does not match the private key"
say "  pair matches"

grep -q 'BEGIN PUBLIC KEY' "$NEW_PUB" \
  || die "public key is not SPKI format — the browser's crypto.subtle.importKey('spki', ...) will reject it"
say "  public key is SPKI (browser-importable)"

# Same scheme the app uses: RSA-OAEP with SHA-256 for both digest and MGF1.
head -c 32 /dev/urandom > "$WORK/probe.bin"
oaep_encrypt "$NEW_PUB" "$WORK/probe.bin" "$WORK/probe.enc"
openssl pkeyutl -decrypt -inkey "$NEW_PRIV" \
  -pkeyopt rsa_padding_mode:oaep \
  -pkeyopt rsa_oaep_md:sha256 \
  -pkeyopt rsa_mgf1_md:sha256 \
  -in "$WORK/probe.enc" -out "$WORK/probe.dec" 2>/dev/null
cmp -s "$WORK/probe.bin" "$WORK/probe.dec" \
  || die "RSA-OAEP/SHA-256 round trip failed — this pair would not authenticate"
say "  RSA-OAEP/SHA-256 round trip OK"

# --------------------------------------------------------------- shred survey
step "Surveying key material to shred"
declare -a SWEEP=()
sweep_add() {
  local f="$1"
  [[ -f "$f" ]] || return 0
  # Never sweep an install destination — it is erased in place before the new
  # key is written there (see "Erasing old key material in place" below).
  is_destination "$f" && return 0
  local c
  for c in "${SWEEP[@]:-}"; do [[ "$c" == "$f" ]] && return 0; done
  SWEEP+=("$f")
}

# Copies of the retiring keys anywhere in the repo (env/backup/ included).
while IFS= read -r -d '' f; do
  if [[ ${#OLD_HASHES[@]} -gt 0 ]] && hash_is_old "$(sha "$f")"; then
    sweep_add "$f"
  fi
done < <(find "$ROOT_DIR" -type f -name '*.pem' -print0 2>/dev/null)

# Stray copies in /tmp. The script's own WORK/BACKUP dirs live under /tmp too;
# they hold the NEW key and are shredded by the exit trap, so skip them here.
while IFS= read -r -d '' f; do
  if [[ "$f" == "$WORK"/* || "$f" == "$BACKUP"/* ]]; then
    continue
  elif [[ $ALL_TMP_PEM -eq 1 ]]; then
    sweep_add "$f"
  elif [[ ${#OLD_HASHES[@]} -gt 0 ]] && hash_is_old "$(sha "$f")"; then
    sweep_add "$f"
  else
    say "  skipping  $f (not a copy of these keys; --all-tmp-pem to include)"
  fi
done < <(find /tmp -type f -name '*.pem' -print0 2>/dev/null)

if [[ ${#SWEEP[@]} -eq 0 ]]; then
  say "  no stray copies found"
else
  for f in "${SWEEP[@]}"; do say "  will shred  $f"; done
fi
say "  (the three destinations below are erased in place, then rewritten)"

say ""
say "Will replace (same filenames, no code changes needed):"
say "  $PRIV_REPO"
say "  $PUB_REPO"
say "  $PRIV_DEPLOYED"
say "Then restart: $SERVICE"

if [[ $DRY_RUN -eq 1 ]]; then
  step "Dry run — discarding the new pair, nothing changed"
  exit 0
fi

need_root

if [[ $ASSUME_YES -ne 1 ]]; then
  [[ -t 0 ]] || die "not a TTY and --yes not given; refusing to rotate unattended"
  say ""
  read -r -p "Rotate now? Every student is logged out and old keys are destroyed. [y/N] " reply
  [[ "$reply" =~ ^[Yy]$ ]] || die "aborted by user"
fi

# ----------------------------------------------------------------- install
step "Backing up current keys (temporarily, shredded on success)"
# A manifest maps backup slot -> original path. Encoding the path into the
# filename would not survive the round trip for any path containing '_'.
: > "$BACKUP/manifest"
backup_n=0
for f in "$PRIV_REPO" "$PUB_REPO" "$PRIV_DEPLOYED"; do
  if [[ -f "$f" ]]; then
    backup_n=$((backup_n + 1))
    cp -p "$f" "$BACKUP/slot.$backup_n"
    printf '%s\t%s\n' "slot.$backup_n" "$f" >> "$BACKUP/manifest"
  fi
done
say "  backed up $backup_n file(s) to $BACKUP"

# Keep the repo copies owned by whoever owns them now, not by root.
repo_owner() {
  local f="$1"
  if [[ -e "$f" ]]; then stat -c '%U:%G' "$f"; else stat -c '%U:%G' "$ROOT_DIR"; fi
}
PRIV_OWNER="$(repo_owner "$PRIV_REPO")"
PUB_OWNER="$(repo_owner "$PUB_REPO")"

step "Erasing old key material in place"
# install(1) overwrites the file but does not securely erase the previous
# bytes, so shred each destination first. Backups above cover the rollback.
for f in "${DEST_PATHS[@]}"; do
  shred_file "$f"
done

step "Installing the new keypair"
install -m 600 -o "${PRIV_OWNER%%:*}" -g "${PRIV_OWNER##*:}" "$NEW_PRIV" "$PRIV_REPO"
say "  $PRIV_REPO"
install -m 644 -o "${PUB_OWNER%%:*}" -g "${PUB_OWNER##*:}" "$NEW_PUB" "$PUB_REPO"
say "  $PUB_REPO"

if getent group "$SERVICE_GROUP" >/dev/null 2>&1; then
  install -m 640 -o root -g "$SERVICE_GROUP" "$NEW_PRIV" "$PRIV_DEPLOYED"
else
  warn "group $SERVICE_GROUP not found; installing $PRIV_DEPLOYED as root:root 600"
  install -m 600 -o root -g root "$NEW_PRIV" "$PRIV_DEPLOYED"
fi
say "  $PRIV_DEPLOYED"

rollback() {
  warn "rolling back to the previous keypair"
  local slot dst
  if [[ -f "$BACKUP/manifest" ]]; then
    while IFS=$'\t' read -r slot dst; do
      [[ -n "$slot" && -f "$BACKUP/$slot" ]] || continue
      if cp -p "$BACKUP/$slot" "$dst" 2>/dev/null; then
        warn "restored $dst"
      else
        warn "COULD NOT RESTORE $dst — recover it from $BACKUP/$slot by hand"
      fi
    done < "$BACKUP/manifest"
  else
    warn "no backup manifest found; nothing to restore"
  fi
  systemctl restart "$SERVICE" 2>/dev/null || warn "could not restart $SERVICE"
  die "rotation failed and was rolled back — the old key is still in service"
}

step "Restarting $SERVICE"
systemctl restart "$SERVICE" || rollback

printf '  waiting for health'
healthy=0
for _ in $(seq 1 "$HEALTH_TIMEOUT"); do
  if curl -fsS -o /dev/null "$BASE_URL/healthz" 2>/dev/null; then healthy=1; break; fi
  printf '.'; sleep 1
done
printf '\n'
[[ $healthy -eq 1 ]] || rollback
say "  healthz OK"

# ------------------------------------------------- prove the new key works
step "Authenticating against the live service with the new public key"
challenge="$(curl -fsS "$BASE_URL/api/auth/challenge" 2>/dev/null)" || rollback
cid="$(printf '%s' "$challenge" | json_field challenge_id)"
nonce_b64="$(printf '%s' "$challenge" | json_field nonce)"
[[ -n "$cid" && -n "$nonce_b64" ]] || rollback

printf '%s' "$nonce_b64" | base64 -d > "$WORK/nonce.bin" || rollback
oaep_encrypt "$NEW_PUB" "$WORK/nonce.bin" "$WORK/nonce.enc" || rollback
ct_b64="$(base64 < "$WORK/nonce.enc" | tr -d '\n')"

code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE_URL/api/auth/verify" \
  -H 'Content-Type: application/json' \
  -d "{\"challenge_id\":\"$cid\",\"ciphertext\":\"$ct_b64\"}" 2>/dev/null)"

if [[ "$code" != "204" ]]; then
  warn "/api/auth/verify returned $code, expected 204"
  rollback
fi
say "  new key authenticated (HTTP 204)"

# ------------------------------------------------------------------- shred
step "Shredding retired key material"
for f in "${SWEEP[@]:-}"; do
  [[ -n "$f" ]] && shred_file "$f"
done
say "  backups and scratch copies shredded on exit"

step "Done"
say "New public key — distribute this to students out of band:"
say "  $PUB_REPO"
say ""
say "Everyone must paste it to log back in; the app clears the stale key from"
say "localStorage automatically and shows the login form."
