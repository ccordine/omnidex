import { escapeHTML } from "./dom";

export function renderProjectTerminalShell(projectLocation: string): string {
  return `
    <div data-project-tab-panel="terminal" class="flex min-h-0 flex-col gap-3">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="min-w-0">
          <p class="truncate font-mono text-[11px] text-zinc-500">${escapeHTML(projectLocation)}</p>
          <p class="mt-1 text-xs text-zinc-500">Interactive shell on the host via the bridge. Login shell in the project directory.</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <span data-terminal-target="status" class="text-xs text-zinc-500">Idle</span>
          <button type="button" data-action="terminal#reconnect" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200 transition hover:border-cyan-300/40 hover:bg-cyan-300/10">Reconnect</button>
          <button type="button" data-action="terminal#toggleFullscreen" data-terminal-target="fullscreenButton" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200 transition hover:border-cyan-300/40 hover:bg-cyan-300/10">Fullscreen</button>
        </div>
      </div>
      <div data-terminal-target="frame" class="relative min-h-0 flex-1 overflow-hidden rounded-xl border border-white/10 bg-[#09090b] shadow-[inset_0_0_0_1px_rgba(255,255,255,.04)]">
        <div data-terminal-target="mount" class="h-full min-h-[420px] w-full p-1"></div>
        <div class="pointer-events-none absolute inset-x-0 top-0 flex justify-end p-3 opacity-0 transition-opacity duration-150 terminal-fullscreen-controls">
          <button type="button" data-action="terminal#toggleFullscreen" class="pointer-events-auto rounded-md border border-white/15 bg-zinc-950/80 px-3 py-1.5 text-xs font-semibold text-zinc-100 backdrop-blur hover:border-cyan-300/40 hover:bg-zinc-900/90">Exit fullscreen</button>
        </div>
      </div>
    </div>
  `;
}
