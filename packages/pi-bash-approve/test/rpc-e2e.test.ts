import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, mkdir, realpath, writeFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { test, expect } from "bun:test";

type RpcMessage = Record<string, unknown>;

test("real pi rpc emits approval confirm for protected out-of-bounds read", async () => {
  const piCommand = findPiCommand();
  if (!piCommand) {
    console.warn("Skipping rpc e2e test: pi binary is not available");
    return;
  }
  const tempRoot = await mkdtemp(path.join(os.tmpdir(), "pi-bash-approve-rpc-e2e-"));
  const repoDir = path.join(tempRoot, "repo");
  const outsideFile = path.join(tempRoot, "outside.txt");
  const agentDir = path.join(tempRoot, "agent-home");
  const projectConfigDir = path.join(repoDir, ".pi");
  await mkdir(repoDir, { recursive: true });
  await mkdir(agentDir, { recursive: true });
  await mkdir(projectConfigDir, { recursive: true });
  await writeFile(outsideFile, "top secret\n", "utf8");
  await writeFile(path.join(projectConfigDir, "bash-approve.json"), JSON.stringify({ enabled: true }), "utf8");
  await run("git", ["init", "-q"], repoDir);
  const effectiveRepoDir = await realpath(repoDir);

  const client = startRpcPi({
    piCommand,
    cwd: repoDir,
    agentDir,
    extensions: [
      path.resolve(import.meta.dir, "../extensions/index.ts"),
      path.resolve(import.meta.dir, "./fixtures/rpc-e2e-harness.ts"),
    ],
  });

  try {
    const response = await client.send({
      id: "prompt-1",
      type: "prompt",
      message: `/pi-bash-approve-read-e2e ${outsideFile}`,
    });
    expect(response).toMatchObject({
      id: "prompt-1",
      type: "response",
      command: "prompt",
      success: true,
    });

    const confirm = await client.waitFor(
      (message) => message.type === "extension_ui_request" && message.method === "confirm",
    );

    expect(confirm.title).toBe("Allow out-of-bounds tool access?");
    expect(String(confirm.message)).toContain("Tool:\nread");
    expect(String(confirm.message)).toContain(`Target:\n${outsideFile}`);
    expect(String(confirm.message)).toContain(`Working directory:\n${effectiveRepoDir}`);

    client.write({
      type: "extension_ui_response",
      id: String(confirm.id),
      confirmed: false,
    });

    const notify = await client.waitFor(
      (message) => message.type === "extension_ui_request" && message.method === "notify",
    );
    expect(String(notify.message)).toContain("blocked by user");
  } finally {
    await client.close();
    await rm(tempRoot, { recursive: true, force: true });
  }
});

test("real pi rpc emits approval confirm for protected bash ask command", async () => {
  const piCommand = findPiCommand();
  if (!piCommand) {
    console.warn("Skipping rpc e2e test: pi binary is not available");
    return;
  }
  const tempRoot = await mkdtemp(path.join(os.tmpdir(), "pi-bash-approve-rpc-e2e-bash-"));
  const repoDir = path.join(tempRoot, "repo");
  const agentDir = path.join(tempRoot, "agent-home");
  const projectConfigDir = path.join(repoDir, ".pi");
  await mkdir(repoDir, { recursive: true });
  await mkdir(agentDir, { recursive: true });
  await mkdir(projectConfigDir, { recursive: true });
  await writeFile(path.join(projectConfigDir, "bash-approve.json"), JSON.stringify({ enabled: true }), "utf8");
  await run("git", ["init", "-q"], repoDir);
  const effectiveRepoDir = await realpath(repoDir);

  const client = startRpcPi({
    piCommand,
    cwd: repoDir,
    agentDir,
    extensions: [
      path.resolve(import.meta.dir, "../extensions/index.ts"),
      path.resolve(import.meta.dir, "./fixtures/rpc-e2e-harness.ts"),
    ],
  });

  const command = "git tag v1.0.0";
  try {
    const response = await client.send({
      id: "prompt-bash-1",
      type: "prompt",
      message: `/pi-bash-approve-bash-e2e ${command}`,
    });
    expect(response).toMatchObject({
      id: "prompt-bash-1",
      type: "response",
      command: "prompt",
      success: true,
    });

    const confirm = await client.waitFor(
      (message) => message.type === "extension_ui_request" && message.method === "confirm",
    );

    expect(confirm.title).toBe("Allow bash command?");
    expect(String(confirm.message)).toContain(`Command:\n${command}`);
    expect(String(confirm.message)).toContain(`Working directory:\n${effectiveRepoDir}`);
    expect(String(confirm.message)).toContain("Reason:\ngit tag");

    client.write({
      type: "extension_ui_response",
      id: String(confirm.id),
      confirmed: false,
    });

    const notify = await client.waitFor(
      (message) => message.type === "extension_ui_request" && message.method === "notify",
    );
    expect(String(notify.message)).toContain("blocked by user");
  } finally {
    await client.close();
    await rm(tempRoot, { recursive: true, force: true });
  }
});

