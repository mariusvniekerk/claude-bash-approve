export type RunClassification =
  | "plugin-intercepted"
  | "permission-hook-not-wired"
  | "permission-hook-missing"
  | "plugin-not-loaded"
  | "command-failed"

export type RunSummaryInput = {
  permissionAsked: number
  permissionReplied: number
  hooks: {
    toolExecuteBefore: boolean
    permissionAsk: boolean
  }
  commandCompleted: boolean
}

export function classifyRun(input: RunSummaryInput): RunClassification {
  if (!input.hooks.toolExecuteBefore) return "plugin-not-loaded"
  if (!input.hooks.permissionAsk) return "permission-hook-missing"
  if (input.permissionAsked > 0) return "permission-hook-not-wired"
  if (!input.commandCompleted) return "command-failed"
  return "plugin-intercepted"
}
