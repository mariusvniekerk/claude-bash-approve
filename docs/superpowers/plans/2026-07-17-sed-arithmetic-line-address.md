# Sed Arithmetic Line-Address Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Auto-approve read-only sed ranges whose arithmetic address contains a proven numeric line-number pipeline without broadening approval to arbitrary nested command output.

**Architecture:** Reuse the existing line-number provenance parser in `sed_address.go` directly from the arithmetic-expression validator in `sed.go`. Keep recursive nested-command decision propagation unchanged so destructive or prompt-gated commands retain their current decisions.

**Tech Stack:** Go, `mvdan.cc/sh/v3/syntax`, Testify

## Global Constraints

- Only proven line-number-producing substitutions may appear inside arithmetic sed addresses.
- Wrong-field, non-numeric, and process-substitution forms remain prompt-gated.
- Nested destructive commands continue to propagate denial.
- Preserve unrelated working-tree changes.
- Never bypass Git verification hooks.

---

### Task 1: Recognize Proven Line Pipelines Inside Sed Arithmetic

**Files:**
- Modify: `hooks/bash-approve/sed_address.go`
- Modify: `hooks/bash-approve/sed.go`
- Test: `hooks/bash-approve/sed_test.go`

**Interfaces:**
- Consumes: `lineNumberFieldForCall(call *syntax.CallExpr, ctx evalContext) (int, bool)`, `isLineNumberPipelineMiddleCall(call *syntax.CallExpr, ctx evalContext) bool`, and `isCutColonFieldCall(call *syntax.CallExpr, ctx evalContext, field int) bool`.
- Produces: `isLineNumberPipelineCmdSubst(cs *syntax.CmdSubst, ctx evalContext) bool`, used by assignment provenance and sed arithmetic validation.

- [ ] **Step 1: Add the focused regression and conservative-boundary tests**

Add these cases to `TestEvaluate_SedFlows` in `hooks/bash-approve/sed_test.go`:

```go
t.Run("read-only sed with line-number substitution inside arithmetic allowed", func(t *testing.T) {
	r := evaluateAll(`sed -n "$(($(grep -n 'func (s \*Server) resolveItem(' internal/server/huma_routes.go | cut -d: -f1)+55)),+60p" internal/server/huma_routes.go`)
	require.NotNil(t, r)
	assert.Equal(t, "sed", r.reason)
	assert.Equal(t, decisionAllow, r.decision)
})

t.Run("read-only sed with wrong line field inside arithmetic asks", func(t *testing.T) {
	r := evaluateAll(`sed -n "$(($(grep -rn 'func x' internal/server | cut -d: -f1)+55)),+60p" file`)
	require.NotNil(t, r)
	assert.Equal(t, decisionAsk, r.decision)
})

t.Run("read-only sed with non-line command inside arithmetic asks", func(t *testing.T) {
	r := evaluateAll(`sed -n "$(( $(cat line.txt) + 1 ))p" file`)
	require.NotNil(t, r)
	assert.Equal(t, decisionAsk, r.decision)
})
```

- [ ] **Step 2: Run the focused test and verify the regression fails**

Run:

```bash
cd hooks/bash-approve && go test ./... -run TestEvaluate_SedFlows -count=1
```

Expected: FAIL because the telemetry command receives `decisionAsk` instead of `decisionAllow`; the two conservative cases pass.

- [ ] **Step 3: Share the existing line-number provenance parser**

Refactor `hooks/bash-approve/sed_address.go` so the word-based helper delegates after extracting its command substitution:

```go
func isLineNumberPipelineAssignment(w *syntax.Word, ctx evalContext) bool {
	return isLineNumberPipelineCmdSubst(singleCmdSubstWord(w), ctx)
}

func isLineNumberPipelineCmdSubst(cs *syntax.CmdSubst, ctx evalContext) bool {
	calls := cmdSubstPipeCalls(cs)
	if len(calls) < 2 {
		return false
	}
	lineField, ok := lineNumberFieldForCall(calls[0], ctx)
	if !ok {
		return false
	}
	for _, call := range calls[1 : len(calls)-1] {
		if !isLineNumberPipelineMiddleCall(call, ctx) {
			return false
		}
	}
	return isCutColonFieldCall(calls[len(calls)-1], ctx, lineField)
}
```

