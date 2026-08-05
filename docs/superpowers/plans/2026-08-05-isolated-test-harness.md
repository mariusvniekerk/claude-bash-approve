# Guarded Isolated-Test Harness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan directly in the current agent, task-by-task. Never use subagent-driven development. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Auto-approve telemetry 84211 and its guarded Bun variant without approving arbitrary traps, environment rewrites, or Node scripts.

**Approved spec/design:** docs/superpowers/specs/2026-08-05-isolated-test-harness-design.md

**Architecture:** Parse the complete shell file before ordinary statement merging and offer it to one dedicated harness recognizer. The recognizer proves exact relationships across five setup/cleanup statements and delegates only the final test runner through existing command policy; any mismatch returns control to the existing evaluator.

**Tech Stack:** Go 1.26.3, mvdan.cc/sh/v3 AST, testify, SQLite telemetry, prek, golangci-lint.

## Global Constraints

- Recognition is all-or-nothing across exactly five statements.
- Creation, case validation, chmod, trap cleanup, and scratch paths use the same variable.
- The guard accepts only /tmp, /private/tmp, and /var/folders descendants.
- Arbitrary trap payloads, PATH components, Node scripts, environment targets, and extra statements remain unrecognized.
- Existing command policy decides whether the final runner is allowed.
- Preserve unrelated .opencode, .codex, and .vite-hooks state.
- Never pass --no-verify to Git.

---

### Task 1: RED telemetry and fail-closed coverage

**Files:**
- Create: hooks/bash-approve/isolated_test_harness_test.go

**Interfaces:**
- Consumes: evaluateAll(command string) *result.
- Produces: regression expectations for telemetry 84211, guarded Bun telemetry 87832, and mutations of each proof boundary.

- [ ] **Step 1: Add exact positive cases**

Use these exact commands as raw Go strings named telemetry84211 and guardedBunHarness:

