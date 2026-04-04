#!/usr/bin/env bash
# Shim script for Claude Code hook.
# Recompiles the Go binary if main.go is newer, then runs it.
#
# Configure in ~/.claude/settings.json:
#   "hooks": {
#     "PreToolUse": [
#       {
#         "matcher": "Bash",
#         "hooks": [{"type": "command", "command": "~/.claude/hooks/bash-approve/run-hook.sh"}]
#       },
#       {
#         "matcher": "Read|Grep",
#         "hooks": [{"type": "command", "command": "~/.claude/hooks/bash-approve/run-hook.sh"}]
#       }
#     ]
#   }

HOOK_DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY="$HOOK_DIR/approve-bash"

# Recompile if binary missing or any .go file is newer
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
        echo "bash-approve: build failed, falling through to manual approval" >&2
        exit 0
    fi
fi

exec "$BINARY"
