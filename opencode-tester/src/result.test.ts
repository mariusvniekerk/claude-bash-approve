import { describe, expect, test } from "bun:test"

import { classifyRun } from "./result"

describe("classifyRun", () => {
  test("reports plugin intercept when no permission prompt occurs", () => {
    expect(
      classifyRun({
        permissionAsked: 0,
        permissionReplied: 0,
        hooks: { toolExecuteBefore: true, permissionAsk: true },
        commandCompleted: true,
      }),
    ).toEqual("plugin-intercepted")
  })

  test("reports runtime gap when permission prompt still occurs", () => {
    expect(
      classifyRun({
        permissionAsked: 1,
        permissionReplied: 1,
        hooks: { toolExecuteBefore: true, permissionAsk: true },
        commandCompleted: true,
      }),
    ).toEqual("permission-hook-not-wired")
  })
})
