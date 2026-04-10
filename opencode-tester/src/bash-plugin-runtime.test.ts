import { beforeEach, describe, expect, mock, test } from "bun:test"

import { BashApprovePlugin } from "../../opencode/bash-approve.plugin.ts"

type FakeProc = {
  exitCode: number
  stdout: Buffer
  stderr: Buffer
}

function fakeProc(stdout: string, stderr = "", exitCode = 0): FakeProc {
  return {
    exitCode,
    stdout: Buffer.from(stdout),
    stderr: Buffer.from(stderr),
  }
}

function createShell(proc: FakeProc, calls: Array<{ cwd: string }>) {
  return {
    cwd(cwd: string) {
      return () => ({
        quiet() {
          return {
            nothrow: async () => {
              calls.push({ cwd })
              return proc
            },
          }
        },
      })
    },
  }
}

describe("BashApprovePlugin", () => {
  const fetchMock = mock(async () => new Response("ok", { status: 200 }))

  beforeEach(() => {
    fetchMock.mockClear()
    globalThis.fetch = fetchMock as unknown as typeof fetch
  })

  function createClient(replies: Array<{ sessionID: string; requestID: string; reply: string }>) {
    return {
      postSessionIdPermissionsPermissionId: async (input: {
        path: { id: string; permissionID: string }
        body?: { response: string }
      }) => {
        replies.push({
          sessionID: input.path.id,
          requestID: input.path.permissionID,
          reply: input.body?.response ?? "",
        })
      },
    }
  }

  test("replies once for cached allow decisions on bash permission events", async () => {
    const shellCalls: Array<{ cwd: string }> = []
    const replies: Array<{ sessionID: string; requestID: string; reply: string }> = []
    const plugin = await BashApprovePlugin({
      directory: "/repo",
      client: createClient(replies),
      serverUrl: new URL("http://127.0.0.1:4096"),
      $: createShell(fakeProc('{"decision":"allow"}'), shellCalls) as never,
    } as never)

    await plugin["tool.execute.before"]?.(
      { tool: "bash", sessionID: "session-1", callID: "call-1" },
      { args: { command: "git status", workdir: "/repo" } },
    )

    await plugin.event?.({
      event: {
        type: "permission.asked",
        properties: {
          id: "perm-1",
          sessionID: "session-1",
          permission: "bash",
          patterns: ["git *"],
          tool: { callID: "call-1" },
        },
      },
    } as never)

    expect(shellCalls).toHaveLength(1)
    expect(replies).toEqual([{ sessionID: "session-1", requestID: "perm-1", reply: "once" }])
    expect(fetchMock).not.toHaveBeenCalled()
  })

  test("replies reject for cached deny decisions on bash permission events", async () => {
    const replies: Array<{ sessionID: string; requestID: string; reply: string }> = []
    const plugin = await BashApprovePlugin({
      directory: "/repo",
      client: createClient(replies),
      serverUrl: new URL("http://127.0.0.1:4096"),
      $: createShell(fakeProc('{"decision":"deny","reason":"dangerous"}'), []) as never,
    } as never)

    await plugin["tool.execute.before"]?.(
      { tool: "bash", sessionID: "session-1", callID: "call-1" },
      { args: { command: "rm -rf tmp", workdir: "/repo" } },
    )

    await plugin.event?.({
      event: {
        type: "permission.asked",
        properties: {
          id: "perm-1",
          sessionID: "session-1",
          permission: "bash",
          patterns: ["rm *"],
          tool: { callID: "call-1" },
        },
      },
    } as never)

    expect(replies).toEqual([{
      sessionID: "session-1",
      requestID: "perm-1",
      reply: "reject",
    }])
    expect(fetchMock).not.toHaveBeenCalled()
  })

  test("does not reply for ask decisions", async () => {
    const replies: Array<{ sessionID: string; requestID: string; reply: string }> = []
    const plugin = await BashApprovePlugin({
      directory: "/repo",
      client: createClient(replies),
      serverUrl: new URL("http://127.0.0.1:4096"),
      $: createShell(fakeProc('{"decision":"ask"}'), []) as never,
    } as never)

    await plugin["tool.execute.before"]?.(
      { tool: "bash", sessionID: "session-1", callID: "call-1" },
      { args: { command: "git status", workdir: "/repo" } },
    )

    await plugin.event?.({
      event: {
        type: "permission.asked",
        properties: {
          id: "perm-1",
          sessionID: "session-1",
          permission: "bash",
          patterns: ["git *"],
          tool: { callID: "call-1" },
        },
      },
    } as never)

    expect(replies).toHaveLength(0)
  })

  test("ignores non-bash permission events", async () => {
    const shellCalls: Array<{ cwd: string }> = []
    const replies: Array<{ sessionID: string; requestID: string; reply: string }> = []
    const plugin = await BashApprovePlugin({
      directory: "/repo",
      client: createClient(replies),
      serverUrl: new URL("http://127.0.0.1:4096"),
      $: createShell(fakeProc('{"decision":"allow"}'), shellCalls) as never,
    } as never)

    await plugin.event?.({
      event: {
        type: "permission.asked",
        properties: {
          id: "perm-1",
          sessionID: "session-1",
          permission: "edit",
          patterns: ["*"],
        },
      },
    } as never)

    expect(shellCalls).toHaveLength(0)
    expect(replies).toHaveLength(0)
  })

  test("does not guess from permission event metadata when no matching bash call was captured", async () => {
    const shellCalls: Array<{ cwd: string }> = []
    const replies: Array<{ sessionID: string; requestID: string; reply: string }> = []
    const plugin = await BashApprovePlugin({
      directory: "/repo",
      client: createClient(replies),
      serverUrl: new URL("http://127.0.0.1:4096"),
      $: createShell(fakeProc('{"decision":"allow"}'), shellCalls) as never,
    } as never)

    await plugin.event?.({
      event: {
        type: "permission.asked",
        properties: {
          id: "perm-1",
          sessionID: "session-1",
          permission: "bash",
          patterns: ["git *"],
          tool: { callID: "missing-call" },
        },
      },
    } as never)

    expect(shellCalls).toHaveLength(0)
    expect(replies).toHaveLength(0)
  })

test("uses the OpenCode client for permission replies instead of raw fetch", async () => {
    const replies: Array<{ sessionID: string; requestID: string; reply: string }> = []
    globalThis.fetch = mock(async () => {
      throw new Error("plugin should not use raw fetch")
    }) as unknown as typeof fetch

    const plugin = await BashApprovePlugin({
      directory: "/repo",
      client: createClient(replies),
      serverUrl: new URL("http://127.0.0.1:4096"),
      $: createShell(fakeProc('{"decision":"allow"}'), []) as never,
    } as never)

    await plugin["tool.execute.before"]?.(
      { tool: "bash", sessionID: "session-1", callID: "call-1" },
      { args: { command: "git status", workdir: "/repo" } },
    )

    await plugin.event?.({
      event: {
        type: "permission.asked",
        properties: {
          id: "perm-1",
          sessionID: "session-1",
          permission: "bash",
          patterns: ["git *"],
          tool: { callID: "call-1" },
        },
      },
    } as never)

    expect(replies).toEqual([{ sessionID: "session-1", requestID: "perm-1", reply: "once" }])
  })

  test("stays silent by default during bash evaluation", async () => {
    const errorCalls: string[] = []
    const originalConsoleError = console.error
    console.error = (...args: unknown[]) => {
      errorCalls.push(args.map(String).join(" "))
    }

    try {
      const plugin = await BashApprovePlugin({
        directory: "/repo",
        client: createClient([]),
        serverUrl: new URL("http://127.0.0.1:4096"),
        $: createShell(fakeProc('{"decision":"allow"}'), []) as never,
      } as never)

      await plugin["tool.execute.before"]?.(
        { tool: "bash", sessionID: "session-1", callID: "call-1" },
        { args: { command: "git status", workdir: "/repo" } },
      )
    } finally {
      console.error = originalConsoleError
    }

    expect(errorCalls).toEqual([])
  })
})
