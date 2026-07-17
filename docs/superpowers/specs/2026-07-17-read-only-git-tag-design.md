# Read-only `git tag` approval design

## Problem

The hook currently assigns `ask` to every `git tag` invocation. This correctly
protects tag creation and mutation, but it also prompts for read-only queries
such as bare `git tag`, `git tag -l`, `git tag --sort=-creatordate`, and
`git tag --contains <commit>`. Telemetry confirms that these query forms cause
repeated benign prompts.

## Design

Add a more-specific rule before the existing catch-all `git tag` rule. The new
rule will classify bare tag listing and explicitly read-only query modes as
`git read op`, allowing them under the existing read-operation policy. The
catch-all `git tag` rule will remain unchanged and continue returning `ask` for
tag creation, deletion, signing, verification, and force-update forms.

The read-only whitelist will cover:

- bare `git tag`, including use with Git's `-C <path>` prefix;
- explicit list mode (`-l` and `--list`);
- filtering modes such as `--contains`, `--no-contains`, `--points-at`,
  `--merged`, and `--no-merged`;
- list presentation options used without a mutating mode, including `--sort`.

Unknown or ambiguous forms will fall through to the existing `ask` rule. This
keeps the safety boundary conservative.

## Alternatives considered

1. Allow only bare `git tag`. This fixes the newest prompt but leaves the other
   telemetry-confirmed query false positives.
2. Whitelist read-only query forms. This fixes the observed class of false
   prompts while preserving the existing mutation gate. This is the selected
   approach.
3. Introduce a complete semantic parser for every `git tag` option. This could
   classify more combinations, but its complexity is not justified by the
   observed commands.

## Testing

Add focused behavior tests that first demonstrate the regression:

- read-only list/query forms resolve to `allow` with reason `git read op`;
- representative mutations such as tag creation, deletion, and force update
  continue to resolve to `ask` with reason `git tag`;
- a read-only tag query inside a command chain does not make the chain ask.

Run the focused Go test during the red/green cycle, then run `go test ./...` and
`golangci-lint run ./...` from `hooks/bash-approve`.
