import path from "node:path";
import type { PiBashApproveConfig } from "./config";
import { resolveConfig } from "./config";
import { chooseRuntimePath } from "./runtime-client";
import type { ProtectedToolContext } from "./overrides/shared";

export type ExecutionResolver = (ctx: ProtectedToolContext) => Promise<{
  runtimePath: string;
  config: PiBashApproveConfig;
}>;

/**
 * Build a per-execution resolver from the installed package location.
 *
 * Both the real extension entrypoint and the RPC test harness need the same "where is the runtime
 * for this installed package?" logic. Centralizing it avoids two copies of path-resolution code
 * that would otherwise drift independently.
 */
export function createExecutionResolver(packageDir: string): ExecutionResolver {
  const repoRoot = path.resolve(packageDir, "../..");
  const bundledRuntimePath = path.join(packageDir, "runtime", "run-pi-runtime.sh");
  const repoLocalRuntimePath = path.join(repoRoot, "hooks", "bash-approve", "run-pi-runtime.sh");

  return async (ctx: ProtectedToolContext) => {
    const config = await resolveConfig({ cwd: ctx.cwd });
    const runtimePath = chooseRuntimePath({
      packageDir,
      repoRoot,
      explicitRuntimePath: config.runtimePath,
      bundledRuntimePath,
      repoLocalRuntimePath,
    });
    return { runtimePath, config };
  };
}
