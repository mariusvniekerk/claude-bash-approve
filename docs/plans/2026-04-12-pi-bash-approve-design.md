# Pi Bash-Approve Integration Design

**Date:** 2026-04-12  
**Status:** Working Draft

## Goal

Integrate the existing `bash-approve` policy engine into **pi-coding-agent** as a reusable pi package, reusing the current Go runtime as the source of truth for policy decisions while adapting its output into pi's extension-driven permission model.

The initial pi integration should protect the same meaningful surfaces we can support safely without inventing a large new policy layer:

- `bash` via the existing AST/pattern-based approval engine
- `read` and `grep` with the same repo/worktree boundary rules as the Claude-based integration
- `find` and `ls` with pi-specific repo/worktree boundary rules intentionally modeled after the existing `read` behavior

The integration should **not** attempt to protect human-driven `user_bash` (`!` / `!!`) commands. Human operators are trusted to manage those themselves.

## User-Approved Design Decisions

These decisions were explicitly chosen during design discussion:

1. **Scope posture:** Design for broader future permission coverage, but implement the current package around a constrained, clearly documented supported-tool set.
2. **Unknown / non-terminal outcomes:** If the runtime does not explicitly allow or deny (`ask` / `noop`-style outcomes), then:
   - **interactive / RPC modes:** prompt the user
   - **non-UI modes:** block
3. **Delivery model:** Support both:
   - project-local development in this repo
   - reusable pi package installation for general use
4. **Policy engine source of truth:** Reuse the existing Go runtime rather than reimplementing policy in TypeScript.
5. **TypeScript contract:** Define explicit TypeScript types for the runtime JSON emitted for pi.
6. **`user_bash` coverage:** Out of scope for v1.
7. **Two-stage preflight/execution approval state:** Rejected as unnecessary overhead for the chosen scope.
8. **Non-bash tool support parity:** Stick to strict parity with current Claude behavior for repo-bounded tools (`read`, `grep`), but additionally support pi-native repo-bounded `find` and `ls`.
9. **Documentation requirement:** Write a reference document describing the currently supported tool-call classes and semantics so future expansion can refer back to a stable baseline.

## Pi Constraints That Shape The Design

Pi does **not** provide a built-in permission API comparable to OpenCode's permission request/reply model. Its permission surfaces are extension-owned.

The relevant pi primitives are:

- `tool_call` event hooks can block or mutate tool calls
- built-in tools can be overridden by registering a tool with the same name
- extension UI can prompt users with `ctx.ui.confirm()` / `ctx.ui.select()`
- RPC mode supports these prompts through the extension UI protocol
- print/json modes do not support interactive prompting (`ctx.hasUI === false`)

Pi documentation also explicitly notes an important semantic constraint:

> `tool_call` input is mutable, later handlers can observe earlier mutations, and pi does not re-validate after mutation.

That makes a pure `tool_call` gate insufficient as an authoritative enforcement mechanism when we care that the command approved is the command executed.

## Recommended Architecture

### High-level recommendation

Use a **single-stage authoritative built-in tool override** rather than a two-stage preflight design.

The pi package should override the built-in protected tools and consult the Go runtime immediately before actual execution. This keeps the design simpler while still ensuring the runtime evaluates the exact command/path payload that the overridden tool will execute.

### Why not a `tool_call`-only gate?

A `tool_call`-only design was considered and rejected because:

- later handlers can mutate the tool input after approval
- pi does not re-run validation after such mutation
- it does not improve our chosen non-`user_bash` scope enough to justify the weaker invariant

### Why not the earlier two-stage design?

A two-stage design (preflight approval + execution-time validation) would have been justifiable if we also needed to protect `user_bash` or coordinate multiple approval surfaces. Once `user_bash` was declared out of scope, that design became unnecessary complexity.

### Final structure

- **Go runtime remains the source of truth** for:
  - bash AST parsing and wrapper/core command matching
  - deny/ask/noop/allow decisions
  - repo/worktree path boundary logic
  - telemetry
- **TypeScript pi package becomes the adapter** for:
  - invoking the Go runtime
  - validating/parsing its JSON
  - mapping decisions into pi behavior
  - prompting the user when needed
  - delegating allowed execution to the corresponding built-in pi tool implementations

## Package Boundaries

To keep the codebase understandable and reusable, the pi integration should live behind a dedicated package boundary.

### Proposed structure

- `hooks/bash-approve/`
  - Go source of truth
  - current Claude/OpenCode support remains here
  - new pi adapter mode should also live here
