import path from "node:path";
import type { PiBashApproveConfig } from "./config";
import { resolveConfig } from "./config";
import { chooseRuntimePath } from "./runtime-client";
import type { ProtectedToolContext } from "./overrides/shared";

export type ExecutionResolver = (ctx: ProtectedToolContext) => Promise<{
  runtimePath?: string;
  config: PiBashApproveConfig;
}>;

export function resolveExecutionForConfig(input: {
  packageDir: string;
  cwd: string;
  config: PiBashApproveConfig;
  chooseRuntimePathImpl?: typeof chooseRuntimePath;
}): {
  runtimePath?: string;
  config: PiBashApproveConfig;
} {
  if (input.config.enabled === false) {
    return { config: input.config, runtimePath: undefined };
  }

  const repoRoot = path.resolve(input.packageDir, "../..");
  const bundledRuntimePath = path.join(input.packageDir, "runtime", "run-pi-runtime.sh");
  const repoLocalRuntimePath = path.join(repoRoot, "hooks", "bash-approve", "run-pi-runtime.sh");
  const runtimePath = (input.chooseRuntimePathImpl ?? chooseRuntimePath)({
    packageDir: input.packageDir,
    repoRoot,
    explicitRuntimePath: input.config.runtimePath,
    bundledRuntimePath,
    repoLocalRuntimePath,
  });
  return { config: input.config, runtimePath };
}

/**
 * Build a per-execution resolver from the installed package location.
 *
 * Both the real extension entrypoint and the RPC test harness need the same "where is the runtime
 * for this installed package?" logic. Centralizing it avoids two copies of path-resolution code
 * that would otherwise drift independently.
 */
export function createExecutionResolver(packageDir: string): ExecutionResolver {
  return async (ctx: ProtectedToolContext) => {
    const config = await resolveConfig({ cwd: ctx.cwd });
    return resolveExecutionForConfig({
      packageDir,
      cwd: ctx.cwd,
      config,
    });
  };
}
