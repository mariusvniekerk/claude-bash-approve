import type { PiRuntimeDecisionOutput, PiRuntimeOutput } from "./runtime-contract";

export type NormalizedDecision =
  | { kind: "execute" }
  | { kind: "prompt" }
  | { kind: "block" };

/**
 * Translate the Go runtime's hook-style decisions into concrete pi behavior.
 *
 * `noop` means the runtime has no objection, so pi executes without prompting. An explicit
 * `ask` still requires UI confirmation and blocks when no UI is available.
 */
export function normalizeDecision(output: PiRuntimeOutput | PiRuntimeDecisionOutput, options: { hasUI: boolean }): NormalizedDecision {
  if (output.kind === "error") return { kind: "block" };
  switch (output.decision) {
    case "allow":
    case "noop":
      return { kind: "execute" };
    case "deny":
      return { kind: "block" };
    case "ask":
      return options.hasUI ? { kind: "prompt" } : { kind: "block" };
  }
}
