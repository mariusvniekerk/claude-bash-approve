# OpenCode Tester

Small harness for exercising one real OpenCode bash tool call with the local `bash-approve` plugin enabled.

It uses `@opencode-ai/sdk` to:

- start a local headless `opencode serve`
- subscribe to typed OpenCode events
- create a session and send a prompt
- reply to leaked permission requests so the run completes

## Requirements

- Bun
- `opencode` installed and available on `PATH`

## Usage

```bash
cd opencode-tester
bun run src/index.ts --dir ..
```

Optional flags:

- `--command "git status --short --branch"`
- `--reply once|always|reject`
- `--timeout-ms 120000`

The script prints a JSON summary including:

- any typed `permission.asked` and `permission.replied` events
- final `bash` tool part state
- any session errors
- a final classification such as `plugin-intercepted`, `native-ask`, or `plugin-not-loaded`
