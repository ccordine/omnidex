import { escapeHTML } from "./dom";

function tabPanelClass(tab: string, activeTab: string): string {
  return tab === activeTab ? "" : " hidden";
}

export function renderProjectScreenShell(projectLocation: string, activeTab = "scrum"): string {
  return `
    <div data-project-tab-panel="screen" class="flex min-h-0 flex-1 flex-col gap-3${tabPanelClass("screen", activeTab)}">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="min-w-0">
          <p class="truncate font-mono text-[11px] text-zinc-500">${escapeHTML(projectLocation)}</p>
          <p class="mt-1 text-xs text-zinc-500">Live monitor view from the host via the bridge. Plain HTTP MJPEG for low-latency LAN viewing.</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <span data-screen-target="status" class="text-xs text-zinc-500">Idle</span>
          <button type="button" data-action="screen#reconnect" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200 transition hover:border-cyan-300/40 hover:bg-cyan-300/10">Reconnect</button>
          <button type="button" data-action="screen#toggleFullscreen" data-screen-target="fullscreenButton" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200 transition hover:border-cyan-300/40 hover:bg-cyan-300/10">Fullscreen</button>
        </div>
      </div>

      <div class="flex flex-wrap items-end gap-3 rounded-xl border border-white/10 bg-zinc-950/60 p-4">
        <label class="block min-w-[180px] flex-1">
          <span class="text-xs text-zinc-500">Monitor</span>
          <select data-screen-target="monitorSelect" data-action="change->screen#changeMonitor" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40">
            <option value="">Loading…</option>
          </select>
        </label>
        <label class="block w-28">
          <span class="text-xs text-zinc-500">FPS</span>
          <select data-screen-target="fpsSelect" data-action="change->screen#changeQuality" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40">
            <option value="8">8</option>
            <option value="12" selected>12</option>
            <option value="15">15</option>
            <option value="20">20</option>
            <option value="24">24</option>
          </select>
        </label>
        <label class="block w-32">
          <span class="text-xs text-zinc-500">Scale</span>
          <select data-screen-target="scaleSelect" data-action="change->screen#changeQuality" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40">
            <option value="100">100%</option>
            <option value="75" selected>75%</option>
            <option value="50">50%</option>
            <option value="35">35%</option>
          </select>
        </label>
        <div class="min-w-[220px] flex-1">
          <span class="text-xs text-zinc-500">Phone / LAN stream URL</span>
          <input data-screen-target="streamUrl" readonly class="mt-1 w-full truncate rounded-md border border-white/10 bg-zinc-900/70 px-3 py-2 font-mono text-[11px] text-cyan-200 outline-none" value="" />
        </div>
      </div>

      <div data-screen-target="frame" class="relative min-h-0 flex-1 overflow-hidden rounded-xl border border-white/10 bg-black shadow-[inset_0_0_0_1px_rgba(255,255,255,.04)]">
        <img data-screen-target="stream" alt="Host monitor stream" class="h-full min-h-[420px] w-full select-none object-contain" />
        <div data-screen-target="placeholder" class="pointer-events-none absolute inset-0 grid place-items-center p-6 text-center text-sm text-zinc-500">
          Open this tab to start the monitor stream.
        </div>
        <div class="pointer-events-none absolute inset-x-0 top-0 flex justify-end p-3 opacity-0 transition-opacity duration-150 screen-fullscreen-controls">
          <button type="button" data-action="screen#toggleFullscreen" class="pointer-events-auto rounded-md border border-white/15 bg-zinc-950/80 px-3 py-1.5 text-xs font-semibold text-zinc-100 backdrop-blur hover:border-cyan-300/40 hover:bg-zinc-900/90">Exit fullscreen</button>
        </div>
      </div>
    </div>
  `;
}
