---
name: bash-approve-telemetry
description: Use when analyzing bash-approve hook decisions, finding commands that needed user approval, identifying candidates for auto-approval, or querying the telemetry database
---

# Bash-Approve Telemetry

Query the bash-approve hook's decision log to find approval candidates and analyze command patterns.

**If arguments are provided** (e.g. `/bash-approve-telemetry kubectl`), filter all queries to commands matching that argument using `WHERE command LIKE '%<arg>%'`. Focus analysis on that command family rather than showing all decisions.

## Database Location

Telemetry is stored in the user state directory:

- If `XDG_STATE_HOME` is set to an absolute path, use `$XDG_STATE_HOME/claude-bash-approve/telemetry.db`
- Otherwise, use `~/.local/state/claude-bash-approve/telemetry.db`

There is also legacy compatibility for the common old path `~/.claude/hooks/bash-approve/telemetry.db`: the hook attempts a one-time migration from that location. If migration succeeds but legacy cleanup fails, the new state-directory path is still authoritative.

## Schema

```sql
CREATE TABLE decisions (
    id       INTEGER PRIMARY KEY,
    ts       TEXT DEFAULT (datetime('now')),  -- UTC
    payload  TEXT,    -- full JSON input from Claude Code hook
    command  TEXT,    -- the bash command string
    decision TEXT,    -- allow | deny | ask | no-opinion
    reason   TEXT     -- human-readable label(s), pipe-separated
);
```

### Decision Values

| Decision | Meaning | Hook output |
|----------|---------|-------------|
| `allow` | Auto-approved by a matching rule | Emits allow decision |
| `deny` | Explicitly blocked with reason shown to Claude (e.g. `rm -r`, `git stash`, `go mod vendor`) | Emits deny decision |
| `ask` | Recognized command, user is prompted to confirm — **terminal**, no further hooks run (e.g. `git tag`) | Emits ask decision |
| `no-opinion` | No matching pattern, or matched but no decision — exits silently so the next hook in the chain can handle it (e.g. `git push`, `gh pr create`) | No output, exit 0 |

Priority when merging chain/multiline decisions: **deny > ask > no-opinion > allow**.

## Quick Reference Queries

Before running queries, resolve the telemetry database path like this:

```bash
state_home="${XDG_STATE_HOME}"
if [ -z "$state_home" ] || [ "${state_home#/}" = "$state_home" ]; then
  state_home="$HOME/.local/state"
fi
db_path="$state_home/claude-bash-approve/telemetry.db"
```

Run queries with `sqlite3 "$db_path" '...SQL...'`.

### Summary of recent decisions (past 7 days)

```sql
SELECT decision, count(*) FROM decisions
WHERE ts >= datetime('now', '-7 days')
GROUP BY decision ORDER BY count(*) DESC;
```

### Commands that triggered "ask" grouped by reason

```sql
SELECT reason, count(*) as cnt FROM decisions
WHERE ts >= datetime('now', '-7 days') AND decision = 'ask'
GROUP BY reason ORDER BY cnt DESC;
```

### Commands with "no-opinion" — candidates for new rules

```sql
SELECT command, count(*) as cnt FROM decisions
WHERE ts >= datetime('now', '-7 days') AND decision = 'no-opinion'
GROUP BY command ORDER BY cnt DESC LIMIT 30;
```

### Distinct unrecognized commands (no-opinion) with examples

```sql
SELECT command, reason, ts FROM decisions
WHERE ts >= datetime('now', '-7 days') AND decision = 'no-opinion'
ORDER BY ts DESC LIMIT 50;
```

### Most frequent commands overall

```sql
SELECT command, decision, count(*) as cnt FROM decisions
WHERE ts >= datetime('now', '-7 days')
GROUP BY command, decision ORDER BY cnt DESC LIMIT 30;
```

### Time range check

```sql
SELECT min(ts), max(ts), count(*) FROM decisions;
```

## Finding Auto-Approval Candidates

The best candidates for new auto-approve rules are commands that:
1. Show `no-opinion` (hook has no matching pattern at all)
2. Appear frequently (high count = high annoyance)
3. Are clearly safe (read-only, local, idempotent)

**Workflow:**
1. Run the "no-opinion grouped by command" query above
2. Identify repetitive safe commands (e.g., a CLI tool used often)
3. Add a new pattern in the bash-approve source repo's `rules.go` under `allCommandPatterns`
4. Or, if an existing pattern is in `disabled` in `categories.yaml`, move it to `enabled`

**Source code** is at `~/code/agent-skills/hooks/bash-approve/` — edits go here, not in `~/.claude/hooks/bash-approve/` (that's the deployed hook bundle).

**Four decision types for new patterns:**
- `allow` (default) — auto-approve silently
- `WithDecision("deny")` + `WithDenyReason("...")` — block the command with a reason shown to Claude (e.g. `go mod vendor`, `rm -r`, `git stash`)
- `WithDecision("ask")` — prompt user to confirm, terminal (e.g. `git tag`)
- `WithDecision("")` — no opinion, pass to next hook in chain (e.g. `git push`, `gh pr create`)

Priority in chains: **deny > ask > no-opinion > allow**. If any segment is deny, the whole chain is deny.

For commands that show `no-opinion` — these either have no matching pattern (candidates for new rules) or are intentionally deferred to the next hook. Check if a reason is logged to distinguish.

For commands that show `ask` — these are **terminal gates**. The user will always be prompted. Do not confuse with `no-opinion` which passes to the next hook.

## Configuration File

`~/.claude/hooks/bash-approve/categories.yaml` controls which rule categories are active:

```yaml
enabled:
  - all           # enable everything not explicitly disabled
disabled:
  - git push      # keep requiring confirmation
```

Fine-grained category names match the first tag in each pattern's `tags()` call in `rules.go`.

## Common Mistakes

- **Timestamps are UTC** — adjust when comparing to local time.
- **`no-opinion` vs `ask`** — `no-opinion` means either no rule matched or a rule matched with `WithDecision("")` (deferred to next hook). `ask` means a rule matched with `WithDecision("ask")` and the user will always be prompted (terminal). New rules fix unmatched no-opinion; `ask` is intentional.
- **Database path** — only use `$XDG_STATE_HOME/claude-bash-approve/telemetry.db` when `XDG_STATE_HOME` is an absolute path. Otherwise use `~/.local/state/claude-bash-approve/telemetry.db`.
- **Legacy files** — `~/.claude/hooks/bash-approve/telemetry.db` is a compatibility/migration source, not the authoritative location after migration. Even if legacy cleanup fails, query the state-directory database.
