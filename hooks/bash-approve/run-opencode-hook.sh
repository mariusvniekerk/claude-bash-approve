#!/usr/bin/env bash
# Shim script for OpenCode plugin integration.
# Recompiles the Go binary if sources changed, then runs it in OpenCode mode.

HOOK_DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY="$HOOK_DIR/approve-bash"

needs_rebuild=false
if [ ! -f "$BINARY" ]; then
    needs_rebuild=true
else
    for src in "$HOOK_DIR"/*.go "$HOOK_DIR"/go.mod "$HOOK_DIR"/go.sum; do
        if [ "$src" -nt "$BINARY" ]; then
            needs_rebuild=true
            break
        fi
    done
fi

if [ "$needs_rebuild" = true ]; then
    if ! (cd "$HOOK_DIR" && go build -o "$BINARY" .) 2>&1 >&2; then
        echo "bash-approve: build failed, falling back to OpenCode ask flow" >&2
        exit 0
    fi
fi

exec "$BINARY" --opencode
