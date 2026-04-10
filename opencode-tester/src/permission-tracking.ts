export type PermissionTrackingState = {
  bashRequestIDs: Set<string>
  testerReplyPendingRequestIDs: Set<string>
  testerReplyRequestIDs: Set<string>
  pluginReplyRequestIDs: Set<string>
  repliedRequestIDs: Set<string>
}

export function createPermissionTrackingState(): PermissionTrackingState {
  return {
    bashRequestIDs: new Set<string>(),
    testerReplyPendingRequestIDs: new Set<string>(),
    testerReplyRequestIDs: new Set<string>(),
    pluginReplyRequestIDs: new Set<string>(),
    repliedRequestIDs: new Set<string>(),
  }
}

export function isBashPermission(permission: string) {
  return permission === "bash"
}

export function recordBashPermissionAsked(state: PermissionTrackingState, requestID: string) {
  state.bashRequestIDs.add(requestID)
}

export function markTesterReplyPending(state: PermissionTrackingState, requestID: string) {
  state.testerReplyPendingRequestIDs.add(requestID)
}

export function markTesterReplySucceeded(state: PermissionTrackingState, requestID: string) {
  state.testerReplyPendingRequestIDs.delete(requestID)
  state.testerReplyRequestIDs.add(requestID)
  state.pluginReplyRequestIDs.delete(requestID)
}

export function markTesterReplyFailed(state: PermissionTrackingState, requestID: string) {
  state.testerReplyPendingRequestIDs.delete(requestID)
  state.testerReplyRequestIDs.delete(requestID)

  if (state.repliedRequestIDs.has(requestID)) {
    state.pluginReplyRequestIDs.add(requestID)
  }
}

export function recordPermissionReplied(state: PermissionTrackingState, requestID: string) {
  if (!state.bashRequestIDs.has(requestID)) return false

  state.repliedRequestIDs.add(requestID)

  if (state.testerReplyRequestIDs.has(requestID) || state.testerReplyPendingRequestIDs.has(requestID)) {
    state.testerReplyRequestIDs.add(requestID)
    state.pluginReplyRequestIDs.delete(requestID)
    return true
  }

  state.pluginReplyRequestIDs.add(requestID)
  return true
}

export function shouldIgnoreReplyError(state: PermissionTrackingState, requestID: string, error: unknown) {
  if (state.repliedRequestIDs.has(requestID)) return true

  const message = error instanceof Error ? error.message : String(error)
  return /already|duplicate|handled|satisfied/i.test(message)
}
