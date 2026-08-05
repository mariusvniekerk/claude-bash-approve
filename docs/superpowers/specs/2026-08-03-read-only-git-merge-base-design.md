# Read-only `git merge-base` design

## Problem

Telemetry decision `84177` prompted for a command that fetches and prints Git
revision metadata. The evaluator labels `git merge-base HEAD origin/main` as a
`git write op` because the write-operation regular expression accepts a word
boundary after `merge`; a hyphen creates that boundary. That false write label
also prevents the following `xargs git rev-parse --short` stage from receiving
read-only pipeline provenance, so the full command resolves to `ask`.

Repository-scope probes also still launch Git with `os/exec` and a local
environment filter. Use the maintained `go.kenn.io/kit/git/cmd` runner as
requested so those probes inherit its safe automation defaults instead of
duplicating part of that policy here.

## Decision

Add `merge-base` to the explicit Git read-operation set. Tighten the Git write
subcommand terminator from a generic word boundary to whitespace or end of
command, so hyphenated commands cannot be captured by a shorter write-command
prefix. Pattern order continues to classify genuine `git merge` as a write
operation and existing Git read/write category behavior remains unchanged.

Replace the repository probe's direct Git subprocess construction with
`gitcmd.New().Output`. Keep the existing `gitOutput` interface and trimmed
stdout behavior so callers do not change. This dependency change is limited to
the existing repository/worktree path checks; command evaluation still parses
the requested shell text and does not execute it.

## Alternatives considered

1. Add `merge-base` and tighten the write boundary. This is selected because it
   fixes both the observed classification and the underlying prefix bug with a
   small, explicit change.
2. Add only `merge-base` ahead of the write pattern. This fixes the default
   ordering but leaves `merge-base` misclassified if read categories are
   filtered while write categories remain enabled.
3. Replace every Git pattern with a parsed subcommand dispatcher. That would be
   broader than this regression requires and would increase review risk.

## Testing and verification

Add evaluator-level regression coverage for direct `git merge-base`, the exact
telemetry `84177` pipeline, and genuine `git merge`. First observe the new tests
fail for the current classification, then apply the minimal pattern change and
observe them pass. Existing repository-scope tests exercise the `gitOutput`
seam after the safe-runner migration. Finish with the full shuffled Go suite,
lint, an isolated build replay of the telemetry command, CI, merge, installation
from merged `main`, and a final installed-binary replay.
