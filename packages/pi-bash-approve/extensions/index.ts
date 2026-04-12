import path from "node:path";
import { fileURLToPath } from "node:url";
import type { ExtensionAPI } from "@mariozechner/pi-coding-agent";
import { createExecutionResolver } from "../src/execution-resolver";
import {
  createProtectedBashTool,
  createProtectedFindTool,
  createProtectedGrepTool,
  createProtectedLsTool,
  createProtectedReadTool,
} from "../src/overrides/tools";

const extensionDir = path.dirname(fileURLToPath(import.meta.url));

/**
 * Register protected replacements for pi's built-in file/bash tools.
 *
 * Runtime/config resolution intentionally happens per execution rather than once at startup so a
 * long-lived pi session can cross repo boundaries without carrying the previous project's policy
 * configuration forward.
 */
export default function (pi: ExtensionAPI) {
  const packageDir = path.resolve(extensionDir, "..");
  const resolveExecution = createExecutionResolver(packageDir);

  pi.registerTool(createProtectedBashTool(resolveExecution));
  pi.registerTool(createProtectedReadTool(resolveExecution));
  pi.registerTool(createProtectedGrepTool(resolveExecution));
  pi.registerTool(createProtectedFindTool(resolveExecution));
  pi.registerTool(createProtectedLsTool(resolveExecution));
}
