import { test, expect } from "bun:test";
import { mkdtemp, mkdir, rm, symlink, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { createDiscoveredSkillReadPolicy } from "../src/discovered-skill-reads";

test("allows only visible discovered skill entry files and follows their install symlinks", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "pi-discovered-skill-read-"));
  const repoDir = path.join(root, "repo");
  const skillDir = path.join(root, "source", "example-skill");
  const hiddenSkillDir = path.join(root, "source", "hidden-skill");
  const unrelatedSkillDir = path.join(root, "source", "unrelated-skill");
  const installedDir = path.join(root, "installed-skill");
  const skillFile = path.join(skillDir, "SKILL.md");
  const hiddenSkillFile = path.join(hiddenSkillDir, "SKILL.md");
  const unrelatedSkillFile = path.join(unrelatedSkillDir, "SKILL.md");
  await mkdir(repoDir, { recursive: true });
  await mkdir(skillDir, { recursive: true });
  await mkdir(hiddenSkillDir, { recursive: true });
  await mkdir(unrelatedSkillDir, { recursive: true });
  await writeFile(skillFile, "skill instructions\n", "utf8");
  await writeFile(hiddenSkillFile, "hidden skill instructions\n", "utf8");
  await writeFile(unrelatedSkillFile, "unrelated skill instructions\n", "utf8");
  await symlink(skillDir, installedDir);

  try {
    const policy = createDiscoveredSkillReadPolicy();
    policy.refresh(
      [
        {
          filePath: path.join(installedDir, "SKILL.md"),
          disableModelInvocation: false,
        },
        {
          filePath: hiddenSkillFile,
          disableModelInvocation: true,
        },
      ],
      repoDir,
    );

    expect(policy.allows(path.join(installedDir, "SKILL.md"), repoDir)).toBe(true);
    expect(policy.allows(path.relative(repoDir, skillFile), repoDir)).toBe(true);
    expect(policy.allows(skillFile, repoDir)).toBe(true);
    expect(policy.allows(hiddenSkillFile, repoDir)).toBe(false);
    expect(policy.allows(unrelatedSkillFile, repoDir)).toBe(false);

    policy.refresh([], repoDir);
    expect(policy.allows(skillFile, repoDir)).toBe(false);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
