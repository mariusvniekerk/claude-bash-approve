import path from "node:path"
import { spawn } from "node:child_process"
import process from "node:process"

import {
  createOpencodeClient,
  type Event,
  type EventMessagePartUpdated,
  type EventPermissionAsked,
  type EventPermissionReplied,
  type Part,
  type ToolPart,
} from "@opencode-ai/sdk/v2"

import {
  createPermissionTrackingState,
  isBashPermission,
  markTesterReplyFailed,
  markTesterReplyPending,
  markTesterReplySucceeded,
  recordBashPermissionAsked,
  recordPermissionReplied,
  shouldIgnoreReplyError,
} from "./permission-tracking.js"
import { classifyRun } from "./result.js"

type Reply = "once" | "always" | "reject"

const HOOK_TRACE_PREFIX = "[bash-approve-hook]"

function parseArgs(argv: string[]) {
  const options = {
    directory: path.resolve(process.cwd(), ".."),
    command: "git status --short --branch",
    reply: "once" as Reply,
    timeoutMs: 30_000,
    replyDelayMs: 750,
  }

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i]
    const next = argv[i + 1]
    if (arg === "--dir" && next) {
      options.directory = path.resolve(next)
      i += 1
      continue
    }
    if (arg === "--command" && next) {
      options.command = next
      i += 1
      continue
    }
    if (arg === "--reply" && next && ["once", "always", "reject"].includes(next)) {
      options.reply = next as Reply
      i += 1
      continue
    }
    if (arg === "--timeout-ms" && next && !Number.isNaN(Number(next))) {
      options.timeoutMs = Number(next)
      i += 1
      continue
    }
    if (arg === "--reply-delay-ms" && next && !Number.isNaN(Number(next))) {
      options.replyDelayMs = Number(next)
      i += 1
    }
  }

  return options
}

function matchServerUrl(line: string) {
  if (!line.startsWith("opencode server listening")) return undefined
  const match = line.match(/on\s+(https?:\/\/\S+)/)
  return match?.[1]
}

function createLineReader(onLine: (line: string) => void) {
  let buffer = ""
  return (chunk: Buffer | string) => {
    buffer += chunk.toString()
    const lines = buffer.split("\n")
    buffer = lines.pop() ?? ""
    for (const line of lines) onLine(line)
  }
}

async function startServer() {
  const args = ["serve", "--hostname=127.0.0.1", "--port=0", "--print-logs", "--log-level=DEBUG"]
  const proc = spawn("opencode", args, {
    env: {
      ...process.env,
      OPENCODE_CONFIG_CONTENT: JSON.stringify({ logLevel: "DEBUG" }),
    },
    stdio: ["ignore", "pipe", "pipe"],
  })

  const lines: string[] = []

  const url = await new Promise<string>((resolve, reject) => {
    const timeout = setTimeout(() => {
      proc.kill()
      reject(new Error("Timed out waiting for opencode server to start"))
    }, 15_000)

    const onLine = (line: string) => {
      lines.push(line)
      const url = matchServerUrl(line)
      if (!url) return
      clearTimeout(timeout)
      resolve(url)
    }

    const read = createLineReader(onLine)
    proc.stdout.on("data", read)
    proc.stderr.on("data", read)
    proc.on("error", (error) => {
      clearTimeout(timeout)
      reject(error)
    })
    proc.on("exit", (code) => {
      clearTimeout(timeout)
      reject(new Error(`opencode serve exited before startup with code ${code}`))
    })
  })

  return {
    url,
    lines,
    async close() {
      proc.kill()
    },
  }
}

function isBashPartEvent(event: Event): event is EventMessagePartUpdated {
  return event.type === "message.part.updated" && event.properties.part.type === "tool"
}

function isToolPart(part: Part): part is ToolPart {
  return part.type === "tool"
}

function isCompletedToolPart(part: ToolPart) {
  return part.state.status === "completed" || part.state.status === "error"
}

function observedHook(lines: string[], hookName: string) {
  return lines.some((line) => line.includes(`${HOOK_TRACE_PREFIX} ${hookName}`))
}

