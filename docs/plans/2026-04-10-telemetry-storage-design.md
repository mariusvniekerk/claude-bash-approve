# Telemetry Storage Migration Design

**Date:** 2026-04-10  
**Status:** Proposed

## Goal

Move the hook telemetry database from the legacy executable-adjacent location to a user-scoped XDG-style state directory, while preserving existing telemetry through a one-time compatibility migration.

## Current State

`hooks/bash-approve/telemetry.go` currently resolves the database path by calling `os.Executable()` and storing `telemetry.db` next to the compiled `approve-bash` binary. In the common install layout, that means telemetry is written under `~/.claude/hooks/bash-approve/telemetry.db`.

This works operationally, but it mixes mutable runtime data with installed executable assets and does not follow the expected XDG-style placement for stateful telemetry.

## Requirements

1. Store telemetry in a semantically appropriate user state location rather than next to the binary.
2. Respect XDG configuration when present.
3. Fall back to a sensible per-user default when explicit XDG state configuration is absent.
4. Preserve existing telemetry with a one-time migration from the legacy executable-adjacent database.
5. Keep telemetry best-effort: failures must never block or break hook execution.
6. Update user-facing documentation and the telemetry analysis skill to describe the new location and migration behavior.

## Recommended Approach

Use a dedicated helper in `hooks/bash-approve/telemetry.go` to resolve a user-scoped telemetry directory and database path.

Resolution order:

1. If `XDG_STATE_HOME` is set to a non-empty absolute path, use `$XDG_STATE_HOME/claude-bash-approve/telemetry.db`.
2. Otherwise, use `~/.local/state/claude-bash-approve/telemetry.db`.
3. If the home directory cannot be resolved, return `nil` and keep telemetry disabled for that invocation.

If `XDG_STATE_HOME` is unset, empty, or non-absolute, ignore it and use the fallback path rather than writing to a relative location.

This change is intentionally framed around XDG-style behavior for the Unix-like environments where this hook is normally installed and used. Even though the original request mentioned config, telemetry is operational state, so the canonical documented destination is the XDG state area rather than configuration.

## Migration Strategy

On startup, before opening the database:

1. Resolve the new telemetry path.
2. If the new database path exists and is usable, use it.
3. Resolve the legacy database path next to the executable.
4. If the new database path exists but is not usable:
   - if the legacy path is resolvable and the legacy database still exists, remove the unusable destination artifacts and retry migration from legacy
   - otherwise, leave the unusable destination untouched and disable telemetry for that run
5. If the new database path does not exist:
   - if the legacy path is not resolvable, skip migration and continue with a fresh database at the new location
   - if the legacy path is resolvable but no legacy database exists, continue with a fresh database at the new location
   - if the legacy database exists, ensure the destination parent directory exists and perform one-time migration into the new location
6. If migration succeeds, continue using the new path.
7. If migration fails, clean up any partial destination artifacts created by that migration attempt, then disable telemetry for that run without affecting hook behavior.

Decision table:

| Destination state | Legacy path / file state | Behavior |
|---|---|---|
| usable destination exists | anything | use destination |
| unusable destination exists | legacy path resolvable and legacy DB exists | remove unusable destination artifacts, retry migration |
| unusable destination exists | legacy path unresolvable or legacy DB missing | disable telemetry for this run |
| destination missing | legacy path unresolvable | create fresh DB at new location |
| destination missing | legacy path resolvable but legacy DB missing | create fresh DB at new location |
| destination missing | legacy path resolvable and legacy DB exists | migrate legacy DB |

The migration should use a copy-then-validate-then-delete-source flow rather than a rename-first flow. That avoids losing the only durable copy if validation fails after the destination file appears. Migration scope includes the main database file and common SQLite sidecar files if they exist (`telemetry.db-wal`, `telemetry.db-shm`, and `telemetry.db-journal`) so preserved telemetry is not lost during an active/unclean state. Migration succeeds only if the main database file and every sidecar file that existed at the source were copied successfully.

Because telemetry is explicitly best-effort and hook invocations are short-lived, migration may use normal filesystem copy operations rather than a SQLite-specific backup/checkpoint flow. Preserving the durable on-disk database state is required; preserving in-flight writes from a concurrently active legacy database is not a requirement.

A migration is considered successful only when the destination main database file exists, all required copied sidecars are present, and the hook can open the destination and run schema initialization successfully. After a successful validated copy, the implementation should attempt to remove the migrated legacy files. If that cleanup fails, the hook should still continue using the new database path and leave the legacy files in place. Once the destination database is successfully materialized and usable, migration should not be retried on subsequent runs.

If a migration attempt creates partial destination artifacts but does not reach that success condition, those artifacts should be removed as part of failure cleanup so a later run can retry migration cleanly. The same cleanup-and-retry rule applies when a later run finds an unusable destination while the legacy database still exists.

The migration logic should be idempotent under concurrent hook invocations. If two processes race and one creates or migrates the destination first, the other process should treat an already-present usable destination as success and continue with the new path rather than failing or attempting to overwrite it.

## Error Handling

Telemetry must remain non-fatal:

- directory creation failures disable telemetry for the current run
- destination-exists-but-unusable cases disable telemetry for the current run unless a legacy DB still exists and the retry-migration path succeeds
- migration failures disable telemetry for the current run and do not affect command approval behavior
- database open or schema creation failures continue to return `nil`
- write failures in `logDecision` remain ignored

## Testing Strategy

Add focused tests around path resolution and migration behavior rather than relying only on integration through SQLite open calls.

Expected test coverage:

1. resolves the new telemetry path from valid XDG state configuration
2. falls back to the default user-scoped directory when XDG state is unset, empty, or non-absolute
3. disables telemetry when the home directory cannot be resolved
4. uses the new database directly when it already exists and is usable
5. cleans up and retries migration when the destination exists but is unusable and the legacy DB still exists
6. disables telemetry when the destination exists but is unusable and no legacy DB remains
7. skips migration cleanly when the legacy executable path cannot be resolved
8. migrates the legacy executable-adjacent database when the new database does not exist yet
9. migrates SQLite sidecar files when present
10. removes partial destination artifacts after migration failure so a later run can retry
11. does not re-migrate when the new database already exists and is usable
12. treats destination-created-during-migration as success
13. preserves best-effort behavior on migration/setup failure

These branches should be made testable by factoring filesystem and environment lookups behind small helpers or package-level indirections that tests can override deterministically.

To make this testable, the telemetry code should be split into small helpers for:

- resolving the target telemetry directory/path
- resolving the legacy path
- ensuring the destination directory exists
- performing one-time migration
- validating whether migration completed successfully enough to open the destination DB
- opening the database and initializing schema only after path/migration decisions are complete

## Files Expected to Change

- `hooks/bash-approve/telemetry.go` — path resolution, migration, and helper extraction
- `hooks/bash-approve/*_test.go` or `main_test.go` — path and migration tests
- `README.md` — telemetry location, examples, migration note
- `.claude/skills/bash-approve-telemetry/SKILL.md` — database location and query examples

## Non-Goals

- changing telemetry schema
- adding telemetry retention or pruning
- merging multiple historical databases
- introducing configurable telemetry opt-in/out behavior
- maintaining permanent dual-read support for old and new paths

## Open Decision Resolved

The user approved the semantically appropriate XDG-style state/data location instead of config and requested a one-time compatibility migration of the existing database.
