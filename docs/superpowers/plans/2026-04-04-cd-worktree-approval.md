# CD Worktree Approval Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restrict automatic `cd` approval to linked worktree roots from the current repository and return no-decision otherwise.

**Architecture:** Extend command evaluation with hook `cwd` context, then apply a repo-aware validator to the `cd` pattern. Keep validator fallback behavior pattern-specific so existing commands retain their current ask/deny semantics.

**Tech Stack:** Go, `mvdan.cc/sh/v3`, Git CLI, `testify`

---

### Task 1: Thread Evaluation Context

**Files:**
- Modify: `hooks/bash-approve/main.go`
- Modify: `hooks/bash-approve/rules.go`
- Modify: `hooks/bash-approve/curl.go`
- Test: `hooks/bash-approve/main_test.go`

- [ ] **Step 1: Write the failing test**

```go
func evaluateAllInDir(cmd, cwd string) *result {
	return evaluate(cmd, evalContext{cwd: cwd}, allWrapperPatterns, allCommandPatterns)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestCDApproval_CurrentRepoWorktreesOnly -v`
Expected: FAIL because `cd` validation has no access to `cwd`.

- [ ] **Step 3: Write minimal implementation**

```go
type evalContext struct {
	cwd string
}

type HookInput struct {
	ToolName  string    `json:"tool_name"`
	ToolInput BashInput `json:"tool_input"`
	Cwd       string    `json:"cwd"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestCDApproval_CurrentRepoWorktreesOnly -v`
Expected: test reaches validator logic instead of failing for missing context.

- [ ] **Step 5: Commit**

```bash
git add hooks/bash-approve/main.go hooks/bash-approve/rules.go hooks/bash-approve/curl.go hooks/bash-approve/main_test.go
git commit -m "fix: scope cd approval to current repo worktrees"
```

### Task 2: Restrict CD Approval

**Files:**
- Modify: `hooks/bash-approve/main.go`
- Modify: `hooks/bash-approve/rules.go`
- Test: `hooks/bash-approve/main_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestCDApproval_CurrentRepoWorktreesOnly(t *testing.T) {
	repo := initGitRepo(t)
	linkedWorktree := filepath.Join(t.TempDir(), "feature-worktree")
	runGit(t, repo, "worktree", "add", "-b", "feature", linkedWorktree, "HEAD")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestCDApproval_CurrentRepoWorktreesOnly -v`
Expected: FAIL because `cd` is still blanket-approved or incorrectly scoped.

- [ ] **Step 3: Write minimal implementation**

```go
func isCurrentRepoWorktreeCD(args []*syntax.Word, ctx evalContext) bool {
	currentCommonDir, err := gitResolvedPath(ctx.cwd, "rev-parse", "--git-common-dir")
	if err != nil {
		return false
	}

	targetTopLevel, err := gitResolvedPath(resolvedTarget, "rev-parse", "--show-toplevel")
	if err != nil || targetTopLevel != resolvedTarget {
		return false
	}

	targetCommonDir, err := gitResolvedPath(resolvedTarget, "rev-parse", "--git-common-dir")
	if err != nil {
		return false
	}

	return currentCommonDir == targetCommonDir
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestCDApproval_CurrentRepoWorktreesOnly -v`
Expected: PASS for same-repo linked worktrees and no-decision for all other destinations.

- [ ] **Step 5: Commit**

```bash
git add hooks/bash-approve/main.go hooks/bash-approve/rules.go hooks/bash-approve/main_test.go
git commit -m "fix: restrict cd auto-approval to linked worktrees"
```

### Task 3: Verify Full Hook Behavior

**Files:**
- Modify: `hooks/bash-approve/main_test.go`

- [ ] **Step 1: Write the failing test**

```go
t.Run("unrelated directory is no-opinion", func(t *testing.T) {
	otherDir := t.TempDir()
	r := evaluateAllInDir("cd "+otherDir, repo)
	assert.Empty(t, r.decision)
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./...`
Expected: FAIL until validator fallback for `cd` is changed from `ask` to no-decision.

- [ ] **Step 3: Write minimal implementation**

```go
NewPattern(`^cd\s`, tags("cd", "shell"), WithValidator(isCurrentRepoWorktreeCD), WithValidatorFallback(""))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./...`
Expected: PASS with all existing command behavior unchanged apart from scoped `cd` approval.

- [ ] **Step 5: Commit**

```bash
git add hooks/bash-approve/main.go hooks/bash-approve/rules.go hooks/bash-approve/main_test.go
git commit -m "test: cover scoped cd worktree approval"
```
