import { test, expect } from "bun:test";
import { parseRuntimeOutput } from "../src/runtime-parse";

test("parses valid allow decision", () => {
  expect(parseRuntimeOutput('{"version":1,"kind":"decision","tool":"bash","decision":"allow"}')).toEqual({
    version: 1,
    kind: "decision",
    tool: "bash",
    decision: "allow",
  });
});

test("rejects unknown decision", () => {
  expect(() => parseRuntimeOutput('{"version":1,"kind":"decision","tool":"bash","decision":"review"}')).toThrow(/unknown decision/i);
});
