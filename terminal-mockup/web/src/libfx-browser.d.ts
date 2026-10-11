// libfx 0.0.13 ships JavaScript without declarations. This intentionally models
// only the terminal API used here, following https://fx.sh/docs/lib/api.
declare module 'libfx/browser' {
  import type { Terminal } from '@xterm/xterm'
  export interface TerminalAdapter {
    readonly cols: number
    readonly rows: number
    write(bytes: Uint8Array): void
    onData(callback: (data: string) => void): () => void
    onResize(callback: () => void): () => void
    onKeyData?(callback: (data: string) => void): () => void
    drain?(): void | Promise<void>
  }
  export interface TerminalRuntime {
    interactive: Promise<void>
    exited: Promise<number>
    write(data: string | Uint8Array): void
    resize(): void
    abort(): void
  }
  export interface TerminalOptions {
    terminal: TerminalAdapter
    env?: Record<string, string>
    args?: string[]
    fetch?: typeof fetch
    wasm?: string | URL
    configStore?: { get(id: string): string | null; set(id: string, value: string): void }
    workspace?: {
      info: { version: 1; root: string; cwd: string; home: string; gitAvailable: false; ephemeral: true }
      permission: 'allow-sandboxed' | 'prompt'
      exec(input: { command: string; cwd: string; signal: AbortSignal; timeoutMs: number; outputLimitBytes: number }):
        Promise<{ stdout: string; stderr: string; exitCode: number }>
    }
  }
  export function createFxTerminal(options: TerminalOptions): Promise<TerminalRuntime>
  export function supportsJspi(): boolean
  export function xtermAdapter(terminal: Terminal): TerminalAdapter
}
