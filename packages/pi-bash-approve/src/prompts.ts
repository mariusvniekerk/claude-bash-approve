/**
 * Keep prompt formatting stable so approval UX and RPC/e2e assertions are anchored to a single
 * canonical shape instead of each caller inventing its own wording.
 */
export function buildBashPrompt(command: string, cwd: string, reason?: string) {
  return ["Command:", command, "", "Working directory:", cwd, reason ? `\nReason:\n${reason}` : ""].join("\n");
}

/**
 * Use the same structure for path-oriented tools so prompts stay comparable across read/grep/find/ls
 * while still surfacing the exact target the Go runtime evaluated.
 */
export function buildPathPrompt(tool: string, target: string, cwd: string, reason?: string) {
  return ["Tool:", tool, "", "Target:", target, "", "Working directory:", cwd, reason ? `\nReason:\n${reason}` : ""].join("\n");
}
