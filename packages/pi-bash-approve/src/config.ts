import { access, readFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

/**
 * Configuration stays intentionally small because the Go runtime remains the policy authority.
 *
 * These values only control how the TypeScript adapter locates and invokes that runtime, plus
 * whether the adapter is active at all.
 */
export type PiBashApproveConfig = {
  enabled?: boolean;
  runtimePath?: string;
  categoriesPath?: string;
};

async function exists(filePath: string) {
  try {
    await access(filePath);
    return true;
  } catch {
    return false;
  }
}

async function readConfig(filePath: string): Promise<PiBashApproveConfig | undefined> {
  if (!(await exists(filePath))) return undefined;
  return JSON.parse(await readFile(filePath, "utf8")) as PiBashApproveConfig;
}

/**
 * Anchor config lookup to the repository root when one exists so all subdirectories in the same
 * repo/worktree see the same policy configuration.
 */
async function findRepoRoot(start: string): Promise<string | undefined> {
  let current = path.resolve(start);
  while (true) {
    if (await exists(path.join(current, ".git"))) return current;
    const parent = path.dirname(current);
    if (parent === current) return undefined;
    current = parent;
  }
}

/**
 * Non-repo directories still get project-local config, but the upward walk stops before `$HOME`
 * so `~/.pi/...` remains the only supported global location and accidental pseudo-global configs do
 * not shadow it.
 */
async function findProjectConfigOutsideRepo(start: string, homeDir?: string): Promise<string | undefined> {
  let current = path.resolve(start);
  const stopDir = homeDir ? path.resolve(homeDir) : undefined;
  while (true) {
    if (stopDir && current === stopDir) return undefined;
    const candidate = path.join(current, ".pi", "bash-approve.json");
    if (await exists(candidate)) return candidate;
    const parent = path.dirname(current);
    if (parent === current) return undefined;
    current = parent;
  }
}

/**
 * Resolve config against the effective project boundary for the current tool execution.
 *
 * Doing this per `ctx.cwd` prevents config bleed when a single pi session traverses multiple repos
 * or worktrees.
 */
export async function resolveConfig(input: { cwd: string; homeDir?: string }): Promise<PiBashApproveConfig> {
  const homeDir = input.homeDir ?? os.homedir();
  const repoRoot = await findRepoRoot(input.cwd);
  const projectConfigPath = repoRoot
    ? path.join(repoRoot, ".pi", "bash-approve.json")
    : await findProjectConfigOutsideRepo(input.cwd, homeDir);

  const projectConfig = projectConfigPath ? await readConfig(projectConfigPath) : undefined;
  if (projectConfig) {
    return {
      enabled: true,
      ...projectConfig,
    };
  }

  const globalConfig = await readConfig(path.join(homeDir, ".pi", "agent", "bash-approve.json"));
  return {
    enabled: true,
    ...globalConfig,
  };
}
