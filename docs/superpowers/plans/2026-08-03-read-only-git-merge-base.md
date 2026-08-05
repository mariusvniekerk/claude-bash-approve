# Read-only git merge-base Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan directly in the current agent, task-by-task. Never use subagent-driven development. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Auto-approve telemetry decision 84177 while preserving genuine Git write gating and moving repository probes onto kenn-io/kit's safe Git runner.

**Approved spec/design:** docs/superpowers/specs/2026-08-03-read-only-git-merge-base-design.md

**Architecture:** Keep Git classification in ordered evaluator patterns, but require an exact subcommand terminator so a hyphenated read command cannot inherit a shorter write command's policy. Keep the repository-probe function boundary and replace its subprocess implementation with gitcmd.Runner.

**Tech Stack:** Go 1.26.3, mvdan.cc/sh/v3, testify, go.kenn.io/kit/git/cmd v0.17.1, SQLite telemetry, prek, golangci-lint.

## Global Constraints

- Preserve real git merge as git write op.
- A disabled Git read category must not let merge-base match the write pattern.
- Replay exact telemetry decision 84177.
- Remove the ad-hoc production Git environment filter.
- Do not stage .opencode/plugins/bash-approve.ts, .codex/, or .vite-hooks/.
- Never pass --no-verify to Git.

---

### Task 1: Exact Git subcommand classification

**Files:**
- Modify: hooks/bash-approve/main_test.go
- Modify: hooks/bash-approve/xargs_test.go
- Modify: hooks/bash-approve/rules.go

**Interfaces:**
- Consumes: evaluateAll(command string) *result and Evaluate(command string, cfg Config, ctx evalContext) *result.
- Produces: merge-base labeled git read op; listed write commands match only before whitespace or end of input.

- [ ] **Step 1: Add direct and filtered-category failing regressions**

Add this read case to TestEvaluate_Approved:

~~~go
{"git merge-base", "git merge-base HEAD origin/main", "git read op"},
~~~

Add:

~~~go
func TestGitMergeBaseDoesNotMatchWritePattern(t *testing.T) {
	cfg := Config{Enabled: []string{"git write op"}, Disabled: []string{"git read op"}}
	r := Evaluate("git merge-base HEAD origin/main", cfg, evalContext{})
	assert.Nil(t, r)
}
~~~

- [ ] **Step 2: Add telemetry 84177 to TestXargsReadOnlyPipelineCanAppendToAllowedCommands**

~~~go
{
	name: "telemetry 84177 merge-base pipeline",
	cmd:  "git fetch origin main && printf 'origin/main ' && git rev-parse --short origin/main && printf 'merge-base ' && git merge-base HEAD origin/main | xargs git rev-parse --short && printf 'head ' && git rev-parse --short HEAD",
},
~~~

- [ ] **Step 3: Verify RED**

~~~bash
cd hooks/bash-approve
go test ./... -run 'TestEvaluate_Approved|TestGitMergeBaseDoesNotMatchWritePattern|TestXargsReadOnlyPipelineCanAppendToAllowedCommands' -count=1
~~~

Expected: FAIL because merge-base is a git write op, the filtered result is non-nil, and 84177 asks.

- [ ] **Step 4: Implement the minimal classifier fix**

Add merge-base to the Git read alternatives. Replace the Git write pattern's trailing word boundary with:

~~~go
(?:\s|$)
~~~

The complete write matcher must still list the existing write subcommands unchanged.

- [ ] **Step 5: Verify GREEN**

Run Step 3 again. Expected: PASS, including the existing git merge feature-branch write case.

- [ ] **Step 6: Commit**

Use the mandatory commit skill, stage only Task 1 files, and commit as: Allow read-only git merge-base pipelines.

### Task 2: Safe Git runner migration

**Files:**
- Modify: hooks/bash-approve/git.go
- Modify: hooks/bash-approve/main_test.go
- Modify: hooks/bash-approve/go.mod
- Modify: hooks/bash-approve/go.sum

**Interfaces:**
- Consumes: gitcmd.New(), Runner.Output, and Runner.Run.
- Produces: unchanged gitOutput and runGit interfaces backed by the safe runner.

- [ ] **Step 1: Add a failing global-config isolation regression**

