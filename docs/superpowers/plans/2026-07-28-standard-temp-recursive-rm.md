# Standard Temporary-Directory Recursive-rm Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Auto-approve recursive removal of explicit strict descendants of standard temporary-directory roots without weakening recursive-rm protections elsewhere.

**Architecture:** Extend the existing resolver in `rm.go`; do not add a second command pattern or reuse the broader write-target policy. Resolve each target and each standard temporary root through the existing symlink-aware path resolver, then accept the command only when every target is strictly below either the active repository or one of the standard temporary roots.

**Tech Stack:** Go, `mvdan.cc/sh/v3/syntax`, Testify, Git-aware evaluator tests.

## Global Constraints

- Standard temporary roots are `/tmp`, `/var/tmp`, `/private/tmp`, and `/private/var/tmp`.
- The roots themselves remain denied; only strict descendants are allowed.
- Repository roots remain denied even when the repository is beneath a standard temporary root.
- Unquoted globs, unresolved or dynamic targets, symlink escapes, and mixed safe/unsafe target lists remain denied.
- Unquoted parameter and command substitutions remain denied as targets because Bash may field-split or pathname-expand their values.
- Existing explicit in-repository recursive removal remains allowed.
- Do not add configuration or broaden any non-`rm` command policy.
- Follow TDD: observe the focused regression test fail before editing production code.
- Direct `go test` commands use `-shuffle=on` and do not use `-count=1`.
- Never bypass Git verification with `--no-verify`.

## File Structure

- Modify `hooks/bash-approve/rm.go`: own recursive-rm target normalization and allowed-scope decisions.
- Modify `hooks/bash-approve/main_test.go`: own evaluator-level regression coverage for recursive-rm decisions.

---

### Task 1: Permit explicit descendants of standard temporary roots

**Files:**
- Modify: `hooks/bash-approve/rm.go:10-73`
- Test: `hooks/bash-approve/main_test.go:660-1950`

**Interfaces:**
- Consumes: `resolveRecursiveRmTarget(target string) (string, error)`, `repoRootForCwd(cwd string) string`, `wordLiteralPathWithContext(word *syntax.Word, ctx evalContext) string`, and `hasUnquotedGlob(word *syntax.Word) bool`.
- Produces: `hasUnquotedRmTargetExpansion(word *syntax.Word) bool`, `recursiveRmTargetInAllowedScope(cwd, target string) bool`, and `recursiveRmTargetStrictlyBelow(root, target string) bool`, used only by `resolveRecursiveRmDecision`.

- [ ] **Step 1: Add the evaluator-level regression test**

Add this focused test beside the existing `rm -rf explicit in-repo directory allowed` case in `hooks/bash-approve/main_test.go`:

```go
t.Run("rm -rf standard temporary descendants allowed", func(t *testing.T) {
		repo := t.TempDir()
		require.NoError(t, exec.Command("git", "-C", repo, "init").Run())

		for _, target := range []string{
			"/tmp/bash-approve-recursive-rm-test",
			"/var/tmp/bash-approve-recursive-rm-test",
			"/private/tmp/bash-approve-recursive-rm-test",
			"/private/var/tmp/bash-approve-recursive-rm-test",
		} {
			r := evaluateAllInDir("rm -rf "+target, repo)
			require.NotNil(t, r, target)
			assert.Equal(t, decisionAllow, r.decision, target)
		}
	})

	t.Run("rm -rf standard temporary boundaries remain denied", func(t *testing.T) {
		repo := t.TempDir()
		require.NoError(t, exec.Command("git", "-C", repo, "init").Run())

		for _, command := range []string{
			"rm -rf /tmp",
			"rm -rf /var/tmp",
			"rm -rf /private/tmp",
			"rm -rf /private/var/tmp",
			"rm -rf /tmp/bash-approve-*",
			"rm -rf /tmp/bash-approve-safe /etc/bash-approve-unsafe",
		} {
			r := evaluateAllInDir(command, repo)
			require.NotNil(t, r, command)
			assert.Equal(t, decisionDeny, r.decision, command)
		}
	})

	t.Run("rm -rf temporary symlink escape remains denied", func(t *testing.T) {
		repo := t.TempDir()
		require.NoError(t, exec.Command("git", "-C", repo, "init").Run())
		tempRoot, err := os.MkdirTemp("/tmp", "bash-approve-rm-symlink.")
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, os.RemoveAll(tempRoot)) })
		escape := filepath.Join(tempRoot, "escape")
		require.NoError(t, os.Symlink("/", escape))

		r := evaluateAllInDir("rm -rf "+escape, repo)
		require.NotNil(t, r)
		assert.Equal(t, decisionDeny, r.decision)
	})

	t.Run("rm -rf repository root under temp remains denied", func(t *testing.T) {
		repo, err := os.MkdirTemp("/tmp", "bash-approve-rm-repo.")
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, os.RemoveAll(repo)) })
		require.NoError(t, exec.Command("git", "-C", repo, "init").Run())

		r := evaluateAllInDir("rm -rf .", repo)
		require.NotNil(t, r)
		assert.Equal(t, decisionDeny, r.decision)
	})

	t.Run("rm -rf unquoted static expansion remains denied", func(t *testing.T) {
		repo := t.TempDir()
		require.NoError(t, exec.Command("git", "-C", repo, "init").Run())

		for _, command := range []string{
			`targets='/tmp/bash-approve-safe /etc'; rm -rf $targets`,
			`targets='/tmp/bash-approve-*'; rm -rf $targets`,
			`args='-- /etc'; rm -rf $args /tmp/bash-approve-safe`,
		} {
			r := evaluateAllInDir(command, repo)
			require.NotNil(t, r, command)
			assert.Equal(t, decisionDeny, r.decision, command)
		}
	})
```

