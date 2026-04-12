import {
  createBashTool,
  createFindTool,
  createGrepTool,
  createLsTool,
  createReadTool,
} from "@mariozechner/pi-coding-agent";
import { toRuntimeInput } from "../tool-inputs";
import { runRuntime } from "../runtime-client";
import { adjudicateAndExecute, type ProtectedToolContext } from "./shared";
import type { ExecutionResolver } from "../execution-resolver";
import type { PiRuntimeInput } from "../runtime-contract";

/**
 * All protected built-in tools follow the same lifecycle: resolve runtime/config for the current
 * cwd, ask the Go policy engine for a decision, then delegate execution back to pi's native tool.
 *
 * Keeping that sequence in one factory removes the per-tool boilerplate while still letting each
 * tool supply its own built-in factory and runtime-input mapping.
 */
function createProtectedTool<TParams>(input: {
  toolName: PiRuntimeInput["tool"];
  createTool(cwd: string): any;
  resolveExecution: ExecutionResolver;
  mapInput(params: TParams, ctx: ProtectedToolContext): PiRuntimeInput;
}) {
  const template = input.createTool(process.cwd());
  return {
    ...template,
    async execute(id: string, params: TParams, signal: AbortSignal | undefined, onUpdate: unknown, ctx: ProtectedToolContext) {
      const { runtimePath, config } = await input.resolveExecution(ctx);
      return adjudicateAndExecute({
        toolName: input.toolName,
        runtimeInput: input.mapInput(params, ctx),
        ctx,
        config,
        runtimePath,
        runRuntime,
        builtInExecute: async () => input.createTool(ctx.cwd).execute(id, params, signal, onUpdate, ctx),
      });
    },
  };
}

export function createProtectedBashTool(resolveExecution: ExecutionResolver) {
  return createProtectedTool({
    toolName: "bash",
    createTool: createBashTool,
    resolveExecution,
    mapInput: (params: { command: string }, ctx) => toRuntimeInput("bash", params as Record<string, unknown>, ctx.cwd),
  });
}

export function createProtectedReadTool(resolveExecution: ExecutionResolver) {
  return createProtectedTool({
    toolName: "read",
    createTool: createReadTool,
    resolveExecution,
    mapInput: (params: { path: string; offset?: number; limit?: number }, ctx) => toRuntimeInput("read", params as Record<string, unknown>, ctx.cwd),
  });
}

export function createProtectedGrepTool(resolveExecution: ExecutionResolver) {
  return createProtectedTool({
    toolName: "grep",
    createTool: createGrepTool,
    resolveExecution,
    mapInput: (params: { pattern: string; path?: string; paths?: string[] }, ctx) => toRuntimeInput("grep", params as Record<string, unknown>, ctx.cwd),
  });
}

export function createProtectedFindTool(resolveExecution: ExecutionResolver) {
  return createProtectedTool({
    toolName: "find",
    createTool: createFindTool,
    resolveExecution,
    mapInput: (params: { pattern: string; path?: string }, ctx) => toRuntimeInput("find", params as Record<string, unknown>, ctx.cwd),
  });
}

export function createProtectedLsTool(resolveExecution: ExecutionResolver) {
  return createProtectedTool({
    toolName: "ls",
    createTool: createLsTool,
    resolveExecution,
    mapInput: (params: { path?: string }, ctx) => toRuntimeInput("ls", params as Record<string, unknown>, ctx.cwd),
  });
}
