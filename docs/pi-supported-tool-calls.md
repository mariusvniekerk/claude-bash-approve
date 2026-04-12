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

Pi is not a hook chain, so `noop` does **not** fall through to a later permission hook. The pi package maps decisions like this:

### Interactive / RPC modes
- `allow` → execute
- `deny` → block
- `ask` → prompt
- `noop` → prompt

### Non-UI modes
- `allow` → execute
- `deny` → block
- `ask` → block
- `noop` → block

Runtime failures and contract parsing failures also fail closed and block execution.
