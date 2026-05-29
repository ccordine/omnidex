import { escapeHTML, trimText } from "./dom";
import {
  COLUMN_LABELS,
  PLAYABLE_COLUMNS,
  SCRUM_COLUMNS,
  nextColumn,
  prevColumn,
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
  const canPlay = PLAYABLE_COLUMNS.has(card.column);
  const hasJob = Boolean(card.job_id?.trim());
  const isRunning = card.play_state === "running";
  const isQueued = card.play_state === "queued";
  const hasActiveRunner = Boolean(playQueue?.running_card_id);
  const showSync = hasJob && card.column !== "done";
  const showDone = card.column === "review";
  const next = nextColumn(card.column);
  const prev = prevColumn(card.column);
  const refCount = card.ref_files?.length ?? 0;
  const chatCount = card.chat?.length ?? 0;
  const stateBadge = playStateBadge(card);
  const moveOptions = SCRUM_COLUMNS.map((col) => {
    const selected = col === card.column ? " selected" : "";
    return `<option value="${escapeHTML(col)}"${selected}>${escapeHTML(COLUMN_LABELS[col] ?? col)}</option>`;
  }).join("");

  return `
    <article class="scrum-card group cursor-pointer rounded-lg border border-white/10 bg-zinc-950/70 p-3 shadow-[0_10px_30px_rgba(0,0,0,.22)] transition hover:border-cyan-300/30" data-card-id="${escapeHTML(card.id)}" data-action="click->scrum#openCard">
      <div class="flex items-start justify-between gap-2">
        <h4 class="text-sm font-semibold leading-snug text-zinc-100">${escapeHTML(card.title)}</h4>
        <div class="flex shrink-0 flex-col items-end gap-1">
          ${stateBadge}
          ${hasJob ? `<span class="rounded bg-cyan-300/15 px-1.5 py-0.5 font-mono text-[10px] text-cyan-200">#${escapeHTML(card.job_id ?? "")}</span>` : ""}
        </div>
      </div>
      ${card.description ? `<p class="mt-2 text-xs leading-relaxed text-zinc-400">${escapeHTML(trimText(card.description, 140))}</p>` : ""}
      <div class="mt-3 flex flex-wrap gap-1.5 text-[10px] uppercase tracking-wide text-zinc-500">
        ${total > 0 ? `<span class="rounded border border-white/10 px-1.5 py-0.5">${done}/${total} checklist</span>` : ""}
        ${refCount > 0 ? `<span class="rounded border border-white/10 px-1.5 py-0.5">${refCount} refs</span>` : ""}
        ${chatCount > 0 ? `<span class="rounded border border-white/10 px-1.5 py-0.5">${chatCount} msgs</span>` : ""}
      </div>
      <div class="mt-3 flex flex-wrap items-center gap-1.5 border-t border-white/10 pt-3">
        ${canPlay && !isQueued ? `<button type="button" data-action="click->scrum#stopCardClick scrum#play" data-card-id="${escapeHTML(card.id)}" class="inline-flex items-center gap-1 rounded bg-cyan-300 px-2 py-1 text-[11px] font-semibold text-zinc-950 transition hover:bg-cyan-200"><span aria-hidden="true">▶</span> ${hasActiveRunner && !isRunning ? "Queue" : "Play"}</button>` : ""}
        ${canPlay && hasActiveRunner && !isRunning && !isQueued ? `<button type="button" data-action="click->scrum#stopCardClick scrum#pivotPlay" data-card-id="${escapeHTML(card.id)}" class="rounded border border-violet-300/30 bg-violet-300/10 px-2 py-1 text-[11px] font-semibold text-violet-100 transition hover:bg-violet-300/20">Play now</button>` : ""}
        ${isRunning ? `<button type="button" data-action="click->scrum#stopCardClick scrum#pausePlay" data-card-id="${escapeHTML(card.id)}" class="rounded border border-amber-300/30 bg-amber-300/10 px-2 py-1 text-[11px] font-semibold text-amber-100 transition hover:bg-amber-300/20">Pause</button>` : ""}
        ${isQueued && hasActiveRunner ? `<button type="button" data-action="click->scrum#stopCardClick scrum#pivotPlay" data-card-id="${escapeHTML(card.id)}" class="rounded border border-violet-300/30 px-2 py-1 text-[11px] text-violet-100 transition hover:bg-violet-300/10">Jump queue</button>` : ""}
        ${showSync ? `<button type="button" data-action="click->scrum#stopCardClick scrum#syncJob" data-card-id="${escapeHTML(card.id)}" class="rounded border border-white/10 px-2 py-1 text-[11px] text-zinc-200 transition hover:border-cyan-300/40 hover:bg-cyan-300/10">Sync job</button>` : ""}
        ${showDone ? `<button type="button" data-action="click->scrum#stopCardClick scrum#markDone" data-card-id="${escapeHTML(card.id)}" class="rounded border border-emerald-400/30 bg-emerald-400/10 px-2 py-1 text-[11px] font-semibold text-emerald-200 transition hover:bg-emerald-400/20">Done</button>` : ""}
        ${prev ? `<button type="button" data-action="click->scrum#stopCardClick scrum#retreat" data-card-id="${escapeHTML(card.id)}" class="rounded border border-white/10 px-2 py-1 text-[11px] text-zinc-300 transition hover:border-white/20" title="Move left">←</button>` : ""}
        ${next ? `<button type="button" data-action="click->scrum#stopCardClick scrum#advance" data-card-id="${escapeHTML(card.id)}" class="rounded border border-white/10 px-2 py-1 text-[11px] text-zinc-300 transition hover:border-white/20" title="Move right">→</button>` : ""}
        <select data-action="click->scrum#stopCardClick change->scrum#moveSelect" data-card-id="${escapeHTML(card.id)}" class="ml-auto max-w-[7.5rem] rounded border border-white/10 bg-zinc-900 px-1.5 py-1 text-[10px] text-zinc-300 outline-none" aria-label="Move card column">${moveOptions}</select>
        <button type="button" data-action="click->scrum#stopCardClick scrum#deleteCard" data-card-id="${escapeHTML(card.id)}" class="rounded border border-rose-400/20 px-2 py-1 text-[11px] text-rose-300 opacity-0 transition group-hover:opacity-100 hover:bg-rose-400/10" title="Delete card">×</button>
      </div>
    </article>
  `;
}

function renderColumn(column: string, cards: ScrumCard[], playQueue?: ScrumBoardResponse["play_queue"]): string {
  const label = COLUMN_LABELS[column] ?? column;
  const accent = COLUMN_ACCENT[column] ?? "border-white/10 bg-zinc-900/40";
  const items = cards.length ? cards.map((card) => renderCard(card, playQueue)).join("") : `<p class="rounded-md border border-dashed border-white/10 px-3 py-6 text-center text-xs text-zinc-500">No cards</p>`;
  return `
    <div class="scrum-column flex min-h-0 min-w-[280px] flex-1 flex-col rounded-xl border ${accent} p-3" data-column="${escapeHTML(column)}">
      <header class="mb-3 flex items-center justify-between gap-2">
        <h3 class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-200">${escapeHTML(label)}</h3>
        <span class="rounded-full bg-black/30 px-2 py-0.5 font-mono text-[11px] text-zinc-400">${cards.length}</span>
      </header>
      <div class="scrollbar min-h-0 flex-1 space-y-3 overflow-y-auto pr-1">${items}</div>
    </div>
  `;
}

export function renderScrumBoard(board: ScrumBoard, cardsByCol: Record<string, ScrumCard[]>, playQueue?: ScrumBoardResponse["play_queue"]): string {
  const columns = board.columns?.length ? board.columns : [...SCRUM_COLUMNS];
  const queueHint =
    playQueue?.running_card_id || (playQueue?.queued_count ?? 0) > 0
      ? `<div class="mb-3 rounded-lg border border-violet-300/20 bg-violet-950/20 px-3 py-2 text-xs text-violet-100">${playQueue?.running_card_id ? "Omnidex is running a card" : "Play queue idle"}${(playQueue?.queued_count ?? 0) > 0 ? ` · ${playQueue?.queued_count} queued` : ""}</div>`
      : "";
  return `${queueHint}<div class="flex min-h-0 gap-3 overflow-x-auto pb-1">${columns.map((column) => renderColumn(column, cardsByCol[column] ?? [], playQueue)).join("")}</div>`;
}

export function renderScrumEmptyState(message: string): string {
  return `<div class="flex h-full min-h-[240px] items-center justify-center rounded-xl border border-dashed border-white/10 p-8 text-sm text-zinc-500">${escapeHTML(message)}</div>`;
}
