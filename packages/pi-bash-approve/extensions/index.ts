import path from "node:path";
import { fileURLToPath } from "node:url";
import type { ExtensionAPI } from "@mariozechner/pi-coding-agent";
import { createDiscoveredSkillReadPolicy } from "../src/discovered-skill-reads";
import { createExecutionResolver } from "../src/execution-resolver";
import { createProtectedToolCallHandler } from "../src/tool-call-hook";

const extensionDir = path.dirname(fileURLToPath(import.meta.url));

/**
 * Register a pre-execution policy gate for pi's built-in file/bash tools.
 *
 * Runtime/config resolution intentionally happens per execution rather than once at startup so a
 * long-lived pi session can cross repo boundaries without carrying the previous project's policy
 * configuration forward.
 */
export default function (pi: ExtensionAPI) {
  const packageDir = path.resolve(extensionDir, "..");
  const resolveExecution = createExecutionResolver(packageDir);
  const discoveredSkillReads = createDiscoveredSkillReadPolicy();
  let bypassProtection = false;

  pi.registerFlag("no-bash-approve", {
    description: "Disable pi-bash-approve protection for this session",
    type: "boolean",
    default: false,
  });

  pi.on("session_start", () => {
    bypassProtection = pi.getFlag("no-bash-approve") === true;
    discoveredSkillReads.clear();
  });

  pi.on("before_agent_start", (event) => {
    discoveredSkillReads.refresh(
      event.systemPromptOptions.skills ?? [],
      event.systemPromptOptions.cwd,
    );
  });

  pi.on(
    "tool_call",
    createProtectedToolCallHandler(resolveExecution, undefined, {
      shouldBypass: () => bypassProtection,
      isDiscoveredSkillRead: (filePath, cwd) => discoveredSkillReads.allows(filePath, cwd),
    }),
  );
}
