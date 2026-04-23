#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "$0")/.." && pwd)"

cd "$repo_dir"
bun install --frozen-lockfile
bun run --cwd packages/pi-bash-approve typecheck
