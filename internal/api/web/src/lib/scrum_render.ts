import { escapeHTML, trimText } from "./dom";
import {
  COLUMN_LABELS,
  SCRUM_COLUMNS,
  pickScrumFocusCard,
  type ScrumBoard,
  type ScrumBoardResponse,
  type ScrumCard,
} from "./scrum_types";

const COLUMN_ACCENT: Record<string, string> = {
  backlog: "border-zinc-500/40 bg-zinc-900/50",
  ready: "border-sky-400/35 bg-sky-950/30",
  assigned: "border-violet-400/35 bg-violet-950/25",
  in_progress: "border-amber-400/35 bg-amber-950/25",
  review: "border-cyan-400/35 bg-cyan-950/25",
  blocked: "border-rose-400/35 bg-rose-950/25",
  done: "border-emerald-400/35 bg-emerald-950/25",
};

function checklistProgress(card: ScrumCard): { done: number; total: number } {
  const total = card.checklist?.length ?? 0;
  const done = (card.checklist ?? []).filter((item) => item.done).length;
  return { done, total };
}

function playStateBadge(card: ScrumCard): string {
  switch (card.play_state) {
    case "running":
      return `<span class="rounded-full border border-amber-300/40 bg-amber-300/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-200">Running</span>`;
    case "queued":
      return `<span class="rounded-full border border-violet-300/40 bg-violet-300/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-violet-200">Queued${card.queue_order ? ` #${card.queue_order}` : ""}</span>`;
    case "paused":
      return `<span class="rounded-full border border-zinc-400/40 bg-zinc-400/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-zinc-300">Paused</span>`;
    default:
      return "";
  }
}

