# Scoped chmod approval design

## Problem

Telemetry decision `84057` shows a routine cleanup chain falling through to a
permission prompt even though its `ls`, recursive `rm`, and final `test`
segments are all approved. The remaining segment is `chmod -R u+w` on an
explicit directory beneath the macOS runtime `TMPDIR`. `chmod` is currently
unrecognized for every target, so chain pessimism turns the whole command into
no-opinion.

## Decision

Add a dedicated `chmod` decision resolver. It may approve a command only when:

- the command has a literal conventional mode operand;
- every target is statically resolved;
- every target is a strict descendant of the active repository/worktree, a
  fixed temporary root, or the absolute resolved process `TMPDIR`; and
- every option is a recognized no-argument `chmod` option.

The rule's baseline remains no-opinion. A repository/worktree root or temporary
root itself, unknown or argument-taking options such as `--reference`,
recursive symlink-traversal flags, globs, unproven expansions, symlink escapes,
and mixed safe/unsafe target lists all remain no-opinion. The change does not
approve `chmod` elsewhere and does not change `trap` policy.

The resolver will reuse the recursive-rm path boundary rather than introduce a
second path-prefix implementation. The shared helpers will receive neutral
names where needed; recursive-rm behavior remains unchanged.

## Alternatives considered

1. Add a parsed resolver using the existing resolved path boundary. This is the
   selected approach because it covers the observed command without trusting
   textual prefixes or arbitrary filesystem targets.
2. Match `/tmp`, `/var/folders`, or repository-looking strings in the command
   text. This cannot safely handle symlinks, normalization, mixed targets, or
   the actual process `TMPDIR` boundary.
3. Approve all `chmod`. This would allow permission changes to durable user and
   system paths and is broader than the accepted temp/worktree scope.

## Testing and verification

Use evaluator-level tests for a literal runtime-`TMPDIR` target, an in-repo
target, and the exact telemetry cleanup chain. Add rejection coverage for the
allowed roots themselves, mixed targets, a symlink escape, a glob, a dynamic
target, and `--reference`. Observe the focused allow test fail before adding
production code, then run the full shuffled Go suite, lint, and isolated binary
replays before and after installation.
