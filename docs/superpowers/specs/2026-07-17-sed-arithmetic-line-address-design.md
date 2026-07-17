# Sed Arithmetic Line-Address Design

## Problem

Read-only `sed` commands may calculate a range from a line-number search, for
example:

```bash
sed -n "$(($(grep -n 'func x' file.go | cut -d: -f1)+55)),+60p" file.go
```

The hook currently asks for confirmation because the sed-program validator
rejects every command substitution nested inside an arithmetic expansion.
Telemetry recorded this exact false positive twice on 2026-07-17. The same
line-number pipeline is already accepted when used directly or assigned to a
tracked sed-address variable.

## Decision

Allow a command substitution nested in a read-only sed arithmetic expression
only when it satisfies the existing line-number-pipeline provenance rules.
Those rules establish that the pipeline extracts the numeric line field from
a supported line-number search such as `grep -n`, `rg -n`, or `git grep -n`.

Do not treat ordinary auto-approval of the nested commands as sufficient. A
read-only command can still emit arbitrary text, and arbitrary text in a sed
program is broader than a numeric address. Process substitutions remain
rejected.

## Implementation

Reuse the parser in `sed_address.go` by exposing its line-number-pipeline check
for an existing command-substitution AST node. The word-based assignment check
will delegate to the same helper, avoiding a second interpretation of pipeline
shape and field provenance.

The sed arithmetic walker will accept arithmetic expressions containing no
substitutions as before. When it encounters a command substitution, it will
require the shared line-number-pipeline check to pass. The existing recursive
substitution evaluator remains responsible for propagating `deny`, `ask`, and
no-opinion decisions from commands inside the substitution.

## Safety Boundaries

- Trusted numeric line-number pipelines inside arithmetic addresses are
  auto-approved.
- Pipelines extracting the wrong colon-delimited field continue to ask.
- Other allowed but non-numeric commands inside arithmetic continue to ask.
- Process substitutions inside arithmetic continue to ask.
- Destructive nested commands continue to deny through recursive decision
  propagation.

## Testing

Add one focused regression test using the telemetry command shape and assert an
allow decision. Add focused negative cases for a wrong extracted field and a
non-line-number command. Retain the existing destructive nested-command test to
prove that its deny decision still propagates. Run the sed-focused tests, then
the complete Go test suite and linter.