- `packages/pi-bash-approve/`
  - reusable pi package
  - extension entrypoint
  - TypeScript adapter/runtime client
  - config loading
  - approval UI logic
  - package-level docs

### Rationale

This preserves a clean separation between three integration surfaces:

1. Claude hook mode
2. OpenCode adapter mode
3. pi adapter mode

They should share the same engine, but not share host-specific output formats or extension logic.

## New Go Adapter Mode

The existing OpenCode adapter mode is bash-only and is not the right machine interface for pi because pi v1 needs support for more than bash.

### Recommendation

Add a new explicit runtime mode:

```bash
go run ./hooks/bash-approve --pi
```

This mode should be a machine-oriented contract specifically for pi and should be independent from:

- default Claude hook output
- existing `--opencode` output

## TypeScript Runtime Contracts

The pi package should define explicit TypeScript types for both the stdin input contract and stdout output contract used by the Go runtime.

### Input contract

```ts
export type PiRuntimeInput =
  | {
      tool: "bash";
      command: string;
      cwd: string;
    }
  | {
      tool: "read";
      path: string;
      cwd: string;
    }
  | {
      tool: "grep";
      pattern: string;
      path?: string;
      paths?: string[];
      cwd: string;
    }
  | {
      tool: "find";
      path?: string;
      pattern: string;
      cwd: string;
    }
  | {
      tool: "ls";
      path?: string;
      cwd: string;
    };
```

Notes:

- `cwd` is always explicit.
- `grep` models both `path` and `paths` because current repo-bounded grep logic uses both.
- `find` and `ls` are intentionally simple path-rooted inspections for v1.

### Output contract

Use a versioned, tagged union:

```ts
export type PiSupportedTool = "bash" | "read" | "grep" | "find" | "ls";

export type PiPolicyDecision = "allow" | "deny" | "ask" | "noop";

export type PiRuntimeDecisionOutput = {
  version: 1;
  kind: "decision";
  tool: PiSupportedTool;
  decision: PiPolicyDecision;
  reason?: string;
};

export type PiRuntimeErrorCode =
  | "invalid-input"
  | "unsupported-tool"
  | "config-error"
  | "internal-error";

export type PiRuntimeErrorOutput = {
  version: 1;
  kind: "error";
  error: {
    code: PiRuntimeErrorCode;
    message: string;
  };
};

export type PiRuntimeOutput = PiRuntimeDecisionOutput | PiRuntimeErrorOutput;
```

### Contract expectations

- Successful evaluations emit `kind: "decision"`.
- Structured failures emit `kind: "error"`.
- The runtime may still exit non-zero for error outputs; pi should parse stdout when present but treat any runtime error conservatively.
- Unknown `version`, `kind`, `decision`, or malformed output should be treated as invalid and fail closed.

## Supported Tool Classes In Pi v1

### Supported

#### `bash`
Uses the full Go policy engine.

Behavior:
- `allow` → execute
- `deny` → block
- `ask` → prompt in UI modes, block in non-UI modes
- `noop` → prompt in UI modes, block in non-UI modes

#### `read`
Uses existing repo/worktree-bounded path logic from the Claude-based integration.

Behavior:
- allow only when the resolved path is inside the current repo or linked worktree root
- otherwise emit `noop`
- pi maps `noop` to prompt/block based on UI availability

#### `grep`
Uses existing repo/worktree-bounded path logic from the Claude-based integration.

Behavior:
- allow only when all referenced paths stay inside the current repo/worktree boundary
- no-path behavior should follow current Go semantics
- otherwise emit `noop`
- pi maps `noop` to prompt/block based on UI availability

#### `find`
Pi-specific addition for v1.

Behavior:
- treat `find` as a read-like path-bounded tool
- effective search root:
  - `path` if provided
  - otherwise `cwd`
- allow only when the effective root is inside the current repo/worktree boundary
- otherwise emit `noop`

#### `ls`
Pi-specific addition for v1.

Behavior:
- treat `ls` as a read-like path-bounded tool
- effective target path:
  - `path` if provided
  - otherwise `cwd`
- allow only when the effective target is inside the current repo/worktree boundary
- otherwise emit `noop`

### Explicitly unsupported in v1

These should be called out in package docs and the support reference doc:

- `write`
- `edit`
- `user_bash` (`!` / `!!`)
- custom extension tools
- session lifecycle actions (`/new`, `/fork`, `/resume`, etc.)
- advanced persistent approval memory (`allow always`)
- sandbox enforcement as a default package behavior

