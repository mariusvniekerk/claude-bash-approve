import { describe, expect, test } from "bun:test"

import { classifyRun } from "./result.js"

describe("classifyRun", () => {
  test("reports plugin missing when the bash hook never fires", () => {
    expect(
      classifyRun({
        permissionAsked: 0,
        permissionReplied: 0,
        pluginReplied: false,
        hooks: {
          toolExecuteBefore: false,
        },
        commandCompleted: false,
      }),
    ).toEqual("plugin-not-loaded")
  })

  test("reports native ask when the plugin defers after a bash permission prompt", () => {
    expect(
      classifyRun({
        permissionAsked: 1,
        permissionReplied: 0,
        pluginReplied: false,
        hooks: {
          toolExecuteBefore: true,
        },
        commandCompleted: false,
      }),
    ).toEqual("native-ask")
  })

  test("reports plugin intercept when the plugin replies to the permission event", () => {
    expect(
      classifyRun({
        permissionAsked: 1,
        permissionReplied: 1,
        pluginReplied: true,
        hooks: {
          toolExecuteBefore: true,
        },
        commandCompleted: false,
      }),
    ).toEqual("plugin-intercepted")
  })

  test("reports plugin defer when bash completes without a plugin reply", () => {
    expect(
      classifyRun({
        permissionAsked: 0,
        permissionReplied: 0,
        pluginReplied: false,
        hooks: {
          toolExecuteBefore: true,
        },
        commandCompleted: true,
      }),
    ).toEqual("plugin-deferred")
  })

  test("reports command failure when bash never completes and no permission path explains it", () => {
    expect(
      classifyRun({
        permissionAsked: 0,
        permissionReplied: 0,
        pluginReplied: false,
        hooks: {
          toolExecuteBefore: true,
        },
        commandCompleted: false,
      }),
    ).toEqual("command-failed")
  })
})
