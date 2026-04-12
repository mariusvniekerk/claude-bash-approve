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

export async function adjudicateAndExecute<T>(input: {
  toolName: PiRuntimeInput["tool"];
  runtimeInput: PiRuntimeInput;
  ctx: ProtectedToolContext;
  config: PiBashApproveConfig;
  runtimePath: string;
  runRuntime(runtimePath: string, input: PiRuntimeInput, configPath?: string): Promise<PiRuntimeOutput>;
  builtInExecute(): Promise<T>;
}): Promise<T> {
  if (input.config.enabled === false) {
    return input.builtInExecute();
  }

  return runProtectedTool(async () => {
    const output = await input.runRuntime(input.runtimePath, input.runtimeInput, input.config.categoriesPath);
    const action = normalizeDecision(output, { hasUI: input.ctx.hasUI });
    if (action.kind === "block") {
      throw new PolicyBlockError("blocked by policy");
    }
    if (action.kind === "prompt") {
      if (!input.ctx.hasUI || !input.ctx.ui) {
        throw new PolicyBlockError("approval required but no UI is available");
      }
      const reason = output.kind === "decision" ? output.reason : output.error.message;
      const message = input.toolName === "bash"
        ? buildBashPrompt((input.runtimeInput as Extract<PiRuntimeInput, { tool: "bash" }>).command, input.ctx.cwd, reason)
        : buildPathPrompt(input.toolName, pathLabel(input.runtimeInput), input.ctx.cwd, reason);
      const title = input.toolName === "bash" ? "Allow bash command?" : "Allow out-of-bounds tool access?";
      const confirmed = await input.ctx.ui.confirm(title, message);
      if (!confirmed) {
        throw new PolicyBlockError("blocked by user");
      }
    }
    return input.builtInExecute();
  });
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
