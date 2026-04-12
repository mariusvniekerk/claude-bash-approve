declare module "node:fs/promises" {
  export function access(path: string): Promise<void>;
}

declare module "node:path" {
  const path: {
    resolve(...parts: string[]): string;
  };
  export default path;
}

interface ImportMeta {
  readonly dir: string;
}