Retarget existing tests whose purpose is to prove that statically recognized
recursive-rm spellings deny an unsafe target. In each existing subtest that
asserts `decisionDeny` for a command ending in `/tmp/stuff`, change only that
target to `/`. Also change the statically split command
`rm $(echo -rf /tmp/x)` to `rm $(echo -rf /)`. Leave tests that assert
no-opinion unchanged because their contract is that the dynamic flag is not
recognized, and leave commands targeting the temporary roots themselves
unchanged because deleting `/tmp` or `/var/tmp` remains denied.

The representative edits are:

```go
r := evaluateAll("rm -rf /")
r := evaluateAll("rm '-rf' /")
r := evaluateAll(`rm $(echo -e '\055rf') /`)
r := evaluateAll(`rm $(echo -rf /)`)
```

After editing, this query must produce no tests that both use a temporary
descendant and assert a deny decision:

```bash
rg -n 'rm .*?/tmp/(stuff|x)' hooks/bash-approve/main_test.go
```

Remaining matches are permitted only in the explicitly no-opinion test cases
or comments describing their one-argument parsing behavior.

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd hooks/bash-approve
go test ./... -run 'TestNoOpinionDecision/rm_-rf_(standard_temporary|temporary_symlink)' -shuffle=on
```

Expected: FAIL because each standard temporary descendant currently resolves to `deny`, while the retargeted destructive cases, boundaries, and escape case remain denied.

- [ ] **Step 3: Extend the resolver with the minimal allowed-scope logic**

In `hooks/bash-approve/rm.go`, add the roots near the resolver:

```go
var recursiveRmStandardTempRoots = [...]string{
	"/tmp",
	"/var/tmp",
	"/private/tmp",
	"/private/var/tmp",
}
```

Reject unquoted target expansions immediately after literal decoding and before
option classification:

```go
if hasUnquotedRmTargetExpansion(arg) {
	return nil
}
```

Add the unquoted-expansion guard:

```go
func hasUnquotedRmTargetExpansion(word *syntax.Word) bool {
	for _, part := range word.Parts {
		switch part.(type) {
		case *syntax.ParamExp, *syntax.CmdSubst:
			return true
		}
	}
	return false
}
```

Keep the target-scope condition after option classification:

```go
if hasUnquotedGlob(arg) || !recursiveRmTargetInAllowedScope(ctx.cwd, literal) {
	return nil
}
```

Replace `recursiveRmTargetInCurrentRepo` with these helpers:

```go
func recursiveRmTargetInAllowedScope(cwd, target string) bool {
	targetPath := target
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(cwd, targetPath)
	}
	targetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	targetPath, err = resolveRecursiveRmTarget(targetPath)
	if err != nil {
		return false
	}

	repoRoot := repoRootForCwd(cwd)
	if repoRoot != "" {
		if targetPath == repoRoot {
			return false
		}
		if recursiveRmTargetStrictlyBelow(repoRoot, targetPath) {
			return true
		}
	}
	for _, root := range recursiveRmStandardTempRoots {
		resolvedRoot, err := resolveRecursiveRmTarget(root)
		if err == nil && recursiveRmTargetStrictlyBelow(resolvedRoot, targetPath) {
			return true
		}
	}
	return false
}

func recursiveRmTargetStrictlyBelow(root, target string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != "." && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
```

Keep `resolveRecursiveRmTarget` unchanged so existing-component symlink resolution and non-existent suffix handling remain shared by repository and temporary targets.

- [ ] **Step 4: Run the focused test and verify GREEN**

Run:

```bash
cd hooks/bash-approve
go test ./... -run 'TestNoOpinionDecision/rm_-rf_(standard_temporary|temporary_symlink)' -shuffle=on
```

Expected: PASS. The four descendant cases return `allow`; the roots, glob, symlink escape, and mixed target list return `deny`.

- [ ] **Step 5: Run the complete Go test suite**

Run:

```bash
cd hooks/bash-approve
go test ./... -shuffle=on
```

Expected: PASS with no package failures.

- [ ] **Step 6: Run the linter**

Run:

```bash
cd hooks/bash-approve
golangci-lint run ./...
```

Expected: PASS with no findings.

- [ ] **Step 7: Review and commit the implementation**

Review only the intended files:

```bash
git diff --check
git diff -- hooks/bash-approve/rm.go hooks/bash-approve/main_test.go
git status --short
```

Invoke the mandatory `commit-push-pr:commit` skill, stage only
`hooks/bash-approve/rm.go`, `hooks/bash-approve/main_test.go`, and this plan,
then create one commit with the suggested subject
`Allow recursive cleanup under standard temp roots`.
