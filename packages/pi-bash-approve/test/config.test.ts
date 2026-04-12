import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { test, expect } from "bun:test";
import { resolveConfig } from "../src/config";

test("repo-root config is discovered when cwd starts in a subdirectory", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "pi-bash-approve-config-"));
  await mkdir(path.join(root, ".git"));
  await mkdir(path.join(root, ".pi"));
  await mkdir(path.join(root, "sub", "dir"), { recursive: true });
  await writeFile(path.join(root, ".pi", "bash-approve.json"), JSON.stringify({ enabled: false }));

  const config = await resolveConfig({ cwd: path.join(root, "sub", "dir"), homeDir: root });
  expect(config.enabled).toBe(false);
});

test("global config is used when no project-local config exists", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "pi-bash-approve-config-"));
  const home = path.join(root, "home");
  await mkdir(path.join(home, ".pi", "agent"), { recursive: true });
  await writeFile(path.join(home, ".pi", "agent", "bash-approve.json"), JSON.stringify({ enabled: false }));

  const config = await resolveConfig({ cwd: root, homeDir: home });
  expect(config.enabled).toBe(false);
});

test("upward walk stops before ~/.pi pseudo-global path", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "pi-bash-approve-config-"));
  const home = path.join(root, "home");
  const cwd = path.join(home, "work", "nested");
  await mkdir(path.join(home, ".pi"), { recursive: true });
  await mkdir(path.join(home, ".pi", "agent"), { recursive: true });
  await mkdir(cwd, { recursive: true });
  await writeFile(path.join(home, ".pi", "bash-approve.json"), JSON.stringify({ enabled: false }));
  await writeFile(path.join(home, ".pi", "agent", "bash-approve.json"), JSON.stringify({ enabled: true }));

  const config = await resolveConfig({ cwd, homeDir: home });
  expect(config.enabled).toBe(true);
});
