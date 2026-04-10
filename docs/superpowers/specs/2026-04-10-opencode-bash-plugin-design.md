# OpenCode Bash Plugin Design

## Goal

Make the OpenCode bash approval plugin correct for the maintained `anomalyco/opencode` runtime by intercepting bash permissions through the runtime paths that actually execute today, while removing dependence on hook surfaces that are declared but not wired.

## Runtime Contract

Investigation of the maintained OpenCode codebase established the current contract this repo must target.

- Local `.ts` plugins are auto-loaded from `.opencode/plugins/` and `~/.config/opencode/plugins/`.
- `tool.execute.before` and `tool.execute.after` are real runtime hook entrypoints.
- `permission.asked` and `permission.replied` are real bus events.
- `permission.ask` exists in the plugin type surface but there is no maintained-runtime call site invoking `Plugin.trigger("permission.ask", ...)`.
- Bash permissions still require `.permission.bash["*"] = "ask"` so OpenCode enters the permission flow and emits `permission.asked`.

This plugin should therefore treat `tool.execute.before` plus `event(permission.asked)` as the authoritative interception path.

## Correct Behavior

### Bash command capture

- On `tool.execute.before`, when `input.tool === "bash"`, capture the exact command, cwd, and call identifiers.
- Evaluate the command once at this point through the Go hook runtime.
- Cache the resulting decision under a stable key that can be matched against the later permission request.
- Ignore non-bash tools entirely.

### Permission interception

- On `event(permission.asked)`, only handle requests where `event.properties.permission === "bash"`.
- Match the permission request to cached bash metadata from the same session.
- If the cached decision is `allow`, reply `once` via the OpenCode permission API.
- If the cached decision is `deny`, reply `reject` via the OpenCode permission API.
- If the cached decision is `ask`, do nothing and let OpenCode present its native prompt.
- If no matching cached decision exists, do nothing rather than guessing.

### Lifecycle cleanup

- On `tool.execute.after`, clear any cached bash state for that call.
- Expire stale cached entries defensively so abandoned calls do not accumulate forever.
- Treat reply races as benign when OpenCode or another responder has already handled the request.

## Failure Model

- Hook execution failures must degrade to `ask`, never silently `allow`.
- The plugin must not auto-reply to any non-bash permission request.
- Missing hook execution should be observable in the tester and reported as runtime wiring failure, not as plugin success.
- A duplicate or already-satisfied permission reply error should be classified as a race, not as a plugin failure.

## Tester Design

The current tests created false confidence because they modeled a runtime where `permission.ask` mattered. The tester must instead model the maintained runtime contract.

- Track whether `tool.execute.before` fired.
- Track whether a bash `permission.asked` event occurred.
- Track whether the tester itself replied, versus the plugin replying first.
- Distinguish these outcomes in classification:
  - plugin not loaded
  - plugin loaded but permission flow missing
  - plugin deferred to native ask
  - plugin intercepted with allow
  - plugin intercepted with reject
  - runtime or command failure

The tester should only count bash permission events. Other permission traffic must be ignored.

## Installer and Config

- Replace JSON detection in `install-opencode.sh` with `jq`; do not inspect JSON using grep.
- Treat `.permission.bash["*"] == "ask"` as required configuration, not as evidence that the plugin is bypassed.
- Fix the global installer build step with `go build -buildvcs=false` so `--global` and `--both` work in copied runtime directories.

## Test Strategy

- Unit-test the plugin's event-driven permission logic around allow, deny, ask, and non-bash ignore paths.
- Unit-test tester classification against maintained-runtime scenarios rather than synthetic hook assumptions.
- Add at least one integration-style harness test proving that a bash permission request can be intercepted through `tool.execute.before` plus `permission.asked`.
- Verify installer config detection with representative `opencode.json` files using `jq` semantics.

## Non-Goals

- Do not preserve compatibility with the archived `opencode-ai/opencode` codebase.
- Do not rely on `permission.ask` as a required control path until the maintained runtime actually wires it.
- Do not broaden this into unrelated OpenCode plugin framework changes.
