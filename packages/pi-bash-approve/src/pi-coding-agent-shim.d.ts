declare module "@mariozechner/pi-coding-agent" {
  export type ExtensionContext = {
    cwd: string;
    hasUI: boolean;
    ui: {
      confirm(title: string, message: string): Promise<boolean>;
      notify(message: string, type?: "info" | "warning" | "error"): void;
    };
    signal?: AbortSignal;
  };

  export type ToolCallEvent = {
    type: "tool_call";
    toolCallId: string;
    toolName: string;
    input: Record<string, unknown>;
  };

  export type ToolCallEventResult = {
    block?: boolean;
    reason?: string;
  };

  export type ExtensionAPI = {
    on(
      event: "session_start",
      handler: (
        event: unknown,
        ctx: ExtensionContext,
      ) => Promise<void> | void,
    ): void;
    on(
      event: "tool_call",
      handler: (
        event: ToolCallEvent,
        ctx: ExtensionContext,
      ) => Promise<ToolCallEventResult | undefined> | ToolCallEventResult | undefined,
    ): void;
    registerFlag(name: string, options: {
      description?: string;
      type: "boolean" | "string";
      default?: boolean | string;
    }): void;
    getFlag(name: string): boolean | string | undefined;
    registerTool(tool: any): void;
    registerCommand(name: string, options: {
      description: string;
      handler(args: string, ctx: any): Promise<void> | void;
    }): void;
  };

  export function createBashTool(cwd: string): any;
  export function createReadTool(cwd: string): any;
  export function createGrepTool(cwd: string): any;
  export function createFindTool(cwd: string): any;
  export function createLsTool(cwd: string): any;
}
