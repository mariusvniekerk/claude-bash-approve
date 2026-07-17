# Safe environment prefixes, shell delegation, and svelte-mcp approval

## Scope

Reduce unnecessary approval prompts for known-safe local workflows without
blanket-approving arbitrary environment variables, login-shell payloads, or
`vp exec` commands.

## Design

- Add `safe_env_prefixes` to `categories.yaml`. Configured prefixes extend the
  built-in environment-variable allowlist. Existing hard-deny and ask-only
  environment variable rules retain precedence.
- Configure the local installation with `KENN_` as a safe environment-variable
  prefix.
- Recognize trusted `bash` and `zsh` invocations using `-c` or `-lc` only when
  the command argument is a static string. Parse and evaluate that inner shell
  command with the existing evaluator. Dynamic payloads remain unrecognized.
- Recognize only `vp exec svelte-mcp get-documentation` and
  `vp exec svelte-mcp svelte-autofixer`. Other `vp exec` tools remain
  unrecognized unless separately covered.

## Safety and errors

- Configured safe prefixes never bypass hard-denied or ask-only variable names.
- Invalid or dynamic shell command strings fall through to the normal approval
  path.
- Inner shell commands preserve the evaluator's deny, ask, no-opinion, and allow
  decisions.
- Empty prefixes are ignored so they cannot match every environment variable.

## Verification

- Regression tests cover config parsing and prefix precedence.
- Regression tests cover safe and unsafe `bash`/`zsh -lc` payloads.
- Regression tests cover the two supported svelte-mcp operations and reject an
  unrelated `vp exec` operation.
- A root Vite+ configuration owns typechecking for both TypeScript workspaces
  through the tsgo-backed `vp check` path. Workspace-specific `tsc` pins and
  duplicate pre-commit typecheck hooks are removed.
- Run the complete Go test suite and linter.
