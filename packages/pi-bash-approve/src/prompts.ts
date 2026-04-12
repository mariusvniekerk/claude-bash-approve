export function buildBashPrompt(command: string, cwd: string, reason?: string) {
  return ["Command:", command, "", "Working directory:", cwd, reason ? `\nReason:\n${reason}` : ""].join("\n");
}

export function buildPathPrompt(tool: string, target: string, cwd: string, reason?: string) {
  return ["Tool:", tool, "", "Target:", target, "", "Working directory:", cwd, reason ? `\nReason:\n${reason}` : ""].join("\n");
}
