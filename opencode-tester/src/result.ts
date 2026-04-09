export type RunClassification =
  | "plugin-intercepted"
  | "permission-hook-not-wired"
  | "permission-hook-missing"
  | "plugin-not-loaded"
  | "command-failed"

export type RunSummaryInput = {
  permissionAsked: number
  permissionReplied: number
  pluginReplied: boolean
  commandCompleted: boolean
}

export function classifyRun(input: RunSummaryInput): RunClassification {
  if (input.pluginReplied && input.commandCompleted) return "plugin-intercepted"
  if (input.permissionAsked > 0) return "permission-hook-not-wired"
  if (!input.commandCompleted) return "command-failed"
  return "plugin-intercepted"
}
