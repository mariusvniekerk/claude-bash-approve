import path from "node:path";
import { normalizeDecision } from "../decision-policy";
import { buildBashPrompt, buildPathPrompt } from "../prompts";
import { runProtectedTool } from "../protected-tool-queue";
import type { PiBashApproveConfig } from "../config";
import type { PiRuntimeInput, PiRuntimeOutput } from "../runtime-contract";
import { PolicyBlockError } from "../errors";

export type ProtectedToolContext = {
  cwd: string;
  hasUI: boolean;
  ui?: {
    confirm(title: string, message: string): Promise<boolean>;
  };
};

export type ProtectedToolAccessResult =
  | { kind: "execute" }
  | { kind: "block"; reason: string };

type ProtectedToolAccessInput = {
  runtimeInput: PiRuntimeInput;
  ctx: ProtectedToolContext;
  config: PiBashApproveConfig;
  runtimePath?: string;
  runRuntime(runtimePath: string, input: PiRuntimeInput, configPath?: string): Promise<PiRuntimeOutput>;
};

/**
 * Centralize the policy handoff so every protected built-in tool shares the same fail-closed rules.
 *
 * This is where pi-specific behavior diverges from the underlying Go runtime contract: we bypass the
 * adapter entirely when disabled, serialize active checks to keep approval UX sane, and convert
 * denied/rejected approvals into tool errors instead of synthetic successful tool results.
 */
export async function adjudicateAndExecute<T>(input: {
  toolName: PiRuntimeInput["tool"];
  runtimeInput: PiRuntimeInput;
  ctx: ProtectedToolContext;
  config: PiBashApproveConfig;
  runtimePath?: string;
  runRuntime(runtimePath: string, input: PiRuntimeInput, configPath?: string): Promise<PiRuntimeOutput>;
  builtInExecute(): Promise<T>;
}): Promise<T> {
  const decision = await adjudicateToolAccess(input);
  if (decision.kind === "block") {
    throw new PolicyBlockError(decision.reason);
  }
  return input.builtInExecute();
}

/**
 * Evaluate whether a protected tool call may proceed before any built-in execution happens.
 *
 * The override implementation uses this to decide whether to run the original tool, and the newer
 * `tool_call` hook path uses the same logic to block conflicting extensions without replacing pi's
 * built-in tools.
 */
export async function adjudicateToolAccess(input: ProtectedToolAccessInput): Promise<ProtectedToolAccessResult> {
  if (input.config.enabled === false) {
    return { kind: "execute" };
  }
  if (!input.runtimePath) {
    return { kind: "block", reason: "protected runtime is unavailable" };
  }
  const runtimePath = input.runtimePath;

  return runProtectedTool(async () => {
    const output = await input.runRuntime(runtimePath, input.runtimeInput, input.config.categoriesPath);
    const action = normalizeDecision(output, { hasUI: input.ctx.hasUI });
    if (action.kind === "block") {
      return { kind: "block", reason: decisionReason(output, "blocked by policy") ?? "blocked by policy" };
    }
    if (action.kind === "prompt") {
      if (!input.ctx.hasUI || !input.ctx.ui) {
        return { kind: "block", reason: "approval required but no UI is available" };
      }
      const reason = decisionReason(output);
      const prompt = buildApprovalPrompt(input.runtimeInput, input.ctx.cwd, reason);
      const confirmed = await input.ctx.ui.confirm(prompt.title, prompt.message);
      if (!confirmed) {
        return { kind: "block", reason: "blocked by user" };
      }
    }
    return { kind: "execute" };
  });
}

/**
 * Surface the most useful human-readable target in approval prompts rather than echoing raw JSON.
 */
function buildApprovalPrompt(input: PiRuntimeInput, cwd: string, reason?: string) {
  if (input.tool === "bash") {
    return {
      title: "Allow bash command?",
      message: buildBashPrompt(input.command, cwd, reason),
    };
  }

  return {
    title: "Allow out-of-bounds tool access?",
    message: buildPathPrompt(input.tool, pathLabel(input), cwd, reason),
  };
}

function decisionReason(output: PiRuntimeOutput, fallback?: string): string | undefined {
  if (output.kind === "decision") {
    return output.reason ?? fallback;
  }
  return output.error.message || fallback;
}

function pathLabel(input: PiRuntimeInput): string {
  switch (input.tool) {
    case "read":
      return input.path;
    case "grep":
      return input.path ?? input.paths?.join(",") ?? input.pattern;
    case "find":
      return input.path ?? ".";
    case "ls":
      return input.path ?? ".";
    default:
      return "";
  }
}
