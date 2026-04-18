#!/usr/bin/env bash
# Shim script for Codex hook integration.
# Recompiles the Go binary if sources changed, then runs it in Codex mode.

RUNTIME_DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY="$RUNTIME_DIR/approve-bash"

needs_rebuild=false
if [ ! -f "$BINARY" ]; then
    needs_rebuild=true
else
    for src in "$RUNTIME_DIR"/*.go "$RUNTIME_DIR"/go.mod "$RUNTIME_DIR"/go.sum; do
        if [ "$src" -nt "$BINARY" ]; then
            needs_rebuild=true
            break
        fi
    done
fi

if [ "$needs_rebuild" = true ]; then
    if ! (cd "$RUNTIME_DIR" && go build -buildvcs=false -o "$BINARY" .) 2>&1 >&2; then
        echo "bash-approve: build failed, falling back to Codex approval flow" >&2
        exit 0
    fi
fi

exec "$BINARY" --codex
