import path from "node:path"
import process from "node:process"

import {
  createOpencodeClient,
  createOpencodeServer,
  type Event,
  type EventMessagePartUpdated,
  type EventPermissionAsked,
  type EventPermissionReplied,
  type ToolPart,
} from "@opencode-ai/sdk/v2"

import { classifyRun } from "./result"

type Reply = "once" | "always" | "reject"

function parseArgs(argv: string[]) {
  const options = {
    directory: path.resolve(process.cwd(), ".."),
    command: "git status --short --branch",
    reply: "once" as Reply,
    timeoutMs: 120_000,
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
    }
  }

  return options
}

function isBashPartEvent(event: Event): event is EventMessagePartUpdated {
  return event.type === "message.part.updated" && event.properties.part.type === "tool"
}

function isCompletedToolPart(part: ToolPart) {
  return part.state.status === "completed" || part.state.status === "error"
}

async function main() {
  const options = parseArgs(process.argv.slice(2))
  const pluginPath = path.join(options.directory, ".opencode", "plugins", "bash-approve.js")
  const configPath = path.join(options.directory, "opencode.json")

  const server = await createOpencodeServer({
    port: 0,
    timeout: 15_000,
    config: {
      logLevel: "DEBUG",
    },
  })

  const client = createOpencodeClient({
    baseUrl: server.url,
    directory: options.directory,
  })

  const eventAbort = new AbortController()
  const events = await client.event.subscribe({ signal: eventAbort.signal })
  const permissionAsked: EventPermissionAsked[] = []
  const permissionReplied: EventPermissionReplied[] = []
  const bashParts: ToolPart[] = []
  const errors: string[] = []

  let sessionID = ""

  const completion = new Promise<void>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`Timed out after ${options.timeoutMs}ms`)), options.timeoutMs)

    ;(async () => {
      try {
        for await (const event of events.stream) {
          const typed = event as Event
          if (typed.type === "permission.asked") {
            if (typed.properties.sessionID !== sessionID) continue
            permissionAsked.push(typed)
            await client.permission.reply({
              requestID: typed.properties.id,
              reply: options.reply,
            })
            continue
          }

          if (typed.type === "permission.replied") {
            if (typed.properties.sessionID !== sessionID) continue
            permissionReplied.push(typed)
            continue
          }

          if (typed.type === "session.error") {
            if (typed.properties.sessionID !== sessionID) continue
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
            if (part.sessionID !== sessionID || part.tool !== "bash") continue
            if (!isCompletedToolPart(part)) continue
            bashParts.push(part)
            continue
          }

          if (typed.type === "session.status") {
            if (typed.properties.sessionID !== sessionID) continue
            if (typed.properties.status.type === "idle") {
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
    const created = await client.session.create({
      title: "OpenCode bash approve tester",
    })
    if (!created.data?.id) {
      throw new Error("Failed to create OpenCode session")
    }
    sessionID = created.data.id

    await client.session.prompt({
      sessionID,
      parts: [
        {
          type: "text",
          text: `Use the bash tool exactly once to run ${JSON.stringify(options.command)} in the current project. Do not use any other tools. Reply with only the raw command output.`,
        },
      ],
    })

    await completion
  } finally {
    eventAbort.abort()
    server.close()
  }

  const latestBashPart = bashParts.at(-1)
  const commandCompleted = latestBashPart?.state.status === "completed"
  const commandError = latestBashPart?.state.status === "error" ? latestBashPart.state.error : undefined
  const commandOutput = latestBashPart?.state.status === "completed" ? latestBashPart.state.output : undefined

  const summary = {
    directory: options.directory,
    command: options.command,
    pluginPath,
    configPath,
    sessionID,
    permissionAsked,
    permissionReplied,
    bashParts,
    errors,
    commandCompleted,
    commandError,
    commandOutput,
    classification: classifyRun({
      permissionAsked: permissionAsked.length,
      permissionReplied: permissionReplied.length,
      hooks: {
        toolExecuteBefore: true,
        permissionAsk: true,
      },
      commandCompleted,
    }),
  }

  process.stdout.write(JSON.stringify(summary, null, 2) + "\n")
  process.exit(0)
}

await main()
