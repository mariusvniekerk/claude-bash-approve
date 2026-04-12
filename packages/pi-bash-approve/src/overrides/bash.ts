import { createBashTool } from "@mariozechner/pi-coding-agent";
import type { PiBashApproveConfig } from "../config";
import { adjudicateAndExecute, type ProtectedToolContext } from "./shared";
import { runRuntime } from "../runtime-client";

export function createProtectedBashTool(runtimePath: string, config: PiBashApproveConfig) {
  const template = createBashTool(process.cwd());
  return {
    ...template,
    async execute(id: string, params: { command: string }, signal: AbortSignal | undefined, onUpdate: unknown, ctx: ProtectedToolContext) {
      return adjudicateAndExecute({
        toolName: "bash",
        runtimeInput: { tool: "bash", command: params.command, cwd: ctx.cwd },
        ctx,
        config,
        runtimePath,
        runRuntime,
        builtInExecute: async () => createBashTool(ctx.cwd).execute(id, params, signal, onUpdate, ctx),
      });
    },
  };
}
