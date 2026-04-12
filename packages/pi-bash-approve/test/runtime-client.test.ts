import { access } from "node:fs/promises";
import path from "node:path";
import { test } from "bun:test";

test("runtime shim exists in staged package runtime", async () => {
  const shim = path.resolve(import.meta.dir, "../runtime/run-pi-runtime.sh");
  await access(shim);
});
