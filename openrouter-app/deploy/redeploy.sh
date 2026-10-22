#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"
./deploy/build.sh
sudo install -m 755 ./openrouter-app /usr/local/bin/openrouter-app
sudo systemctl restart openrouter-app

echo "Redeployed. Health check: curl http://127.0.0.1:8080/healthz"
