# Pi Bash-Approve Implementation Plan

> **For agentic workers:** REQUIRED: Use `/skill:orchestrator-implements` (in-session, orchestrator implements), `/skill:subagent-driven-development` (in-session, subagents implement), or `/skill:executing-plans` (parallel session) to implement this plan. Steps use checkbox syntax for tracking.

**Goal:** Add a reusable pi package that reuses the existing Go `bash-approve` runtime to protect pi `bash`, `read`, `grep`, `find`, and `ls` tool calls with documented config/runtime behavior.

**Architecture:** Extend the Go runtime with a dedicated `--pi` JSON adapter mode plus explicit config-path support, then add a package-local pi extension that overrides protected built-in tools and delegates policy decisions to the runtime before execution. Keep repo/worktree path rules in Go, keep host-policy/UI behavior in TypeScript, and ship the runtime as a staged package-local bundle for reusable pi installs.

**Tech Stack:** Go 1.25+, TypeScript, pi-coding-agent extensions, Bun/tsc, existing Go test suite, git/roborev review loop.

---

## File Map

### Go runtime
- Modify: `hooks/bash-approve/main.go`
- Modify: `hooks/bash-approve/git.go`
- Modify: `hooks/bash-approve/main_test.go`
- Create: `hooks/bash-approve/pi_test.go`

### Pi package
- Modify: `package.json`
- Create: `packages/pi-bash-approve/package.json`
- Create: `packages/pi-bash-approve/README.md`
- Create: `packages/pi-bash-approve/extensions/index.ts`
- Create: `packages/pi-bash-approve/src/runtime-contract.ts`
- Create: `packages/pi-bash-approve/src/runtime-parse.ts`
- Create: `packages/pi-bash-approve/src/runtime-client.ts`
- Create: `packages/pi-bash-approve/src/decision-policy.ts`
- Create: `packages/pi-bash-approve/src/protected-tool-queue.ts`
- Create: `packages/pi-bash-approve/src/prompts.ts`
- Create: `packages/pi-bash-approve/src/tool-inputs.ts`
- Create: `packages/pi-bash-approve/src/config.ts`
- Create: `packages/pi-bash-approve/src/errors.ts`
- Create: `packages/pi-bash-approve/src/overrides/bash.ts`
- Create: `packages/pi-bash-approve/src/overrides/read.ts`
- Create: `packages/pi-bash-approve/src/overrides/grep.ts`
- Create: `packages/pi-bash-approve/src/overrides/find.ts`
- Create: `packages/pi-bash-approve/src/overrides/ls.ts`
- Create: `packages/pi-bash-approve/runtime/run-pi-runtime.sh`
- Create: `packages/pi-bash-approve/test/runtime-parse.test.ts`
- Create: `packages/pi-bash-approve/test/decision-policy.test.ts`
- Create: `packages/pi-bash-approve/test/config.test.ts`
- Create: `packages/pi-bash-approve/test/tool-inputs.test.ts`
- Create: `packages/pi-bash-approve/test/runtime-client.test.ts`

### Scripts / docs
- Create: `scripts/sync-pi-runtime.sh`
- Modify: `README.md`
- Create: `docs/pi-supported-tool-calls.md`
- Create: `docs/pi-runtime-contract.md`

---

### Task 1: Add Go `--pi` adapter mode and explicit config-path support

**TDD scenario:** New feature — full TDD cycle

**Files:**
- Modify: `hooks/bash-approve/main.go`
- Modify: `hooks/bash-approve/main_test.go`
- Create: `hooks/bash-approve/pi_test.go`

- [ ] **Step 1: Write failing tests for the `--pi` contract**

Add tests in `hooks/bash-approve/pi_test.go` covering these cases:

