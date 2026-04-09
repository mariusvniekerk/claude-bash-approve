import { describe, expect, test } from "bun:test"

import { classifyRun } from "./result.js"

describe("classifyRun", () => {
  test("reports plugin intercept when no permission prompt occurs", () => {
    expect(
      classifyRun({
        permissionAsked: 0,
        permissionReplied: 0,
        pluginReplied: false,
        commandCompleted: true,
      }),
    ).toEqual("plugin-intercepted")
  })

  test("reports runtime gap when permission prompt still occurs", () => {
    expect(
      classifyRun({
        permissionAsked: 1,
        permissionReplied: 1,
        pluginReplied: false,
        commandCompleted: true,
      }),
    ).toEqual("permission-hook-not-wired")
  })

  test("reports plugin intercept when plugin replies from event telemetry", () => {
    expect(
      classifyRun({
        permissionAsked: 1,
        permissionReplied: 1,
        pluginReplied: true,
        commandCompleted: true,
      }),
    ).toEqual("plugin-intercepted")
  })
})
