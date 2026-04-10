# OpenCode Bash Plugin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the OpenCode bash approval flow around the maintained runtime's actual event path so bash approvals and rejections happen reliably, tester coverage reflects reality, and installation config is correct.

**Architecture:** Use `tool.execute.before` to capture and evaluate bash commands, then use `event(permission.asked)` as the sole authoritative interception point for replying `once` or `reject`. Update the tester to model that contract directly and fix the installer so it configures `.permission.bash["*"]` with `jq` and builds global runtime binaries with `-buildvcs=false`.

**Tech Stack:** TypeScript, Bun, OpenCode plugin runtime, Go hook binary, jq

---

### Task 1: Lock The Runtime Contract In Tests

**Files:**
- Modify: `opencode-tester/src/result.test.ts`
- Create: `opencode-tester/src/bash-plugin-runtime.test.ts`
- Test: `opencode-tester/src/result.test.ts`
- Test: `opencode-tester/src/bash-plugin-runtime.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
test("classifies plugin defer when bash permission is asked but plugin does not reply", () => {
  expect(
    classifyRun({
      hooks: { toolExecuteBefore: true },
      permissionAsked: 1,
      permissionReplied: 0,
      pluginReplied: false,
      commandCompleted: false,
    }),
  ).toBe("native-ask")
})

test("replies only to bash permission.asked events", async () => {
  const state = createPluginState()
  recordBashExecution(state, {
    sessionID: "session-1",
    callID: "call-1",
    command: "ls",
    cwd: "/repo",
    decision: "allow",
  })

  const result = await handlePermissionAsked(state, {
    id: "perm-1",
    sessionID: "session-1",
    permission: "edit",
  })

  expect(result).toBe("ignored")
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bun test opencode-tester/src/result.test.ts opencode-tester/src/bash-plugin-runtime.test.ts`
Expected: FAIL because current classifications and plugin state helpers still model the wrong runtime.

- [ ] **Step 3: Write minimal implementation**

```ts
export type PluginState = {
  executions: Map<string, { command: string; cwd: string; decision: "allow" | "deny" | "ask" }>
}

export function createPluginState(): PluginState {
  return { executions: new Map() }
}

export function executionKey(sessionID: string, callID: string) {
  return `${sessionID}:${callID}`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bun test opencode-tester/src/result.test.ts opencode-tester/src/bash-plugin-runtime.test.ts`
Expected: PASS for the new maintained-runtime expectations.

- [ ] **Step 5: Commit**

```bash
git add opencode-tester/src/result.test.ts opencode-tester/src/bash-plugin-runtime.test.ts
git commit -m "test: lock OpenCode bash runtime contract"
```

### Task 2: Rebuild The Plugin Around The Event Path

**Files:**
- Modify: `opencode/bash-approve.plugin.ts`
- Modify: `opencode-tester/src/index.ts`
- Test: `opencode-tester/src/bash-plugin-runtime.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
test("permission.asked with cached allow replies once", async () => {
  const replies: Array<{ requestID: string; reply: string }> = []
  const plugin = await BashApprovePlugin({
    client: { permission: { reply: async (input) => replies.push(input) } },
  } as any)

  await plugin["tool.execute.before"]?.(
    { tool: "bash", sessionID: "session-1", callID: "call-1" },
    { args: { command: "ls", workdir: "/repo" } },
  )

  await plugin.event?.({
    event: {
      type: "permission.asked",
      properties: { id: "perm-1", sessionID: "session-1", permission: "bash" },
    },
  })

  expect(replies).toEqual([{ requestID: "perm-1", reply: "once" }])
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bun test opencode-tester/src/bash-plugin-runtime.test.ts`
Expected: FAIL because the plugin still relies on the wrong permission hook assumptions.

- [ ] **Step 3: Write minimal implementation**

```ts
const pending = new Map<string, BashDecision>()

return {
  "tool.execute.before": async (input, output) => {
    if (input.tool !== "bash") return
    pending.set(`${input.sessionID}:${input.callID}`, await evaluate(output.args.command, output.args.workdir))
  },
  event: async ({ event }) => {
    if (event.type !== "permission.asked") return
    if (event.properties.permission !== "bash") return
    const decision = pending.get(`${event.properties.sessionID}:${latestCallIDForSession(event.properties.sessionID)}`)
    if (decision === "allow") await client.permission.reply({ requestID: event.properties.id, reply: "once" })
    if (decision === "deny") await client.permission.reply({ requestID: event.properties.id, reply: "reject" })
  },
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bun test opencode-tester/src/bash-plugin-runtime.test.ts`
Expected: PASS for allow, deny, ask, and non-bash cases on the event path.

- [ ] **Step 5: Commit**

```bash
git add opencode/bash-approve.plugin.ts opencode-tester/src/index.ts opencode-tester/src/bash-plugin-runtime.test.ts
git commit -m "fix: intercept bash permissions through runtime events"
```

### Task 3: Make Runtime State Matching Correct And Explicit

