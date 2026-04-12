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

test("prompts on noop when UI is available", async () => {
  const prompts: string[] = [];
  await adjudicateAndExecute({
    toolName: "read",
    runtimeInput: { tool: "read", path: "../secret", cwd: "/repo" },
    ctx: { cwd: "/repo", hasUI: true, ui: { confirm: async (title) => { prompts.push(title); return true; } } },
    config: {},
    runtimePath: "/runtime",
    runRuntime: async () => ({ version: 1, kind: "decision", tool: "read", decision: "noop", reason: "read" }),
    builtInExecute: async () => ({ ok: true }),
  });
  expect(prompts).toEqual(["Allow out-of-bounds tool access?"]);
});

test("throws when prompt is rejected", async () => {
  await expect(adjudicateAndExecute({
    toolName: "grep",
    runtimeInput: { tool: "grep", pattern: "x", path: "../secret", cwd: "/repo" },
    ctx: { cwd: "/repo", hasUI: true, ui: { confirm: async () => false } },
    config: {},
    runtimePath: "/runtime",
    runRuntime: async () => ({ version: 1, kind: "decision", tool: "grep", decision: "noop" }),
    builtInExecute: async () => ({ ok: true }),
  })).rejects.toThrow(/blocked by user/i);
});