```go
func TestPiMode_BashAllow(t *testing.T) {
    cfg := Config{Enabled: []string{"all"}}
    out := runPiModeForTest(t, cfg, `{"tool":"bash","command":"git status","cwd":"`+initGitRepo(t)+`"}`)
    assert.JSONEq(t, `{"version":1,"kind":"decision","tool":"bash","decision":"allow","reason":"git read op"}`, out)
}

func TestPiMode_ReadInRepoAllow(t *testing.T) {
    repo := initGitRepo(t)
    writeFile(t, filepath.Join(repo, "README.md"), "hello")
    out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, `{"tool":"read","path":"README.md","cwd":"`+repo+`"}`)
    assert.JSONEq(t, `{"version":1,"kind":"decision","tool":"read","decision":"allow","reason":"read"}`, out)
}

func TestPiMode_UnsupportedTool(t *testing.T) {
    out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, `{"tool":"write","cwd":"/tmp"}`)
    assert.JSONEq(t, `{"version":1,"kind":"error","error":{"code":"unsupported-tool","message":"unsupported tool: write"}}`, out)
}
```

Also add explicit tests for:
- malformed JSON input → `kind:"error"`, `code:"invalid-input"`
- `grep` with repo-local path → allow
- `find` in repo → allow
- `ls` in repo → allow
- unknown config path with `--config` → `kind:"error"`, `code:"config-error"`

- [ ] **Step 2: Run the failing tests and confirm they fail for the right reason**

Run:

```bash
cd hooks/bash-approve
go test ./... -run 'TestPiMode|TestLoadConfig'
```

Expected:
- FAIL because `--pi` mode helpers/types do not exist yet
- FAIL because explicit config-path support is not implemented yet

- [ ] **Step 3: Implement minimal `--pi` contract types and config-path plumbing**

In `hooks/bash-approve/main.go`, add:

```go
type PiInput struct {
    Tool    string   `json:"tool"`
    Command string   `json:"command,omitempty"`
    Path    string   `json:"path,omitempty"`
    Paths   []string `json:"paths,omitempty"`
    Pattern string   `json:"pattern,omitempty"`
    Cwd     string   `json:"cwd"`
}

type PiDecisionOutput struct {
    Version  int    `json:"version"`
    Kind     string `json:"kind"`
    Tool     string `json:"tool,omitempty"`
    Decision string `json:"decision,omitempty"`
    Reason   string `json:"reason,omitempty"`
    Error    *PiError `json:"error,omitempty"`
}

type PiError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}
```

Add helpers to:
- detect `--pi`
- parse optional `--config <path>`
- load config from explicit path when present
- emit structured `kind:"error"` outputs for invalid input/config/unsupported tools

- [ ] **Step 4: Implement Go-side pi tool evaluation**

Add a helper like:

```go
func evaluatePiToolUse(input PiInput, cfg Config) PiDecisionOutput
```

with dispatch:
- `bash` → existing `Evaluate(...)`
- `read` → existing `evaluateReadTool(...)`
- `grep` → existing `evaluateGrepTool(...)`
- `find` / `ls` → new helpers added in Task 2

Map runtime results to output JSON:
- `result == nil` or `decision == ""` → `decision:"noop"`
- `decisionAllow` / `decisionDeny` / `decisionAsk` → same decision
- deny uses `denyReason` when present

- [ ] **Step 5: Run the focused Go tests again**

Run:

```bash
cd hooks/bash-approve
go test ./... -run 'TestPiMode|TestLoadConfig'
```

Expected:
- PASS for the new `--pi` contract/config tests

- [ ] **Step 6: Commit the runtime adapter task**

```bash
git add hooks/bash-approve/main.go hooks/bash-approve/main_test.go hooks/bash-approve/pi_test.go
git commit -m "feat: add pi runtime adapter mode"
```

---

### Task 2: Extend Go repo/worktree boundary logic for `find` and `ls`

**TDD scenario:** New feature — full TDD cycle

**Files:**
- Modify: `hooks/bash-approve/git.go`
- Create: `hooks/bash-approve/pi_test.go`

- [ ] **Step 1: Write failing tests for `find` and `ls` boundary behavior**

Add tests in `hooks/bash-approve/pi_test.go`:

