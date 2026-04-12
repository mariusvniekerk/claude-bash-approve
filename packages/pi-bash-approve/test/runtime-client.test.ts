import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { test, expect } from "bun:test";
import { chooseRuntimePath } from "../src/runtime-client";

test("prefers repo-local runtime when present", async () => {
  const tempRoot = await mkdtemp(path.join(os.tmpdir(), "pi-runtime-path-"));
  const packageDir = path.join(tempRoot, "packages", "pi-bash-approve");
  const repoLocalRuntimePath = path.join(tempRoot, "hooks", "bash-approve", "run-pi-runtime.sh");
  const bundledRuntimePath = path.join(packageDir, "runtime", "run-pi-runtime.sh");
  await mkdir(path.dirname(repoLocalRuntimePath), { recursive: true });
  await mkdir(path.dirname(bundledRuntimePath), { recursive: true });
  await writeFile(repoLocalRuntimePath, "", "utf8");
  await writeFile(bundledRuntimePath, "", "utf8");

  try {
    const chosen = chooseRuntimePath({
      packageDir,
      repoRoot: tempRoot,
      explicitRuntimePath: undefined,
      bundledRuntimePath,
      repoLocalRuntimePath,
    });
    expect(chosen).toBe(repoLocalRuntimePath);
  } finally {
    await rm(tempRoot, { recursive: true, force: true });
  }
});

test("falls back to bundled runtime when repo-local runtime is absent", async () => {
  const tempRoot = await mkdtemp(path.join(os.tmpdir(), "pi-runtime-path-"));
  const packageDir = path.join(tempRoot, "packages", "pi-bash-approve");
  const repoLocalRuntimePath = path.join(tempRoot, "hooks", "bash-approve", "run-pi-runtime.sh");
  const bundledRuntimePath = path.join(packageDir, "runtime", "run-pi-runtime.sh");

  try {
    expect(() => chooseRuntimePath({
      packageDir,
      repoRoot: tempRoot,
      explicitRuntimePath: undefined,
      bundledRuntimePath,
      repoLocalRuntimePath,
    })).toThrow(/runtime/i);
  } finally {
    await rm(tempRoot, { recursive: true, force: true });
  }
});
