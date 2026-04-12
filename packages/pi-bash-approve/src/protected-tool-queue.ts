let queue: Promise<unknown> = Promise.resolve();

export function runProtectedTool<T>(task: () => Promise<T>): Promise<T> {
  const next = queue.then(task, task);
  queue = next.then(() => undefined, () => undefined);
  return next;
}
