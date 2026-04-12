import { access } from "node:fs/promises";
import path from "node:path";
import { test, expect } from "bun:test";
import { chooseRuntimePath } from "../src/runtime-client";

test("runtime shim exists in staged package runtime", async () => {
  const shim = path.resolve(import.meta.dir, "../runtime/run-pi-runtime.sh");
  await access(shim);
});

test("prefers repo-local runtime in source checkout", () => {
  const chosen = chooseRuntimePath({
    packageDir: "/repo/packages/pi-bash-approve",
    repoRoot: "/repo",
    explicitRuntimePath: undefined,
    bundledRuntimePath: "/repo/packages/pi-bash-approve/runtime/run-pi-runtime.sh",
    repoLocalRuntimePath: "/repo/hooks/bash-approve/run-pi-runtime.sh",
  });
  expect(chosen).toBe("/repo/hooks/bash-approve/run-pi-runtime.sh");
});
