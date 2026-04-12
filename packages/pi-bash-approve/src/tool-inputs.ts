import type { PiRuntimeInput } from "./runtime-contract";

/**
 * Normalize pi tool arguments into the narrower JSON contract expected by the Go runtime.
 *
 * This adapter intentionally performs only shallow coercion; semantic validation still belongs to
 * the runtime so policy decisions remain single-sourced in Go.
 */
export function toRuntimeInput(toolName: string, input: Record<string, unknown>, cwd: string): PiRuntimeInput {
  switch (toolName) {
    case "bash":
      return { tool: "bash", command: String(input.command ?? ""), cwd };
    case "read":
      return { tool: "read", path: String(input.path ?? ""), cwd };
    case "grep":
      return {
        tool: "grep",
        pattern: String(input.pattern ?? ""),
        path: typeof input.path === "string" ? input.path : undefined,
        paths: Array.isArray(input.paths) ? input.paths.map(String) : undefined,
        cwd,
      };
    case "find":
      return {
        tool: "find",
        pattern: String(input.pattern ?? ""),
        path: typeof input.path === "string" ? input.path : undefined,
        cwd,
      };
    case "ls":
      return {
        tool: "ls",
        path: typeof input.path === "string" ? input.path : undefined,
        cwd,
      };
    default:
      throw new Error(`unsupported tool: ${toolName}`);
  }
}
