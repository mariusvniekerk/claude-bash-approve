import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
import {
  chmod,
  mkdtemp,
  mkdir,
  readFile,
  realpath,
  rm,
  writeFile,
} from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { test, expect } from "bun:test";

type RpcMessage = Record<string, unknown>;

test("real pi rpc preserves noop, deny, and ask boundaries", async () => {
  const piCommand = findPiCommand();
  if (!piCommand) {
    console.warn("Skipping rpc e2e test: pi binary is not available");
    return;
  }

  const tempRoot = await mkdtemp(path.join(os.tmpdir(), "pi-bash-approve-rpc-e2e-"));
  const repoDir = path.join(tempRoot, "repo");
  const scriptsDir = path.join(repoDir, "scripts");
  const outsideFile = path.join(tempRoot, "outside.txt");
  const interactiveSentinel = path.join(tempRoot, "noop-interactive.txt");
  const headlessSentinel = path.join(tempRoot, "noop-headless.txt");
  const denySentinel = path.join(tempRoot, "deny-executed.txt");
  const agentDir = path.join(tempRoot, "agent-home");
  const stateDir = path.join(agentDir, "state");
  const fakeBinDir = path.join(tempRoot, "fake-bin");
  const runtimeDir = path.join(tempRoot, "runtime");
  const runtimeBinary = path.join(runtimeDir, "approve-bash");
  const runtimeShim = path.join(runtimeDir, "run-pi-runtime.sh");
  const gitConfig = path.join(tempRoot, "gitconfig");
  const projectConfigDir = path.join(repoDir, ".pi");
  const packageRoot = path.resolve(import.meta.dir, "../../..");
  const runtimeSourceDir = path.join(packageRoot, "hooks", "bash-approve");

  await mkdir(scriptsDir, { recursive: true });
  await mkdir(agentDir, { recursive: true });
  await mkdir(stateDir, { recursive: true });
  await mkdir(fakeBinDir, { recursive: true });
  await mkdir(runtimeDir, { recursive: true });
  await mkdir(projectConfigDir, { recursive: true });
  await writeFile(gitConfig, "", "utf8");
  await writeFile(outsideFile, "outside fixture\n", "utf8");
  await writeFile(
    path.join(scriptsDir, "project-task"),
    "#!/bin/sh\nprintf 'executed\\n' > \"$1\"\n",
    "utf8",
  );
  await chmod(path.join(scriptsDir, "project-task"), 0o755);
  await writeFile(
    path.join(fakeBinDir, "go"),
    "#!/bin/sh\nprintf 'executed\\n' > \"$DENY_SENTINEL\"\n",
    "utf8",
  );
  await chmod(path.join(fakeBinDir, "go"), 0o755);

  const gitEnvironment = isolatedEnvironment({
    GIT_CONFIG_GLOBAL: gitConfig,
    GIT_CONFIG_NOSYSTEM: "1",
  });
  await run(
    "go",
    ["build", "-buildvcs=false", "-o", runtimeBinary, "."],
    runtimeSourceDir,
    gitEnvironment,
  );
  await writeFile(
    runtimeShim,
    `#!/bin/sh\nexec ${JSON.stringify(runtimeBinary)} --pi "$@"\n`,
    "utf8",
  );
  await chmod(runtimeShim, 0o755);
  await run("git", ["init", "-q"], repoDir, gitEnvironment);
  await writeFile(
    path.join(projectConfigDir, "bash-approve.json"),
    JSON.stringify({ enabled: true, runtimePath: runtimeShim }),
    "utf8",
  );
  const effectiveRepoDir = await realpath(repoDir);

  const client = startRpcPi({
    piCommand,
    cwd: repoDir,
    agentDir,
    stateDir,
    gitConfig,
    extensions: [
      path.resolve(import.meta.dir, "../extensions/index.ts"),
      path.resolve(import.meta.dir, "./fixtures/rpc-e2e-harness.ts"),
    ],
    env: {
      DENY_SENTINEL: denySentinel,
      PATH: `${fakeBinDir}${path.delimiter}${process.env.PATH ?? ""}`,
    },
  });

  try {
    const interactiveCommand = `scripts/project-task ${JSON.stringify(interactiveSentinel)}`;
    const interactiveResponse = await client.send({
      id: "noop-interactive",
      type: "prompt",
      message: `/pi-bash-approve-bash-e2e ${interactiveCommand}`,
    });
    expect(interactiveResponse).toMatchObject({
      id: "noop-interactive",
      type: "response",
      command: "prompt",
      success: true,
    });
    await client.waitFor((message) => (
      message.type === "extension_ui_request"
      && message.method === "notify"
      && String(message.message).includes(interactiveCommand)
    ));
    expect(await readFile(interactiveSentinel, "utf8")).toBe("executed\n");
    expect(client.confirmRequests()).toHaveLength(0);

    const headlessCommand = `scripts/project-task ${JSON.stringify(headlessSentinel)}`;
    await client.send({
      id: "noop-headless",
      type: "prompt",
      message: `/pi-bash-approve-bash-headless-e2e ${headlessCommand}`,
    });
    await client.waitFor((message) => (
      message.type === "extension_ui_request"
      && message.method === "notify"
      && String(message.message).includes(headlessCommand)
    ));
    expect(await readFile(headlessSentinel, "utf8")).toBe("executed\n");
    expect(client.confirmRequests()).toHaveLength(0);

    await client.send({
      id: "noop-read",
      type: "prompt",
      message: `/pi-bash-approve-read-e2e ${outsideFile}`,
    });
    const readNotify = await client.waitFor((message) => (
      message.type === "extension_ui_request"
      && message.method === "notify"
      && String(message.message).includes("read succeeded: outside fixture")
    ));
    expect(String(readNotify.message)).toContain("outside fixture");
    expect(client.confirmRequests()).toHaveLength(0);

    await client.send({
      id: "deny",
      type: "prompt",
      message: "/pi-bash-approve-bash-e2e go mod vendor",
    });
    const denyNotify = await client.waitFor((message) => (
      message.type === "extension_ui_request"
      && message.method === "notify"
      && String(message.message).includes("go mod vendor is banned")
    ));
    expect(String(denyNotify.message)).toContain("go mod vendor is banned");
    await expect(readFile(denySentinel, "utf8")).rejects.toThrow();
    expect(client.confirmRequests()).toHaveLength(0);

    const askCommand = "git tag v1.0.0";
    const askResponsePromise = client.send({
      id: "ask",
      type: "prompt",
      message: `/pi-bash-approve-bash-e2e ${askCommand}`,
    });
    const confirm = await client.waitFor(
      (message) => message.type === "extension_ui_request" && message.method === "confirm",
    );
    expect(confirm.title).toBe("Allow bash command?");
    expect(String(confirm.message)).toContain(`Command:\n${askCommand}`);
    expect(String(confirm.message)).toContain(`Working directory:\n${effectiveRepoDir}`);
    expect(String(confirm.message)).toContain("Reason:\ngit tag");

    client.write({
      type: "extension_ui_response",
      id: String(confirm.id),
      confirmed: false,
    });
    const askNotify = await client.waitFor((message) => (
      message.type === "extension_ui_request"
      && message.method === "notify"
      && String(message.message).includes("blocked by user")
    ));
    await askResponsePromise;
    expect(String(askNotify.message)).toContain("blocked by user");
    expect(client.confirmRequests()).toHaveLength(1);
  } finally {
    await client.close();
    await rm(tempRoot, { recursive: true, force: true });
  }
}, 60_000);

