# Shared Runtime + Codex Installer Implementation Plan

> **For agentic workers:** REQUIRED: Use `/skill:orchestrator-implements` (in-session, orchestrator implements), `/skill:subagent-driven-development` (in-session, subagents implement), or `/skill:executing-plans` (parallel session) to implement this plan. Steps use checkbox syntax for tracking.

**Goal:** Move Claude/OpenCode global installs to one shared installed runtime and add a Codex installer that wires Codex hooks to that same runtime.

**Architecture:** Introduce shared shell helpers for resolving and installing a single runtime bundle under the user data directory, then have each client installer write only client-specific config pointing at shared shims. Add a Codex hook shim/config path that mirrors existing Claude/OpenCode adapter structure while keeping runtime compilation and telemetry anchored in one place.

**Tech Stack:** Bash, jq, Go runtime, Codex hooks JSON/TOML config, OpenCode TypeScript plugin template

---

### Task 1: Add installer helper library

**TDD scenario:** New feature — full TDD cycle

**Files:**
- Create: `install-lib.sh`
- Modify: `install-opencode.test.sh`
- Test: `install-opencode.test.sh`

- [ ] **Step 1: Write failing test for shared runtime path helpers**

```bash
RUNTIME_ROOT="$(shared_runtime_root)"
RUNTIME_DIR="$(shared_runtime_dir)"
[ "$RUNTIME_ROOT" = "$HOME/.local/share/claude-bash-approve" ]
[ "$RUNTIME_DIR" = "$HOME/.local/share/claude-bash-approve/runtime" ]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash install-opencode.test.sh`
Expected: FAIL with `shared_runtime_root: command not found` or `shared_runtime_dir: command not found`

- [ ] **Step 3: Write minimal helper library**

```bash
shared_runtime_root() {
    printf '%s\n' "$HOME/.local/share/claude-bash-approve"
}

shared_runtime_dir() {
    printf '%s/runtime\n' "$(shared_runtime_root)"
}
```

Also add helper functions for:
- `shared_runtime_binary_path`
- `shared_runtime_claude_hook_path`
- `shared_runtime_opencode_hook_path`
- `shared_runtime_codex_hook_path`
- `install_shared_runtime_bundle`
- `build_shared_runtime_binary`

- [ ] **Step 4: Run test to verify it passes**

Run: `bash install-opencode.test.sh`
Expected: PASS for helper-path assertions, later steps may still fail until subsequent tasks

- [ ] **Step 5: Commit**

```bash
git add install-lib.sh install-opencode.test.sh
git commit -m "refactor: add shared runtime installer helpers"
```

### Task 2: Move Claude installer to shared runtime

**TDD scenario:** New feature — full TDD cycle

**Files:**
- Modify: `install.sh`
- Create: `install-shared-runtime.test.sh`
- Test: `install-shared-runtime.test.sh`

- [ ] **Step 1: Write failing test for shared Claude hook path**

```bash
EXPECTED_RUN_HOOK="$HOME/.local/share/claude-bash-approve/runtime/run-hook.sh"
ACTUAL_RUN_HOOK="$(installed_claude_hook_command)"
[ "$ACTUAL_RUN_HOOK" = "$EXPECTED_RUN_HOOK" ]
```

Test should also assert that the installer no longer writes Go sources into `~/.claude/hooks/bash-approve/`.

- [ ] **Step 2: Run test to verify it fails**

Run: `bash install-shared-runtime.test.sh`
Expected: FAIL because `install.sh` still targets `~/.claude/hooks/bash-approve/run-hook.sh`

- [ ] **Step 3: Update installer to use shared runtime**

```bash
# source install-lib.sh
RUNTIME_DIR="$(shared_runtime_dir)"
RUN_HOOK="$(shared_runtime_claude_hook_path)"
install_shared_runtime_bundle
build_shared_runtime_binary
```

Remove per-client runtime copy logic from `install.sh`. Keep settings merge behavior, but point Claude hook config at shared `run-hook.sh`.

- [ ] **Step 4: Run test to verify it passes**

Run: `bash install-shared-runtime.test.sh`
Expected: PASS with settings pointing at shared runtime path

- [ ] **Step 5: Commit**

```bash
git add install.sh install-lib.sh install-shared-runtime.test.sh
git commit -m "refactor: point claude install at shared runtime"
```

### Task 3: Move OpenCode installer to shared runtime

**TDD scenario:** Modifying code with existing tests — run existing tests first

**Files:**
- Modify: `install-opencode.sh`
- Modify: `install-opencode.test.sh`
- Test: `install-opencode.test.sh`

- [ ] **Step 1: Run existing test first**

Run: `bash install-opencode.test.sh`
Expected: PASS before changes

- [ ] **Step 2: Add failing assertions for shared global hook path**

```bash
EXPECTED="$HOME/.local/share/claude-bash-approve/runtime/run-opencode-hook.sh"
ACTUAL="$(installed_opencode_global_hook_path)"
[ "$ACTUAL" = "$EXPECTED" ]
```

Also assert global install does not copy runtime into `~/.config/opencode/bash-approve/`.

- [ ] **Step 3: Run test to verify it fails**

