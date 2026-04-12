import { createGrepTool } from "@mariozechner/pi-coding-agent";
import type { PiBashApproveConfig } from "../config";
import { adjudicateAndExecute, type ProtectedToolContext } from "./shared";
import { runRuntime } from "../runtime-client";

export function createProtectedGrepTool(runtimePath: string, config: PiBashApproveConfig) {
  const template = createGrepTool(ctxCwdFallback());
  return {
    ...template,
    async execute(id: string, params: { pattern: string; path?: string; paths?: string[] }, signal: AbortSignal | undefined, onUpdate: unknown, ctx: ProtectedToolContext) {
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

function ctxCwdFallback() {
  return process.cwd();
}
