# pi-bash-approve

Reusable pi package for integrating the Go `bash-approve` runtime with pi-coding-agent.

## Current support

Protected tool classes:
- `bash`
- `read`
- `grep`
- `find`
- `ls`

Not yet protected:
- `write`
- `edit`
- `user_bash`
- custom extension tools

See `../../docs/pi-supported-tool-calls.md` for the support matrix and `../../docs/pi-runtime-contract.md` for the runtime wire contract.

## Config

Global config:
- `~/.pi/agent/bash-approve.json`

Project-local config:
- `<repo-or-worktree-root>/.pi/bash-approve.json`

Current fields:

```json
{
  "enabled": true,
  "runtimePath": "/optional/runtime/path",
  "categoriesPath": "/optional/categories/path"
}
```

Protected executions are serialized internally by default; that is not currently configurable.

## Runtime

For source-tree, local-path, and git installs, the extension resolves the runtime from the same installed repository checkout under `hooks/bash-approve/run-pi-runtime.sh`.

That keeps `hooks/bash-approve/` as the single source of truth instead of maintaining a duplicated Go runtime tree under the package itself.

Go is currently required to build the runtime on first use.
