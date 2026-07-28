# Standard temporary-directory recursive-rm approval design

## Problem

The hook permits recursive removal of an explicit directory strictly beneath
the active repository, but denies the same operation beneath the operating
system's standard temporary directories. Telemetry recorded five denied
explicit `/tmp/...` deletions in the last seven days, including generated test
and review scratch directories. These are routine cleanup operations rather
than attempts to remove durable project or user data.

## Decision

Extend the existing recursive-rm decision resolver with a second allowed
scope: strict descendants of the standard temporary-directory roots. The
supported roots are `/tmp`, `/var/tmp`, and their resolved macOS counterparts
`/private/tmp` and `/private/var/tmp`.

The existing active-repository allowance remains unchanged. A recursive-rm
command is allowed only when every target is accepted by one of these two
scopes.

## Path handling and safety boundaries

Reuse the resolver's current literal decoding, path normalization, and symlink
resolution. For each target:

- resolve existing path components before checking its scope;
- allow only a strict descendant of an approved root, never the root itself;
- reject unquoted globs and targets that cannot be resolved statically;
- reject a path beneath a temporary root when an existing symlink escapes that
  root; and
- reject the entire command when any target is outside the repository and
  standard temporary-directory scopes.

Commands targeting `/`, home directories, repository roots, arbitrary
external directories, or mixed safe and unsafe targets retain the existing
deny decision. The change does not add configuration or broaden any non-`rm`
command policy.

## Alternatives considered

1. Extend the recursive-rm resolver with standard temporary roots. This keeps
   destructive-operation policy localized and preserves the existing
   fail-closed parsing. This is the selected approach.
2. Reuse the general safe-write-prefix list. This would couple recursive
   deletion to redirection and download policy, so a future write-safe prefix
   could silently become delete-safe.
3. Add configurable recursive-delete prefixes. This adds configuration and
   misconfiguration risk without a current need for user-defined roots.

## Testing

Add focused behavior tests for recursive removal of explicit descendants of
each standard temporary root. Add focused rejection cases for a temporary root
itself, an unquoted glob, a symlink escape, and a command mixing a temporary
target with an unsafe target. These tests exercise the resolver's policy rather
than filesystem-library behavior.

Run the focused recursive-rm tests during the red/green cycle, followed by
`go test ./...` and `golangci-lint run ./...` from `hooks/bash-approve`.
