export type RunClassification =
  | "plugin-intercepted"
  | "native-ask"
  | "plugin-deferred"
  | "plugin-not-loaded"
  | "command-failed"

export type RunSummaryInput = {
  permissionAsked: number
  permissionReplied: number
  pluginReplied: boolean
  hooks: {
    toolExecuteBefore: boolean
  }
  commandCompleted: boolean
}

export function classifyRun(input: RunSummaryInput): RunClassification {
  if (!input.hooks.toolExecuteBefore) return "plugin-not-loaded"
  if (input.pluginReplied) return "plugin-intercepted"
  if (input.permissionAsked > 0) return "native-ask"
  if (!input.commandCompleted) return "command-failed"
  return "plugin-deferred"
}