~~~go
func TestGitOutputIgnoresGlobalAliases(t *testing.T) {
	xdgConfig := t.TempDir()
	configDir := filepath.Join(xdgConfig, "git")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "config"),
		[]byte("[alias]\n\tbashapprove-probe = !printf compromised\n"),
		0o600,
	))
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)

	_, err := gitOutput(t.TempDir(), "bashapprove-probe")
	require.Error(t, err)
}
~~~

- [ ] **Step 2: Verify RED**

~~~bash
cd hooks/bash-approve
go test ./... -run '^TestGitOutputIgnoresGlobalAliases$' -count=1
~~~

Expected: FAIL because direct Git reads the injected alias and exits successfully.

- [ ] **Step 3: Add the safe runner dependency**

~~~bash
cd hooks/bash-approve
go get go.kenn.io/kit/git/cmd@v0.17.1
~~~

Expected: go.mod and go.sum record go.kenn.io/kit v0.17.1. Because every
published kit release requires Go 1.26.3 and kit's module graph requires newer
modernc SQLite, libc, and x/sys minimums, accept those MVS upgrades together.

- [ ] **Step 4: Replace production Git execution**

In git.go, remove os/exec, add context and gitcmd "go.kenn.io/kit/git/cmd", remove envWithoutGitVars, and use:

~~~go
func gitOutput(dir string, args ...string) (string, error) {
	out, err := gitcmd.New().Output(context.Background(), dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
~~~

- [ ] **Step 5: Move the test fixture onto the safe runner**

In main_test.go, add context, remove os/exec, add gitcmd, and use:

~~~go
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	stdout, stderr, err := gitcmd.New().Run(context.Background(), dir, nil, args...)
	require.NoErrorf(t, err, "git %v failed: %s", args, string(stderr))
	return string(stdout)
}
~~~

- [ ] **Step 6: Verify GREEN**

~~~bash
cd hooks/bash-approve
go test ./... -run 'TestGitOutputIgnoresGlobalAliases|TestEvaluate.*(Read|Grep|Find|Ls)|Test.*Repo|Test.*Worktree|TestChmod|TestRecursiveRm' -count=1
~~~

Expected: PASS with path behavior unchanged and the injected alias ignored.

- [ ] **Step 7: Commit**

Use the mandatory commit skill, stage only Task 2 files, and commit as: Use kenn kit for safe git probes.

### Task 3: Verification, PR, merge, and deployment

**Files:**
- Create: docs/superpowers/plans/2026-08-03-read-only-git-merge-base.md
- Verify: all files committed by Tasks 1 and 2.

**Interfaces:**
- Consumes: approve-bash JSON stdin and the merged GitHub PR.
- Produces: merged main, installed runtime, and replay evidence.

- [ ] **Step 1: Run full local verification**

~~~bash
cd hooks/bash-approve
go test ./... -shuffle=on -count=1
golangci-lint run ./...
~~~

Expected: both exit 0.

- [ ] **Step 2: Build and replay in isolated temporary directories**

Build approve-bash in a newly created temporary directory. With a separate temporary XDG_STATE_HOME and installed categories.yaml, send Pi-format JSON for telemetry 84177 and git merge feature-branch.

Expected: 84177 allows with merge-base labeled git read op; genuine merge allows with git write op. Remove only exact temporary paths created here.
- [ ] **Step 3: Commit this plan**

Use the mandatory commit skill, stage only this plan, and commit as: Plan read-only merge-base handling.

- [ ] **Step 4: Push and open the PR**

Use the commit-push-PR skill. Push codex/allow-git-merge-base and open against main. Include telemetry 84177, exact-boundary rationale, kenn-io/kit migration, and verification. End agent-authored GitHub text with:

~~~html
<sup>generated by a clanker</sup>
~~~

- [ ] **Step 5: Wait for CI and merge**

Monitor checks. Fix failures without deleting or resolving comments. Merge only after required checks pass.

- [ ] **Step 6: Install merged main**

Use isolate-production. In a clean temporary checkout at the merge commit, run:

~~~bash
python3 install.py install --target all
~~~

Expected: shared runtime under ~/.local/share/claude-bash-approve is rebuilt.

- [ ] **Step 7: Replay installed runtime**

With fresh isolated state, replay 84177 and genuine merge against the installed approve-bash. Report PR URL, merge commit, installed hash, and both decisions.
