# Runtime TMPDIR recursive-rm approval design

## Problem

The recursive-rm resolver permits explicit descendants of `/tmp`, `/var/tmp`,
and their `/private` aliases. On macOS, however, `TMPDIR` commonly resolves to
`/var/folders/<...>/T`. Telemetry and replay against the installed hook show
that cleanup beneath this runtime directory still receives the recursive-rm
deny decision.

## Decision

Treat an absolute, symlink-resolved `TMPDIR` from the hook process environment
as one additional temporary root. Apply the same strict-descendant predicate
used for fixed temporary roots. The root itself remains denied, and an unset,
empty, relative, or filesystem-root `TMPDIR` adds no trusted scope.

The command text cannot widen the policy with an inline `TMPDIR=...`
assignment: the resolver reads the hook process environment, not evaluated
shell assignments. Existing target literal, glob, expansion, mixed-target,
repository-root, and symlink-escape protections remain unchanged.

## Alternatives considered

1. Trust the resolved process `TMPDIR`. This matches the operating system's
   runtime temporary-directory selection without broadly trusting
   `/var/folders`; this is the selected approach.
2. Add `/var/folders` as a fixed root. This is too broad because it includes
   per-user caches and other non-temporary data outside the active `T` root.
3. Add a configurable recursive-delete root. This adds configuration and
   misconfiguration risk when the process environment already identifies the
   intended runtime root.

## Testing and verification

Add evaluator-level regression coverage showing that a strict descendant of an
absolute `TMPDIR` is allowed while the root itself, a relative `TMPDIR`, and a
repository root designated as `TMPDIR` remain denied. The allow case must fail
before production code changes. Then run the full shuffled Go suite, lint, an
isolated binary replay, CI, and a post-install replay against the installed
binary.
