import { describe, expect, test } from "bun:test"

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

describe("permission tracking", () => {
  test("tracks only bash permission requests", () => {
    const state = createPermissionTrackingState()

    expect(isBashPermission("bash")).toBe(true)
    expect(isBashPermission("edit")).toBe(false)

    recordBashPermissionAsked(state, "bash-1")

    expect(recordPermissionReplied(state, "edit-1")).toBe(false)
    expect(recordPermissionReplied(state, "bash-1")).toBe(true)
    expect([...state.pluginReplyRequestIDs]).toEqual(["bash-1"])
  })

  test("attributes successful tester replies to the tester", () => {
    const state = createPermissionTrackingState()

    recordBashPermissionAsked(state, "bash-1")
    markTesterReplyPending(state, "bash-1")
    markTesterReplySucceeded(state, "bash-1")

    expect(recordPermissionReplied(state, "bash-1")).toBe(true)
    expect([...state.testerReplyRequestIDs]).toEqual(["bash-1"])
    expect(state.pluginReplyRequestIDs.size).toBe(0)
  })

  test("reclassifies pending tester replies as plugin replies when the tester loses the race", () => {
    const state = createPermissionTrackingState()

    recordBashPermissionAsked(state, "bash-1")
    markTesterReplyPending(state, "bash-1")

    expect(recordPermissionReplied(state, "bash-1")).toBe(true)

    markTesterReplyFailed(state, "bash-1")

    expect(state.testerReplyRequestIDs.size).toBe(0)
    expect([...state.pluginReplyRequestIDs]).toEqual(["bash-1"])
  })

  test("ignores duplicate reply errors once a request is already satisfied", () => {
    const state = createPermissionTrackingState()

    recordBashPermissionAsked(state, "bash-1")
    expect(recordPermissionReplied(state, "bash-1")).toBe(true)

    expect(shouldIgnoreReplyError(state, "bash-1", new Error("permission already handled"))).toBe(true)
    expect(shouldIgnoreReplyError(state, "bash-2", new Error("boom"))).toBe(false)
  })
})
