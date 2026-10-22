#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"
./deploy/build.sh
sudo install -m 755 ./ipytutor /usr/local/bin/ipytutor
sudo systemctl restart ipytutor

echo "Redeployed. Health check: curl http://127.0.0.1:8090/healthz"
