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
          permissionAsk: false,
        },
        commandCompleted: false,
      }),
    ).toEqual("plugin-not-loaded")
  })

  test("reports missing permission hook when bash executes without permission.ask", () => {
    expect(
      classifyRun({
        permissionAsked: 0,
        permissionReplied: 0,
        pluginReplied: false,
        hooks: {
          toolExecuteBefore: true,
          permissionAsk: false,
        },
        commandCompleted: false,
      }),
    ).toEqual("permission-hook-missing")
  })

  test("reports plugin intercept when no permission prompt occurs", () => {
    expect(
      classifyRun({
        permissionAsked: 0,
        permissionReplied: 0,
        pluginReplied: false,
        hooks: {
          toolExecuteBefore: true,
          permissionAsk: true,
        },
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
        hooks: {
          toolExecuteBefore: true,
          permissionAsk: true,
        },
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
        hooks: {
          toolExecuteBefore: true,
          permissionAsk: true,
        },
        commandCompleted: true,
      }),
    ).toEqual("plugin-intercepted")
  })

  test("reports plugin intercept when the plugin rejects before bash completes", () => {
    expect(
      classifyRun({
        permissionAsked: 1,
        permissionReplied: 1,
        pluginReplied: true,
        hooks: {
          toolExecuteBefore: true,
          permissionAsk: true,
        },
        commandCompleted: false,
      }),
    ).toEqual("plugin-intercepted")
  })
})