```go
func TestPiMode_FindInRepoAllow(t *testing.T) {
    repo := initGitRepo(t)
    require.NoError(t, os.MkdirAll(filepath.Join(repo, "subdir"), 0o755))
    out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, `{"tool":"find","path":"subdir","pattern":"*.go","cwd":"`+repo+`"}`)
    assert.JSONEq(t, `{"version":1,"kind":"decision","tool":"find","decision":"allow","reason":"find"}`, out)
}

func TestPiMode_FindOutsideRepoNoop(t *testing.T) {
    repo := initGitRepo(t)
    outside := t.TempDir()
    out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, `{"tool":"find","path":"`+outside+`","pattern":"*.go","cwd":"`+repo+`"}`)
    assert.JSONEq(t, `{"version":1,"kind":"decision","tool":"find","decision":"noop","reason":"find"}`, out)
}

func TestPiMode_LsDefaultCwdAllow(t *testing.T) {
    repo := initGitRepo(t)
    out := runPiModeForTest(t, Config{Enabled: []string{"all"}}, `{"tool":"ls","cwd":"`+repo+`"}`)
    assert.JSONEq(t, `{"version":1,"kind":"decision","tool":"ls","decision":"allow","reason":"ls"}`, out)
}
```

Also add coverage for:
- `find` with omitted path uses `cwd`
- `ls` with explicit relative path in repo → allow
- `ls` with out-of-repo absolute path → noop
- linked worktree path → allow when same repo

- [ ] **Step 2: Run just the new boundary tests to confirm they fail**

Run:

```bash
cd hooks/bash-approve
go test ./... -run 'TestPiMode_(Find|Ls)'
```

Expected:
- FAIL because `find` / `ls` helpers and reasons are not implemented yet

- [ ] **Step 3: Implement `find` and `ls` repo-bounded evaluators in Go**

In `hooks/bash-approve/git.go`, add minimal helpers:

```go
func evaluateFindTool(input PiInput, ctx evalContext) *result {
    target := input.Path
    if target == "" {
        target = "."
    }
    if pathInCurrentRepo(ctx.cwd, target) {
        return approved("find")
    }
    return &result{reason: "find", decision: ""}
}

func evaluateLsTool(input PiInput, ctx evalContext) *result {
    target := input.Path
    if target == "" {
        target = "."
    }
    if pathInCurrentRepo(ctx.cwd, target) {
        return approved("ls")
    }
    return &result{reason: "ls", decision: ""}
}
```

Keep path canonicalization in existing helpers like `pathInCurrentRepo(...)`.

- [ ] **Step 4: Re-run the focused `find` / `ls` tests**

Run:

```bash
cd hooks/bash-approve
go test ./... -run 'TestPiMode_(Find|Ls)'
```

Expected:
- PASS for new repo/worktree boundary cases

- [ ] **Step 5: Commit the boundary support task**

```bash
git add hooks/bash-approve/git.go hooks/bash-approve/pi_test.go
git commit -m "feat: add pi repo-bounded find and ls support"
```

---

### Task 3: Create the reusable pi package scaffold and staged runtime sync flow

**TDD scenario:** New feature / new files — full TDD cycle where practical; config/script/docs scaffolding may use judgment

**Files:**
- Modify: `package.json`
- Create: `packages/pi-bash-approve/package.json`
- Create: `packages/pi-bash-approve/README.md`
- Create: `packages/pi-bash-approve/runtime/run-pi-runtime.sh`
- Create: `scripts/sync-pi-runtime.sh`

- [ ] **Step 1: Write a failing package/runtime smoke test entrypoint expectation**

Add a package test placeholder in `packages/pi-bash-approve/test/runtime-client.test.ts` that expects the runtime shim path to exist:

```ts
test("runtime shim exists in staged package runtime", async () => {
  const shim = path.resolve(import.meta.dirname, "../runtime/run-pi-runtime.sh");
  await expect(fs.access(shim)).resolves.toBeUndefined();
});
```

- [ ] **Step 2: Run the new package test and confirm it fails**

Run:

