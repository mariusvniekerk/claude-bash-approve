# Scoped chmod Approval Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Auto-approve parsed `chmod` commands whose every target is a strict descendant of the active repository/worktree or a trusted temporary root.

**Architecture:** Add a `chmod` command pattern with a no-opinion baseline and a parsed decision resolver. Reuse the recursive-rm resolver's symlink-aware target boundary after giving the shared helpers neutral names; approve only when options, mode, and every target are statically proven.

**Tech Stack:** Go, `mvdan.cc/sh/v3/syntax`, Testify, evaluator-level regression tests.

## Global Constraints

- Unsafe or unproven `chmod` commands remain no-opinion, not deny or ask.
- Every target must be a strict descendant of the active repository/worktree, `/tmp`, `/var/tmp`, their `/private` aliases, or the absolute resolved process `TMPDIR`.
- Allowed roots themselves remain no-opinion.
- Every target must be static; unquoted globs, unproven expansions, symlink escapes, and mixed safe/unsafe lists remain no-opinion.
- Only recognized no-argument options are accepted; `--reference` and unknown options remain no-opinion.
- A literal octal or conventional symbolic mode is required.
- `trap` policy is outside this change.
- Follow TDD and observe telemetry command `84057` fail before production changes.
- Direct `go test` commands use `-shuffle=on`.
- Never bypass Git verification with `--no-verify`.

## File Structure

- Create `hooks/bash-approve/chmod.go`: parse `chmod` options, mode, and targets, then resolve the scoped decision.
- Create `hooks/bash-approve/chmod_test.go`: own focused allow and no-opinion behavior.
- Modify `hooks/bash-approve/rm.go`: give the shared static-expansion and target-scope helpers neutral names.
- Modify `hooks/bash-approve/rules.go`: register `chmod` with a no-opinion baseline and the resolver.

---

### Task 1: Approve chmod only for proven scoped targets

**Files:**
- Create: `hooks/bash-approve/chmod.go`
- Create: `hooks/bash-approve/chmod_test.go`
- Modify: `hooks/bash-approve/rm.go`
- Modify: `hooks/bash-approve/rules.go`

**Interfaces:**
- Consumes: `wordLiteral`, `wordLiteralPathWithContext`, `hasUnquotedGlob`, and the existing recursive-rm path resolution helpers.
- Produces: `resolveChmodDecision(args []*syntax.Word, ctx evalContext) *result`, `isChmodMode(string) bool`, `isChmodNoArgOption(string) bool`, `hasUnquotedPathExpansion(*syntax.Word) bool`, and `destructiveTargetInAllowedScope(cwd, target string) bool`.

- [ ] **Step 1: Write focused failing evaluator tests**

Create `hooks/bash-approve/chmod_test.go` with an allow test that initializes a fixture repository before changing `TMPDIR`, then exercises both accepted scopes and telemetry `84057`:

```go
func TestChmodScopedTargetsAllowed(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init").Run())
	require.NoError(t, os.Mkdir(filepath.Join(repo, "generated"), 0o755))
	root := "/var/folders/bash-approve-tests/T"
	t.Setenv("TMPDIR", root)
	tempTarget := filepath.Join(root, "forge-ci-e2e.123")

	for _, command := range []string{
		"chmod -R u+w " + tempTarget,
		"chmod 700 generated",
		"ls -ld " + tempTarget + "; chmod -R u+w " + tempTarget + "; rm -rf " + tempTarget + "; test ! -e " + tempTarget,
	} {
		r := evaluateAllInDir(command, repo)
		require.NotNil(t, r, command)
		assert.Equal(t, decisionAllow, r.decision, command)
	}
}
```

Add one focused boundary table. Each case must return a non-nil result with an empty decision because the new `chmod` pattern preserves no-opinion:

```go
func TestChmodUnprovenTargetsRemainNoOpinion(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init").Run())
	root := "/var/folders/bash-approve-tests/T"
	t.Setenv("TMPDIR", root)

	for _, command := range []string{
		"chmod 700 .",
		"chmod 700 " + root,
		"chmod 700 /etc/bash-approve-unsafe",
		"chmod 700 " + filepath.Join(root, "safe") + " /etc/bash-approve-unsafe",
		"chmod 700 " + filepath.Join(root, "*") ,
		"target=$(date); chmod 700 \"$target\"",
		"chmod --reference=/tmp/reference " + filepath.Join(root, "safe"),
	} {
		r := evaluateAllInDir(command, repo)
		require.NotNil(t, r, command)
		assert.Empty(t, r.decision, command)
	}
}
```

Add a separate symlink-escape subtest using a real symlink beneath `/tmp` that resolves to `/etc`; assert no-opinion. This catches removal of the existing symlink-aware target resolver.

