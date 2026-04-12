#!/usr/bin/env bash
set -euo pipefail

RUNTIME_DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY="$RUNTIME_DIR/approve-bash"

needs_rebuild=false
if [ ! -f "$BINARY" ]; then
  needs_rebuild=true
else
  for src in "$RUNTIME_DIR"/*.go "$RUNTIME_DIR"/go.mod "$RUNTIME_DIR"/go.sum "$RUNTIME_DIR"/categories.yaml; do
    if [ -e "$src" ] && [ "$src" -nt "$BINARY" ]; then
      needs_rebuild=true
      break
    fi
  done
fi

if [ "$needs_rebuild" = true ]; then
  (cd "$RUNTIME_DIR" && go build -o "$BINARY" .) 1>&2
fi

exec "$BINARY" --pi "$@"