```bash
bun test packages/pi-bash-approve/test/runtime-client.test.ts
```

Expected:
- FAIL because the package/runtime scaffold does not exist yet

- [ ] **Step 3: Add the new workspace and package metadata**

Update root `package.json` workspaces to include:

```json
{
  "workspaces": [
    "opencode-tester",
    "packages/pi-bash-approve"
  ]
}
```

Create `packages/pi-bash-approve/package.json` with:
- `private: true` for now
- `type: "module"`
- `pi.extensions: ["./extensions"]`
- scripts for `typecheck` and `test`
- `peerDependencies` for `@mariozechner/pi-coding-agent`, `@sinclair/typebox`, `@mariozechner/pi-ai`

- [ ] **Step 4: Create the staged runtime shim and sync script**

Create `packages/pi-bash-approve/runtime/run-pi-runtime.sh` modeled after the existing hook shim, but invoking `--pi` and accepting an optional config arg.

Create `scripts/sync-pi-runtime.sh` to copy:
- `hooks/bash-approve/*.go`
- `hooks/bash-approve/go.mod`
- `hooks/bash-approve/go.sum`
- `hooks/bash-approve/categories.yaml`

into `packages/pi-bash-approve/runtime/`.

- [ ] **Step 5: Re-run the shim existence test**

Run:

```bash
bun test packages/pi-bash-approve/test/runtime-client.test.ts
```

Expected:
- PASS for the existence/smoke expectation

- [ ] **Step 6: Commit the package scaffold task**

```bash
git add package.json packages/pi-bash-approve scripts/sync-pi-runtime.sh
git commit -m "feat: scaffold pi bash-approve package"
```

---

### Task 4: Implement TypeScript runtime adapter, config resolution, and decision policy

**TDD scenario:** New feature — full TDD cycle

**Files:**
- Create: `packages/pi-bash-approve/src/runtime-contract.ts`
- Create: `packages/pi-bash-approve/src/runtime-parse.ts`
- Create: `packages/pi-bash-approve/src/runtime-client.ts`
- Create: `packages/pi-bash-approve/src/decision-policy.ts`
- Create: `packages/pi-bash-approve/src/tool-inputs.ts`
- Create: `packages/pi-bash-approve/src/config.ts`
- Create: `packages/pi-bash-approve/src/errors.ts`
- Create: `packages/pi-bash-approve/src/protected-tool-queue.ts`
- Create: `packages/pi-bash-approve/src/prompts.ts`
- Create: `packages/pi-bash-approve/test/runtime-parse.test.ts`
- Create: `packages/pi-bash-approve/test/decision-policy.test.ts`
- Create: `packages/pi-bash-approve/test/config.test.ts`
- Create: `packages/pi-bash-approve/test/tool-inputs.test.ts`
- Create: `packages/pi-bash-approve/test/runtime-client.test.ts`

- [ ] **Step 1: Write failing TypeScript unit tests**

Add tests covering:

`runtime-parse.test.ts`
```ts
test("parses valid allow decision", () => {
  expect(parseRuntimeOutput('{"version":1,"kind":"decision","tool":"bash","decision":"allow"}')).toEqual({
    version: 1,
    kind: "decision",
    tool: "bash",
    decision: "allow",
  });
});

test("rejects unknown decision", () => {
  expect(() => parseRuntimeOutput('{"version":1,"kind":"decision","tool":"bash","decision":"review"}')).toThrow(/unknown decision/i);
});
```

`decision-policy.test.ts`
```ts
test("ask becomes prompt with UI", () => {
  expect(normalizeDecision({ version: 1, kind: "decision", tool: "bash", decision: "ask" }, { hasUI: true })).toEqual({ kind: "prompt" });
});

test("noop becomes deny without UI", () => {
  expect(normalizeDecision({ version: 1, kind: "decision", tool: "read", decision: "noop" }, { hasUI: false })).toEqual({ kind: "block" });
});
```

