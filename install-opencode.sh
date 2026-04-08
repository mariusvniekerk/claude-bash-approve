#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
SOURCE_HOOK_DIR="$REPO_DIR/hooks/bash-approve"
PLUGIN_TEMPLATE="$REPO_DIR/opencode/bash-approve.plugin.js.tmpl"
PROJECT_ROOT="$REPO_DIR"
GLOBAL_DIR="$HOME/.config/opencode"

MODE="project"
FORCE=false

while [ "$#" -gt 0 ]; do
    case "$1" in
        --project)
            MODE="project"
            ;;
        --global)
            MODE="global"
            ;;
        --both)
            MODE="both"
            ;;
        --force)
            FORCE=true
            ;;
        *)
            echo "Unknown argument: $1" >&2
            echo "Usage: ./install-opencode.sh [--project|--global|--both] [--force]" >&2
            exit 1
            ;;
    esac
    shift
done

if ! command -v go >/dev/null 2>&1; then
    echo "ERROR: Go is required to build the bash-approve runtime." >&2
    exit 1
fi

render_plugin() {
    local hook_path="$1"
    local plugin_path="$2"
    local escaped_hook

    escaped_hook=$(printf '%s' "$hook_path" | sed 's/[\\&|]/\\&/g')
    mkdir -p "$(dirname "$plugin_path")"
    sed "s|__HOOK_PATH__|$escaped_hook|g" "$PLUGIN_TEMPLATE" > "$plugin_path"
}

ensure_config() {
    local config_file="$1"

    if [ ! -f "$config_file" ]; then
        mkdir -p "$(dirname "$config_file")"
        cat > "$config_file" <<'EOF'
{
  "$schema": "https://opencode.ai/config.json",
  "permission": {
    "bash": {
      "*": "ask"
    }
  }
}
EOF
        echo "    Created $config_file with bash ask permissions."
        return
    fi

    if grep -Fq '"bash"' "$config_file" 2>/dev/null && grep -Fq '"ask"' "$config_file" 2>/dev/null; then
        echo "    Existing OpenCode config already mentions bash ask permissions; leaving it unchanged."
        return
    fi

    echo "    OpenCode config exists at $config_file."
    echo '    Required permission snippet: {"permission":{"bash":{"*":"ask"}}}'

    if [ "$FORCE" != true ]; then
        echo "    Re-run with --force to merge it automatically (requires jq)."
        return
    fi

    if ! command -v jq >/dev/null 2>&1; then
        echo "ERROR: --force requires jq to merge into existing OpenCode config." >&2
        exit 1
    fi

    cp "$config_file" "$config_file.bak"
    jq '
      .permission //= {} |
      .permission.bash =
        (if (.permission.bash | type) == "object"
         then .permission.bash + {"*": "ask"}
         else {"*": "ask"}
         end)
    ' "$config_file.bak" > "$config_file"
    echo "    Merged bash ask permissions into $config_file (backup at $config_file.bak)."
}

install_project() {
    local plugin_path="$PROJECT_ROOT/.opencode/plugins/bash-approve.js"
    local config_file="$PROJECT_ROOT/opencode.json"
    local hook_path="$SOURCE_HOOK_DIR/run-opencode-hook.sh"

    echo "==> Installing OpenCode project plugin"
    render_plugin "$hook_path" "$plugin_path"
    echo "    Wrote plugin: $plugin_path"
    ensure_config "$config_file"
}

install_global() {
    local runtime_dir="$GLOBAL_DIR/bash-approve"
    local plugin_path="$GLOBAL_DIR/plugins/bash-approve.js"
    local config_file="$GLOBAL_DIR/opencode.json"
    local hook_path="$runtime_dir/run-opencode-hook.sh"

    echo "==> Installing OpenCode global plugin"
    mkdir -p "$runtime_dir"
    cp "$SOURCE_HOOK_DIR"/*.go "$runtime_dir/"
    cp "$SOURCE_HOOK_DIR"/go.mod "$SOURCE_HOOK_DIR"/go.sum "$runtime_dir/"
    cp "$SOURCE_HOOK_DIR"/categories.yaml "$SOURCE_HOOK_DIR"/run-opencode-hook.sh "$runtime_dir/"
    chmod +x "$runtime_dir/run-opencode-hook.sh"
    (cd "$runtime_dir" && go build -o "$runtime_dir/approve-bash" .)
    echo "    Installed runtime: $runtime_dir"
    render_plugin "$hook_path" "$plugin_path"
    echo "    Wrote plugin: $plugin_path"
    ensure_config "$config_file"
}

echo "==> Installing claude-bash-approve for OpenCode"

case "$MODE" in
    project)
        install_project
        ;;
    global)
        install_global
        ;;
    both)
        install_project
        install_global
        ;;
esac

echo ""
echo "==> Done. Restart OpenCode after installation."
