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

The package ships a staged runtime bundle under `runtime/`. During local development in this source checkout, runtime resolution prefers the live Go sources under `hooks/bash-approve/` over the staged package bundle.

Go is currently required to build the staged runtime on first use.
