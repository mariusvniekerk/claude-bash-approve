import { test, expect } from "bun:test";
import { createProtectedToolCallHandler } from "../src/tool-call-hook";

const baseCtx = {
  cwd: "/repo",
  hasUI: true,
  ui: {
    confirm: async () => true,
  },
};

test("ignores unprotected tools", async () => {
  const calls: string[] = [];
  const handler = createProtectedToolCallHandler(
    async () => {
      calls.push("resolve");
      return { config: {}, runtimePath: "/runtime" };
    },
    async () => {
      calls.push("runtime");
      return { version: 1, kind: "decision", tool: "bash", decision: "allow" };
    },
  );

  const result = await handler(
    { type: "tool_call", toolCallId: "1", toolName: "write", input: { path: "x", content: "y" } },
    baseCtx,
  );

  expect(result).toBeUndefined();
  expect(calls).toEqual([]);
});

test("bypasses protection when the no-bash-approve flag is active", async () => {
  const calls: string[] = [];
  const handler = createProtectedToolCallHandler(
    async () => {
      calls.push("resolve");
      return { config: {}, runtimePath: "/runtime" };
    },
    async () => {
      calls.push("runtime");
      return { version: 1, kind: "decision", tool: "bash", decision: "deny" };
    },
    {
      shouldBypass: () => true,
    },
  );

  const result = await handler(
    { type: "tool_call", toolCallId: "1", toolName: "bash", input: { command: "rm -rf ." } },
    baseCtx,
  );

  expect(result).toBeUndefined();
  expect(calls).toEqual([]);
});

test("bypasses runtime when config is disabled", async () => {
  const calls: string[] = [];
  const handler = createProtectedToolCallHandler(
    async () => {
      calls.push("resolve");
      return { config: { enabled: false }, runtimePath: undefined };
    },
    async () => {
      calls.push("runtime");
      return { version: 1, kind: "decision", tool: "read", decision: "deny" };
    },
  );

  const result = await handler(
    { type: "tool_call", toolCallId: "1", toolName: "read", input: { path: "../secret" } },
    baseCtx,
  );

  expect(result).toBeUndefined();
  expect(calls).toEqual(["resolve"]);
});

test("blocks protected tool calls when the runtime is unavailable", async () => {
  const handler = createProtectedToolCallHandler(async () => ({
    config: {},
    runtimePath: undefined,
  }));

  const result = await handler(
    { type: "tool_call", toolCallId: "1", toolName: "bash", input: { command: "git status" } },
    baseCtx,
  );

  expect(result).toEqual({
    block: true,
    reason: "protected runtime is unavailable",
  });
});

test("allows protected tool calls when the runtime returns allow", async () => {
  const handler = createProtectedToolCallHandler(
    async () => ({ config: {}, runtimePath: "/runtime" }),
    async () => ({ version: 1, kind: "decision", tool: "grep", decision: "allow" }),
  );

  const result = await handler(
    { type: "tool_call", toolCallId: "1", toolName: "grep", input: { pattern: "todo", path: "src" } },
    baseCtx,
  );

  expect(result).toBeUndefined();
});

test("prompts on noop and blocks when the user rejects", async () => {
  const prompts: string[] = [];
  const handler = createProtectedToolCallHandler(
    async () => ({ config: {}, runtimePath: "/runtime" }),
    async () => ({ version: 1, kind: "decision", tool: "read", decision: "noop", reason: "outside repo" }),
  );

  const result = await handler(
    { type: "tool_call", toolCallId: "1", toolName: "read", input: { path: "../secret" } },
    {
      ...baseCtx,
      ui: {
        confirm: async (title) => {
          prompts.push(title);
          return false;
        },
      },
    },
  );

  expect(prompts).toEqual(["Allow out-of-bounds tool access?"]);
  expect(result).toEqual({
    block: true,
    reason: "blocked by user",
  });
});

test("blocks noop decisions without UI", async () => {
  const handler = createProtectedToolCallHandler(
    async () => ({ config: {}, runtimePath: "/runtime" }),
    async () => ({ version: 1, kind: "decision", tool: "ls", decision: "noop", reason: "outside repo" }),
  );

  const result = await handler(
    { type: "tool_call", toolCallId: "1", toolName: "ls", input: { path: "../secret" } },
    {
      cwd: "/repo",
      hasUI: false,
    },
  );

  expect(result).toEqual({
    block: true,
    reason: "outside repo",
  });
});
