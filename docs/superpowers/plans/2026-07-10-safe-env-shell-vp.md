# Safe Environment Prefixes, Shell Delegation, and svelte-mcp Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Auto-approve configured local environment namespaces and narrowly scoped shell/svelte-mcp workflows while preserving all existing deny and ask behavior.

**Architecture:** Extend `Config` and evaluation context with additive safe environment prefixes. Add a call-expression dispatcher for static `bash`/`zsh -c` payloads that recursively invokes the existing parser/evaluator, and add explicit patterns for the two approved svelte-mcp operations.

**Tech Stack:** Go, `mvdan.cc/sh/v3/syntax`, YAML v4, testify.

## Global Constraints

- Hard-deny and ask-only environment variables take precedence over configured prefixes.
- Only literal shell payloads are delegated.
- Only `get-documentation` and `svelte-autofixer` are added under `vp exec svelte-mcp`.
- Preserve unrelated worktree changes and do not commit unless requested.

---

### Task 1: Configurable safe environment prefixes

**Files:**
- Modify: `hooks/bash-approve/main.go`
- Modify: `hooks/bash-approve/env.go`
- Modify: `hooks/bash-approve/main_test.go`
- Modify: `hooks/bash-approve/env_test.go`
- Modify: `hooks/bash-approve/categories.yaml`

- [x] Add failing parsing and evaluation tests for `safe_env_prefixes`.
- [x] Confirm an unknown `KENN_` variable still asks before implementation.
- [x] Add `Config.SafeEnvPrefixes`, propagate it through `evalContext`, and check non-empty configured prefixes after built-in safety checks.
- [x] Confirm configured `KENN_` allows the test command while configured `PATH` and dangerous loader variables retain their current decisions.
- [x] Document the additive setting in the sample configuration.

### Task 2: Static shell command delegation

**Files:**
- Modify: `hooks/bash-approve/main.go`
- Modify: `hooks/bash-approve/main_test.go`

- [x] Add failing tests for `/bin/zsh -lc` and `bash -c` around safe, denied, unknown, and dynamic inner commands.
- [x] Confirm the safe case fails before implementation.
- [x] Add a dispatcher that accepts `bash` or `zsh`, permits only `-c`/`-lc`, requires exactly one literal payload argument, parses it, and evaluates it with existing rules.
- [x] Confirm deny/no-opinion decisions from the inner command propagate unchanged.

### Task 3: Scoped svelte-mcp commands

**Files:**
- Modify: `hooks/bash-approve/rules.go`
- Modify: `hooks/bash-approve/main_test.go`

- [x] Add failing positive cases for `get-documentation` and `svelte-autofixer`, plus a negative unrelated-tool case.
- [x] Confirm the positive cases fail before implementation.
- [x] Add explicit command patterns for the two operations.
- [x] Confirm unrelated `vp exec` remains unrecognized.

### Task 4: Local configuration and verification

**Files:**
- Modify: the deployed `categories.yaml` selected by the locally installed hook.

- [x] Locate the actual installed binary/config paths used by Claude and Codex without overwriting unrelated settings.
- [x] Add `safe_env_prefixes: [KENN_]` to the shared or applicable local configuration files.
- [x] Run `go test ./...` and `golangci-lint run ./...` from `hooks/bash-approve`.
- [x] Exercise the three telemetry command shapes through the local binary/config and inspect the resulting decisions.

### Task 5: Migrate repository typechecking to Vite+

**Files:**
- Modify: `package.json`
- Modify: `opencode-tester/package.json`
- Modify: `opencode-tester/tsconfig.json`
- Modify: `packages/pi-bash-approve/package.json`
- Modify: `prek.toml`
- Modify: `bun.lock`
- Create: `vite.config.ts`
- Create: `scripts/typecheck-vite-plus.sh`
- Delete: `scripts/typecheck-opencode.sh`
- Delete: `scripts/typecheck-pi.sh`

- [x] Confirm `scripts/typecheck-pi.sh` fails because the pi workspace cannot resolve `tsc`.
- [x] Run `vp migrate` to add Vite+ 0.2.4, generate `vite.config.ts`, and remove the unsupported OpenCode `baseUrl` setting.
- [x] Configure one root `vp check --no-fmt --no-lint` script with `typeAware` and `typeCheck` enabled.
- [x] Remove workspace-specific TypeScript compiler pins and replace the duplicate pre-commit hooks with `scripts/typecheck-vite-plus.sh`.
- [x] Run `bun run typecheck`; expect 0 type errors across both workspaces.
- [x] Run `prek run --all-files`; expect every repository hook to pass before committing.
