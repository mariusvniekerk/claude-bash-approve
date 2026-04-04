#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
SOURCE_HOOK_DIR="$REPO_DIR/hooks/bash-approve"
CLAUDE_DIR="$HOME/.claude"
HOOK_DIR="$CLAUDE_DIR/hooks/bash-approve"
RUN_HOOK="$HOOK_DIR/run-hook.sh"
SETTINGS_FILE="$CLAUDE_DIR/settings.json"

echo "==> Installing claude-bash-approve"

# Check prerequisites
if ! command -v go &>/dev/null; then
    echo "ERROR: Go is not installed. Install Go 1.23+ from https://go.dev/dl/" >&2
    exit 1
fi

# Ensure ~/.claude exists
mkdir -p "$CLAUDE_DIR"
mkdir -p "$HOOK_DIR"

echo "==> Copying hook sources to $HOOK_DIR"
cp "$SOURCE_HOOK_DIR"/*.go "$HOOK_DIR"/
cp "$SOURCE_HOOK_DIR"/go.mod "$HOOK_DIR"/
cp "$SOURCE_HOOK_DIR"/go.sum "$HOOK_DIR"/
cp "$SOURCE_HOOK_DIR"/categories.yaml "$HOOK_DIR"/
cp "$SOURCE_HOOK_DIR"/run-hook.sh "$HOOK_DIR"/
chmod +x "$RUN_HOOK"

# Build the binary in the deployed directory to verify everything compiles
echo "==> Building hook binary..."
(cd "$HOOK_DIR" && go build -o "$HOOK_DIR/approve-bash" .)
echo "    Built: $HOOK_DIR/approve-bash"

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
      },
      {
        "matcher": "Read|Grep",
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
    # settings.json exists — check if the expected deployed hook is already configured
    if grep -Fq "\"command\": \"$RUN_HOOK\"" "$SETTINGS_FILE" 2>/dev/null \
        && grep -Fq '"matcher": "Bash"' "$SETTINGS_FILE" 2>/dev/null \
        && grep -Fq '"matcher": "Read|Grep"' "$SETTINGS_FILE" 2>/dev/null; then
        echo "==> Hook already present in $SETTINGS_FILE — skipping settings update."
    else
        echo "==> $SETTINGS_FILE exists but does not contain the deployed bash-approve hook."
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
        echo '      },'
        echo '      {'
        echo '        "matcher": "Read|Grep",'
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
            READ_GREP_HOOK_JSON=$(cat <<ENDJSON
{
  "matcher": "Read|Grep",
  "hooks": [
    {
      "type": "command",
      "command": "$RUN_HOOK"
    }
  ]
}
ENDJSON
)
            jq --argjson bash_hook "$HOOK_JSON" --argjson read_grep_hook "$READ_GREP_HOOK_JSON" '
              .hooks //= {} |
              .hooks.PreToolUse //= [] |
              .hooks.PreToolUse |= map(
                if .matcher == "Bash" or .matcher == "Read|Grep" then
                  empty
                else
                  .
                end
              ) |
              .hooks.PreToolUse += [$bash_hook, $read_grep_hook]
            ' "$SETTINGS_FILE.bak" > "$SETTINGS_FILE"
            echo "    Merged hook into existing settings."
        fi
    fi
fi

echo ""
echo "==> Done. Restart Claude Code for the hook to take effect."
