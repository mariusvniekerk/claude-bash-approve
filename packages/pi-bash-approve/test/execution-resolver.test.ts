import { test, expect } from "bun:test";
import { resolveExecutionForConfig } from "../src/execution-resolver";

test("disabled config bypasses runtime lookup", () => {
  let chooserCalls = 0;
  const resolved = resolveExecutionForConfig({
    packageDir: "/repo/packages/pi-bash-approve",
    cwd: "/repo",
    config: { enabled: false },
    chooseRuntimePathImpl: () => {
      chooserCalls++;
      return "/repo/hooks/bash-approve/run-pi-runtime.sh";
    },
  });

  expect(chooserCalls).toBe(0);
  expect(resolved).toEqual({
    config: { enabled: false },
    runtimePath: undefined,
  });
});