Run: `bash install-opencode.test.sh`
Expected: FAIL because installer still uses `~/.config/opencode/bash-approve/run-opencode-hook.sh`

- [ ] **Step 4: Write minimal implementation**

```bash
# source install-lib.sh
hook_path="$(shared_runtime_opencode_hook_path)"
install_shared_runtime_bundle
build_shared_runtime_binary
render_plugin "$hook_path" "$plugin_path"
```

Project and global OpenCode installs should both reference shared runtime hook path.

- [ ] **Step 5: Run test to verify it passes**

Run: `bash install-opencode.test.sh`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add install-opencode.sh install-opencode.test.sh install-lib.sh
git commit -m "refactor: point opencode install at shared runtime"
```

### Task 4: Add Codex shim and installer

**TDD scenario:** New feature — full TDD cycle

**Files:**
- Create: `hooks/bash-approve/run-codex-hook.sh`
- Create: `install-codex.sh`
- Create: `install-codex.test.sh`
- Test: `install-codex.test.sh`

- [ ] **Step 1: Write failing test for Codex config wiring**

```bash
[ -f "$TEST_HOME/.codex/hooks.json" ]
[ -f "$TEST_HOME/.codex/config.toml" ]
rg -n 'codex_hooks = true' "$TEST_HOME/.codex/config.toml"
rg -n 'run-codex-hook.sh' "$TEST_HOME/.codex/hooks.json"
```

Test both user-global and project-local install modes if both are supported.

- [ ] **Step 2: Run test to verify it fails**

Run: `bash install-codex.test.sh`
Expected: FAIL because `install-codex.sh` and `run-codex-hook.sh` do not exist

- [ ] **Step 3: Write minimal implementation**

```bash
# install-codex.sh
install_shared_runtime_bundle
build_shared_runtime_binary
ensure_codex_feature_enabled
ensure_codex_hooks_json
```

```bash
# run-codex-hook.sh
exec "$BINARY" --codex
```

If Go runtime lacks `--codex`, add minimal adapter support in later task before final verification.

- [ ] **Step 4: Run test to verify it passes**

Run: `bash install-codex.test.sh`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add install-codex.sh install-codex.test.sh hooks/bash-approve/run-codex-hook.sh
git commit -m "feat: add codex installer"
```

### Task 5: Add Codex runtime adapter mode

**TDD scenario:** New feature — full TDD cycle

**Files:**
- Modify: `hooks/bash-approve/main.go`
- Create: `hooks/bash-approve/codex_test.go`
- Modify: `hooks/bash-approve/run-codex-hook.sh`
- Test: `hooks/bash-approve/codex_test.go`

- [ ] **Step 1: Write failing test for Codex hook contract**

```go
func TestCodexPermissionRequestAllow(t *testing.T) {
    payload := []byte(`{"hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"git status"},"cwd":"/repo"}`)
    out := runApproveBashCLI(t, []string{"--codex"}, payload)
    assert.JSONEq(t, `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}},"continue":true}`, out)
}
```

Add deny and no-decision coverage too.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd hooks/bash-approve && go test -run TestCodex -v`
Expected: FAIL with unknown flag `--codex` or missing output contract

- [ ] **Step 3: Write minimal implementation**

Add `--codex` mode to parse Codex hook stdin JSON and emit Codex-compatible JSON for `PermissionRequest`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd hooks/bash-approve && go test -run TestCodex -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add hooks/bash-approve/main.go hooks/bash-approve/codex_test.go hooks/bash-approve/run-codex-hook.sh
git commit -m "feat: add codex hook adapter"
```

### Task 6: Update docs

**TDD scenario:** Trivial change — use judgment

**Files:**
- Modify: `README.md`
- Test: none

- [ ] **Step 1: Update install docs**

Document:
- shared runtime location
- Claude/OpenCode now point to shared runtime
- new `install-codex.sh`
- Codex config files and hook feature flag behavior

- [ ] **Step 2: Review rendered commands for correctness**

Run: `rg -n "install-codex|shared runtime|\.codex/hooks.json|codex_hooks" README.md`
Expected: matching lines present and accurate

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document shared runtime and codex install"
```

### Task 7: Full verification

**TDD scenario:** Modifying tested code — run existing tests first

**Files:**
- Modify: none expected
- Test: `install-opencode.test.sh`, `install-codex.test.sh`, `install-shared-runtime.test.sh`, `hooks/bash-approve/codex_test.go`

- [ ] **Step 1: Run installer regression tests**

Run: `bash install-opencode.test.sh && bash install-codex.test.sh && bash install-shared-runtime.test.sh`
Expected: all PASS

- [ ] **Step 2: Run Go runtime tests**

Run: `cd hooks/bash-approve && go test ./...`
Expected: PASS

- [ ] **Step 3: Run typecheck**

Run: `bun run typecheck`
Expected: PASS

- [ ] **Step 4: Commit final fixes if needed**

```bash
git add install-lib.sh install.sh install-opencode.sh install-codex.sh hooks/bash-approve/main.go hooks/bash-approve/run-codex-hook.sh README.md install-opencode.test.sh install-codex.test.sh install-shared-runtime.test.sh hooks/bash-approve/codex_test.go
git commit -m "feat: share runtime across installers"
```
