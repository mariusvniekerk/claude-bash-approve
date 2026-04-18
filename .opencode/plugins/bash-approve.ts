import path from "node:path"
import type { Hooks, PluginInput } from "@opencode-ai/plugin"

type Decision = {
  decision: "allow" | "deny" | "ask"
  reason?: string | undefined
}

type PendingRequest = {
  command: string
  cwd: string
  evaluation: Decision
}

type PermissionReply = "once" | "always" | "reject"

type ToolExecuteBeforeInput = Parameters<NonNullable<Hooks["tool.execute.before"]>>[0]
type ToolExecuteBeforeOutput = Parameters<NonNullable<Hooks["tool.execute.before"]>>[1]
type ToolExecuteAfterInput = Parameters<NonNullable<Hooks["tool.execute.after"]>>[0]
type EventInput = Parameters<NonNullable<Hooks["event"]>>[0]

type PermissionRequestLike = {
  permission: string
  sessionID: string
  patterns: string[]
  tool?: {
    callID: string
  }
}

type PermissionAskedEvent = {
  type: "permission.asked"
  properties: PermissionRequestLike & {
    id: string
  }
}

const HOOK_TRACE_PREFIX = "[bash-approve-hook]"
const DEBUG_HOOKS = process.env.BASH_APPROVE_DEBUG === "1"

type BashApprovePluginInput = PluginInput & {
  $: NonNullable<PluginInput["$"]>
}

function requestKey(sessionID: string, callID: string) {
  return `${sessionID}:${callID}`
}

function sessionEntries(pending: Map<string, PendingRequest>, sessionID: string) {
  return [...pending.entries()].filter(([key]) => key.startsWith(`${sessionID}:`))
}

function resolveCwd(directory: string, workdir: string | undefined) {
  if (!workdir) return directory
  return path.isAbsolute(workdir) ? workdir : path.resolve(directory, workdir)
}

function parseDecision(stdout: string): Decision {
  if (!stdout) return { decision: "ask" }

  try {
    const parsed = JSON.parse(stdout) as Partial<Decision>
    if (parsed.decision === "allow" || parsed.decision === "deny" || parsed.decision === "ask") {
      return {
        decision: parsed.decision,
        reason: parsed.reason ?? undefined,
      }
    }
  } catch {
    // Fall through to defer-to-ask behavior below.
  }

  return { decision: "ask", reason: stdout }
}

function traceHook(name: string) {
  if (!DEBUG_HOOKS) return
  console.error(`${HOOK_TRACE_PREFIX} ${name}`)
}

export const BashApprovePlugin = async ({ client, directory, $ }: BashApprovePluginInput): Promise<Hooks> => {
  const hookPath = path.resolve(directory, "hooks/bash-approve/run-opencode-hook.sh")
  const pending = new Map<string, PendingRequest>()

  async function evaluate(command: string, cwd: string): Promise<Decision> {
    const proc = await $.cwd(cwd)`printf '%s' ${JSON.stringify({ tool: "bash", command, cwd })} | ${hookPath}`
      .quiet()
      .nothrow()

    if (proc.exitCode !== 0) {
      return { decision: "ask", reason: proc.stderr.toString().trim() }
    }

    return parseDecision(proc.stdout.toString().trim())
  }

  async function safeEvaluate(command: string, cwd: string): Promise<Decision> {
    try {
      return await evaluate(command, cwd)
    } catch (error) {
      return {
        decision: "ask",
        reason: error instanceof Error ? error.message : String(error),
      }
    }
  }

  function lookupRequest(info: { sessionID: string; tool?: { callID: string } }) {
    if (info.tool) {
      return pending.get(requestKey(info.sessionID, info.tool.callID))
    }

    const candidates = sessionEntries(pending, info.sessionID)
    if (candidates.length !== 1) return undefined
    return candidates[0]?.[1]
  }

  async function replyPermission(sessionID: string, requestID: string, reply: PermissionReply) {
    await client.postSessionIdPermissionsPermissionId({
      path: {
        id: sessionID,
        permissionID: requestID,
      },
      query: {
        directory,
      },
      body: {
        response: reply,
      },
    })
  }

  return {
    "tool.execute.before": async (input: ToolExecuteBeforeInput, output: ToolExecuteBeforeOutput) => {
      if (input.tool !== "bash") return
      traceHook("tool.execute.before")

      const info = {
        command: output.args.command,
        cwd: resolveCwd(directory, output.args.workdir),
      }

      pending.set(requestKey(input.sessionID, input.callID), {
        command: info.command,
        cwd: info.cwd,
        evaluation: await safeEvaluate(info.command, info.cwd),
      })
    },

    "tool.execute.after": async (input: ToolExecuteAfterInput) => {
      if (input.tool !== "bash") return
      pending.delete(requestKey(input.sessionID, input.callID))
    },

    event: async ({ event }: EventInput) => {
      const permissionEvent = event as EventInput["event"] | PermissionAskedEvent
      if (permissionEvent.type !== "permission.asked") return
      if (permissionEvent.properties.permission !== "bash") return

      const request = lookupRequest(permissionEvent.properties)
      if (!request) return
      const decision = request.evaluation

      if (decision.decision === "allow") {
        await replyPermission(permissionEvent.properties.sessionID, permissionEvent.properties.id, "once")
        return
      }

      if (decision.decision === "deny") {
        await replyPermission(permissionEvent.properties.sessionID, permissionEvent.properties.id, "reject")
      }
    },
  }
}
