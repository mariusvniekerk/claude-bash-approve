# Guarded isolated-test harness approval design

## Problem

Telemetry decision `84211` records the exact command in the reported Codex
prompt as `noop`. The command is a routine unit-test launch with a private
temporary HOME, but the evaluator cannot prove the compound workflow safe.
Five boundaries independently fall through: a `mktemp` command substitution,
a `case` guard, an EXIT cleanup `trap`, `env -u`, and Node launching the local
Vite+ entrypoint. A similar current Vite/Bun workflow appears in telemetry
decision `87832`, so changing one emitted command would not remove the recurring
prompt family.

## Decision

Recognize the complete guarded isolated-test harness as one fail-closed AST
construct before ordinary statement-by-statement evaluation. Approval requires
all of the following, in order:

1. One variable is assigned from literal `mktemp -d -t NAME.XXXXXX`, where the
   template is a filename rather than a path.
2. A `case` statement checks that same variable and accepts only `/tmp/*`,
   `/private/tmp/*`, or `/var/folders/*`; every other value exits nonzero.
3. `chmod 700` targets exactly that variable.
4. An EXIT trap recursively removes exactly that variable, optionally preserving
   and restoring the prior exit status. No other trap action is accepted.
5. `env` may unset only literal variable names, sets HOME to the temporary root,
   sets XDG directories to fixed descendants, optionally sets known cache
   directories to fixed descendants, and uses a PATH made entirely from trusted
   absolute toolchain/system directories.
6. The launched command resolves through existing approval policy. The special
   Node form is limited to a trusted absolute Node binary executing the local
   `node_modules/vite-plus/bin/vp` entrypoint; it is evaluated as the equivalent
   existing `vp` command. Arbitrary Node scripts remain unrecognized.

The recognizer returns an `isolated test harness` allow result only when the
entire script matches. On any structural or value mismatch it declines to
handle the command, and the existing evaluator produces its normal ask, deny,
or no-opinion result. Existing primitive policies for arbitrary `trap`, `case`,
`env -u`, `mktemp`, PATH changes, and Node programs do not broaden.

## Alternatives considered

1. Approve the guarded harness as a unit. This is selected because safety comes
   from relationships across statements: the same variable must connect
   creation, validation, permissions, cleanup, environment paths, and launch.
2. Add general approval rules for each missing primitive. This is more reusable
   but would approve arbitrary traps and Node scripts and would lose the
   cross-statement proof that makes the observed workflow safe.
3. Require agents to use a shorter direct Vite+ command. Direct Vite+ is already
   approved, but dropping isolated HOME/XDG state and guaranteed cleanup makes
   tests less hermetic and does not address the recurring Bun variant.

## Testing and verification

First add evaluator-level RED regressions for telemetry `84211` and the guarded
Bun variant represented by `87832`. Add fail-closed cases that alter one proof
boundary at a time: missing or widened case guard, mismatched chmod/cleanup
variable, extra trap action, outside HOME/XDG target, unsafe PATH component,
arbitrary Node script, and extra shell statements. After implementation, run
the focused tests, the shuffled Go suite, lint, and isolated binary replays.
Merge only after CI passes, install from the exact merged `main` commit, and
replay the reported command against the installed binary.
