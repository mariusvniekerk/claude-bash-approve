let queue: Promise<unknown> = Promise.resolve();

/**
 * Serialize protected tool work so multiple sibling tool calls do not race the runtime or stack
 * several approval dialogs on top of each other.
 */
export function runProtectedTool<T>(task: () => Promise<T>): Promise<T> {
  const next = queue.then(task, task);
  queue = next.then(() => undefined, () => undefined);
  return next;
}
