# Pi Runtime Contract

The reusable pi package talks to the Go runtime through a dedicated `--pi` JSON adapter mode.

## Invocation

The pi runtime shim ultimately runs:

```bash
approve-bash --pi [--config /path/to/categories.yaml]
```

JSON is sent on stdin and a single JSON object is emitted on stdout.

## Input

```json
{"tool":"bash","command":"git status","cwd":"/repo"}
```

Supported tagged input variants:

- `{"tool":"bash","command":string,"cwd":string}`
- `{"tool":"read","path":string,"cwd":string}`
- `{"tool":"grep","pattern":string,"path"?:string,"paths"?:string[],"cwd":string}`
- `{"tool":"find","pattern":string,"path"?:string,"cwd":string}`
- `{"tool":"ls","path"?:string,"cwd":string}`

## Output

### Decision output

```json
{
  "version": 1,
  "kind": "decision",
  "tool": "bash",
  "decision": "allow",
  "reason": "git read op"
}
```

### Error output

```json
{
  "version": 1,
  "kind": "error",
  "error": {
    "code": "invalid-input",
    "message": "invalid pi input"
  }
}
```

## Rules

- `version` is currently `1`
- `kind` is `decision` or `error`
- supported decisions are `allow`, `deny`, `ask`, `noop`
- supported error codes are `invalid-input`, `unsupported-tool`, `config-error`, `internal-error`

## Failure behavior

The TypeScript adapter treats malformed JSON, unknown versions, unknown decisions, and runtime invocation failures as fatal and blocks the protected tool call.