- [ ] **Step 2: Verify RED**

Run:

```bash
cd hooks/bash-approve
go test ./... -run 'TestChmod' -shuffle=on
```

Expected: FAIL because allowed `chmod` commands are currently unrecognized. Boundary cases may be nil before the command pattern exists; the failure must include telemetry-shaped `chmod -R u+w` receiving no approval.

- [ ] **Step 3: Register the fail-closed chmod pattern**

In `hooks/bash-approve/rules.go`, add before the recursive-rm pattern:

```go
NewPattern(`^chmod\b`, tags("chmod", "shell destructive", "shell"), WithDecision(""), WithDecisionResolver(resolveChmodDecision)),
```

The empty baseline preserves current behavior whenever the resolver cannot prove safety.

- [ ] **Step 4: Generalize the existing shared helpers**

In `hooks/bash-approve/rm.go`, rename `hasUnquotedRmTargetExpansion` to `hasUnquotedPathExpansion` and `recursiveRmTargetInAllowedScope` to `destructiveTargetInAllowedScope`, updating the two recursive-rm call sites without changing their bodies.

- [ ] **Step 5: Implement the minimal parsed resolver**

Create `hooks/bash-approve/chmod.go`:

```go
package main

import (
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

var chmodModePattern = regexp.MustCompile(`^(?:[0-7]{3,4}|[ugoa]*[+=-][rwxXstugo]*(?:,[ugoa]*[+=-][rwxXstugo]*)*)$`)

func resolveChmodDecision(args []*syntax.Word, ctx evalContext) *result {
	if ctx.cwd == "" || len(args) < 3 {
		return nil
	}

	modeIndex := 1
	for modeIndex < len(args) {
		literal := wordLiteral(args[modeIndex])
		if literal == "" {
			return nil
		}
		if literal == "--" {
			modeIndex++
			break
		}
		if isChmodMode(literal) {
			break
		}
		if !isChmodNoArgOption(literal) {
			return nil
		}
		modeIndex++
	}
	if modeIndex >= len(args) || !isChmodMode(wordLiteral(args[modeIndex])) {
		return nil
	}

	targetIndex := modeIndex + 1
	if targetIndex < len(args) && wordLiteral(args[targetIndex]) == "--" {
		targetIndex++
	}
	if targetIndex >= len(args) {
		return nil
	}
	for _, target := range args[targetIndex:] {
		literal := wordLiteralPathWithContext(target, ctx)
		if literal == "" || hasUnquotedPathExpansion(target) || hasUnquotedGlob(target) ||
			!destructiveTargetInAllowedScope(ctx.cwd, literal) {
			return nil
		}
	}
	return &result{decision: decisionAllow}
}

func isChmodMode(value string) bool {
	return chmodModePattern.MatchString(value)
}

func isChmodNoArgOption(value string) bool {
	if strings.HasPrefix(value, "--") {
		switch value {
		case "--recursive", "--changes", "--quiet", "--silent", "--verbose", "--preserve-root", "--no-preserve-root":
			return true
		default:
			return false
		}
	}
	if len(value) < 2 || value[0] != '-' {
		return false
	}
	for _, option := range value[1:] {
		if !strings.ContainsRune("RfhvcHLP", option) {
			return false
		}
	}
	return true
}
```

- [ ] **Step 6: Verify GREEN and regression safety**

Run:

```bash
cd hooks/bash-approve
gofmt -w chmod.go chmod_test.go rm.go rules.go
go test ./... -run 'TestChmod|TestNoOpinionDecision/rm_-rf' -shuffle=on
go test ./... -shuffle=on
golangci-lint run ./...
```

Expected: all commands exit 0 with no failures or lint findings.

- [ ] **Step 7: Replay the exact telemetry command in isolated state**

Build the branch binary into a mode-0700 scratch directory and use a scratch `XDG_STATE_HOME`. Replay telemetry `84057` and each segment. Expected decisions: `ls=allow`, `chmod=allow`, `rm=allow`, `test=allow`, and exact chain `allow`. Also replay `chmod -R u+w /etc` and a mixed target list; both must remain `noop`.

- [ ] **Step 8: Commit, push, open a PR, merge on green CI, and install**

Stage only the design/plan, `chmod.go`, `chmod_test.go`, `rm.go`, and `rules.go`. Follow the mandatory commit workflow and use the `kenn-io/kit` SAFE Git runner for every Git command. Inspect the full branch history and every introduced blob, push, and open a rationale-first PR. If required CI passes, squash-merge it. Export the exact merged commit, retain a private backup of the installed runtime, install globally for Codex, and replay telemetry `84057` against the installed binary.
