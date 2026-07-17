# Read-only `git tag` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Auto-approve provably read-only `git tag` listing and query forms while preserving the terminal prompt for tag creation and mutation.

**Architecture:** Add a narrow read-only `git tag` pattern before the existing catch-all ask pattern. A Git-specific argument validator will accept only literal, whitelisted list/query forms; ambiguous or mutating combinations will fall back to `ask`, and ordinary creation/deletion commands will continue matching the existing `git tag` gate.

**Tech Stack:** Go 1.25, `mvdan.cc/sh/v3/syntax`, `testify`, regular-expression command patterns.

## Global Constraints

- Bare and explicitly read-only tag queries must be labeled `git read op` and return `allow`.
- Tag creation, deletion, signing, verification, and force-update forms must remain labeled `git tag` and return `ask`.
- Unknown, dynamic, or mixed query/mutation forms must fail closed to `ask`.
- The telemetry command `git tag | tail -5; echo ---; gh release list --repo kenn-io/agentsview --limit 3 2>&1` must return `allow`.
- The unrelated dynamic sed and awk command-execution prompts remain out of scope.

---

### Task 1: Specify `git tag` decision behavior

**Files:**
- Create: `hooks/bash-approve/git_tag_test.go`

**Interfaces:**
- Consumes: `evaluateAll(cmd string) *result`, `decisionAllow`, and `decisionAsk` from the existing evaluator tests.
- Produces: Regression coverage for the public command-decision behavior.

- [ ] **Step 1: Write the failing read-only and mutation tests**

