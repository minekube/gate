// Type definitions for WidgetBot Crate
interface CrateOptions {
  server: string;
  channel: string;
  location?: [string, string];
  color?: string;
  glyph?: [string, string];
  css?: string;
  indicator?: boolean;
  notifications?: boolean;
  timeout?: number;
}

interface CrateInstance {
  kill: () => void;
  toggle: () => void;
  show: () => void;
  hide: () => void;
  on: (event: string, callback: () => void) => void;
}

declare global {
  interface Window {
    Crate: new (options: CrateOptions) => CrateInstance;
  }
}

export {};
