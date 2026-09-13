import { test, expect } from "bun:test";
import { adjudicateAndExecute } from "../src/overrides/shared";

test("delegates to built-in execute on allow", async () => {
  const calls: string[] = [];
  const result = await adjudicateAndExecute({
    toolName: "bash",
    runtimeInput: { tool: "bash", command: "git status", cwd: "/repo" },
    ctx: { cwd: "/repo", hasUI: true, ui: { confirm: async () => true } },
    config: {},
    runtimePath: "/runtime",
    runRuntime: async () => ({ version: 1, kind: "decision", tool: "bash", decision: "allow" }),
    builtInExecute: async () => {
      calls.push("executed");
      return { ok: true };
    },
  });
  expect(calls).toEqual(["executed"]);
  expect(result).toEqual({ ok: true });
});

test("executes noop with UI without prompting", async () => {
  const calls: string[] = [];
  const result = await adjudicateAndExecute({
    toolName: "read",
    runtimeInput: { tool: "read", path: "../fixture.txt", cwd: "/repo" },
    ctx: {
      cwd: "/repo",
      hasUI: true,
      ui: {
        confirm: async () => {
          calls.push("confirm");
          return false;
        },
      },
    },
    config: {},
    runtimePath: "/runtime",
    runRuntime: async () => ({
      version: 1,
      kind: "decision",
      tool: "read",
      decision: "noop",
      reason: "read",
    }),
    builtInExecute: async () => {
      calls.push("execute");
      return { ok: true };
    },
  });

  expect(calls).toEqual(["execute"]);
  expect(result).toEqual({ ok: true });
});

test("executes noop without UI", async () => {
  const calls: string[] = [];
  await adjudicateAndExecute({
    toolName: "bash",
    runtimeInput: { tool: "bash", command: "scripts/project-task --check", cwd: "/repo" },
    ctx: { cwd: "/repo", hasUI: false },
    config: {},
    runtimePath: "/runtime",
    runRuntime: async () => ({
      version: 1,
      kind: "decision",
      tool: "bash",
      decision: "noop",
    }),
    builtInExecute: async () => {
      calls.push("execute");
      return { ok: true };
    },
  });

  expect(calls).toEqual(["execute"]);
});

test("throws when an ask prompt is rejected", async () => {
  await expect(adjudicateAndExecute({
    toolName: "grep",
    runtimeInput: { tool: "grep", pattern: "x", path: "../fixture", cwd: "/repo" },
    ctx: { cwd: "/repo", hasUI: true, ui: { confirm: async () => false } },
    config: {},
    runtimePath: "/runtime",
    runRuntime: async () => ({
      version: 1,
      kind: "decision",
      tool: "grep",
      decision: "ask",
    }),
    builtInExecute: async () => ({ ok: true }),
  })).rejects.toThrow(/blocked by user/i);
});

test("does not execute after an explicit deny", async () => {
  const calls: string[] = [];
  await expect(adjudicateAndExecute({
    toolName: "bash",
    runtimeInput: { tool: "bash", command: "test-operation --blocked", cwd: "/repo" },
    ctx: { cwd: "/repo", hasUI: true, ui: { confirm: async () => true } },
    config: {},
    runtimePath: "/runtime",
    runRuntime: async () => ({
      version: 1,
      kind: "decision",
      tool: "bash",
      decision: "deny",
      reason: "blocked test operation",
    }),
    builtInExecute: async () => {
      calls.push("execute");
      return { ok: true };
    },
  })).rejects.toThrow(/blocked test operation/i);

  expect(calls).toEqual([]);
});

test("does not execute after a runtime error", async () => {
  const calls: string[] = [];
  await expect(adjudicateAndExecute({
    toolName: "read",
    runtimeInput: { tool: "read", path: "README.md", cwd: "/repo" },
    ctx: { cwd: "/repo", hasUI: false },
    config: {},
    runtimePath: "/runtime",
    runRuntime: async () => ({
      version: 1,
      kind: "error",
      error: { code: "internal-error", message: "runtime failed" },
    }),
    builtInExecute: async () => {
      calls.push("execute");
      return { ok: true };
    },
  })).rejects.toThrow(/runtime failed/i);

  expect(calls).toEqual([]);
});

test("bypasses runtime and prompting when config is disabled", async () => {
  const calls: string[] = [];
  const result = await adjudicateAndExecute({
    toolName: "bash",
    runtimeInput: { tool: "bash", command: "git tag v1.0.0", cwd: "/repo" },
    ctx: { cwd: "/repo", hasUI: true, ui: { confirm: async () => { calls.push("confirm"); return false; } } },
    config: { enabled: false },
    runtimePath: undefined,
    runRuntime: async () => {
      calls.push("runtime");
      return { version: 1, kind: "decision", tool: "bash", decision: "ask" };
    },
    builtInExecute: async () => {
      calls.push("execute");
      return { ok: true };
    },
  });
  expect(calls).toEqual(["execute"]);
  expect(result).toEqual({ ok: true });
});
