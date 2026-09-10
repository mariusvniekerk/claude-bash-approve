import type { ToolCallEvent, ToolCallEventResult } from "@mariozechner/pi-coding-agent";
import type { ExecutionResolver } from "./execution-resolver";
import type { SupportedPiTool } from "./runtime-contract";
import { runRuntime } from "./runtime-client";
import { toRuntimeInput } from "./tool-inputs";
import { adjudicateToolAccess, type ProtectedToolContext } from "./overrides/shared";

const protectedToolNames = new Set<SupportedPiTool>(["bash", "read", "grep", "find", "ls"]);

function isProtectedToolName(toolName: string): toolName is SupportedPiTool {
  return protectedToolNames.has(toolName as SupportedPiTool);
}

/**
 * Gate protected built-in tools before pi executes them so other extensions can still own
 * rendering or post-processing behavior.
 */
export function createProtectedToolCallHandler(
  resolveExecution: ExecutionResolver,
  runRuntimeImpl: typeof runRuntime = runRuntime,
  options?: {
    shouldBypass?: () => boolean;
    isDiscoveredSkillRead?: (filePath: string, cwd: string) => boolean;
  },
) {
  return async (
    event: ToolCallEvent,
    ctx: ProtectedToolContext,
  ): Promise<ToolCallEventResult | undefined> => {
    if (options?.shouldBypass?.()) {
      return undefined;
    }
    if (!isProtectedToolName(event.toolName)) {
      return undefined;
    }
    if (
      event.toolName === "read" &&
      typeof event.input.path === "string" &&
      options?.isDiscoveredSkillRead?.(event.input.path, ctx.cwd)
    ) {
      return undefined;
    }

    const { runtimePath, config } = await resolveExecution(ctx);
    const decision = await adjudicateToolAccess({
      runtimeInput: toRuntimeInput(event.toolName, event.input as Record<string, unknown>, ctx.cwd),
      ctx,
      config,
      runtimePath,
      runRuntime: runRuntimeImpl,
    });
    if (decision.kind === "block") {
      return { block: true, reason: decision.reason };
    }
    return undefined;
  };
}
