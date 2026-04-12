import path from "node:path";
import type { ExtensionAPI } from "@mariozechner/pi-coding-agent";
import { createProtectedBashTool } from "../src/overrides/bash";
import { createProtectedReadTool } from "../src/overrides/read";
import { createProtectedGrepTool } from "../src/overrides/grep";
import { createProtectedFindTool } from "../src/overrides/find";
import { createProtectedLsTool } from "../src/overrides/ls";
import { resolveConfig } from "../src/config";
import { chooseRuntimePath } from "../src/runtime-client";

export default function (pi: ExtensionAPI) {
  const packageDir = path.resolve(import.meta.dir, "..");
  const repoRoot = path.resolve(packageDir, "../..");
  const bundledRuntimePath = path.join(packageDir, "runtime", "run-pi-runtime.sh");
  const repoLocalRuntimePath = path.join(repoRoot, "hooks", "bash-approve", "run-hook.sh");

  const resolveExecution = async (ctx: { cwd: string }) => {
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

  pi.registerTool(createProtectedBashTool(resolveExecution));
  pi.registerTool(createProtectedReadTool(resolveExecution));
  pi.registerTool(createProtectedGrepTool(resolveExecution));
  pi.registerTool(createProtectedFindTool(resolveExecution));
  pi.registerTool(createProtectedLsTool(resolveExecution));
}