function startRpcPi(input: {
  piCommand: string;
  cwd: string;
  agentDir: string;
  stateDir: string;
  gitConfig: string;
  extensions: string[];
  env: NodeJS.ProcessEnv;
}) {
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
      env: isolatedEnvironment({
        ...input.env,
        PI_CODING_AGENT_DIR: input.agentDir,
        XDG_STATE_HOME: input.stateDir,
        GIT_CONFIG_GLOBAL: input.gitConfig,
        GIT_CONFIG_NOSYSTEM: "1",
      }),
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
          reject(new Error(`Timed out waiting for RPC message. stderr: ${stderrBuffer}; seen: ${JSON.stringify(seen)}`));
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
    confirmRequests() {
      return seen.filter((message) => (
        message.type === "extension_ui_request" && message.method === "confirm"
      ));
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

function isolatedEnvironment(overrides: NodeJS.ProcessEnv = {}) {
  const environment = { ...process.env };
  for (const name of Object.keys(environment)) {
    if (name.startsWith("GIT_")) {
      delete environment[name];
    }
  }
  return { ...environment, ...overrides };
}

function findPiCommand() {
  if (process.env.PI_BIN) {
    return process.env.PI_BIN;
  }
  const probe = spawnSync("pi", ["--help"], { stdio: "ignore" });
  if (probe.error) {
    return undefined;
  }
  return "pi";
}

async function run(
  command: string,
  args: string[],
  cwd: string,
  env: NodeJS.ProcessEnv,
) {
  const child = spawn(command, args, {
    cwd,
    env,
    stdio: ["ignore", "pipe", "pipe"],
  });
  let stderr = "";
  child.stderr.on("data", (chunk) => {
    stderr += String(chunk);
  });
  const [code] = (await once(child, "close")) as [number | null];
  if ((code ?? 1) !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed: ${stderr}`);
  }
}
