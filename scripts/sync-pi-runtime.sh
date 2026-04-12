#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SRC_DIR="$ROOT_DIR/hooks/bash-approve"
DST_DIR="$ROOT_DIR/packages/pi-bash-approve/runtime"

mkdir -p "$DST_DIR"
cp "$SRC_DIR"/*.go "$DST_DIR"/
cp "$SRC_DIR"/go.mod "$SRC_DIR"/go.sum "$DST_DIR"/
cp "$SRC_DIR"/categories.yaml "$DST_DIR"/
chmod +x "$DST_DIR/run-pi-runtime.sh"
