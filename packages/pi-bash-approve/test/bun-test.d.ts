declare module "bun:test" {
  export const test: (
    name: string,
    fn: () => unknown | Promise<unknown>,
    timeout?: number,
  ) => void;
  export const expect: (...args: any[]) => any;
}

interface ImportMeta {
  readonly dir: string;
}
