import path from "node:path";
import { fileURLToPath } from "node:url";
import type { ExtensionAPI } from "@mariozechner/pi-coding-agent";
import { createExecutionResolver } from "../../src/execution-resolver";
import { createProtectedBashTool, createProtectedReadTool } from "../../src/overrides/tools";

const fixtureDir = path.dirname(fileURLToPath(import.meta.url));

export default function (pi: ExtensionAPI) {
  const packageDir = path.resolve(fixtureDir, "../..");
  const resolveExecution = createExecutionResolver(packageDir);

  pi.registerCommand("pi-bash-approve-read-e2e", {
    description: "Exercise the protected read tool through pi RPC",
    handler: async (args: string, ctx: any) => {
      const target = args.trim();
      if (!target) {
        ctx.ui.notify("missing target path", "error");
        return;
      }

      try {
        const tool = createProtectedReadTool(resolveExecution);
        const result: any = await tool.execute(
          "pi-bash-approve-read-e2e",
          { path: target },
          ctx.signal,
          undefined,
          ctx,
        );
        const text = result.content
          .filter((item: any): item is { type: "text"; text: string } => item.type === "text")
          .map((item: { type: "text"; text: string }) => item.text)
          .join("\n");
        ctx.ui.notify(`read succeeded: ${text.slice(0, 80)}`, "info");
      } catch (error) {
        ctx.ui.notify(error instanceof Error ? error.message : String(error), "warning");
      }
    },
  });

  pi.registerCommand("pi-bash-approve-bash-e2e", {
    description: "Exercise the protected bash tool through pi RPC",
    handler: async (args: string, ctx: any) => {
      const command = args.trim();
      if (!command) {
        ctx.ui.notify("missing bash command", "error");
        return;
      }

      try {
        const tool = createProtectedBashTool(resolveExecution);
        await tool.execute(
          "pi-bash-approve-bash-e2e",
          { command },
          ctx.signal,
          undefined,
          ctx,
        );
        ctx.ui.notify("bash succeeded", "info");
      } catch (error) {
        ctx.ui.notify(error instanceof Error ? error.message : String(error), "warning");
      }
    },
  });
}
