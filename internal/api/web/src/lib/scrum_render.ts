import { escapeHTML } from "./dom";

function renderScrumBoardLoadingOverlay(message = "Working…"): string {
  return `
    <div data-scrum-target="boardOverlay" class="pointer-events-none absolute inset-0 z-10 hidden items-center justify-center rounded-xl border border-white/10 bg-zinc-950/80 backdrop-blur-sm" role="status" aria-live="polite" aria-hidden="true">
      <div class="flex flex-col items-center gap-3 px-6 text-center">
        <div class="h-9 w-9 animate-spin rounded-full border-2 border-cyan-300/25 border-t-cyan-300 shadow-[0_0_24px_rgba(103,232,249,.35)]"></div>
        <p data-scrum-target="boardOverlayMessage" class="text-sm font-medium text-cyan-100">${escapeHTML(message)}</p>
      </div>
    </div>
  `;
}

export function renderProjectScrumShell(projectLocation: string): string {
  return `
    <div data-project-tab-panel="scrum" class="flex min-h-0 flex-col gap-3">
      <div class="grid shrink-0 grid-cols-1 items-center gap-3 lg:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
        <p class="truncate font-mono text-[11px] text-zinc-500 lg:justify-self-start">${escapeHTML(projectLocation)}</p>
        <div data-scrum-target="focus" data-recyclr-sink="scrum-focus" class="flex justify-center lg:justify-self-center"></div>
        <div class="flex flex-wrap items-center justify-end gap-2 lg:justify-self-end">
          <span data-scrum-target="status" class="text-xs text-zinc-500"></span>
          <button type="button" data-action="scrum#openCreateCardModal" class="rounded-md bg-cyan-300 px-3 py-1.5 text-xs font-semibold text-zinc-950 transition hover:bg-cyan-200">+ Card</button>
          <button type="button" data-action="scrum#refresh" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-300 transition hover:border-cyan-300/40 hover:text-zinc-100">Refresh</button>
        </div>
      </div>
      <div data-scrum-target="flowSummary" data-recyclr-sink="scrum-flow-summary" class="hidden shrink-0"></div>
      <div data-scrum-target="columns" data-recyclr-sink="scrum-columns" class="shrink-0"></div>
      <div class="relative scrollbar flex min-h-0 flex-1 flex-col overflow-x-auto overflow-y-hidden" data-scrum-board-scroll>
        ${renderScrumBoardLoadingOverlay()}
        <div data-scrum-target="board" data-recyclr-sink="scrum-board" class="scrum-kanban min-h-0"></div>
      </div>
    </div>
  `;
}