`config.test.ts`
```ts
test("repo-root config is discovered when cwd starts in a subdirectory", async () => {
  // setup repoRoot/.pi/bash-approve.json and repoRoot/sub/dir cwd
});

test("global config is used when no project-local config exists", async () => {
  // setup only ~/.pi/agent/bash-approve.json equivalent fixture
});

test("upward walk stops before ~/.pi pseudo-global path", async () => {
  // setup ~/.pi/bash-approve.json and assert it is ignored
});
```

`tool-inputs.test.ts`
```ts
test("maps grep path and paths to runtime input", () => {
  expect(toRuntimeInput("grep", { pattern: "foo", path: "src", paths: ["test"] }, "/repo")).toEqual({
    tool: "grep",
    pattern: "foo",
    path: "src",
    paths: ["test"],
    cwd: "/repo",
  });
});
```

`runtime-client.test.ts`
```ts
test("prefers repo-local runtime in source checkout", async () => {
  // fake filesystem/process env and assert chosen runtime path
});
```

- [ ] **Step 2: Run the package tests and confirm they fail**

Run:

```bash
bun test packages/pi-bash-approve/test/*.test.ts
```

Expected:
- FAIL because adapter modules do not exist yet

- [ ] **Step 3: Implement the runtime contract and parser**

In `runtime-contract.ts`, define the tagged unions from the design.

In `runtime-parse.ts`, add a strict parser that:
- `JSON.parse`s stdout
- validates `version`, `kind`, and union members
- throws stable parse/schema errors on invalid shapes

- [ ] **Step 4: Implement config resolution and runtime path selection**

In `config.ts`, implement:
- global config path: `~/.pi/agent/bash-approve.json`
- project config path: `<repo-or-worktree-root>/.pi/bash-approve.json`
- non-repo fallback: upward walk for `.pi/bash-approve.json`, stopping at `$HOME` and never treating `~/.pi/bash-approve.json` as valid
- cache keyed by effective project boundary, not raw cwd

In `runtime-client.ts`, implement runtime selection precedence:
1. explicit `runtimePath`
2. repo-local source runtime when inside this repo checkout
3. bundled runtime shim

- [ ] **Step 5: Implement decision normalization, prompts, and queue**

In `decision-policy.ts`, map runtime outputs to internal actions:
- `allow` → execute
- `deny` → block
- `ask` / `noop` with UI → prompt
- `ask` / `noop` without UI → block
- runtime/parse failures → block

In `prompts.ts`, add stable prompt text builders for:
- bash approval prompts
- repo-boundary prompts for read-like tools

In `protected-tool-queue.ts`, add a minimal async mutex/queue.

- [ ] **Step 6: Re-run the full package test suite**

Run:

```bash
bun test packages/pi-bash-approve/test/*.test.ts
bun run --cwd packages/pi-bash-approve typecheck
```

Expected:
- PASS for adapter unit tests
- PASS for package typecheck

- [ ] **Step 7: Commit the adapter core task**

```bash
git add packages/pi-bash-approve/src packages/pi-bash-approve/test package.json
git commit -m "feat: add pi runtime adapter core"
```

---

### Task 5: Implement protected pi tool overrides and extension entrypoint

**TDD scenario:** New feature — full TDD cycle

**Files:**
- Create: `packages/pi-bash-approve/extensions/index.ts`
- Create: `packages/pi-bash-approve/src/overrides/bash.ts`
- Create: `packages/pi-bash-approve/src/overrides/read.ts`
- Create: `packages/pi-bash-approve/src/overrides/grep.ts`
- Create: `packages/pi-bash-approve/src/overrides/find.ts`
- Create: `packages/pi-bash-approve/src/overrides/ls.ts`
- Modify: `packages/pi-bash-approve/test/runtime-client.test.ts` (or add `overrides.test.ts`)

- [ ] **Step 1: Write failing override tests**

Add override-level tests that assert:

```ts
test("bash override delegates to built-in execute on allow", async () => {
  // fake runtime output allow + fake built-in tool execute spy
});

test("read override prompts on noop when UI is available", async () => {
  // fake noop + fake ctx.ui.confirm => true
});

test("grep override throws when prompt is rejected", async () => {
  // fake noop + fake ctx.ui.confirm => false
});
```

