import { access, readFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

export type PiBashApproveConfig = {
  enabled?: boolean;
  runtimePath?: string;
  categoriesPath?: string;
  serializeProtectedToolExecutions?: boolean;
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

async function findRepoRoot(start: string): Promise<string | undefined> {
  let current = path.resolve(start);
  while (true) {
    if (await exists(path.join(current, ".git"))) return current;
    const parent = path.dirname(current);
    if (parent === current) return undefined;
    current = parent;
  }
}

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
      serializeProtectedToolExecutions: true,
      ...projectConfig,
    };
  }

  const globalConfig = await readConfig(path.join(homeDir, ".pi", "agent", "bash-approve.json"));
  return {
    enabled: true,
    serializeProtectedToolExecutions: true,
    ...globalConfig,
  };
}
