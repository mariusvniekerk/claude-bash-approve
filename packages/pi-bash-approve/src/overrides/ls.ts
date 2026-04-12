import { createLsTool } from "@mariozechner/pi-coding-agent";
import type { PiBashApproveConfig } from "../config";
import { adjudicateAndExecute, type ProtectedToolContext } from "./shared";
import { runRuntime } from "../runtime-client";

export function createProtectedLsTool(runtimePath: string, config: PiBashApproveConfig) {
  const template = createLsTool(process.cwd());
  return {
    ...template,
    async execute(id: string, params: { path?: string }, signal: AbortSignal | undefined, onUpdate: unknown, ctx: ProtectedToolContext) {
      return adjudicateAndExecute({
        toolName: "ls",
        runtimeInput: { tool: "ls", path: params.path, cwd: ctx.cwd },
        ctx,
        config,
        runtimePath,
        runRuntime,
        builtInExecute: async () => createLsTool(ctx.cwd).execute(id, params, signal, onUpdate, ctx),
      });
    },
  };
}
