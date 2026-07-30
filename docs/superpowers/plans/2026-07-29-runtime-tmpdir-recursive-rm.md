# Runtime TMPDIR Recursive-rm Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Auto-approve recursive removal of explicit strict descendants of the hook process's absolute runtime `TMPDIR` without trusting all of `/var/folders`.

**Architecture:** Extend the existing allowed-scope resolver in `rm.go`. Resolve the target as today, then compare it against both fixed temporary roots and one optional absolute process `TMPDIR` through the existing symlink-aware strict-descendant helper.

**Tech Stack:** Go, `mvdan.cc/sh/v3/syntax`, Testify, Git-aware evaluator tests.

## Global Constraints

- Only the hook process environment's `TMPDIR` is considered; inline shell assignments do not change the trusted root.
- `TMPDIR` must be non-empty and absolute.
- A `TMPDIR` that resolves to a filesystem root adds no trusted scope.
- The resolved `TMPDIR` root itself remains denied; only strict descendants are allowed.
- Repository roots remain denied even when `TMPDIR` points at the repository.
- Existing literal, glob, expansion, mixed-target, and symlink-escape protections remain unchanged.
- Follow TDD and observe the focused allow regression fail before editing production code.
- Direct `go test` commands use `-shuffle=on`.
- Never bypass Git verification with `--no-verify`.

## File Structure

- Modify `hooks/bash-approve/rm.go`: derive and validate the optional runtime temporary root.
- Modify `hooks/bash-approve/main_test.go`: cover the runtime-root allowance and safety boundaries through evaluator behavior.

---

### Task 1: Permit strict descendants of absolute process TMPDIR

**Files:**
- Modify: `hooks/bash-approve/rm.go`
- Test: `hooks/bash-approve/main_test.go`

**Interfaces:**
- Consumes: `os.Getenv("TMPDIR")`, `resolveRecursiveRmTarget(target string) (string, error)`, and `recursiveRmTargetStrictlyBelow(root, target string) bool`.
- Produces: `recursiveRmRuntimeTempRoot() string`, used only by `recursiveRmTargetInAllowedScope`.

- [ ] **Step 1: Write focused failing evaluator tests**

Add subtests beside the existing standard-temporary-root cases. Use the literal nonstandard root `/var/folders/bash-approve-tests/T` so the behavior is independent of the test runner's host `TMPDIR`:

```go
t.Run("rm -rf runtime TMPDIR descendant allowed", func(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init").Run())
	root := "/var/folders/bash-approve-tests/T"
	t.Setenv("TMPDIR", root)

	r := evaluateAllInDir("rm -rf "+filepath.Join(root, "middleman-e2e.123"), repo)
	require.NotNil(t, r)
	assert.Equal(t, decisionAllow, r.decision)
})
```

Add one table covering the policy boundaries:

```go
t.Run("rm -rf runtime TMPDIR boundaries remain denied", func(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init").Run())
	root := "/var/folders/bash-approve-tests/T"

	for _, tc := range []struct {
		name    string
		tmpdir  string
		command string
	}{
		{"root", root, "rm -rf " + root},
		{"filesystem root", "/", "rm -rf /etc/bash-approve-safe"},
		{"empty root", "", "rm -rf " + filepath.Join(root, "child")},
		{"relative root", "var/folders/bash-approve-tests/T", "rm -rf " + filepath.Join(root, "child")},
		{"repository root", repo, "rm -rf ."},
		{"inline assignment", root, "TMPDIR=/etc rm -rf /etc/bash-approve-safe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TMPDIR", tc.tmpdir)
			r := evaluateAllInDir(tc.command, repo)
			require.NotNil(t, r)
			assert.Equal(t, decisionDeny, r.decision)
		})
	}
})
```

- [ ] **Step 2: Verify RED**

Run:

```bash
cd hooks/bash-approve
go test ./... -run 'TestNoOpinionDecision/rm_-rf_runtime_TMPDIR' -shuffle=on
```

Expected: FAIL only for `runtime_TMPDIR_descendant_allowed`, which receives `deny` because no runtime root is currently considered.

- [ ] **Step 3: Implement the minimal runtime-root policy**

Import `os`, then add:

```go
func recursiveRmRuntimeTempRoot() string {
	root := os.Getenv("TMPDIR")
	if !filepath.IsAbs(root) {
		return ""
	}
	return root
}
```

After checking the fixed roots in `recursiveRmTargetInAllowedScope`, resolve the optional root and apply the existing strict-descendant predicate:

```go
runtimeRoot := recursiveRmRuntimeTempRoot()
if runtimeRoot == "" {
	return false
}
resolvedRoot, err := resolveRecursiveRmTarget(runtimeRoot)
if err != nil || filepath.Dir(resolvedRoot) == resolvedRoot {
	return false
}
if recursiveRmTargetStrictlyBelow(resolvedRoot, targetPath) {
	return true
}
```

The explicit empty-root guard keeps unset or empty `TMPDIR` fail-closed before filesystem resolution; the absolute-path check rejects relative values at the same boundary.

- [ ] **Step 4: Verify GREEN and regression safety**

Run:

```bash
cd hooks/bash-approve
go test ./... -run 'TestNoOpinionDecision/rm_-rf_(runtime_TMPDIR|standard_temporary|temporary_symlink|repository_root)' -shuffle=on
go test ./... -shuffle=on
golangci-lint run ./...
```

Expected: all commands exit 0 with no failures or lint findings.

- [ ] **Step 5: Replay the branch binary in isolated state**

Build the branch binary into a mode-0700 scratch directory, send evaluator JSON with a scratch `XDG_STATE_HOME`, and verify:

- an explicit descendant of the current absolute process `TMPDIR` is `allow`;
- the `TMPDIR` root itself is `deny`;
- `/var/folders` outside the current `TMPDIR` is `deny`; and
- a process `TMPDIR` of `/` does not approve an `/etc` descendant.

- [ ] **Step 6: Commit, push, open a PR, merge on green CI, and install**

Stage only the two documentation files, `rm.go`, and `main_test.go`. Follow the mandatory commit workflow, inspect the complete branch history and introduced blobs, push, and open a rationale-first PR. If all required CI checks pass, squash-merge it. Build and install only from the merged commit, retaining a private recovery backup of the previous installed runtime, then replay the installed binary with isolated telemetry state.