async function main() {
  const options = parseArgs(process.argv.slice(2))
  const pluginPath = path.join(options.directory, ".opencode", "plugins", "bash-approve.ts")
  const configPath = path.join(options.directory, "opencode.json")

  const server = await startServer()

  const client = createOpencodeClient({
    baseUrl: server.url,
    directory: options.directory,
  })

  const events = await client.event.subscribe()
  const permissionAsked: EventPermissionAsked[] = []
  const permissionReplied: EventPermissionReplied[] = []
  const bashParts: ToolPart[] = []
  const errors: string[] = []
  const permissionState = createPermissionTrackingState()
  const replyTimers = new Map<string, Timer>()

  let sessionID = ""
  let stage = "starting"

  const completion = new Promise<void>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`Timed out after ${options.timeoutMs}ms`)), options.timeoutMs)

    ;(async () => {
      try {
        for await (const event of events.stream) {
          const typed = event as Event
          if (typed.type === "permission.asked") {
            if (typed.properties.sessionID !== sessionID) continue
            if (!isBashPermission(typed.properties.permission)) continue
            stage = "permission-asked"
            console.error(`[tester] permission asked ${typed.properties.id}`)
            permissionAsked.push(typed)
            recordBashPermissionAsked(permissionState, typed.properties.id)

            const timer = setTimeout(async () => {
              if (permissionState.repliedRequestIDs.has(typed.properties.id)) return
              markTesterReplyPending(permissionState, typed.properties.id)

              try {
                await client.permission.reply({
                  requestID: typed.properties.id,
                  reply: options.reply,
                })
                markTesterReplySucceeded(permissionState, typed.properties.id)
              } catch (error) {
                markTesterReplyFailed(permissionState, typed.properties.id)
                if (shouldIgnoreReplyError(permissionState, typed.properties.id, error)) return
                errors.push(String(error))
              }
            }, options.replyDelayMs)

            replyTimers.set(typed.properties.id, timer)
            continue
          }

          if (typed.type === "permission.replied") {
            if (typed.properties.sessionID !== sessionID) continue
            if (!recordPermissionReplied(permissionState, typed.properties.requestID)) continue
            stage = "permission-replied"
            console.error(`[tester] permission replied ${typed.properties.requestID} ${typed.properties.reply}`)
            permissionReplied.push(typed)

            const timer = replyTimers.get(typed.properties.requestID)
            if (timer) {
              clearTimeout(timer)
              replyTimers.delete(typed.properties.requestID)
            }
            continue
          }

          if (typed.type === "session.error") {
            if (typed.properties.sessionID !== sessionID) continue
            stage = "session-error"
            const error = typed.properties.error
            if (error && "data" in error && error.data && "message" in error.data) {
              errors.push(String(error.data.message))
            } else {
              errors.push(String(error?.name ?? "unknown session error"))
            }
            continue
          }

          if (isBashPartEvent(typed)) {
            const part = typed.properties.part
            if (!isToolPart(part)) continue
            if (part.sessionID !== sessionID || part.tool !== "bash") continue
            if (!isCompletedToolPart(part)) continue
            stage = `bash-${part.state.status}`
            console.error(`[tester] bash tool ${part.state.status}`)
            bashParts.push(part)
            continue
          }

          if (typed.type === "session.status") {
            if (typed.properties.sessionID !== sessionID) continue
            if (typed.properties.status.type === "idle") {
              stage = "idle"
              console.error("[tester] session idle")
              clearTimeout(timer)
              resolve()
              return
            }
          }
        }
      } catch (error) {
        clearTimeout(timer)
        reject(error)
      }
    })().catch((error) => {
      clearTimeout(timer)
      reject(error)
    })
  })

  try {
    await Promise.race([
      (async () => {
        const created = await client.session.create({
          title: "OpenCode bash approve tester",
        })
        if (!created.data?.id) {
          throw new Error("Failed to create OpenCode session")
        }
        sessionID = created.data.id
        stage = "session-created"
        console.error(`[tester] session created ${sessionID}`)

        const promptRequest = client.session.prompt({
          sessionID,
          parts: [
            {
              type: "text",
              text: `Use the bash tool exactly once to run ${JSON.stringify(options.command)} in the current project. Do not use any other tools. Reply with only the raw command output.`,
            },
          ],
        })
        stage = "prompt-dispatched"
        console.error(`[tester] prompt dispatched ${options.command}`)

        await completion
        await promptRequest
      })(),
      new Promise((_, reject) => {
        setTimeout(() => reject(new Error(`tester timeout at stage=${stage}`)), options.timeoutMs)
      }),
    ])
  } catch (error) {
    errors.push(error instanceof Error ? error.message : String(error))
  } finally {
    for (const timer of replyTimers.values()) clearTimeout(timer)
    await server.close()
  }

  const latestBashPart = bashParts.at(-1)
  const commandCompleted = latestBashPart?.state.status === "completed"
  const commandError = latestBashPart?.state.status === "error" ? latestBashPart.state.error : undefined
  const commandOutput = latestBashPart?.state.status === "completed" ? latestBashPart.state.output : undefined
  const hooks = {
    toolExecuteBefore: observedHook(server.lines, "tool.execute.before"),
  }

  const summary = {
    directory: options.directory,
    command: options.command,
    pluginPath,
    configPath,
    sessionID,
    stage,
    permissionAsked,
    permissionReplied,
    hooks,
    pluginReplyRequestIDs: [...permissionState.pluginReplyRequestIDs],
    testerReplyRequestIDs: [...permissionState.testerReplyRequestIDs],
    bashParts,
    errors,
    commandCompleted,
    commandError,
    commandOutput,
    classification: classifyRun({
      permissionAsked: permissionAsked.length,
      permissionReplied: permissionReplied.length,
      pluginReplied: permissionState.pluginReplyRequestIDs.size > 0,
      hooks,
      commandCompleted,
    }),
  }

  process.stdout.write(JSON.stringify(summary, null, 2) + "\n")
  process.exit(errors.length > 0 ? 1 : 0)
}

await main()
