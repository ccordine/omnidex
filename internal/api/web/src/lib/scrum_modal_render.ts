import { escapeHTML, formatDateTime, statusPillClass } from "./dom";
import { renderModelConfigSection } from "./model_config_render";
import { renderAgentConfigSection } from "./agent_config_render";
import type { ModelFieldDefinition } from "./model_config_types";
import type { AgentFieldDefinition } from "./agent_config_types";
import { COLUMN_LABELS, PLAYABLE_COLUMNS, SCRUM_COLUMNS, type ScrumBoard, type ScrumBoardResponse, type ScrumCard } from "./scrum_types";

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

function renderModalPlayActions(card: ScrumCard, playQueue?: ScrumBoardResponse["play_queue"]): string {
  const canPlay = PLAYABLE_COLUMNS.has(card.column);
  const isRunning = card.play_state === "running";
  const isQueued = card.play_state === "queued";
  const hasActiveRunner = Boolean(playQueue?.running_card_id);
  return `
    ${canPlay && !isQueued ? `<button type="button" data-action="scrum#play" data-card-id="${escapeHTML(card.id)}" class="rounded-md bg-cyan-300 px-3 py-1.5 text-xs font-semibold text-zinc-950 hover:bg-cyan-200">▶ ${hasActiveRunner && !isRunning ? "Queue" : "Play"}</button>` : ""}
    ${canPlay && hasActiveRunner && !isRunning && !isQueued ? `<button type="button" data-action="scrum#pivotPlay" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-violet-300/30 bg-violet-300/10 px-3 py-1.5 text-xs font-semibold text-violet-100 hover:bg-violet-300/20">Play now</button>` : ""}
    ${isRunning ? `<button type="button" data-action="scrum#pausePlay" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-amber-300/30 bg-amber-300/10 px-3 py-1.5 text-xs font-semibold text-amber-100 hover:bg-amber-300/20">Pause</button>` : ""}
    ${isQueued && hasActiveRunner ? `<button type="button" data-action="scrum#pivotPlay" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-violet-300/30 px-3 py-1.5 text-xs text-violet-100 hover:bg-violet-300/10">Jump queue</button>` : ""}
  `;
}

export function renderScrumModalDetails(card: ScrumCard): string {
  return `
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <div class="flex items-center justify-between gap-3">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Description</h3>
        <button type="button" data-action="scrum#saveDetails" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-white/10 px-3 py-1.5 text-xs font-semibold text-zinc-200 transition hover:border-cyan-300/40 hover:bg-cyan-300/10">Save</button>
      </div>
      <input data-scrum-field="title" type="text" value="${escapeHTML(card.title)}" class="mt-3 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-lg font-semibold text-zinc-100 outline-none focus:border-cyan-300/40" />
      <textarea data-scrum-field="description" rows="6" class="scrollbar mt-3 w-full resize-y rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm leading-6 text-zinc-200 outline-none focus:border-cyan-300/40">${escapeHTML(card.description || "")}</textarea>
    </section>
  `;
}

export function renderScrumModalChecklist(card: ScrumCard): string {
  const items = (card.checklist ?? []).map((item) => {
    const checked = item.done ? " checked" : "";
    return `<label class="flex items-start gap-3 rounded-md border border-white/10 bg-zinc-950/40 px-3 py-2 text-sm text-zinc-200"><input type="checkbox" data-action="change->scrum#toggleChecklistItem" data-card-id="${escapeHTML(card.id)}" data-item-id="${escapeHTML(item.id)}" class="mt-1 rounded border-white/20 bg-zinc-900 text-cyan-300"${checked} /><span class="${item.done ? "text-zinc-500 line-through" : ""}">${escapeHTML(item.text)}</span></label>`;
  }).join("");
  return `
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Checklist</h3>
      <div class="mt-3 space-y-2">${items || `<p class="text-sm text-zinc-500">No checklist items yet.</p>`}</div>
      <form data-action="submit->scrum#addChecklistItem" data-card-id="${escapeHTML(card.id)}" class="mt-3 flex gap-2">
        <input data-scrum-field="checklistText" type="text" placeholder="Add checklist item" class="min-w-0 flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
        <button type="submit" class="rounded-md bg-cyan-300 px-3 py-2 text-xs font-semibold text-zinc-950 hover:bg-cyan-200">Add</button>
      </form>
    </section>
  `;
}

export function renderScrumModalChat(card: ScrumCard): string {
  const messages = (card.chat ?? []).map((msg) => {
    const shell = msg.role === "user" ? "border-cyan-300/25 bg-cyan-300/10" : "border-white/10 bg-zinc-900/70";
    return `<div class="rounded-md border ${shell} px-3 py-2"><div class="text-[11px] uppercase tracking-wide text-zinc-500">${escapeHTML(msg.role)} · ${escapeHTML(formatDateTime(msg.created_at))}</div><div class="mt-2 whitespace-pre-wrap text-sm leading-6 text-zinc-200">${escapeHTML(msg.content)}</div></div>`;
  }).join("");
  return `
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Card chat</h3>
      <div class="scrollbar mt-3 max-h-[320px] space-y-2 overflow-y-auto pr-1">${messages || `<p class="text-sm text-zinc-500">No messages yet.</p>`}</div>
      <form data-action="submit->scrum#sendChat" data-card-id="${escapeHTML(card.id)}" class="mt-3 flex gap-2">
        <textarea data-scrum-field="chatMessage" rows="2" placeholder="Ask the thinking pilot…" class="scrollbar min-w-0 flex-1 resize-none rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40"></textarea>
        <button type="submit" class="self-end rounded-md bg-cyan-300 px-3 py-2 text-xs font-semibold text-zinc-950 hover:bg-cyan-200">Send</button>
      </form>
    </section>
  `;
}

export function renderScrumModalSidebar(
  card: ScrumCard,
  board: ScrumBoard,
  files: string[] = [],
  modelFields: ModelFieldDefinition[] = [],
  resolvedModelSource = "env",
  agentFields: AgentFieldDefinition[] = [],
  resolvedAgentSource = "env",
  resolvedAgentSystem = "omnidex",
  playQueue?: ScrumBoardResponse["play_queue"],
): string {
  const hasJob = Boolean(card.job_id?.trim());
  const stateBadge = playStateBadge(card);
  const moveOptions = SCRUM_COLUMNS.map((col) => `<option value="${escapeHTML(col)}"${col === card.column ? " selected" : ""}>${escapeHTML(COLUMN_LABELS[col] ?? col)}</option>`).join("");
  const fileOptions = files.filter((file) => !(card.ref_files ?? []).includes(file)).slice(0, 80).map((file) => `<option value="${escapeHTML(file)}">${escapeHTML(file)}</option>`).join("");
  return `
    <aside class="space-y-4">
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Status</h3>
        <div class="mt-3 flex flex-wrap items-center gap-2"><span class="${statusPillClass(card.column)}">${escapeHTML(COLUMN_LABELS[card.column] ?? card.column)}</span>${stateBadge}</div>
        <select data-action="change->scrum#modalMoveSelect" data-card-id="${escapeHTML(card.id)}" class="mt-3 w-full rounded-md border border-white/10 bg-zinc-900 px-2 py-2 text-sm text-zinc-100 outline-none">${moveOptions}</select>
      </section>
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Omnidex</h3>
        <p class="mt-2 font-mono text-xs text-zinc-400">${escapeHTML(board.project_directory || "not set")}</p>
        ${hasJob ? `<p class="mt-2 font-mono text-xs text-cyan-200">Job #${escapeHTML(card.job_id)}</p>` : ""}
        ${(playQueue?.queued_count ?? 0) > 0 || playQueue?.running_card_id ? `<p class="mt-2 text-xs text-violet-200">${playQueue?.running_card_id ? "Omnidex is running a card" : "Play queue idle"}${(playQueue?.queued_count ?? 0) > 0 ? ` · ${playQueue?.queued_count} queued` : ""}</p>` : ""}
        <div class="mt-3 flex flex-wrap gap-2">
          ${renderModalPlayActions(card, playQueue)}
          ${hasJob && card.column !== "done" ? `<button type="button" data-action="scrum#syncJob" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200">Sync job</button>` : ""}
        </div>
      </section>
      ${
        modelFields.length
          ? renderModelConfigSection(modelFields, card.model_config ?? {}, resolvedModelSource, "card", card.id)
          : ""
      }
      ${
        agentFields.length
          ? renderAgentConfigSection(
              agentFields,
              card.agent_config ?? {},
              resolvedAgentSource,
              resolvedAgentSystem,
              "card",
              card.id,
            )
          : ""
      }
      ${fileOptions ? `<section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4"><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Reference files</h3><form data-action="submit->scrum#addRefFile" data-card-id="${escapeHTML(card.id)}" class="mt-3 space-y-2"><select data-scrum-field="refFile" class="w-full rounded-md border border-white/10 bg-zinc-900 px-2 py-2 text-xs text-zinc-100 outline-none"><option value="">Pick project file…</option>${fileOptions}</select><button type="submit" class="w-full rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200">Attach file</button></form></section>` : ""}
      ${card.console_log ? `<section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4"><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Console</h3><pre class="scrollbar mt-3 max-h-48 overflow-auto whitespace-pre-wrap rounded bg-black/40 p-3 font-mono text-[11px] text-zinc-300">${escapeHTML(card.console_log)}</pre></section>` : ""}
    </aside>
  `;
}

export function renderScrumCardModal(
  card: ScrumCard,
  board: ScrumBoard,
  files: string[] = [],
  modelFields: ModelFieldDefinition[] = [],
  resolvedModelSource = "env",
  agentFields: AgentFieldDefinition[] = [],
  resolvedAgentSource = "env",
  resolvedAgentSystem = "omnidex",
): string {
  return `
    <div class="border-b border-white/10 p-4 md:p-5">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div><div class="font-mono text-xs text-cyan-200">${escapeHTML(card.id)}</div><h2 class="mt-1 text-2xl font-semibold tracking-tight text-zinc-100">${escapeHTML(card.title)}</h2></div>
        <div class="flex gap-2"><span class="${statusPillClass(card.column)}">${escapeHTML(COLUMN_LABELS[card.column] ?? card.column)}</span><button type="button" data-action="scrum#closeModal" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300">Close</button></div>
      </div>
    </div>
    <div class="grid min-h-[420px] grid-cols-1 gap-0 lg:grid-cols-[minmax(0,1fr)_320px]">
      <div class="scrollbar space-y-4 overflow-y-auto p-4 md:p-5">
        <div data-recyclr-sink="scrum-modal-details">${renderScrumModalDetails(card)}</div>
        <div data-recyclr-sink="scrum-modal-checklist">${renderScrumModalChecklist(card)}</div>
        <div data-recyclr-sink="scrum-modal-chat">${renderScrumModalChat(card)}</div>
      </div>
      <div class="scrollbar border-t border-white/10 bg-zinc-950/35 p-4 lg:border-l lg:border-t-0 md:p-5" data-recyclr-sink="scrum-modal-sidebar">${renderScrumModalSidebar(card, board, files, modelFields, resolvedModelSource, agentFields, resolvedAgentSource, resolvedAgentSystem)}</div>
    </div>
  `;
}
