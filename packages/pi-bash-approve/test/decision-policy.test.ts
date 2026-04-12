import { test, expect } from "bun:test";
import { normalizeDecision } from "../src/decision-policy";

test("ask becomes prompt with UI", () => {
  expect(normalizeDecision({ version: 1, kind: "decision", tool: "bash", decision: "ask" }, { hasUI: true })).toEqual({ kind: "prompt" });
});

test("noop becomes block without UI", () => {
  expect(normalizeDecision({ version: 1, kind: "decision", tool: "read", decision: "noop" }, { hasUI: false })).toEqual({ kind: "block" });
});
