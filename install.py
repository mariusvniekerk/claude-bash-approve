#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import tomllib
from pathlib import Path
from typing import Any

RUNTIME_FILES = [
    "categories.yaml",
    "run-hook.sh",
    "run-opencode-hook.sh",
    "run-pi-runtime.sh",
    "run-codex-hook.sh",
]

CLAUDE_MATCHERS = ("Bash", "Read|Grep")
CODEX_PERMISSION_MATCHER = "^Bash$"


class InstallError(RuntimeError):
    pass


def repo_root() -> Path:
    return Path(__file__).resolve().parent


def source_hook_dir() -> Path:
    return repo_root() / "hooks" / "bash-approve"


def opencode_plugin_template() -> Path:
    return repo_root() / "opencode" / "bash-approve.plugin.ts"


def absolute_xdg_dir(env_name: str, fallback: Path) -> Path:
    value = os.environ.get(env_name, "")
    if value:
        candidate = Path(value).expanduser()
        if candidate.is_absolute():
            return candidate
    return fallback


def shared_runtime_root() -> Path:
    return absolute_xdg_dir("XDG_DATA_HOME", Path.home() / ".local" / "share") / "claude-bash-approve"


def shared_runtime_binary_path() -> Path:
    return shared_runtime_root() / "approve-bash"


def shared_runtime_claude_hook_path() -> Path:
    return shared_runtime_root() / "run-hook.sh"


def shared_runtime_opencode_hook_path() -> Path:
    return shared_runtime_root() / "run-opencode-hook.sh"


def shared_runtime_codex_hook_path() -> Path:
    return shared_runtime_root() / "run-codex-hook.sh"


def ensure_go_installed() -> None:
    if shutil.which("go"):
        return
    raise InstallError("Go is required to build the bash-approve runtime.")


def ensure_python_version() -> None:
    if sys.version_info >= (3, 11):
        return
    raise InstallError("Python 3.11+ is required for install.py (tomllib).")


def load_json_file(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {}
    try:
        raw = json.loads(path.read_text())
    except json.JSONDecodeError as exc:
        raise InstallError(f"invalid JSON in {path}: {exc}") from exc
    if not isinstance(raw, dict):
        raise InstallError(f"expected JSON object in {path}")
    return raw


def write_json_file(path: Path, data: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2) + "\n")


