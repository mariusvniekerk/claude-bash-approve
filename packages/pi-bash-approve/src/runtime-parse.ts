import type { PiPolicyDecision, PiRuntimeErrorCode, PiRuntimeOutput, SupportedPiTool } from "./runtime-contract";
import { RuntimeContractError } from "./errors";

const SUPPORTED_TOOLS = new Set<SupportedPiTool>(["bash", "read", "grep", "find", "ls"]);
const DECISIONS = new Set<PiPolicyDecision>(["allow", "deny", "ask", "noop"]);
const ERROR_CODES = new Set<PiRuntimeErrorCode>(["invalid-input", "unsupported-tool", "config-error", "internal-error"]);

/**
 * Reject contract drift aggressively so the TypeScript adapter never "best effort" interprets a
 * runtime response it does not actually understand.
 */
export function parseRuntimeOutput(stdout: string): PiRuntimeOutput {
  let value: unknown;
  try {
    value = JSON.parse(stdout);
  } catch {
    throw new RuntimeContractError("invalid runtime json");
  }

  if (!value || typeof value !== "object") throw new RuntimeContractError("runtime output must be an object");
  const record = value as Record<string, unknown>;
  if (record.version !== 1) throw new RuntimeContractError("unknown runtime output version");
  if (record.kind === "decision") {
    if (typeof record.tool !== "string" || !SUPPORTED_TOOLS.has(record.tool as SupportedPiTool)) {
      throw new RuntimeContractError("unknown tool in runtime output");
    }
    if (typeof record.decision !== "string" || !DECISIONS.has(record.decision as PiPolicyDecision)) {
      throw new RuntimeContractError("unknown decision in runtime output");
    }
    if (record.reason !== undefined && typeof record.reason !== "string") {
      throw new RuntimeContractError("reason must be a string");
    }
    return record as PiRuntimeOutput;
  }
  if (record.kind === "error") {
    if (!record.error || typeof record.error !== "object") {
      throw new RuntimeContractError("error payload missing");
    }
    const error = record.error as Record<string, unknown>;
    if (typeof error.code !== "string" || !ERROR_CODES.has(error.code as PiRuntimeErrorCode)) {
      throw new RuntimeContractError("unknown error code in runtime output");
    }
    if (typeof error.message !== "string") {
      throw new RuntimeContractError("error message must be a string");
    }
    return record as PiRuntimeOutput;
  }
  throw new RuntimeContractError("unknown runtime output kind");
}