function renderCard(card: ScrumCard, playQueue?: ScrumBoardResponse["play_queue"]): string {
  const { done, total } = checklistProgress(card);
  const hasJob = Boolean(card.job_id?.trim());
  const stateBadge = playStateBadge(card);
  const refCount = card.ref_files?.length ?? 0;
  const chatCount = card.chat?.length ?? 0;

  return `
    <article class="scrum-card scrum-card-draggable group cursor-grab rounded-lg border border-white/10 bg-zinc-950/70 p-3 shadow-[0_10px_30px_rgba(0,0,0,.22)] transition hover:border-cyan-300/30 active:cursor-grabbing" data-card-id="${escapeHTML(card.id)}" data-scrum-column="${escapeHTML(card.column)}" data-action="click->scrum#openCard">
      <div class="flex items-start justify-between gap-2">
        <h4 class="text-sm font-semibold leading-snug text-zinc-100">${escapeHTML(card.title)}</h4>
        <div class="flex shrink-0 flex-col items-end gap-1">
          ${stateBadge}
          ${hasJob ? `<span class="rounded bg-cyan-300/15 px-1.5 py-0.5 font-mono text-[10px] text-cyan-200">#${escapeHTML(card.job_id ?? "")}</span>` : ""}
        </div>
      </div>
      ${card.description ? `<p class="mt-2 text-xs leading-relaxed text-zinc-400">${escapeHTML(trimText(card.description, 140))}</p>` : ""}
      <div class="mt-3 flex flex-wrap gap-1.5 text-[10px] uppercase tracking-wide text-zinc-500">
        ${total > 0 ? `<span class="rounded border border-white/10 px-1.5 py-0.5">${done}/${total}</span>` : ""}
        ${refCount > 0 ? `<span class="rounded border border-white/10 px-1.5 py-0.5">${refCount} refs</span>` : ""}
        ${chatCount > 0 ? `<span class="rounded border border-white/10 px-1.5 py-0.5">${chatCount} msgs</span>` : ""}
      </div>
    </article>
  `;
}

function renderColumn(column: string, cards: ScrumCard[], playQueue?: ScrumBoardResponse["play_queue"]): string {
  const label = COLUMN_LABELS[column] ?? column;
  const accent = COLUMN_ACCENT[column] ?? "border-white/10 bg-zinc-900/40";
  const items = cards.length ? cards.map((card) => renderCard(card, playQueue)).join("") : `<p class="scrum-column-empty rounded-md border border-dashed border-white/10 px-3 py-6 text-center text-xs text-zinc-500">Drop cards here</p>`;
  return `
    <div class="scrum-column flex min-h-0 min-w-[280px] flex-1 flex-col rounded-xl border ${accent} p-3" data-column="${escapeHTML(column)}" data-scrum-dropzone="${escapeHTML(column)}">
      <header class="mb-3 flex items-center justify-between gap-2">
        <div class="flex items-center gap-2 min-w-0">
          <h3 class="truncate text-xs font-semibold uppercase tracking-[.16em] text-zinc-200">${escapeHTML(label)}</h3>
          <span class="rounded-full bg-black/30 px-2 py-0.5 font-mono text-[11px] text-zinc-400">${cards.length}</span>
        </div>
        <button type="button" data-action="click->scrum#stopCardClick scrum#openCreateCardModal" data-column="${escapeHTML(column)}" class="shrink-0 rounded border border-white/10 px-2 py-0.5 text-[11px] text-zinc-400 transition hover:border-cyan-300/40 hover:text-cyan-200" title="Add card">+</button>
      </header>
      <div class="scrum-column-dropzone scrollbar min-h-[120px] flex-1 space-y-3 overflow-y-auto pr-1">${items}</div>
    </div>
  `;
}

export function renderScrumFocusBar(
  board: ScrumBoard,
  cardsByCol: Record<string, ScrumCard[]>,
  playQueue?: ScrumBoardResponse["play_queue"],
): string {
  const focus = pickScrumFocusCard(board, cardsByCol, playQueue);
  if (!focus) {
    return `
      <div class="flex items-center justify-center gap-2 rounded-xl border border-dashed border-white/10 bg-zinc-950/40 px-4 py-2.5 text-center">
        <span class="text-xs text-zinc-500">Nothing in Assigned or In Progress</span>
      </div>
    `;
  }

  const columnLabel = COLUMN_LABELS[focus.column] ?? focus.column;
  const isRunning = focus.play_state === "running";
  const isQueued = focus.play_state === "queued";
  const hasActiveRunner = Boolean(playQueue?.running_card_id);
  const playLabel = hasActiveRunner && !isRunning ? "Queue" : "Play";

  const stateBadge = isRunning
    ? `<span class="rounded-full border border-amber-300/40 bg-amber-300/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-200">Running</span>`
    : isQueued
      ? `<span class="rounded-full border border-violet-300/40 bg-violet-300/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-violet-200">Queued${focus.queue_order ? ` #${focus.queue_order}` : ""}</span>`
      : `<span class="rounded-full border border-white/10 bg-zinc-900/80 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-zinc-400">${escapeHTML(columnLabel)}</span>`;

  const playButton =
    isRunning || isQueued
      ? ""
      : `<button type="button" data-action="scrum#play" data-card-id="${escapeHTML(focus.id)}" class="rounded-md bg-cyan-300 px-3 py-1.5 text-xs font-semibold text-zinc-950 transition hover:bg-cyan-200" title="Play this card">▶ ${escapeHTML(playLabel)}</button>`;

  const pauseButton = isRunning
    ? `<button type="button" data-action="scrum#pausePlay" data-card-id="${escapeHTML(focus.id)}" class="rounded-md border border-amber-300/40 bg-amber-300/10 px-3 py-1.5 text-xs font-semibold text-amber-100 transition hover:bg-amber-300/20" title="Pause play">⏸ Pause</button>`
    : "";

  const pivotButton =
    hasActiveRunner && !isRunning && !isQueued
      ? `<button type="button" data-action="scrum#pivotPlay" data-card-id="${escapeHTML(focus.id)}" class="rounded-md border border-violet-300/30 bg-violet-300/10 px-3 py-1.5 text-xs font-semibold text-violet-100 transition hover:bg-violet-300/20" title="Play this card now">Play now</button>`
      : "";

  return `
    <div class="flex max-w-2xl items-center gap-3 rounded-xl border border-white/10 bg-zinc-950/70 px-4 py-2.5 shadow-[0_10px_30px_rgba(0,0,0,.18)]">
      <div class="min-w-0 flex-1">
        <p class="text-[10px] font-semibold uppercase tracking-[.18em] text-zinc-500">Now playing</p>
        <button type="button" data-action="scrum#openCard" data-card-id="${escapeHTML(focus.id)}" class="mt-0.5 block max-w-full truncate text-left text-sm font-semibold text-zinc-100 transition hover:text-cyan-200" title="${escapeHTML(focus.title)}">${escapeHTML(focus.title)}</button>
      </div>
      <div class="flex shrink-0 items-center gap-2">
        ${stateBadge}
        ${playButton}
        ${pivotButton}
        ${pauseButton}
      </div>
    </div>
  `;
}

export function renderScrumBoard(board: ScrumBoard, cardsByCol: Record<string, ScrumCard[]>, playQueue?: ScrumBoardResponse["play_queue"]): string {
  const columns = board.columns?.length ? board.columns : [...SCRUM_COLUMNS];
  return `<div class="flex min-h-0 gap-3 overflow-x-auto pb-1">${columns.map((column) => renderColumn(column, cardsByCol[column] ?? [], playQueue)).join("")}</div>`;
}

export function renderScrumEmptyState(message: string): string {
  return `<div class="flex h-full min-h-[240px] items-center justify-center rounded-xl border border-dashed border-white/10 p-8 text-sm text-zinc-500">${escapeHTML(message)}</div>`;
}

export function renderScrumBoardLoadingOverlay(message = "Working…"): string {
  return `
    <div data-scrum-target="boardOverlay" class="pointer-events-none absolute inset-0 z-10 hidden items-center justify-center rounded-xl border border-white/10 bg-zinc-950/80 backdrop-blur-sm">
      <div class="flex flex-col items-center gap-3 px-6 text-center">
        <div class="h-9 w-9 animate-spin rounded-full border-2 border-cyan-300/25 border-t-cyan-300 shadow-[0_0_24px_rgba(103,232,249,.35)]"></div>
        <p data-scrum-target="boardOverlayMessage" class="text-sm font-medium text-cyan-100">${escapeHTML(message)}</p>
      </div>
    </div>
  `;
}

export function renderProjectScrumShell(projectLocation: string): string {
  return `
    <div data-project-tab-panel="scrum" class="flex min-h-[520px] flex-col gap-3">
      <div class="grid grid-cols-1 items-center gap-3 lg:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
        <p class="truncate font-mono text-[11px] text-zinc-500 lg:justify-self-start">${escapeHTML(projectLocation)}</p>
        <div data-scrum-target="focus" class="flex justify-center lg:justify-self-center">
          ${renderScrumFocusBar({ id: "", name: "", project_directory: projectLocation, columns: [...SCRUM_COLUMNS], cards: [], updated_at: "" }, {}, undefined)}
        </div>
        <div class="flex flex-wrap items-center justify-end gap-2 lg:justify-self-end">
          <span data-scrum-target="status" class="text-xs text-zinc-500"></span>
          <button type="button" data-action="scrum#openCreateCardModal" class="rounded-md bg-cyan-300 px-3 py-1.5 text-xs font-semibold text-zinc-950 transition hover:bg-cyan-200">+ Card</button>
          <button type="button" data-action="scrum#refresh" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-300 transition hover:border-cyan-300/40 hover:text-zinc-100">Refresh</button>
        </div>
      </div>

      <div class="relative scrollbar min-h-0 flex-1 overflow-x-auto overflow-y-hidden" data-scrum-board-scroll>
        ${renderScrumBoardLoadingOverlay()}
        <div data-scrum-target="board" class="scrum-kanban h-full min-h-[420px]">
          ${renderScrumEmptyState("Loading scrum board…")}
        </div>
      </div>
    </div>
  `;
}
