declare module "@mariozechner/pi-coding-agent" {
  export type ExtensionAPI = {
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
