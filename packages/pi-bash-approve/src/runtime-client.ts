import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import { RuntimeInvocationError } from "./errors";
import { parseRuntimeOutput } from "./runtime-parse";
import type { PiRuntimeInput, PiRuntimeOutput } from "./runtime-contract";

/**
 * Prefer the canonical runtime that ships with the installed repository checkout when it exists.
 *
 * The pi package is often loaded from a full git/local repo clone, so reaching over to
 * `hooks/bash-approve/` keeps the Go policy engine single-sourced instead of executing a stale
 * package-local copy. The bundled path remains as a fallback for future packaging modes that may
 * not carry the whole repository layout.
 */
export function chooseRuntimePath(input: {
  packageDir: string;
  repoRoot?: string | undefined;
  explicitRuntimePath?: string | undefined;
  bundledRuntimePath: string;
  repoLocalRuntimePath: string;
}) {
  if (input.explicitRuntimePath) return input.explicitRuntimePath;
  if (
    input.repoRoot &&
    path.resolve(input.packageDir) === path.join(path.resolve(input.repoRoot), "packages", "pi-bash-approve") &&
    existsSync(input.repoLocalRuntimePath)
  ) {
    return input.repoLocalRuntimePath;
  }
  if (existsSync(input.bundledRuntimePath)) {
    return input.bundledRuntimePath;
  }
  throw new RuntimeInvocationError("no pi runtime found for this package installation");
}

/**
 * Execute the Go runtime as an external policy authority and parse its strict JSON contract.
 *
 * We intentionally fail closed here: if the subprocess dies before producing parseable JSON, the
 * caller treats that as a policy failure instead of silently allowing the tool call.
 */
export async function runRuntime(runtimePath: string, input: PiRuntimeInput, configPath?: string): Promise<PiRuntimeOutput> {
  const args = configPath ? ["--config", configPath] : [];
  const payload = JSON.stringify(input);
  const child = spawn(runtimePath, args, { stdio: ["pipe", "pipe", "pipe"] });
  let stdout = "";
  let stderr = "";
  child.stdout.on("data", (chunk) => {
    stdout += String(chunk);
  });
  child.stderr.on("data", (chunk) => {
    stderr += String(chunk);
  });
  child.stdin.end(payload);
  const exitCode = await new Promise<number>((resolve, reject) => {
    child.on("error", reject);
    child.on("close", (code) => resolve(code ?? 1));
  });
  if (exitCode !== 0 && !stdout.trim()) {
    throw new RuntimeInvocationError(stderr.trim() || `runtime exited with code ${exitCode}`);
  }
  return parseRuntimeOutput(stdout.trim());
}
