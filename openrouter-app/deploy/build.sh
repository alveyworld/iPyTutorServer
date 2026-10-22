#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT="${1:-$ROOT_DIR/openrouter-app}"

cd "$ROOT_DIR"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$OUTPUT" .

echo "Built $OUTPUT"