## Per-Tool Decision Mapping In Pi

This is the normative policy mapping for the pi package.

### UI-capable modes

Applies to:
- interactive mode
- RPC mode (`ctx.hasUI === true` because pi's extension UI protocol is active)

Mapping:
- `allow` → execute
- `deny` → block
- `ask` → prompt
- `noop` → prompt
- runtime failure / parse failure / schema violation → block

### Non-UI modes

Applies to:
- print mode
- json mode

Mapping:
- `allow` → execute
- `deny` → block
- `ask` → block
- `noop` → block
- runtime failure / parse failure / schema violation → block

This exactly matches the chosen design policy: prompt when possible, otherwise block.

## Approval UX

### `bash` prompt UX

When `bash` yields `ask` or `noop`, show a simple confirmation dialog.

Recommended title:

```text
Allow bash command?
```

Recommended body includes:
- the command text
- the working directory
- the policy reason if present

For v1, use `ctx.ui.confirm()` rather than a richer select menu.

Outcomes:
- confirmed → execute
- rejected / cancelled / timed out → block

### Repo-bounded tool prompt UX (`read` / `grep` / `find` / `ls`)

When these tools yield `noop`, show a simpler path-boundary confirmation dialog.

Recommended title:

```text
Allow out-of-bounds tool access?
```

Recommended body includes:
- tool name
- requested path or paths
- current working directory
- reason, such as:
  - `path outside current repo/worktree`
  - `could not determine repo root`

Again, use `ctx.ui.confirm()` for v1.

## Concurrency Model

Pi can run sibling tool calls concurrently. Even with the simplified single-stage design, concurrent prompts would be confusing and brittle.

### Recommendation

Serialize adjudication and execution for the protected tool set:

- `bash`
- `read`
- `grep`
- `find`
- `ls`

This is a UX and predictability choice rather than a security boundary by itself.

### Why serialize?

It avoids:
- overlapping prompts
- ambiguous approval order
- inconsistent prompt/render behavior when several protected tools appear in one turn

### Possible future optimization

If this proves too conservative, a future revision could allow parallel execution for immediate-`allow` decisions while serializing only prompt-producing cases. That is not needed for v1.

## Runtime Path Discovery

A reusable pi package cannot rely on being run only from this repository checkout.

### Recommended runtime resolution precedence

1. explicit package config `runtimePath`
2. repo-local development path when running from this repository checkout
3. bundled runtime inside the installed pi package
4. otherwise fail clearly

### Development-safety requirement

When the package is being run from this source repository, local development must prefer the live repo-local runtime over staged package assets. The package must not silently execute a stale synced copy from `packages/pi-bash-approve/runtime/` when a newer source-of-truth implementation exists under `hooks/bash-approve/`.

Two acceptable strategies were considered:

- auto-detect the source checkout and prefer the repo-local runtime
- require an explicit development override such as `runtimePath`

For v1, the recommended behavior is **automatic repo-local preference** when the current package is running from this repository checkout. That keeps local verification aligned with the stated source-of-truth model and reduces the chance of validating stale staged assets by accident.

### Why this matters

We need to support both:
- project-local development against this repo
- reusable installation via pi package loading

## Runtime Config Discovery

The current Go runtime resolves `categories.yaml` relative to the executable. That is workable for current hook installs, but not ideal for reusable pi package installation with project/global overrides.

### Recommendation

Add explicit config-path support to the Go runtime, preferably:

```bash
--config <path>
```

### Recommended pi-side config precedence

1. explicit package config `categoriesPath`
2. project-local pi package config path
3. global pi package config path
4. runtime-local bundled `categories.yaml`

Without explicit Go config-path support, project-local pi configuration becomes awkward and tightly coupled to executable placement.

## Edge-Case Decision Matrix

This table defines the expected behavior.

| Case | UI available | Result |
|---|---:|---|
| runtime returns `allow` | yes/no | execute |
| runtime returns `deny` | yes/no | block |
| runtime returns `ask` | yes | prompt |
| runtime returns `ask` | no | block |
| runtime returns `noop` | yes | prompt |
| runtime returns `noop` | no | block |
| runtime exits non-zero | yes/no | block |
| runtime stdout invalid JSON | yes/no | block |
| runtime stdout JSON with unknown version | yes/no | block |
| runtime stdout JSON with unknown decision | yes/no | block |
| prompt approved | yes | execute |
| prompt rejected | yes | block |
| prompt cancelled | yes | block |
| prompt timed out | yes | block |

## Tool-Specific Edge Cases

### `bash`

- empty command → invalid input / block
- multiline command → supported, passed as JSON stdin, not via shell-escaped argv
- huge command → still pass via stdin JSON; if runtime rejects, block
- runtime failure → block

### `read`

- relative path → resolve from `cwd`
- absolute path → evaluate directly
- symlink traversal → keep canonicalization in Go; do not duplicate in TS
- missing file → permission boundary and operational existence are separate concerns; in-bounds permission may still allow a read that later fails because the file does not exist

### `grep`

- combine `path` and `paths` exactly as current Go behavior does
- if no path is provided, follow current cwd/repo-root logic from Go
- any mixed in-bounds/out-of-bounds path set should yield `noop`

### `find`

- missing `path` → default to `cwd`
- relative `path` → resolve from `cwd`
- absolute `path` → evaluate directly
- pattern semantics are not policy-relevant in v1; only the effective root matters

### `ls`

- missing `path` → default to `cwd`
- repo root → allow
- linked worktree root belonging to same repo → allow
- nonexistent path → same rule as `read`: boundary decision and operational existence remain separate

## Error Handling Philosophy

The pi integration should fail closed.

### Runtime invocation failures

Examples:
- runtime path missing
- process spawn failure
- runtime exits with error
- malformed stdout
- schema drift / unrecognized decision

Behavior:
- block the tool call
- surface a clear reason when possible
- do not silently downgrade to prompt or allow

This is stricter than treating runtime failures as ordinary approval-needed cases, and that is intentional. If the policy engine cannot be trusted to evaluate, execution should not proceed.

## Documentation Requirements

This design requires two documentation streams.

### 1. Main pi integration design / implementation docs

These should explain:
- package structure
- runtime discovery
- config precedence
- supported host modes
- approval behavior

### 2. Supported tool-call reference doc

This should explicitly document:

#### Claude-parity support
- `bash`
- `read`
- `grep`

#### Pi-specific v1 additions
- `find`
- `ls`

#### Unsupported surfaces
- `write`
- `edit`
- `user_bash`
- custom tools

#### Decision semantics in pi
- `allow`
- `deny`
- `ask`
- `noop`
- why `noop` becomes prompt/block instead of falling through to another permission layer

This doc is important because otherwise users may incorrectly assume that a “pi bash-approve package” protects every tool uniformly.

## Approaches Considered

### 1. Thin `tool_call` gate only

Rejected.

Reason:
- too weak as an authoritative enforcement mechanism in pi because later handlers can mutate inputs without re-validation

### 2. Full tool override only

Accepted.

Reason:
- simplest design that still ensures the exact executed command/path payload is the one evaluated by policy
- matches the chosen scope after `user_bash` was declared out of scope

### 3. Two-stage approval cache + execution verification

Rejected for v1.

Reason:
- once `user_bash` and broader multi-surface coordination were removed from scope, this became unnecessary complexity

## Compatibility / Platform Notes

The package should be documented as targeting Unix-like environments where the existing runtime assumptions already hold.

Practical v1 stance:
- macOS: supported
- Linux: supported
- Windows: not a supported target for v1

## Exact Package / Module Layout

The pi integration should be implemented as a focused reusable package with small modules that each own one responsibility.

### Proposed layout

```text
packages/pi-bash-approve/
├── package.json
├── README.md
├── extensions/
│   └── index.ts
├── src/
│   ├── config.ts
│   ├── runtime-contract.ts
│   ├── runtime-parse.ts
│   ├── runtime-client.ts
│   ├── decision-policy.ts
│   ├── protected-tool-queue.ts
│   ├── prompts.ts
│   ├── tool-inputs.ts
│   ├── overrides/
│   │   ├── bash.ts
│   │   ├── read.ts
│   │   ├── grep.ts
│   │   ├── find.ts
│   │   └── ls.ts
│   └── errors.ts
└── test/
    ├── runtime-parse.test.ts
    ├── decision-policy.test.ts
    ├── runtime-client.test.ts
    ├── config.test.ts
    └── overrides.test.ts
```

### Module responsibilities

#### `extensions/index.ts`
Single package entrypoint.

Responsibilities:
- register all protected-tool overrides
- create shared queue and config/runtime resolver dependencies
- ensure config is resolved against a stable project/worktree root rather than raw transient cwd
- define when config is refreshed as cwd/project context changes
- avoid owning policy logic directly

### Config refresh / cwd-change requirement

The package must not assume that a single config snapshot loaded at extension start is valid forever. Pi sessions can move across directories or start from a subdirectory, and protected-tool decisions are cwd-sensitive.

Therefore, v1 should define config resolution in terms of the **effective repo/worktree root for the current execution context**:

- for each protected tool execution, determine the current repo/worktree root from `ctx.cwd`
- resolve project config relative to that stable root
- reuse cached config only when the effective root is unchanged
- invalidate/reload when the effective root changes

This prevents one project's `enabled`, `runtimePath`, or `categoriesPath` settings from bleeding into another project's executions.

#### `src/runtime-contract.ts`
Pure TypeScript type definitions for the `--pi` stdin/stdout contract.

Responsibilities:
- define the tagged unions described earlier
- define helper runtime/tool enums and narrow types
- remain side-effect free

#### `src/runtime-parse.ts`
Strict runtime output parsing and validation.

Responsibilities:
- parse stdout JSON
- reject malformed payloads, unknown versions, unknown decisions, and invalid unions
- return typed `PiRuntimeOutput`
- never make policy decisions

#### `src/runtime-client.ts`
Owns process spawning and stdin/stdout handling.

Responsibilities:
- resolve runtime path / shim path
- invoke runtime with `--pi`
- pass `--config` if configured
- write JSON to stdin
- capture stdout/stderr/exit code
- map spawn/invocation failures into internal adapter errors

Important edge-case requirement:
- this module must **spawn directly**, not via `bash -c` or shell interpolation
- command/path payloads must travel only as JSON over stdin

#### `src/decision-policy.ts`
Maps typed runtime results into pi actions.

Responsibilities:
- turn `allow` / `deny` / `ask` / `noop` into normalized actions such as:
  - execute
  - prompt
  - block
- apply UI-availability policy
- keep host-policy decisions out of runtime parsing

#### `src/protected-tool-queue.ts`
Implements the queue/mutex for serialized protected-tool adjudication and execution.

Responsibilities:
- guarantee one protected tool adjudication/execution pipeline at a time
- keep queue semantics independent from specific tools

#### `src/prompts.ts`
Builds user-facing prompt strings.

Responsibilities:
- format `bash` prompt bodies
- format repo-boundary prompt bodies for `read` / `grep` / `find` / `ls`
- centralize wording so tests can snapshot or assert prompt text deterministically

#### `src/tool-inputs.ts`
Normalizes pi tool arguments into the `PiRuntimeInput` contract.

Responsibilities:
- map pi `read`, `grep`, `find`, `ls`, and `bash` arguments to runtime input
- validate required fields before runtime invocation
- keep per-tool input shaping out of the override modules

#### `src/overrides/*.ts`
One override module per protected built-in tool.

Responsibilities:
- define one override wrapper per built-in tool
- call shared adjudication helpers
- delegate allowed executions to the real pi built-in tool factories
- keep tool-specific prompt context and input mapping local to that tool

#### `src/config.ts`
Package config discovery and normalization.

Responsibilities:
- read project/global config files
- resolve `runtimePath` / `categoriesPath`
- normalize booleans and defaults
- keep config precedence rules testable

#### `src/errors.ts`
Internal error classes/helpers.

Responsibilities:
- distinguish runtime invocation errors from parse/schema failures and user-deny outcomes
- keep error text stable enough for tests and documentation

## Built-In Tool Override Strategy

The overrides should preserve pi's built-in rendering and result semantics as much as possible.

### Recommendation

Use pi's built-in tool factories and wrap them, rather than reimplementing tool behavior from scratch:

- `createBashTool`
- `createReadTool`
- `createGrepTool`
- `createFindTool`
- `createLsTool`

### Important detail

Pi docs note that built-in prompt metadata is not inherited automatically when overriding tools unless the override carries it forward. Therefore, each override should be constructed by **spreading the built-in tool definition** and replacing only `execute` (plus label text only if intentionally changed).

That pattern preserves:
- parameters
- prompt metadata
- built-in renderers
- result detail shapes for successful executions

### Cwd edge case

Pi's tool factories are cwd-sensitive. The docs explicitly warn that tool instances created with one cwd should not be reused for a different cwd when correctness matters.

Therefore, the override design should:
- use a built-in tool instance created during registration only as a template for metadata/renderers if needed
- create a **fresh delegate tool instance per execution** using the actual `ctx.cwd`

This avoids stale cwd mismatches after session changes or runtime reuse.

## Deny / Prompt-Reject Result Strategy

A denied or user-rejected protected tool call should be reported to pi and the model as an **error**, not as a successful tool result containing denial text.

### Recommendation

For deny-like outcomes, the override should throw a stable error type/message rather than returning a synthetic success result.

Rationale:
- denial is semantically a failed tool execution
- the model should see it as a blocked operation, not a successful read/bash/etc.
- this avoids fabricating successful built-in result detail shapes for operations that never ran

This should apply to:
- policy `deny`
- user rejected prompt
- prompt timeout/cancel
- runtime invocation failure
- runtime schema/parse failure

## Package Config Model

Keep v1 config intentionally small.

### Proposed config file locations

- global: `~/.pi/agent/bash-approve.json`
- project-local: `<repo-or-worktree-root>/.pi/bash-approve.json`

Project config must be resolved from a stable project boundary, not from the raw current cwd string. The adapter should determine the effective repo/worktree root for `ctx.cwd` and then look for `.pi/bash-approve.json` at that root.

If `ctx.cwd` is not inside a git repo/worktree, the adapter should either:
- walk upward looking for a project-local `.pi/bash-approve.json`, or
- fall back directly to global config when no stable project root can be established

For v1, the recommended behavior is:

1. determine repo/worktree root for `ctx.cwd` when possible
2. if found, use `<root>/.pi/bash-approve.json`
3. if not found, walk upward from `ctx.cwd` looking for `.pi/bash-approve.json`, but stop before crossing into the user's home-directory pseudo-global space
4. specifically, the upward walk must stop at `$HOME` (or filesystem root if no home directory can be determined) and must **not** treat `~/.pi/bash-approve.json` as a valid project config location
5. if no project-local config is found by that point, use global config at `~/.pi/agent/bash-approve.json`

This keeps repo-scoped policy stable when pi starts in a subdirectory, avoids missing a root-level config file in non-repo projects, and prevents cwd-dependent accidental precedence from undocumented pseudo-global paths under `$HOME`.

### Proposed config shape

```ts
export type PiBashApproveConfig = {
  enabled?: boolean;
  runtimePath?: string;
  categoriesPath?: string;
};
```

### Default behavior

- `enabled`: `true`
- `runtimePath`: use explicit override if present; otherwise prefer repo-local runtime in this source checkout, else use the bundled package runtime
- `categoriesPath`: use explicit override if present, else runtime-local `categories.yaml`
- protected executions are serialized internally

### Config precedence

1. explicit runtime/package override passed from extension code (if ever needed for tests)
2. project config resolved from the effective repo/worktree root (or upward walk fallback when no repo root exists)
3. global config
4. package defaults

### Cache key for config reuse

If config caching is used for performance, the cache key must include the effective project boundary rather than only the raw cwd. For v1, the safest cache key is:

- resolved repo/worktree root when available
- otherwise the directory containing the discovered project-local config file
- otherwise a sentinel representing "global-only"

That ensures cached configuration does not leak across unrelated directories or repositories.

### Config edge cases

#### malformed config JSON
- log or surface a clear error to the user when possible
- fail closed for protected tools rather than silently disabling protection

#### nonexistent `runtimePath`
- treat as runtime setup failure
- block protected tools

#### nonexistent `categoriesPath`
- treat as runtime setup failure unless the runtime can safely fall back to bundled config
- do not silently point at an unrelated path

## Runtime Build / Bundle Strategy

For source-tree, local-path, and git installs, the pi package should execute the canonical runtime directly from `hooks/bash-approve/` inside the installed repository checkout.

### Recommendation

Keep `hooks/bash-approve/` as the only source of truth in the source tree. Do not commit a duplicated Go runtime under `packages/pi-bash-approve/`.

### Shim behavior

`hooks/bash-approve/run-pi-runtime.sh` should:
- rebuild the Go binary when sources/config change, mirroring the existing hook shim strategy
- invoke the compiled binary with `--pi`
- pass through `--config` when supplied by the TypeScript adapter

### Why this works for installed pi packages

Git and local-path package installs keep the full repository checkout inside pi's managed package location, so the extension can resolve `hooks/bash-approve/run-pi-runtime.sh` relative to its own installed path without relying on the original development checkout.

### Future npm packaging

If npm distribution is added later, any packaged runtime assets should be generated at packaging time rather than committed as a second authored runtime tree in the source repository.

## Packaging / Release Flow

### Recommended release flow

1. edit Go source of truth under `hooks/bash-approve/`
2. run Go tests for runtime behavior
3. run TypeScript/package tests for adapter behavior
4. publish/install the pi package from a full repo checkout for git/local installs
5. if npm support is added later, generate package-local runtime assets during packaging rather than maintaining committed duplicates

## Concrete Test Strategy

Testing should be split across Go and TypeScript responsibilities.

## Go-side tests

### 1. `--pi` contract tests

Add tests that verify:
- valid `bash` input returns the expected tagged output shape
- valid `read` / `grep` / `find` / `ls` inputs return the expected tagged output shape
- unsupported tools return structured `kind: "error"`
- malformed input returns structured `kind: "error"`
- unknown or missing required fields are rejected deterministically

### 2. Decision mapping tests for new pi-supported tools

Add focused tests for:
- `find` with in-repo relative path → `allow`
- `find` with out-of-repo absolute path → `noop`
- `find` with no path and cwd inside repo → `allow`
- `ls` with in-repo relative path → `allow`
- `ls` with out-of-repo path → `noop`
- `ls` with no path and cwd inside repo → `allow`

### 3. Existing parity regression tests

Add explicit regression coverage ensuring pi adapter behavior does not drift from existing repo-boundary behavior for:
- `read`
- `grep`
- linked worktrees
- symlinked paths
- non-repo cwd behavior

### 4. Config-path tests

If `--config` is added, Go-side coverage should stay limited to runtime-facing behavior:
- explicit `--config` path is honored by the runtime
- missing config path returns structured config error
- bundled/default config still works when no override is provided
- runtime consumes the selected config file consistently once a concrete path is provided

Project-config discovery, upward-walk resolution, and config-cache invalidation are adapter responsibilities and should be tested on the TypeScript side, not duplicated in Go contract tests.

## TypeScript-side tests

### 1. Runtime parsing tests

Test `runtime-parse.ts` for:
- all valid `decision` variants
- all valid `error` variants
- malformed JSON
- wrong `version`
- wrong `kind`
- unknown `decision`
- missing required fields

### 2. Decision-policy tests

Test normalized pi behavior for every combination of:
- decision (`allow` / `deny` / `ask` / `noop`)
- UI available / unavailable
- runtime failure / parse failure

### 3. Runtime-client tests

Use process-level fakes or shim fixtures to verify:
- stdin is written as JSON
- stdout is read fully
- stderr/non-zero exits are surfaced as failures
- direct spawn is used instead of shell interpolation
- config path arguments are passed correctly

### 4. Config tests

Verify:
- project config overrides global config
- defaults are applied when files are absent
- malformed config is handled conservatively
- repo/worktree root discovery drives project config lookup instead of raw cwd alone
- pi started from a repo subdirectory still resolves the repo-root project config
- switching between different repo/worktree roots invalidates cached config and loads the correct project config
- non-repo upward-walk fallback finds a project-local `.pi/bash-approve.json` below `$HOME`
- upward-walk fallback stops at the documented boundary and cannot create an undocumented pseudo-global config
- cache keys prevent config bleed across project-boundary changes

### 5. Override tests

At the package-helper level, test:
- protected tools delegate when decision is `allow`
- protected tools prompt on `ask`/`noop` in UI mode
- protected tools throw on user rejection
- protected tools throw on runtime failure
- queue serialization prevents overlapping adjudication

## Integration / smoke tests

Add at least a small number of end-to-end smoke tests around the staged runtime + adapter package boundary.

Recommended smoke coverage:
- repo-local development runtime invocation automatically prefers the source/runtime path when running from this repository checkout
- bundled-runtime fallback works from an installed/copied package location **outside this repository checkout**, or under an explicit test mode that disables repo-local preference while exercising staged package assets
- prompt-producing path behaves correctly in an environment with mocked UI confirmation

These tests do not need to exercise a real LLM. They only need to validate the protected-tool execution pipeline.

## Rollout / Install Story

The design should explicitly support two usage modes.

### A. Project-local development in this repo

Use local package loading while iterating on the extension/package.

Expected flow:
- develop Go runtime under `hooks/bash-approve/`
- sync staged runtime into `packages/pi-bash-approve/runtime/`
- load the package into pi via local path or temporary extension/package loading
- verify protected tool behavior against this checkout

### B. Reusable pi package install

Expected flow:
- install package through pi package mechanisms (`pi install ...` or local/git/npm source)
- package brings its own staged runtime bundle
- runtime shim compiles on first use if needed
- protected tools are available without needing a sibling repo checkout
- runtime resolution falls back to the bundled runtime only when no explicit override and no repo-local source checkout runtime is in effect

### Runtime dependency requirement

The package should clearly document that Go is required to build the bundled runtime unless a prebuilt strategy is later introduced.

If Go is absent:
- protected tools fail closed
- error message should make the missing prerequisite obvious

## Documentation Set To Create / Update

This design should eventually result in a small, explicit documentation set.

### 1. Root README updates

Update `README.md` to mention:
- pi support exists
- which tool classes are supported in pi v1
- where deeper pi docs live

### 2. Package README

Create:

```text
packages/pi-bash-approve/README.md
```

It should document:
- what the package does
- supported tools
- unsupported tools
- install modes
- config file locations and fields
- Go requirement
- failure/approval behavior by pi mode

### 3. Supported-tool reference doc

Create a stable reference doc at a root docs path, for example:

```text
docs/pi-supported-tool-calls.md
```

This doc should explicitly separate:
- Claude-parity support: `bash`, `read`, `grep`
- pi-specific additions: `find`, `ls`
- unsupported surfaces: `write`, `edit`, `user_bash`, custom tools
- decision semantics in pi
- `noop` behavior in a non-hook-chain host

This is the document future expansions should cite.

### 4. Adapter contract doc

Create an internal reference doc, for example:

```text
docs/pi-runtime-contract.md
```

It should describe:
- `--pi` stdin format
- `--pi` stdout format
- versioning rules
- exit-code expectations
- fail-closed parsing expectations

### 5. Developer sync/build docs

Document how to update the staged runtime bundle, likely in either:
- package README developer section, or
- a short root docs page / script usage note

## Files Expected To Be Added Or Updated In The Future

Likely implementation touchpoints once work is planned:

### Go runtime
- `hooks/bash-approve/main.go` — add `--pi` adapter mode and explicit contract handling
- `hooks/bash-approve/git.go` — extend repo/worktree-bounded logic to support `find` and `ls`
- `hooks/bash-approve/*_test.go` — pi contract tests, parity tests, and `find` / `ls` boundary tests
- runtime config parsing helpers if `--config` is introduced

### Pi package
- `packages/pi-bash-approve/package.json`
- `packages/pi-bash-approve/README.md`
- `packages/pi-bash-approve/extensions/index.ts`
- `packages/pi-bash-approve/src/runtime-contract.ts`
- `packages/pi-bash-approve/src/runtime-parse.ts`
- `packages/pi-bash-approve/src/runtime-client.ts`
- `packages/pi-bash-approve/src/decision-policy.ts`
- `packages/pi-bash-approve/src/protected-tool-queue.ts`
- `packages/pi-bash-approve/src/prompts.ts`
- `packages/pi-bash-approve/src/tool-inputs.ts`
- `packages/pi-bash-approve/src/overrides/bash.ts`
- `packages/pi-bash-approve/src/overrides/read.ts`
- `packages/pi-bash-approve/src/overrides/grep.ts`
- `packages/pi-bash-approve/src/overrides/find.ts`
- `packages/pi-bash-approve/src/overrides/ls.ts`
- `packages/pi-bash-approve/src/config.ts`
- `packages/pi-bash-approve/src/errors.ts`
- `packages/pi-bash-approve/runtime/*`
- `packages/pi-bash-approve/test/*`

### Repo docs / scripts
- `README.md` — pi integration section
- `docs/pi-supported-tool-calls.md`
- `docs/pi-runtime-contract.md`
- `scripts/sync-pi-runtime.sh`
- root package/workspace metadata if needed to include the new package in repo tooling

## Non-Goals For v1

- reimplementing policy logic in TypeScript
- protecting `user_bash`
- adding write/edit policy rules
- introducing persistent “allow always” approval memory
- introducing a broad generalized policy layer for arbitrary custom tools
- making sandboxing a default mandatory behavior
- collapsing Claude, OpenCode, and pi adapter contracts into one shared output format
- building a generalized policy system for all pi built-in tools before there is a demonstrated need
- inventing path-pattern semantics for `find` / `ls` beyond repo/worktree boundary checks

## Review-Driven Adjustments Incorporated

This draft was updated after automated review to tighten two previously underspecified areas:

1. **Config scoping:** project-local config is now defined relative to a stable repo/worktree root (with an upward-walk fallback when no repo root exists), and config reuse must be invalidated when the effective project boundary changes.
2. **Development runtime resolution:** local development in this source checkout must prefer the repo-local runtime over staged package assets so validation does not silently target stale synced runtime code.

## Design Status Summary

At this point the architecture, scope boundaries, supported-tool semantics, package structure, runtime contract, packaging story, and testing/documentation obligations are defined well enough to move into implementation planning once the written design is reviewed and approved.
