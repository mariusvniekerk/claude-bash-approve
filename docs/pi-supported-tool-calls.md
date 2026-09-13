# Pi Supported Tool Calls

Current pi package support is intentionally narrower than a generalized policy layer.

## Claude-parity support

These tool classes reuse the same underlying policy intent already present in the Go runtime:

- `bash` — full command approval engine (`allow` / `deny` / `ask` / `noop`)
- `read` — repo/worktree-bounded allow, otherwise `noop`
- `grep` — repo/worktree-bounded allow, otherwise `noop`

## Pi-specific additions

These are pi-native read-style boundary checks built on the same repo/worktree model:

- `find` — effective search root must stay inside the current repo/worktree boundary
- `ls` — effective target path must stay inside the current repo/worktree boundary

Before each agent run, the extension also reads Pi's structured skill metadata. It permits `read`
for the exact visible skill entry files in that metadata. The exception does not cover other files
in a skill directory or any other out-of-repo tool call.

## Not supported yet

The pi package does **not** currently protect:

- `write`
- `edit`
- `user_bash` (`!` / `!!`)
- custom extension tools
- session lifecycle actions

## Decision semantics in pi

The Go runtime emits one of four decisions:

- `allow`
- `deny`
- `ask`
- `noop`

The pi package treats `noop` as approval to execute without interrupting the user. It maps decisions like this:

### Interactive / RPC modes

- `allow` → execute
- `deny` → block
- `ask` → prompt
- `noop` → execute without a prompt

### Non-UI modes

- `allow` → execute
- `deny` → block
- `ask` → block
- `noop` → execute

This means an out-of-repository `read`, `grep`, `find`, or `ls` call that the runtime classifies as `noop` executes without confirmation. Explicit `deny` decisions still block. Runtime failures and contract parsing failures also fail closed and block execution.
