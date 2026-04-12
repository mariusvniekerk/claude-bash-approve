import { createGrepTool } from "@mariozechner/pi-coding-agent";
import type { PiBashApproveConfig } from "../config";
import { adjudicateAndExecute, type ProtectedToolContext } from "./shared";
import { runRuntime } from "../runtime-client";

type Resolver = (ctx: ProtectedToolContext) => Promise<{ runtimePath: string; config: PiBashApproveConfig }>;

export function createProtectedGrepTool(resolveExecution: Resolver) {
  const template = createGrepTool(process.cwd());
  return {
    ...template,
    async execute(id: string, params: { pattern: string; path?: string; paths?: string[] }, signal: AbortSignal | undefined, onUpdate: unknown, ctx: ProtectedToolContext) {
      const { runtimePath, config } = await resolveExecution(ctx);
      return adjudicateAndExecute({
        toolName: "grep",
        runtimeInput: { tool: "grep", pattern: params.pattern, path: params.path, paths: params.paths, cwd: ctx.cwd },
        ctx,
        config,
        runtimePath,
        runRuntime,
        builtInExecute: async () => createGrepTool(ctx.cwd).execute(id, params, signal, onUpdate, ctx),
      });
    },
  };
}