Also add explicit coverage for:
- no-UI `ask` / `noop` → throw/block
- queue serialization for two protected calls
- runtime-client errors propagate as thrown tool errors

- [ ] **Step 2: Run override tests and confirm failure**

Run:

```bash
bun test packages/pi-bash-approve/test/*.test.ts
```

Expected:
- FAIL because override modules and extension entrypoint do not exist yet

- [ ] **Step 3: Implement per-tool overrides using built-in pi tool factories**

For each override module:
- create the built-in tool via the correct `create*Tool(ctx.cwd)` factory at execution time
- convert tool input via `tool-inputs.ts`
- call runtime client
- normalize decision
- prompt via `ctx.ui.confirm()` when needed
- throw stable errors on deny/reject/runtime failure
- delegate to the built-in tool’s `execute` when allowed

In `extensions/index.ts`, register all five overrides.

- [ ] **Step 4: Re-run package tests and typecheck**

Run:

```bash
bun test packages/pi-bash-approve/test/*.test.ts
bun run --cwd packages/pi-bash-approve typecheck
```

Expected:
- PASS for override behavior
- PASS for package typecheck

- [ ] **Step 5: Commit the override task**

```bash
git add packages/pi-bash-approve/extensions packages/pi-bash-approve/src/overrides packages/pi-bash-approve/test
git commit -m "feat: add protected pi tool overrides"
```

---

### Task 6: Sync runtime assets, update docs, and run final verification

**TDD scenario:** Modifying tested code — run existing tests before and after; docs changes use judgment

**Files:**
- Modify: `README.md`
- Create: `docs/pi-supported-tool-calls.md`
- Create: `docs/pi-runtime-contract.md`
- Modify: `packages/pi-bash-approve/README.md`
- Modify: `scripts/sync-pi-runtime.sh` if needed after live validation

- [ ] **Step 1: Run existing verification before final doc/runtime sync changes**

Run:

```bash
cd hooks/bash-approve && go test ./...
cd ../..
bun test packages/pi-bash-approve/test/*.test.ts
bun run --cwd packages/pi-bash-approve typecheck
bun run --cwd opencode-tester typecheck
```

Expected:
- PASS across Go and TypeScript suites before doc-only finish-up changes

- [ ] **Step 2: Sync the staged runtime and verify staged assets are current**

Run:

```bash
./scripts/sync-pi-runtime.sh
git diff -- packages/pi-bash-approve/runtime
```

Expected:
- staged runtime updated to match `hooks/bash-approve/`
- no unexpected drift outside the runtime bundle

- [ ] **Step 3: Write the missing docs**

Update/create docs with the exact supported surface:

`docs/pi-supported-tool-calls.md`
```md
# Pi Supported Tool Calls

## Claude-parity support
- bash
- read
- grep

## Pi-specific additions
- find
- ls

## Not supported yet
- write
- edit
- user_bash
- custom tools
```

`docs/pi-runtime-contract.md`
```md
# Pi Runtime Contract

## Input
- tool-tagged JSON over stdin

## Output
- { version: 1, kind: "decision", ... }
- { version: 1, kind: "error", ... }
```

Update root/package README sections for install/config/runtime behavior.

- [ ] **Step 4: Run final full verification**

Run:

```bash
cd hooks/bash-approve && go test ./...
cd ../..
bun test packages/pi-bash-approve/test/*.test.ts
bun run --cwd packages/pi-bash-approve typecheck
bun run --cwd opencode-tester typecheck
```

Expected:
- PASS across all verification commands

- [ ] **Step 5: Commit the final integration/docs task**

```bash
git add README.md docs/pi-supported-tool-calls.md docs/pi-runtime-contract.md packages/pi-bash-approve scripts/sync-pi-runtime.sh
git commit -m "docs: add pi package docs and sync runtime bundle"
```