def load_toml_file(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {}
    try:
        parsed = tomllib.loads(path.read_text())
    except tomllib.TOMLDecodeError as exc:
        raise InstallError(f"invalid TOML in {path}: {exc}") from exc
    if not isinstance(parsed, dict):
        raise InstallError(f"expected TOML table in {path}")
    return parsed


def install_shared_runtime_bundle() -> Path:
    runtime_root = shared_runtime_root()
    runtime_root.mkdir(parents=True, exist_ok=True)

    for path in source_hook_dir().glob("*.go"):
        shutil.copy2(path, runtime_root / path.name)
    for name in ("go.mod", "go.sum"):
        shutil.copy2(source_hook_dir() / name, runtime_root / name)
    for name in RUNTIME_FILES:
        dest = runtime_root / name
        shutil.copy2(source_hook_dir() / name, dest)
        dest.chmod(dest.stat().st_mode | 0o111)

    build_shared_runtime_binary(runtime_root)
    return runtime_root


def build_shared_runtime_binary(runtime_root: Path) -> None:
    subprocess.run(
        ["go", "build", "-buildvcs=false", "-o", str(runtime_root / "approve-bash"), "."],
        cwd=runtime_root,
        check=True,
    )


def merge_command_hook(entries: list[dict[str, Any]], matcher: str, command: str) -> list[dict[str, Any]]:
    for entry in entries:
        if entry.get("matcher") != matcher:
            continue
        hooks = entry.setdefault("hooks", [])
        if not isinstance(hooks, list):
            raise InstallError(f"hooks entry for matcher {matcher!r} must be an array")
        if not any(hook.get("type") == "command" and hook.get("command") == command for hook in hooks if isinstance(hook, dict)):
            hooks.append({"type": "command", "command": command})
        return entries

    entries.append({
        "matcher": matcher,
        "hooks": [{"type": "command", "command": command}],
    })
    return entries


def remove_command_hook(entries: list[dict[str, Any]], matcher: str, command: str) -> list[dict[str, Any]]:
    updated: list[dict[str, Any]] = []
    for entry in entries:
        if not isinstance(entry, dict):
            updated.append(entry)
            continue
        if entry.get("matcher") != matcher:
            updated.append(entry)
            continue
        hooks = entry.get("hooks")
        if not isinstance(hooks, list):
            updated.append(entry)
            continue
        kept = [
            hook for hook in hooks
            if not (isinstance(hook, dict) and hook.get("type") == "command" and hook.get("command") == command)
        ]
        if kept:
            updated.append({**entry, "hooks": kept})
    return updated


def prune_empty_json_sections(data: dict[str, Any], path: list[str]) -> None:
    if not path:
        return
    parent = data
    for key in path[:-1]:
        child = parent.get(key)
        if not isinstance(child, dict):
            return
        parent = child
    last = path[-1]
    value = parent.get(last)
    if value in ({}, [], None):
        parent.pop(last, None)


def ensure_claude_install() -> None:
    runtime_root = install_shared_runtime_bundle()
    settings_path = Path.home() / ".claude" / "settings.json"
    data = load_json_file(settings_path)
    hooks = data.setdefault("hooks", {})
    if not isinstance(hooks, dict):
        raise InstallError(f"expected object at hooks in {settings_path}")
    pre_tool = hooks.setdefault("PreToolUse", [])
    if not isinstance(pre_tool, list):
        raise InstallError(f"expected array at hooks.PreToolUse in {settings_path}")

    command = str(shared_runtime_claude_hook_path())
    for matcher in CLAUDE_MATCHERS:
        merge_command_hook(pre_tool, matcher, command)

    write_json_file(settings_path, data)
    print(f"Installed shared runtime: {runtime_root}")
    print(f"Updated Claude settings: {settings_path}")


def uninstall_claude() -> None:
    settings_path = Path.home() / ".claude" / "settings.json"
    if not settings_path.exists():
        return

    data = load_json_file(settings_path)
    hooks = data.get("hooks")
    if not isinstance(hooks, dict):
        return
    pre_tool = hooks.get("PreToolUse")
    if not isinstance(pre_tool, list):
        return

    command = str(shared_runtime_claude_hook_path())
    updated = pre_tool
    for matcher in CLAUDE_MATCHERS:
        updated = remove_command_hook(updated, matcher, command)
    hooks["PreToolUse"] = updated
    prune_empty_json_sections(data, ["hooks", "PreToolUse"])
    prune_empty_json_sections(data, ["hooks"])
    write_json_file(settings_path, data)
    print(f"Removed Claude hook wiring from {settings_path}")


def render_opencode_plugin(destination: Path, hook_path: Path) -> None:
    escaped = str(hook_path).replace("\\", "\\\\").replace('"', '\\"')
    content = opencode_plugin_template().read_text().replace("__HOOK_PATH__", escaped)
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_text(content)


def ensure_opencode_config(config_path: Path) -> None:
    data = load_json_file(config_path)
    permission = data.setdefault("permission", {})
    if not isinstance(permission, dict):
        raise InstallError(f"expected object at permission in {config_path}")
    bash = permission.setdefault("bash", {})
    if not isinstance(bash, dict):
        raise InstallError(f"expected object at permission.bash in {config_path}")
    bash["*"] = "ask"
    if "$schema" not in data:
        data["$schema"] = "https://opencode.ai/config.json"
    write_json_file(config_path, data)


def install_opencode(scope: str, project_root: Path) -> None:
    runtime_root = install_shared_runtime_bundle()
    hook_path = shared_runtime_opencode_hook_path()

    if scope in {"project", "both"}:
        plugin_path = project_root / ".opencode" / "plugins" / "bash-approve.ts"
        config_path = project_root / "opencode.json"
        render_opencode_plugin(plugin_path, hook_path)
        ensure_opencode_config(config_path)
        print(f"Wrote OpenCode project plugin: {plugin_path}")
        print(f"Updated OpenCode project config: {config_path}")

    if scope in {"global", "both"}:
        global_root = Path.home() / ".config" / "opencode"
        plugin_path = global_root / "plugins" / "bash-approve.ts"
        config_path = global_root / "opencode.json"
        render_opencode_plugin(plugin_path, hook_path)
        ensure_opencode_config(config_path)
        print(f"Wrote OpenCode global plugin: {plugin_path}")
        print(f"Updated OpenCode global config: {config_path}")

    print(f"Installed shared runtime: {runtime_root}")


def uninstall_opencode(scope: str, project_root: Path) -> None:
    if scope in {"project", "both"}:
        plugin_path = project_root / ".opencode" / "plugins" / "bash-approve.ts"
        if plugin_path.exists():
            plugin_path.unlink()
            print(f"Removed OpenCode project plugin: {plugin_path}")

    if scope in {"global", "both"}:
        plugin_path = Path.home() / ".config" / "opencode" / "plugins" / "bash-approve.ts"
        if plugin_path.exists():
            plugin_path.unlink()
            print(f"Removed OpenCode global plugin: {plugin_path}")


FEATURES_HEADER_RE = re.compile(r"(?m)^\[features\]\s*$")
CODEX_HOOKS_LINE_RE = re.compile(r"(?m)^\s*codex_hooks\s*=.*$")


def ensure_codex_feature_enabled(config_path: Path) -> None:
    if not config_path.exists():
        config_path.parent.mkdir(parents=True, exist_ok=True)
        config_path.write_text("[features]\ncodex_hooks = true\n")
        return

    parsed = load_toml_file(config_path)
    features = parsed.get("features")
    if isinstance(features, dict) and features.get("codex_hooks") is True:
        return

    text = config_path.read_text()
    if CODEX_HOOKS_LINE_RE.search(text):
        updated = CODEX_HOOKS_LINE_RE.sub("codex_hooks = true", text, count=1)
    elif FEATURES_HEADER_RE.search(text):
        updated = FEATURES_HEADER_RE.sub("[features]\ncodex_hooks = true", text, count=1)
    else:
        if text and not text.endswith("\n"):
            text += "\n"
        updated = text + "[features]\ncodex_hooks = true\n"

    tomllib.loads(updated)
    config_path.write_text(updated)


def remove_codex_feature_if_trivial(config_path: Path) -> None:
    if not config_path.exists():
        return
    text = config_path.read_text()
    updated = re.sub(r"(?m)^\s*codex_hooks\s*=\s*true\s*\n?", "", text)
    try:
        if updated.strip():
            tomllib.loads(updated)
    except tomllib.TOMLDecodeError:
        return
    config_path.write_text(updated if updated.endswith("\n") or not updated else updated + "\n")


def ensure_codex_hooks_file(hooks_path: Path) -> None:
    data = load_json_file(hooks_path)
    hooks = data.setdefault("hooks", {})
    if not isinstance(hooks, dict):
        raise InstallError(f"expected object at hooks in {hooks_path}")
    permission_request = hooks.setdefault("PermissionRequest", [])
    if not isinstance(permission_request, list):
        raise InstallError(f"expected array at hooks.PermissionRequest in {hooks_path}")
    merge_command_hook(permission_request, CODEX_PERMISSION_MATCHER, str(shared_runtime_codex_hook_path()))
    write_json_file(hooks_path, data)


def uninstall_codex_hooks_file(hooks_path: Path) -> None:
    if not hooks_path.exists():
        return
    data = load_json_file(hooks_path)
    hooks = data.get("hooks")
    if not isinstance(hooks, dict):
        return
    permission_request = hooks.get("PermissionRequest")
    if not isinstance(permission_request, list):
        return
    hooks["PermissionRequest"] = remove_command_hook(permission_request, CODEX_PERMISSION_MATCHER, str(shared_runtime_codex_hook_path()))
    prune_empty_json_sections(data, ["hooks", "PermissionRequest"])
    prune_empty_json_sections(data, ["hooks"])
    write_json_file(hooks_path, data)


def install_codex(scope: str, project_root: Path) -> None:
    runtime_root = install_shared_runtime_bundle()

    if scope in {"project", "both"}:
        config_path = project_root / ".codex" / "config.toml"
        hooks_path = project_root / ".codex" / "hooks.json"
        ensure_codex_feature_enabled(config_path)
        ensure_codex_hooks_file(hooks_path)
        print(f"Updated Codex project config: {config_path}")
        print(f"Updated Codex project hooks: {hooks_path}")

    if scope in {"global", "both"}:
        root = Path.home() / ".codex"
        config_path = root / "config.toml"
        hooks_path = root / "hooks.json"
        ensure_codex_feature_enabled(config_path)
        ensure_codex_hooks_file(hooks_path)
        print(f"Updated Codex global config: {config_path}")
        print(f"Updated Codex global hooks: {hooks_path}")

    print(f"Installed shared runtime: {runtime_root}")


def uninstall_codex(scope: str, project_root: Path) -> None:
    if scope in {"project", "both"}:
        config_path = project_root / ".codex" / "config.toml"
        hooks_path = project_root / ".codex" / "hooks.json"
        uninstall_codex_hooks_file(hooks_path)
        remove_codex_feature_if_trivial(config_path)
        print(f"Removed Codex project hook wiring from {hooks_path}")

    if scope in {"global", "both"}:
        root = Path.home() / ".codex"
        config_path = root / "config.toml"
        hooks_path = root / "hooks.json"
        uninstall_codex_hooks_file(hooks_path)
        remove_codex_feature_if_trivial(config_path)
        print(f"Removed Codex global hook wiring from {hooks_path}")


def remove_shared_runtime() -> None:
    runtime_root = shared_runtime_root()
    if runtime_root.exists():
        shutil.rmtree(runtime_root)
        print(f"Removed shared runtime: {runtime_root}")


def install_target(target: str, scope: str, project_root: Path) -> None:
    ensure_python_version()
    ensure_go_installed()
    if target in {"claude", "all"}:
        ensure_claude_install()
    if target in {"opencode", "all"}:
        install_opencode("both" if target == "all" else scope, project_root)
    if target in {"codex", "all"}:
        install_codex("both" if target == "all" else scope, project_root)


def uninstall_target(target: str, scope: str, project_root: Path) -> None:
    ensure_python_version()
    if target in {"claude", "all"}:
        uninstall_claude()
    if target in {"opencode", "all"}:
        uninstall_opencode("both" if target == "all" else scope, project_root)
    if target in {"codex", "all"}:
        uninstall_codex("both" if target == "all" else scope, project_root)
    if target == "all":
        remove_shared_runtime()


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Install or uninstall bash-approve integrations.")
    subparsers = parser.add_subparsers(dest="action", required=True)

    for action in ("install", "uninstall"):
        sub = subparsers.add_parser(action)
        sub.add_argument("--target", choices=["claude", "opencode", "codex", "all"], required=True)
        sub.add_argument("--scope", choices=["project", "global", "both"], default="global")
        sub.add_argument("--project-root", type=Path, default=repo_root(), help=argparse.SUPPRESS)

    return parser.parse_args()


def main() -> int:
    args = parse_args()
    project_root = args.project_root.resolve()
    try:
        if args.action == "install":
            install_target(args.target, args.scope, project_root)
        else:
            uninstall_target(args.target, args.scope, project_root)
    except InstallError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    except subprocess.CalledProcessError as exc:
        print(f"ERROR: command failed: {exc}", file=sys.stderr)
        return exc.returncode or 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
