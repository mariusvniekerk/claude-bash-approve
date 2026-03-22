#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
HOOK_DIR="$REPO_DIR/hooks/bash-approve"
RUN_HOOK="$HOOK_DIR/run-hook.sh"
SETTINGS_FILE="$HOME/.claude/settings.json"

echo "==> Installing claude-bash-approve"

# Check prerequisites
if ! command -v go &>/dev/null; then
    echo "ERROR: Go is not installed. Install Go 1.23+ from https://go.dev/dl/" >&2
    exit 1
fi

# Build the binary to verify everything compiles
echo "==> Building hook binary..."
(cd "$HOOK_DIR" && go build -o "$HOOK_DIR/approve-bash" .)
echo "    Built: $HOOK_DIR/approve-bash"

# Ensure ~/.claude exists
mkdir -p "$HOME/.claude"

# Create or update settings.json with the hook
if [ ! -f "$SETTINGS_FILE" ]; then
    echo "==> Creating $SETTINGS_FILE"
    cat > "$SETTINGS_FILE" <<ENDJSON
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "$RUN_HOOK"
          }
        ]
      }
    ]
  }
}
ENDJSON
    echo "    Created with bash-approve hook."
else
    # settings.json exists — check if hook is already configured
    if grep -q "bash-approve" "$SETTINGS_FILE" 2>/dev/null; then
        echo "==> Hook already present in $SETTINGS_FILE — skipping."
    else
        echo "==> $SETTINGS_FILE exists but does not contain bash-approve hook."
        echo ""
        echo "    Add the following to your \"hooks\" config manually:"
        echo ""
        echo '    "PreToolUse": ['
        echo '      {'
        echo '        "matcher": "Bash",'
        echo '        "hooks": ['
        echo '          {'
        echo '            "type": "command",'
        echo "            \"command\": \"$RUN_HOOK\""
        echo '          }'
        echo '        ]'
        echo '      }'
        echo '    ]'
        echo ""
        echo "    Or run with --force to merge it in (requires jq)."
        if [ "${1:-}" = "--force" ]; then
            if ! command -v jq &>/dev/null; then
                echo ""
                echo "ERROR: --force requires jq to safely merge into existing settings." >&2
                echo "       Install jq (https://jqlang.github.io/jq/) and try again." >&2
                exit 1
            fi
            echo ""
            echo "==> --force specified. Backing up and merging..."
            cp "$SETTINGS_FILE" "$SETTINGS_FILE.bak"
            echo "    Backup: $SETTINGS_FILE.bak"

            HOOK_JSON=$(cat <<ENDJSON
{
  "matcher": "Bash",
  "hooks": [
    {
      "type": "command",
      "command": "$RUN_HOOK"
    }
  ]
}
ENDJSON
)
            jq --argjson hook "$HOOK_JSON" '
              .hooks //= {} |
              .hooks.PreToolUse //= [] |
              .hooks.PreToolUse += [$hook]
            ' "$SETTINGS_FILE.bak" > "$SETTINGS_FILE"
            echo "    Merged hook into existing settings."
        fi
    fi
fi

echo ""
echo "==> Done. Restart Claude Code for the hook to take effect."