**Files:**
- Modify: `opencode/bash-approve.plugin.ts`
- Modify: `opencode-tester/src/index.ts`
- Modify: `opencode-tester/src/result.ts`
- Modify: `opencode-tester/src/result.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
test("plugin-not-loaded when tool.execute.before never fires", () => {
  expect(
    classifyRun({
      hooks: { toolExecuteBefore: false },
      permissionAsked: 0,
      permissionReplied: 0,
      pluginReplied: false,
      commandCompleted: false,
    }),
  ).toBe("plugin-not-loaded")
})

test("plugin reply races are treated as benign", () => {
  const err = new Error("permission already handled")
  expect(shouldIgnoreReplyError(err)).toBe(true)
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bun test opencode-tester/src/result.test.ts opencode-tester/src/bash-plugin-runtime.test.ts`
Expected: FAIL because current state matching and classification still blur together missing hooks, defer behavior, and reply races.

- [ ] **Step 3: Write minimal implementation**

```ts
export function classifyRun(input: RunSummaryInput): RunClassification {
  if (!input.hooks.toolExecuteBefore) return "plugin-not-loaded"
  if (input.pluginReplied) return "plugin-intercepted"
  if (input.permissionAsked > 0 && input.permissionReplied === 0) return "native-ask"
  if (!input.commandCompleted) return "command-failed"
  return "plugin-deferred"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bun test`
Expected: PASS with explicit maintained-runtime classifications and race handling.

- [ ] **Step 5: Commit**

```bash
git add opencode/bash-approve.plugin.ts opencode-tester/src/index.ts opencode-tester/src/result.ts opencode-tester/src/result.test.ts
git commit -m "fix: clarify bash plugin runtime outcomes"
```

### Task 4: Fix Installer Semantics

**Files:**
- Modify: `install-opencode.sh`
- Test: `install-opencode.sh`

- [ ] **Step 1: Write the failing test**

```bash
tmpdir="$(mktemp -d)"
cat >"$tmpdir/opencode.json" <<'EOF'
{
  "permission": {
    "bash": {
      "*": "deny",
      "ls *": "ask"
    }
  }
}
EOF

jq -e '.permission.bash["*"] == "ask"' "$tmpdir/opencode.json"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `./install-opencode.sh --project`
Expected: FAIL or mis-detect current config because grep does not inspect JSON structure correctly.

- [ ] **Step 3: Write minimal implementation**

```bash
if command -v jq >/dev/null 2>&1; then
  if jq -e '.permission.bash["*"] == "ask"' "$CONFIG_FILE" >/dev/null 2>&1; then
    echo "bash permission already configured"
  else
    tmp="$(mktemp)"
    jq '.permission = (.permission // {}) | .permission.bash = ((.permission.bash // {}) + {"*":"ask"})' "$CONFIG_FILE" >"$tmp"
    mv "$tmp" "$CONFIG_FILE"
  fi
else
  echo "jq is required to inspect or update $CONFIG_FILE" >&2
  exit 1
fi

go build -buildvcs=false -o "$TARGET" .
```

- [ ] **Step 4: Run test to verify it passes**

Run: `./install-opencode.sh --project && ./install-opencode.sh --both`
Expected: project and global installs succeed, and existing JSON is inspected structurally through `jq`.

- [ ] **Step 5: Commit**

```bash
git add install-opencode.sh
git commit -m "fix: use jq for OpenCode config installation"
```

### Task 5: Verify End-To-End Behavior

**Files:**
- Modify: `opencode-tester/src/index.ts`
- Test: `opencode-tester/src/bash-plugin-runtime.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
test("integration harness reports plugin-intercepted for deny path", async () => {
  const summary = await runScenario({ command: "rm -rf tmp", replyDelayMs: 500 })
  expect(summary.classification).toBe("plugin-intercepted")
  expect(summary.pluginReplyRequestIDs).toHaveLength(1)
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bun test opencode-tester/src/bash-plugin-runtime.test.ts`
Expected: FAIL until the harness and plugin agree on the actual event flow.

- [ ] **Step 3: Write minimal implementation**

```ts
const hooks = {
  toolExecuteBefore: observedHook(server.lines, "tool.execute.before"),
  permissionAsked: permissionAsked.length > 0,
}

return {
  ...summary,
  hooks,
  classification: classifyRun({
    hooks: { toolExecuteBefore: hooks.toolExecuteBefore },
    permissionAsked: permissionAsked.length,
    permissionReplied: permissionReplied.length,
    pluginReplied: pluginReplyRequestIDs.length > 0,
    commandCompleted,
  }),
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bun test && bun run typecheck && go test ./...`
Expected: all Bun tests pass, typecheck passes, and Go hook tests still pass.

- [ ] **Step 5: Commit**

```bash
git add opencode-tester/src/index.ts opencode-tester/src/bash-plugin-runtime.test.ts
git commit -m "test: verify OpenCode bash interception end to end"
```
