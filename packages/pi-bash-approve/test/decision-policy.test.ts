import { test, expect } from "bun:test";
import { normalizeDecision } from "../src/decision-policy";

test("ask becomes prompt with UI", () => {
  expect(normalizeDecision(
    { version: 1, kind: "decision", tool: "bash", decision: "ask" },
    { hasUI: true },
  )).toEqual({ kind: "prompt" });
});

test("ask becomes block without UI", () => {
  expect(normalizeDecision(
    { version: 1, kind: "decision", tool: "bash", decision: "ask" },
    { hasUI: false },
  )).toEqual({ kind: "block" });
});

test("noop becomes execute with UI", () => {
  expect(normalizeDecision(
    { version: 1, kind: "decision", tool: "read", decision: "noop" },
    { hasUI: true },
  )).toEqual({ kind: "execute" });
});

test("noop becomes execute without UI", () => {
  expect(normalizeDecision(
    { version: 1, kind: "decision", tool: "read", decision: "noop" },
    { hasUI: false },
  )).toEqual({ kind: "execute" });
});

test("deny remains block", () => {
  expect(normalizeDecision(
    { version: 1, kind: "decision", tool: "bash", decision: "deny" },
    { hasUI: true },
  )).toEqual({ kind: "block" });
});

test("runtime errors remain block", () => {
  expect(normalizeDecision(
    {
      version: 1,
      kind: "error",
      error: { code: "internal-error", message: "runtime failed" },
    },
    { hasUI: true },
  )).toEqual({ kind: "block" });
});
