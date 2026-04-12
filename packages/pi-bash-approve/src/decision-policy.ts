import type { PiRuntimeDecisionOutput, PiRuntimeOutput } from "./runtime-contract";

export type NormalizedDecision =
  | { kind: "execute" }
  | { kind: "prompt" }
  | { kind: "block" };

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
