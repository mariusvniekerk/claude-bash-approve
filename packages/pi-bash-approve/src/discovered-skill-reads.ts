import { realpathSync } from "node:fs";
import path from "node:path";

export type DiscoveredSkill = {
  filePath: string;
  disableModelInvocation: boolean;
};

/**
 * Track the exact skill entry files Pi exposed to the model for the current agent run.
 *
 * Pi may discover a skill through a symlinked installation path. Canonicalizing both the loaded
 * metadata and the requested read keeps that entry readable through either the advertised path or
 * its resolved target without granting access to the rest of the skill directory.
 */
export function createDiscoveredSkillReadPolicy() {
  let allowedPaths = new Set<string>();

  return {
    refresh(skills: DiscoveredSkill[], cwd: string) {
      allowedPaths = new Set(
        skills
          .filter((skill) => !skill.disableModelInvocation)
          .map((skill) => canonicalPath(skill.filePath, cwd)),
      );
    },
    clear() {
      allowedPaths.clear();
    },
    allows(filePath: string, cwd: string) {
      return allowedPaths.has(canonicalPath(filePath, cwd));
    },
  };
}

function canonicalPath(filePath: string, cwd: string) {
  const absolutePath = path.resolve(cwd, filePath);
  try {
    return realpathSync(absolutePath);
  } catch {
    return absolutePath;
  }
}
