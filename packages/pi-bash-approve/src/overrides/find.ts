import { createFindTool } from "@mariozechner/pi-coding-agent";
import type { PiBashApproveConfig } from "../config";
import { adjudicateAndExecute, type ProtectedToolContext } from "./shared";
import { runRuntime } from "../runtime-client";

export function createProtectedFindTool(runtimePath: string, config: PiBashApproveConfig) {
  const template = createFindTool(process.cwd());
  return {
    ...template,
    async execute(id: string, params: { pattern: string; path?: string }, signal: AbortSignal | undefined, onUpdate: unknown, ctx: ProtectedToolContext) {
      return adjudicateAndExecute({
        toolName: "find",
        runtimeInput: { tool: "find", pattern: params.pattern, path: params.path, cwd: ctx.cwd },
        ctx,
        config,
        runtimePath,
        runRuntime,
        builtInExecute: async () => createFindTool(ctx.cwd).execute(id, params, signal, onUpdate, ctx),
      });
    },
  };
}
