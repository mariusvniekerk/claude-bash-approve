#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT

cp "$ROOT_DIR/install-opencode.sh" "$TEST_DIR/install-opencode.sh"

export CLAUDE_BASH_APPROVE_INSTALL_LIB=1
# shellcheck source=/dev/null
source "$TEST_DIR/install-opencode.sh"

CONFIG_FILE="$TEST_DIR/opencode.json"
cat > "$CONFIG_FILE" <<'EOF'
{
  "permission": {
    "bash": {
      "*": "deny",
      "ls *": "ask"
    }
  }
}
EOF

FORCE=false
ensure_config "$CONFIG_FILE"
if jq -e '.permission.bash["*"] == "ask"' "$CONFIG_FILE" >/dev/null 2>&1; then
  echo "ensure_config rewrote existing config without --force" >&2
  exit 1
fi

FORCE=true
ensure_config "$CONFIG_FILE"
jq -e '.permission.bash["*"] == "ask"' "$CONFIG_FILE" >/dev/null

FAKE_BIN="$TEST_DIR/bin"
RUNTIME_DIR="$TEST_DIR/runtime"
mkdir -p "$FAKE_BIN" "$RUNTIME_DIR"
cat > "$FAKE_BIN/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" > "$TEST_GO_ARGS_FILE"
EOF
chmod +x "$FAKE_BIN/go"

export PATH="$FAKE_BIN:$PATH"
export TEST_GO_ARGS_FILE="$TEST_DIR/go-args.txt"
build_runtime_binary "$RUNTIME_DIR"

grep -q -- '-buildvcs=false' "$TEST_GO_ARGS_FILE"

echo "install-opencode.sh regression checks passed"
