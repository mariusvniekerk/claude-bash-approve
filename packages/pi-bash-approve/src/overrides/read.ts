import { createReadTool } from "@mariozechner/pi-coding-agent";
import type { PiBashApproveConfig } from "../config";
import { adjudicateAndExecute, type ProtectedToolContext } from "./shared";
import { runRuntime } from "../runtime-client";

type Resolver = (ctx: ProtectedToolContext) => Promise<{ runtimePath: string; config: PiBashApproveConfig }>;

export function createProtectedReadTool(resolveExecution: Resolver) {
  const template = createReadTool(process.cwd());
  return {
    ...template,
    async execute(id: string, params: { path: string; offset?: number; limit?: number }, signal: AbortSignal | undefined, onUpdate: unknown, ctx: ProtectedToolContext) {
      const { runtimePath, config } = await resolveExecution(ctx);
      return adjudicateAndExecute({
        toolName: "read",
        runtimeInput: { tool: "read", path: params.path, cwd: ctx.cwd },
        ctx,
        config,
        runtimePath,
        runRuntime,
        builtInExecute: async () => createReadTool(ctx.cwd).execute(id, params, signal, onUpdate, ctx),
      });
    },
  };
}