Extract the AST-shape check used by `singleCmdSubstPipeCalls`:

```go
func singleCmdSubstPipeCalls(w *syntax.Word) []*syntax.CallExpr {
	return cmdSubstPipeCalls(singleCmdSubstWord(w))
}

func cmdSubstPipeCalls(cs *syntax.CmdSubst) []*syntax.CallExpr {
	if cs == nil || len(cs.Stmts) != 1 {
		return nil
	}
	stmt := cs.Stmts[0]
	if stmt == nil || stmt.Cmd == nil || len(stmt.Redirs) > 0 {
		return nil
	}
	return flattenPipeCalls(stmt.Cmd)
}
```

- [ ] **Step 4: Make arithmetic validation require proven numeric provenance**

In `hooks/bash-approve/sed.go`, pass `ctx` into the arithmetic walker and accept only shared-provenance command substitutions:

```go
case *syntax.ArithmExp:
	*sawDynamic = true
	return arithmeticExpansionHasSafeSedAddresses(p, ctx)
```

Replace `arithmeticExpansionHasNoCommandSubst` with:

```go
func arithmeticExpansionHasSafeSedAddresses(exp *syntax.ArithmExp, ctx evalContext) bool {
	if exp == nil || exp.X == nil {
		return false
	}
	ok := true
	syntax.Walk(exp.X, func(n syntax.Node) bool {
		switch p := n.(type) {
		case *syntax.CmdSubst:
			ok = isLineNumberPipelineCmdSubst(p, ctx)
			return false
		case *syntax.ProcSubst:
			ok = false
			return false
		}
		return ok
	})
	return ok
}
```

- [ ] **Step 5: Run focused tests and verify green**

Run:

```bash
cd hooks/bash-approve && go test ./... -run 'TestEvaluate_SedFlows|TestIsSedSafe' -count=1
```

Expected: PASS, including the pre-existing nested `rm -rf` denial test.

- [ ] **Step 6: Run complete repository verification**

Run:

```bash
cd hooks/bash-approve && go test ./... -count=1
cd hooks/bash-approve && golangci-lint run ./...
```

Expected: both commands exit 0 without failures.

- [ ] **Step 7: Commit the implementation**

Stage only the plan and implementation files and create a rationale-first commit using the mandatory commit workflow. Do not stage `.opencode/plugins/bash-approve.ts`, `.codex/`, or `.vite-hooks/` unless they were changed by the repository's own verification hook and are directly required by this fix.

### Task 2: Deliver and Install the Fix

**Files:**
- Inspect: `install.sh`
- Inspect: `hooks/bash-approve/README.md`

**Interfaces:**
- Consumes: a verified implementation commit on a feature branch and the repository's documented local installer.
- Produces: a merged GitHub pull request and a locally installed hook whose evaluator allows the telemetry command.

- [ ] **Step 1: Push the feature branch and open a pull request**

Use the repository's authenticated GitHub CLI flow. The PR body must explain the telemetry false positive, the numeric provenance boundary, and validation. Any GitHub comment or PR body sent under the user's account must end with:

```html
<sup>generated by a clanker</sup>
```

- [ ] **Step 2: Confirm required PR checks and merge**

Inspect PR checks with `gh`. When all required checks pass, merge the PR using the repository's accepted merge method without deleting review comments or threads.

- [ ] **Step 3: Install the merged version locally**

Update the local checkout to the merged commit if necessary, follow the documented installer command, and do not overwrite unrelated working-tree changes.

- [ ] **Step 4: Verify the installed hook**

Replay the exact telemetry command through the installed hook or its documented evaluation entry point. Expected: `decisionAllow` with reason `sed`. Confirm the installed binary/version points at the merged build.
