import type { PiRuntimeDecisionOutput, PiRuntimeOutput } from "./runtime-contract";

export type NormalizedDecision =
  | { kind: "execute" }
  | { kind: "prompt" }
  | { kind: "block" };

/**
 * Translate the Go runtime's hook-style decisions into concrete pi behavior.
 *
 * `noop` means "fall through" in Claude hook chains, but pi has no later approval hook to defer
 * to, so both `noop` and `ask` become an explicit prompt when UI exists and a block otherwise.
 */
export function normalizeDecision(output: PiRuntimeOutput | PiRuntimeDecisionOutput, options: { hasUI: boolean }): NormalizedDecision {
  if (output.kind === "error") return { kind: "block" };
  switch (output.decision) {
    case "allow":
      return { kind: "execute" };
    case "deny":
      return { kind: "block" };
    case "ask":
    case "noop":
      return options.hasUI ? { kind: "prompt" } : { kind: "block" };
  }
}