```go
func TestGitTagDecision(t *testing.T) {
	readOnly := []string{
		"git tag",
		"git -C /repo tag",
		"git tag -l 'v*'",
		"git tag --list",
		"git tag --sort=-creatordate",
		"git tag --contains 7088ce0",
		"git tag --no-contains 7088ce0",
		"git tag --points-at HEAD",
		"git tag --merged main",
		"git tag --no-merged main",
	}
	for _, cmd := range readOnly {
		t.Run("allow "+cmd, func(t *testing.T) {
			r := evaluateAll(cmd)
			require.NotNil(t, r)
			assert.Equal(t, "git read op", r.reason)
			assert.Equal(t, decisionAllow, r.decision)
		})
	}

	mutating := []string{
		"git tag v1.0.0",
		"git tag -a v1.0.0 -m release",
		"git tag -d v1.0.0",
		"git tag -f v1.0.0 HEAD",
		"git tag -v v1.0.0",
	}
	for _, cmd := range mutating {
		t.Run("ask "+cmd, func(t *testing.T) {
			r := evaluateAll(cmd)
			require.NotNil(t, r)
			assert.Equal(t, "git tag", r.reason)
			assert.Equal(t, decisionAsk, r.decision)
		})
	}

	t.Run("mixed query and mutation asks", func(t *testing.T) {
		r := evaluateAll("git tag --sort=-creatordate -d v1.0.0")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("telemetry chain allows", func(t *testing.T) {
		r := evaluateAll("git tag | tail -5; echo ---; gh release list --repo kenn-io/agentsview --limit 3 2>&1")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `cd hooks/bash-approve && go test ./... -run TestGitTagDecision -count=1`

Expected: FAIL because the current catch-all rule returns `ask` for the read-only cases.

### Task 2: Classify literal read-only tag queries

**Files:**
- Modify: `hooks/bash-approve/git.go`
- Modify: `hooks/bash-approve/rules.go`

**Interfaces:**
- Consumes: `argsValidator`, `wordDecodedLiteralWithContext`, and the raw `[]*syntax.Word` command arguments.
- Produces: `isGitTagReadOnly(args []*syntax.Word, ctx evalContext) bool`.

- [ ] **Step 1: Add the conservative validator**

```go
func isGitTagReadOnly(args []*syntax.Word, ctx evalContext) bool {
	decoded := make([]string, len(args))
	for i, arg := range args {
		value, ok := wordDecodedLiteralWithContext(arg, ctx)
		if !ok {
			return false
		}
		decoded[i] = value
	}

	tagIndex := -1
	for i := 0; i < len(decoded); i++ {
		if decoded[i] != "git" {
			continue
		}
		candidate := i + 1
		if candidate < len(decoded) && decoded[candidate] == "-C" {
			candidate += 2
		}
		if candidate < len(decoded) && decoded[candidate] == "tag" {
			tagIndex = candidate
			break
		}
	}
	if tagIndex < 0 {
		return false
	}
	if tagIndex == len(decoded)-1 {
		return true
	}

	listMode := false
	for _, arg := range decoded[tagIndex+1:] {
		switch {
		case arg == "-l", arg == "--list", arg == "-i", arg == "--ignore-case",
			arg == "--no-column", arg == "--omit-empty":
			listMode = true
		case isGitTagLinesOption(arg), isGitTagListValueOption(arg):
			listMode = true
		case arg == "--":
			if !listMode {
				return false
			}
		case strings.HasPrefix(arg, "-"):
			return false
		default:
			if !listMode {
				return false
			}
		}
	}
	return listMode
}
```

Add small private helpers that recognize only `-n`/`-n<digits>` and the documented query/presentation options `--contains`, `--no-contains`, `--points-at`, `--merged`, `--no-merged`, `--sort`, `--format`, `--column`, and `--color`, in either exact or `--name=value` form. Any other option returns false.

- [ ] **Step 2: Add the more-specific command pattern before the catch-all gate**

```go
NewPattern(
	`^git\s+(-C\s+\S+\s+)?tag(?:\s*$|\s+(?:-l(?:\s|$)|-n\d*(?:\s|$)|-i(?:\s|$)|--(?:list|ignore-case|no-column|omit-empty)(?:\s|$)|--(?:contains|no-contains|points-at|merged|no-merged|sort|format|column|color)(?:=|\s|$)))`,
	tags("git read op", "git"),
	WithValidator(isGitTagReadOnly),
),
NewPattern(`^git\s+(-C\s+\S+\s+)?tag\b`, tags("git tag", "git"), WithDecision("ask")),
```

- [ ] **Step 3: Run the focused test and verify GREEN**

Run: `cd hooks/bash-approve && go test ./... -run TestGitTagDecision -count=1`

Expected: PASS.

### Task 3: Align user-facing category documentation

**Files:**
- Modify: `README.md`
- Modify: `hooks/bash-approve/categories.yaml`

**Interfaces:**
- Consumes: Existing command-category documentation.
- Produces: Documentation distinguishing read-only tag queries from gated mutations.

- [ ] **Step 1: Clarify the decision descriptions**

Update the README decision table and category listing so `git tag` creation/mutation is described as `ask`, while list/query forms are included under `git read op`. Update the `categories.yaml` comments with the same distinction; do not introduce a new category.

- [ ] **Step 2: Run formatting and focused tests**

Run: `cd hooks/bash-approve && gofmt -w git.go git_tag_test.go && go test ./... -run TestGitTagDecision -count=1`

Expected: PASS.

### Task 4: Verify and deliver

**Files:**
- Verify all changed files from Tasks 1-3.

**Interfaces:**
- Consumes: The repository test, lint, telemetry, roborev, and GitHub workflows.
- Produces: A reviewed, merged, and installed release of the rule change.

- [ ] **Step 1: Run repository verification**

Run:

```bash
cd hooks/bash-approve
go test ./... -count=1
golangci-lint run ./...
```

Expected: both commands exit 0.

- [ ] **Step 2: Replay the telemetry regression**

Pipe telemetry decision `71818`'s stored payload into the working-tree hook and verify it returns `allow`. Also evaluate `git tag v1.0.0` and verify it still returns `ask`.

- [ ] **Step 3: Commit the implementation**

Stage only the plan, rule, tests, and documentation. Create a rationale-first commit without bypassing hooks.

- [ ] **Step 4: Run `roborev-fix`**

Use `roborev fix --open --list`, fetch actionable failing jobs, fix all findings, run tests, then comment and close each fixed review. Do not manually start a duplicate roborev review.

- [ ] **Step 5: Push, open, validate, and merge the PR**

Push `fix/read-only-git-tag`, create a PR with `gh`, wait for required checks and review state, address failures without deleting or resolving comments, then merge using the repository-supported strategy.

- [ ] **Step 6: Install and verify the merged runtime**

Update the main checkout after merge, run `python3 install.py install --target all`, then replay the read-only telemetry command and a tag-creation command against the installed hook to confirm `allow` and `ask` respectively.