function startRpcPi(input: { piCommand: string; cwd: string; agentDir: string; extensions: string[] }) {
  const child = spawn(
    input.piCommand,
    [
      "--mode",
      "rpc",
      "--no-session",
      ...input.extensions.flatMap((extensionPath) => ["--extension", extensionPath]),
    ],
    {
      cwd: input.cwd,
      stdio: ["pipe", "pipe", "pipe"],
      env: {
        ...process.env,
        PI_CODING_AGENT_DIR: input.agentDir,
      },
    },
  );

  let stdoutBuffer = "";
  let stderrBuffer = "";
  const closePromise = once(child, "close");
  const seen: RpcMessage[] = [];
  const waiters: Array<{
    predicate: (message: RpcMessage) => boolean;
    resolve(message: RpcMessage): void;
    reject(error: Error): void;
  }> = [];

  child.stdout.on("data", (chunk) => {
    stdoutBuffer += String(chunk);
    while (true) {
      const newlineIndex = stdoutBuffer.indexOf("\n");
      if (newlineIndex === -1) break;
      const line = stdoutBuffer.slice(0, newlineIndex).replace(/\r$/, "");
      stdoutBuffer = stdoutBuffer.slice(newlineIndex + 1);
      if (!line.trim()) continue;
      const message = JSON.parse(line) as RpcMessage;
      seen.push(message);
      const waiterIndex = waiters.findIndex((waiter) => waiter.predicate(message));
      if (waiterIndex !== -1) {
        const [waiter] = waiters.splice(waiterIndex, 1);
        waiter.resolve(message);
      }
    }
  });

  child.stderr.on("data", (chunk) => {
    stderrBuffer += String(chunk);
  });

  child.on("close", (code) => {
    const error = new Error(`pi rpc exited with code ${code}: ${stderrBuffer || stdoutBuffer}`);
    while (waiters.length > 0) {
      const waiter = waiters.shift();
      waiter?.reject(error);
    }
  });

  return {
    async send(message: RpcMessage) {
      this.write(message);
      return this.waitFor((candidate) => candidate.type === "response" && candidate.id === message.id);
    },
    write(message: RpcMessage) {
      child.stdin.write(`${JSON.stringify(message)}\n`);
    },
    waitFor(predicate: (message: RpcMessage) => boolean, timeoutMs = 10_000): Promise<RpcMessage> {
      const existing = seen.find(predicate);
      if (existing) return Promise.resolve(existing);
      return new Promise<RpcMessage>((resolve, reject) => {
        const timer = setTimeout(() => {
          const index = waiters.indexOf(waiter);
          if (index !== -1) waiters.splice(index, 1);
          reject(new Error(`Timed out waiting for RPC message. stderr: ${stderrBuffer}`));
        }, timeoutMs);
        const waiter = {
          predicate,
          resolve(message: RpcMessage) {
            clearTimeout(timer);
            resolve(message);
          },
          reject(error: Error) {
            clearTimeout(timer);
            reject(error);
          },
        };
        waiters.push(waiter);
      });
    },
    async close() {
      if (child.exitCode === null && child.signalCode === null) {
        child.stdin.end();
        child.kill("SIGTERM");
      }
      await closePromise;
    },
  };
}

function findPiCommand(): string | undefined {
  if (process.env.PI_BIN) {
    return process.env.PI_BIN;
  }
  const probe = spawnSync("pi", ["--help"], { stdio: "ignore" });
  if (probe.error) {
    return undefined;
  }
  return "pi";
}

async function run(command: string, args: string[], cwd: string) {
  const child = spawn(command, args, { cwd, stdio: ["ignore", "pipe", "pipe"] });
  let stderr = "";
  child.stderr.on("data", (chunk) => {
    stderr += String(chunk);
  });
  const [code] = (await once(child, "close")) as [number | null];
  if ((code ?? 1) !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed: ${stderr}`);
  }
}
