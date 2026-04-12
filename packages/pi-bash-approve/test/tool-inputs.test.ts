import { test, expect } from "bun:test";
import { toRuntimeInput } from "../src/tool-inputs";

test("maps grep path and paths to runtime input", () => {
  expect(toRuntimeInput("grep", { pattern: "foo", path: "src", paths: ["test"] }, "/repo")).toEqual({
    tool: "grep",
    pattern: "foo",
    path: "src",
    paths: ["test"],
    cwd: "/repo",
  });
});