~~~bash
task_test_home=$(mktemp -d -t forge-pr818-ui-direct.XXXXXX); case "$task_test_home" in /tmp/*|/private/tmp/*|/var/folders/*) ;; *) exit 91 ;; esac; chmod 700 "$task_test_home"; trap 'rm -rf -- "$task_test_home"' EXIT; env -u GIT_CONFIG -u KENN_CONFIG -u KENN_API_URL -u KENN_FORGE_API_URL HOME="$task_test_home" XDG_CONFIG_HOME="$task_test_home/config" XDG_DATA_HOME="$task_test_home/data" XDG_CACHE_HOME="$task_test_home/cache" XDG_STATE_HOME="$task_test_home/state" PATH="/Users/mariusvniekerk/.local/share/mise/installs/node/24/bin:/usr/bin:/bin:/usr/sbin:/sbin" /Users/mariusvniekerk/.local/share/mise/installs/node/24/bin/node ../node_modules/vite-plus/bin/vp test run --project unit ../packages/ui/src/components/detail/EventTimeline.test.ts
~~~

~~~bash
task_frontend_home=$(mktemp -d -t forge-pr817-round3-vitest.XXXXXX); case "$task_frontend_home" in /tmp/*|/private/tmp/*|/var/folders/*) ;; *) exit 91 ;; esac; chmod 700 "$task_frontend_home"; trap 'task_status=$?; rm -rf -- "$task_frontend_home"; exit $task_status' EXIT; env -u GIT_CONFIG -u KENN_CONFIG -u KENN_API_URL HOME="$task_frontend_home" XDG_CONFIG_HOME="$task_frontend_home/config" XDG_DATA_HOME="$task_frontend_home/data" XDG_CACHE_HOME="$task_frontend_home/cache" XDG_STATE_HOME="$task_frontend_home/state" BUN_INSTALL_CACHE_DIR="$task_frontend_home/bun-cache" PATH="/Users/mariusvniekerk/.local/share/mise/installs/node/24/bin:/Users/mariusvniekerk/.local/share/mise/installs/bun/1.3.14/bin:/usr/bin:/bin:/usr/sbin:/sbin" /Users/mariusvniekerk/.local/share/mise/installs/bun/1.3.14/bin/bun run test -- ../packages/ui/src/components/detail/EventTimeline.test.ts
~~~

Assert both evaluateAll results are non-nil, decisionAllow, and reason isolated test harness.

- [ ] **Step 2: Add literal fail-closed table cases**

Create complete command mutations for:

- mktemp without -d;
- a case guard also accepting the home directory;
- chmod targeting a different variable;
- trap removing a different variable;
- trap running an extra echo;
- HOME outside the temporary variable;
- XDG_CONFIG_HOME at a sibling path;
- PATH with an empty or relative component;
- Node executing arbitrary-script.js;
- a sixth shell statement after the runner.

For every mutation, assert the result is nil or its decision is not decisionAllow.

- [ ] **Step 3: Verify RED**

~~~bash
cd hooks/bash-approve
go test ./... -run '^TestGuardedIsolatedTestHarness' -count=1
~~~

Expected: both positive cases fail because they currently return nil/no-opinion; mutation cases remain non-allow.

### Task 2: AST recognizer and existing-policy delegation

**Files:**
- Create: hooks/bash-approve/isolated_test_harness.go
- Modify: hooks/bash-approve/main.go
- Test: hooks/bash-approve/isolated_test_harness_test.go

**Interfaces:**
- Produces: evaluateGuardedTestHarness(stmts []*syntax.Stmt, ctx evalContext, wrapperPats, commandPats []pattern) (*result, bool).
- Consumes: evaluate(command string, ctx evalContext, wrapperPats, commandPats []pattern) *result and isSafeAbsolutePath.
- Internal helpers validate one boundary each: assignment, case, chmod, trap, env layout, PATH, and runner.

- [ ] **Step 1: Add the parsed-file hook**

After evaluate seeds ctx.wrapperPats and ctx.commandPats, before mergeStmtResults, add:

~~~go
if r, handled := evaluateGuardedTestHarness(file.Stmts, ctx, wrapperPats, commandPats); handled {
	return r
}
~~~

- [ ] **Step 2: Require exact five-statement structure**

Create isolated_test_harness.go with:

~~~go
func evaluateGuardedTestHarness(
	stmts []*syntax.Stmt,
	ctx evalContext,
	wrapperPats, commandPats []pattern,
) (*result, bool) {
	if len(stmts) != 5 {
		return nil, false
	}
	tempVar, ok := guardedTempAssignment(stmts[0])
	if !ok ||
		!guardedTempCase(stmts[1], tempVar) ||
		!guardedTempChmod(stmts[2], tempVar) ||
		!guardedTempTrap(stmts[3], tempVar) {
		return nil, false
	}
	r, ok := guardedTempEnvRunner(stmts[4], tempVar, ctx, wrapperPats, commandPats)
	if !ok || r == nil || r.decision != decisionAllow {
		return nil, false
	}
	return approved("isolated test harness"), true
}
~~~

Every statement rejects redirections, negation, background execution, extra assignments, dynamic command names, and unexpected arguments.

- [ ] **Step 3: Validate creation, guard, chmod, and cleanup**

Implement AST checks requiring:

- one standalone scalar assignment from one command substitution;
- mktemp arguments exactly -d -t and a literal filename template ending .XXXXXX;
- a case word exactly referencing the temp variable;
- one empty accept item whose pattern set is exactly /tmp/*, /private/tmp/*, /var/folders/*;
- one wildcard fallback containing only exit with a nonzero literal status;
- chmod 700 targeting exactly the temp variable;
- trap signal EXIT and a static payload parsed as either rm -rf -- the same variable, or status=$?; rm -rf -- the same variable; exit the same status variable.

Prove the trap AST relationship directly rather than using general rm approval, because the path is runtime-derived.

- [ ] **Step 4: Validate env isolation and PATH**

Parse the final call only when its command is env. Accept repeated -u NAME, --unset NAME, and --unset=NAME with literal valid names. Require exactly:

~~~text
HOME=$temp
XDG_CONFIG_HOME=$temp/config
XDG_DATA_HOME=$temp/data
XDG_CACHE_HOME=$temp/cache
XDG_STATE_HOME=$temp/state
~~~

Allow BUN_INSTALL_CACHE_DIR=$temp/bun-cache as the only optional scratch assignment. Require literal nonempty PATH whose every colon-separated component is accepted by isSafeAbsolutePath after adding a path separator.

- [ ] **Step 5: Delegate the final runner**

For a trusted absolute Node executable followed by a relative path ending node_modules/vite-plus/bin/vp, evaluate vp plus the remaining arguments. For other commands, evaluate the original remaining words so existing absolute-path and command rules apply.

Return handled only when delegation returns decisionAllow. Arbitrary Node scripts, unsafe executables, unknown commands, and disabled categories fall back.

- [ ] **Step 6: Verify GREEN and related policies**

~~~bash
cd hooks/bash-approve
go test ./... -run '^TestGuardedIsolatedTestHarness' -count=1
go test ./... -run 'TestEvaluate_Approved|TestEvaluate_Rejected|TestEvaluate_Chains|TestRecursiveRm|TestChmod' -count=1
~~~

Expected: all pass; both harnesses allow and every mutation stays non-allow.

- [ ] **Step 7: Commit implementation**

Use the mandatory commit skill, stage only main.go, isolated_test_harness.go, and isolated_test_harness_test.go, and commit as: Allow guarded isolated test harnesses.

### Task 3: Verification, PR, merge, and deployment

**Files:**
- Create: docs/superpowers/plans/2026-08-05-isolated-test-harness.md
- Verify: Task 2 files and approved spec.

**Interfaces:**
- Consumes: approve-bash Pi JSON stdin, GitHub checks, and install.py.
- Produces: merged main and installed-runtime evidence for telemetry 84211.

- [ ] **Step 1: Commit this plan**

Use the mandatory commit skill, stage only this plan, and commit as: Plan guarded test harness approval.

- [ ] **Step 2: Run full local verification**

~~~bash
cd hooks/bash-approve
go test ./... -shuffle=on -count=1
golangci-lint run ./...
~~~

Expected: both exit 0.

- [ ] **Step 3: Build and replay outside the repository**

Build approve-bash into a private temporary directory. With isolated XDG_STATE_HOME, replay telemetry 84211, guarded Bun, arbitrary Node mutation, unsafe PATH mutation, and recursive removal outside temp.

Expected: guarded harnesses allow; unsafe mutations remain no-opinion or deny.

- [ ] **Step 4: Push and create the PR**

Use commit-push-PR. Push codex/allow-isolated-test-harness and open against main with a rationale-first body. End agent-authored GitHub text with:

~~~html
<sup>generated by a clanker</sup>
~~~

- [ ] **Step 5: Wait for CI and merge**

Monitor required checks. Fix failures without deleting or resolving comments. Squash-merge only after CI passes.

- [ ] **Step 6: Install exact merged main**

Use isolate-production. Back up the installed runtime and modified global integration files to a private retained directory. Clone the exact merge commit into scratch and run:

~~~bash
python3 install.py install --target all
~~~

- [ ] **Step 7: Verify installed runtime**

Replay telemetry 84211 and unsafe mutations against installed approve-bash, report merge commit and SHA-256, remove only this task's deployment scratch, and retain the recovery backup.
