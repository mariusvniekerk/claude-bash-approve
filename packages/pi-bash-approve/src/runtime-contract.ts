export type SupportedPiTool = "bash" | "read" | "grep" | "find" | "ls";
export type PiPolicyDecision = "allow" | "deny" | "ask" | "noop";

export type PiRuntimeInput =
  | { tool: "bash"; command: string; cwd: string }
  | { tool: "read"; path: string; cwd: string }
  | { tool: "grep"; pattern: string; path?: string; paths?: string[]; cwd: string }
  | { tool: "find"; pattern: string; path?: string; cwd: string }
  | { tool: "ls"; path?: string; cwd: string };

export type PiRuntimeDecisionOutput = {
  version: 1;
  kind: "decision";
  tool: SupportedPiTool;
  decision: PiPolicyDecision;
  reason?: string | undefined;
};

export type PiRuntimeErrorCode =
  | "invalid-input"
  | "unsupported-tool"
  | "config-error"
  | "internal-error";

export type PiRuntimeErrorOutput = {
  version: 1;
  kind: "error";
  error: {
    code: PiRuntimeErrorCode;
    message: string;
  };
};

export type PiRuntimeOutput = PiRuntimeDecisionOutput | PiRuntimeErrorOutput;
