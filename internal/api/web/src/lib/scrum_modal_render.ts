import { escapeHTML, formatDateTime, statusPillClass } from "./dom";
import { renderChannelChatMessages, renderChatComposer, scrumMessagesToChat } from "./chat_render";
import { renderChannelSurface } from "./channel_render";
import { renderModelConfigSection } from "./model_config_render";
import { renderAgentConfigSection, renderPreAlphaBadge } from "./agent_config_render";
import type { ModelFieldDefinition } from "./model_config_types";
import type { AgentFieldDefinition } from "./agent_config_types";
import type { RecipeCatalogItem } from "./project_types";
import { COLUMN_LABELS, SCRUM_COLUMNS, isPlayControlUnlocked, type ScrumBoard, type ScrumBoardResponse, type ScrumCard, type ScrumCoachConfig, type ScrumCoachSuggestion } from "./scrum_types";

export type ScrumCardTab = "card" | "tests" | "config" | "recipe" | "channel";

const CARD_TABS: Array<{ id: ScrumCardTab; label: string }> = [
  { id: "card", label: "Card" },
  { id: "tests", label: "Tests" },
  { id: "config", label: "Config" },
  { id: "recipe", label: "Recipe" },
  { id: "channel", label: "Channel" },
];

function tabPanelClass(tab: ScrumCardTab, activeTab: ScrumCardTab): string {
  return tab === activeTab ? "" : " hidden";
}

function countBadge(value: number, tone: "cyan" | "violet" | "amber" = "cyan"): string {
  if (value <= 0) return "";
  const tones = {
    cyan: "border-cyan-300/30 bg-cyan-300/10 text-cyan-100",
    violet: "border-violet-300/30 bg-violet-300/10 text-violet-100",
    amber: "border-amber-300/30 bg-amber-300/10 text-amber-100",
  };
  return `<span class="ml-1.5 inline-flex min-w-[1.25rem] items-center justify-center rounded-full border px-1.5 py-0.5 text-[10px] font-semibold ${tones[tone]}">${value}</span>`;
}

function dotBadge(tone: "cyan" | "violet" | "amber" | "emerald" = "cyan"): string {
  const tones = {
    cyan: "bg-cyan-300",
    violet: "bg-violet-300",
    amber: "bg-amber-300",
    emerald: "bg-emerald-300",
  };
  return `<span class="ml-1.5 inline-flex h-2 w-2 rounded-full ${tones[tone]}"></span>`;
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

function tabBadge(card: ScrumCard, tab: ScrumCardTab): string {
  const checklist = card.checklist ?? [];
  const pending = checklist.filter((item) => !item.done).length;
  const refs = (card.ref_files ?? []).length;
  const chatCount = (card.chat ?? []).length;
  const hasConfigOverrides =
    Object.keys(card.model_config ?? {}).length > 0 || Object.keys(card.agent_config ?? {}).length > 0;
  const hasRecipe = Boolean(card.recipe_id?.trim()) || Object.keys(card.recipe ?? {}).length > 0;
  const isLive = card.play_state === "running" || card.play_state === "queued";

  switch (tab) {
    case "card":
      if ((card.planning_chat ?? []).length > 0) return countBadge((card.planning_chat ?? []).length, "violet");
      if (pending > 0) return countBadge(pending, "amber");
      if (refs > 0) return countBadge(refs, "cyan");
      if (card.card_ticket?.trim()) return dotBadge("emerald");
      return "";
    case "tests": {
      const tests = card.test_criteria ?? [];
      const pendingTests = tests.filter((item) => !item.done).length;
      if (pendingTests > 0) return countBadge(pendingTests, "amber");
      if (tests.length > 0) return dotBadge("emerald");
      return "";
    }
    case "config":
      if (isLive) return dotBadge("amber");
      if (hasConfigOverrides) return dotBadge("cyan");
      return "";
    case "recipe":
      return hasRecipe ? dotBadge("violet") : "";
    case "channel":
      if (isLive) return dotBadge("amber");
      if (chatCount > 0) return countBadge(chatCount, "violet");
      if (card.console_log?.trim()) return dotBadge("cyan");
      return "";
    default:
      return "";
  }
}

export function renderScrumModalTabNav(card: ScrumCard, activeTab: ScrumCardTab): string {
  return CARD_TABS.map((tab) => {
    const active = tab.id === activeTab;
    const classes = active
      ? "border-cyan-300/40 bg-cyan-300/10 text-cyan-100"
      : "border-white/10 text-zinc-400 hover:border-cyan-300/30 hover:text-zinc-200";
    return `<button type="button" data-action="scrum#showCardTab" data-scrum-tab="${tab.id}" class="inline-flex items-center rounded-md border px-3 py-2 text-sm font-medium transition ${classes}">${escapeHTML(tab.label)}${tabBadge(card, tab.id)}</button>`;
  }).join("");
}

function renderModalPlayActions(card: ScrumCard, playQueue?: ScrumBoardResponse["play_queue"]): string {
  const playUnlocked = isPlayControlUnlocked(card);
  const isRunning = card.play_state === "running";
  const isQueued = card.play_state === "queued";
  const hasActiveRunner = Boolean(playQueue?.running_card_id);
  const playLabel = hasActiveRunner && !isRunning ? "Queue" : "Play";
  const playEnabledClass = "rounded-md bg-cyan-300 px-3 py-1.5 text-xs font-semibold text-zinc-950 hover:bg-cyan-200";
  const playDisabledClass = "cursor-not-allowed rounded-md border border-white/10 bg-zinc-900/80 px-3 py-1.5 text-xs font-semibold text-zinc-500 opacity-60";
  const pivotEnabledClass = "rounded-md border border-violet-300/30 bg-violet-300/10 px-3 py-1.5 text-xs font-semibold text-violet-100 hover:bg-violet-300/20";
  const pivotDisabledClass = "cursor-not-allowed rounded-md border border-white/10 bg-zinc-900/80 px-3 py-1.5 text-xs font-semibold text-zinc-500 opacity-60";

  const playButton = playUnlocked && !isQueued
    ? `<button type="button" data-action="scrum#play" data-card-id="${escapeHTML(card.id)}" class="${playEnabledClass}">▶ ${playLabel}</button>`
    : `<button type="button" disabled class="${playDisabledClass}" title="Move card to Assigned to play">▶ ${playLabel}</button>`;

  const pivotButton = playUnlocked && hasActiveRunner && !isRunning && !isQueued
    ? `<button type="button" data-action="scrum#pivotPlay" data-card-id="${escapeHTML(card.id)}" class="${pivotEnabledClass}">Play now</button>`
    : hasActiveRunner && !isRunning && !isQueued
      ? `<button type="button" disabled class="${pivotDisabledClass}" title="Move card to Assigned to play">Play now</button>`
      : "";

  const pauseButton = isRunning
    ? `<button type="button" data-action="scrum#pausePlay" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-amber-300/30 bg-amber-300/10 px-3 py-1.5 text-xs font-semibold text-amber-100 hover:bg-amber-300/20">Pause</button>`
    : playUnlocked
      ? `<button type="button" disabled class="cursor-not-allowed rounded-md border border-white/10 bg-zinc-900/80 px-3 py-1.5 text-xs font-semibold text-zinc-500 opacity-60">Pause</button>`
      : `<button type="button" disabled class="cursor-not-allowed rounded-md border border-white/10 bg-zinc-900/80 px-3 py-1.5 text-xs font-semibold text-zinc-500 opacity-60" title="Move card to Assigned to play">Pause</button>`;

  const jumpQueueButton = isQueued && hasActiveRunner
    ? `<button type="button" data-action="scrum#pivotPlay" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-violet-300/30 px-3 py-1.5 text-xs text-violet-100 hover:bg-violet-300/10">Jump queue</button>`
    : "";

  return `${playButton}${pivotButton}${pauseButton}${jumpQueueButton}`;
}

function renderAssignCTA(card: ScrumCard): string {
  if (card.column === "assigned") return "";
  return `<button type="button" data-action="scrum#assignCard" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-violet-300/40 bg-violet-300/15 px-3 py-1.5 text-xs font-semibold text-violet-100 transition hover:border-violet-200/50 hover:bg-violet-300/25">→ Assigned</button>`;
}

export function renderScrumModalToolbar(
  card: ScrumCard,
  board: ScrumBoard,
  playQueue?: ScrumBoardResponse["play_queue"],
): string {
  const hasJob = Boolean(card.job_id?.trim());
  const moveOptions = SCRUM_COLUMNS.map((col) => `<option value="${escapeHTML(col)}"${col === card.column ? " selected" : ""}>${escapeHTML(COLUMN_LABELS[col] ?? col)}</option>`).join("");
  return `
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-white/10 bg-zinc-950/40 px-4 py-3 md:px-5">
      <div class="flex flex-wrap items-center gap-2">
        <span class="${statusPillClass(card.column)}">${escapeHTML(COLUMN_LABELS[card.column] ?? card.column)}</span>
        ${playStateBadge(card)}
        <select data-action="change->scrum#modalMoveSelect" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-white/10 bg-zinc-900 px-2 py-1.5 text-xs text-zinc-100 outline-none">${moveOptions}</select>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        ${renderAssignCTA(card)}
        ${renderModalPlayActions(card, playQueue)}
        ${card.column === "review" ? `<button type="button" data-action="scrum#markDone" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-emerald-400/30 bg-emerald-400/10 px-3 py-1.5 text-xs font-semibold text-emerald-200 hover:bg-emerald-400/20">Mark done</button>` : ""}
        ${hasJob && card.column !== "done" ? `<button type="button" data-action="scrum#syncJob" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200">Sync job</button>` : ""}
        <button type="button" data-action="scrum#deleteCard" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-rose-400/25 px-3 py-1.5 text-xs text-rose-300 hover:bg-rose-400/10">Delete</button>
      </div>
      ${hasJob ? `<p class="w-full font-mono text-[11px] text-cyan-200/80">Job #${escapeHTML(card.job_id ?? "")} · ${escapeHTML(board.project_directory || "not set")}</p>` : ""}
    </div>
  `;
}

export function renderScrumModalDetails(card: ScrumCard): string {
  return `
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <div class="flex items-center justify-between gap-3">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Summary</h3>
        <button type="button" data-action="scrum#saveDetails" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-white/10 px-3 py-1.5 text-xs font-semibold text-zinc-200 transition hover:border-cyan-300/40 hover:bg-cyan-300/10">Save</button>
      </div>
      <input data-scrum-field="title" type="text" value="${escapeHTML(card.title)}" class="mt-3 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-lg font-semibold text-zinc-100 outline-none focus:border-cyan-300/40" />
      <textarea data-scrum-field="description" rows="8" placeholder="Describe the work, context, and acceptance criteria…" class="scrollbar mt-3 w-full resize-y rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm leading-6 text-zinc-200 outline-none focus:border-cyan-300/40">${escapeHTML(card.description || "")}</textarea>
    </section>
  `;
}

export function renderScrumModalChecklist(card: ScrumCard): string {
  const items = (card.checklist ?? []).map((item) => {
    const checked = item.done ? " checked" : "";
    return `
      <div class="flex items-start gap-2 rounded-md border border-white/10 bg-zinc-950/40 px-3 py-2">
        <label class="flex min-w-0 flex-1 items-start gap-3 text-sm text-zinc-200">
          <input type="checkbox" data-action="change->scrum#toggleChecklistItem" data-card-id="${escapeHTML(card.id)}" data-item-id="${escapeHTML(item.id)}" class="mt-1 rounded border-white/20 bg-zinc-900 text-cyan-300"${checked} />
          <span class="${item.done ? "text-zinc-500 line-through" : ""}">${escapeHTML(item.text)}</span>
        </label>
        <button type="button" data-action="scrum#removeChecklistItem" data-card-id="${escapeHTML(card.id)}" data-item-id="${escapeHTML(item.id)}" class="shrink-0 rounded px-1.5 py-0.5 text-xs text-zinc-500 hover:bg-rose-400/10 hover:text-rose-300" title="Remove">×</button>
      </div>
    `;
  }).join("");
  const done = (card.checklist ?? []).filter((item) => item.done).length;
  const total = (card.checklist ?? []).length;
  return `
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <div class="flex items-center justify-between gap-3">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Checklist${total ? ` · ${done}/${total}` : ""}</h3>
      </div>
      <div class="mt-3 space-y-2">${items || `<p class="text-sm text-zinc-500">No checklist items yet. Add tasks Omnidex should complete.</p>`}</div>
      <form data-action="submit->scrum#addChecklistItem" data-card-id="${escapeHTML(card.id)}" class="mt-3 flex gap-2">
        <input data-scrum-field="checklistText" type="text" placeholder="Add checklist item" class="min-w-0 flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
        <button type="submit" class="rounded-md bg-cyan-300 px-3 py-2 text-xs font-semibold text-zinc-950 hover:bg-cyan-200">Add</button>
      </form>
    </section>
  `;
}

export function renderScrumModalRefFiles(card: ScrumCard, files: string[] = []): string {
  const attached = (card.ref_files ?? []).map((file) => `
    <li class="flex items-center justify-between gap-3 rounded-md border border-white/10 bg-zinc-900/50 px-3 py-2">
      <span class="min-w-0 truncate font-mono text-xs text-zinc-200">${escapeHTML(file)}</span>
      <button type="button" data-action="scrum#removeRefFile" data-card-id="${escapeHTML(card.id)}" data-ref-file="${escapeHTML(file)}" class="shrink-0 text-xs text-rose-300 hover:text-rose-200">Remove</button>
    </li>
  `).join("");
  const fileOptions = files.filter((file) => !(card.ref_files ?? []).includes(file)).slice(0, 80).map((file) => `<option value="${escapeHTML(file)}">${escapeHTML(file)}</option>`).join("");
  return `
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Reference files</h3>
      <p class="mt-1 text-xs text-zinc-500">Attach project files Omnidex should read when playing this card.</p>
      <ul class="mt-3 space-y-2">${attached || `<li class="text-sm text-zinc-500">No files attached.</li>`}</ul>
      ${fileOptions ? `<form data-action="submit->scrum#addRefFile" data-card-id="${escapeHTML(card.id)}" class="mt-3 flex flex-wrap gap-2"><select data-scrum-field="refFile" class="min-w-[12rem] flex-1 rounded-md border border-white/10 bg-zinc-900 px-2 py-2 text-xs text-zinc-100 outline-none"><option value="">Pick project file…</option>${fileOptions}</select><button type="submit" class="rounded-md border border-white/10 px-3 py-2 text-xs text-zinc-200 hover:border-cyan-300/40">Attach</button></form>` : ""}
    </section>
  `;
}

export function renderScrumModalCardTicket(card: ScrumCard): string {
  const savedBadge = card.card_ticket?.trim()
    ? `<span class="rounded-full border border-emerald-400/30 bg-emerald-400/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-emerald-200">Saved</span>`
    : "";
  return `
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4" data-scrum-card-ticket-section>
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div class="flex flex-wrap items-center gap-2">
            <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Card ticket draft</h3>
            ${savedBadge}
          </div>
          <p class="mt-1 text-xs text-zinc-500">Generate a work ticket from a prompt. Edits auto-save when you generate or iterate.</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button type="button" data-action="scrum#generateCardTicket" data-card-id="${escapeHTML(card.id)}" data-scrum-pending="card-ticket-generate" data-scrum-pending-label="Generate" class="rounded-md bg-violet-300 px-3 py-1.5 text-xs font-semibold text-zinc-950 hover:bg-violet-200 disabled:cursor-not-allowed disabled:opacity-60">Generate</button>
          <button type="button" data-action="scrum#iterateCardTicket" data-card-id="${escapeHTML(card.id)}" data-scrum-pending="card-ticket-iterate" data-scrum-pending-label="Iterate" class="rounded-md border border-violet-300/30 bg-violet-300/10 px-3 py-1.5 text-xs font-semibold text-violet-100 hover:bg-violet-300/20 disabled:cursor-not-allowed disabled:opacity-60">Iterate</button>
          <button type="button" data-action="scrum#saveCardTicket" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200 hover:border-cyan-300/40">Save draft</button>
          <span data-scrum-pending-status="card-ticket-generate" class="hidden inline-flex items-center gap-1.5 text-[11px] text-zinc-500" aria-live="polite"><span class="hidden inline-block h-3 w-3 shrink-0 animate-spin rounded-full border-2 border-violet-300/25 border-t-violet-200" data-scrum-pending-spinner></span><span data-scrum-pending-text></span></span>
          <span data-scrum-pending-status="card-ticket-iterate" class="hidden inline-flex items-center gap-1.5 text-[11px] text-zinc-500" aria-live="polite"><span class="hidden inline-block h-3 w-3 shrink-0 animate-spin rounded-full border-2 border-violet-300/25 border-t-violet-200" data-scrum-pending-spinner></span><span data-scrum-pending-text></span></span>
        </div>
      </div>
      <textarea data-scrum-field="cardPromptDraft" rows="3" placeholder="Card prompt — what should the ticket cover?" class="scrollbar mt-3 w-full resize-y rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-violet-300/40">${escapeHTML(card.card_prompt || "")}</textarea>
      <textarea data-scrum-field="cardIterateNotes" rows="2" placeholder="Iterate notes — what to change in the draft below?" class="scrollbar mt-3 w-full resize-y rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-violet-300/40"></textarea>
      <textarea data-scrum-field="cardTicket" rows="12" placeholder="Generated card ticket markdown streams here…" class="scrollbar mt-3 w-full resize-y rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs leading-5 text-zinc-100 outline-none focus:border-violet-300/40">${escapeHTML(card.card_ticket || "")}</textarea>
    </section>
  `;
}

export function renderScrumTagPills(card: ScrumCard | null | undefined, editable = true): string {
  if (!card) {
    return `<p class="text-xs text-zinc-600">No tags yet. Add or suggest tags to build project memory.</p>`;
  }
  const tags = card.tags ?? [];
  if (!tags.length) {
    return `<p class="text-xs text-zinc-600">No tags yet. Add or suggest tags to build project memory.</p>`;
  }
  return `<div class="flex flex-wrap gap-1.5">${tags.map((tag) => {
    const remove = editable
      ? `<button type="button" data-action="scrum#removeCardTag" data-card-id="${escapeHTML(card.id)}" data-tag="${escapeHTML(tag)}" class="ml-1 rounded-full px-1 text-zinc-400 hover:bg-rose-400/20 hover:text-rose-200" title="Remove tag">×</button>`
      : "";
    return `<span class="inline-flex items-center rounded-full border border-cyan-300/25 bg-cyan-300/10 px-2 py-0.5 text-[10px] font-medium text-cyan-100">${escapeHTML(tag)}${remove}</span>`;
  }).join("")}</div>`;
}

export function renderScrumTagSuggestions(options: string[] = []): string {
  return options.map((tag) => `<option value="${escapeHTML(tag)}"></option>`).join("");
}

export function renderScrumModalTagsPanel(card: ScrumCard): string {
  return `
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Tags</h3>
          <p class="mt-1 text-[11px] leading-5 text-zinc-500">Stack labels for memory, research, and similar work later.</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button type="button" data-action="scrum#suggestCardTags" data-card-id="${escapeHTML(card.id)}" data-scrum-pending="tags-suggest" data-scrum-pending-label="Suggest" class="rounded-md border border-violet-300/30 bg-violet-300/10 px-2.5 py-1 text-[11px] font-semibold text-violet-100 hover:bg-violet-300/20 disabled:cursor-not-allowed disabled:opacity-60">Suggest</button>
          <span data-scrum-pending-status="tags-suggest" class="hidden inline-flex items-center gap-1.5 text-[11px] text-zinc-500" aria-live="polite"><span class="hidden inline-block h-3 w-3 shrink-0 animate-spin rounded-full border-2 border-violet-300/25 border-t-violet-200" data-scrum-pending-spinner></span><span data-scrum-pending-text></span></span>
        </div>
      </div>
      <div class="mt-3" data-recyclr-sink="scrum-card-tags">${renderScrumTagPills(card)}</div>
      <form data-action="submit->scrum#addCardTag" data-card-id="${escapeHTML(card.id)}" class="mt-3 flex gap-2">
        <input data-scrum-field="tagInput" type="text" list="scrum-tag-suggestions" placeholder="Search or add tag…" autocomplete="off" data-action="input->scrum#filterTagSuggestions" class="min-w-0 flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
        <datalist id="scrum-tag-suggestions" data-recyclr-sink="scrum-tag-suggestions"></datalist>
        <button type="submit" class="rounded-md bg-cyan-300 px-3 py-2 text-xs font-semibold text-zinc-950 hover:bg-cyan-200">Add</button>
      </form>
    </section>
  `;
}

export function renderScrumModalTestCriteria(card: ScrumCard): string {
  const items = (card.test_criteria ?? []).map((item) => {
    const checked = item.done ? " checked" : "";
    return `
      <div class="flex items-start gap-2 rounded-md border border-emerald-400/15 bg-emerald-400/5 px-3 py-2">
        <label class="flex min-w-0 flex-1 items-start gap-3 text-sm text-zinc-200">
          <input type="checkbox" data-action="change->scrum#toggleTestCriterion" data-card-id="${escapeHTML(card.id)}" data-item-id="${escapeHTML(item.id)}" class="mt-1 rounded border-white/20 bg-zinc-900 text-emerald-300"${checked} />
          <span class="${item.done ? "text-zinc-500 line-through" : ""}">${escapeHTML(item.text)}</span>
        </label>
        <button type="button" data-action="scrum#removeTestCriterion" data-card-id="${escapeHTML(card.id)}" data-item-id="${escapeHTML(item.id)}" class="shrink-0 rounded px-1.5 py-0.5 text-xs text-zinc-500 hover:bg-rose-400/10 hover:text-rose-300" title="Remove">×</button>
      </div>
    `;
  }).join("");
  const done = (card.test_criteria ?? []).filter((item) => item.done).length;
  const total = (card.test_criteria ?? []).length;
  return `
    <section class="rounded-lg border border-emerald-400/20 bg-zinc-950/50 p-4">
      <div class="flex items-center justify-between gap-3">
        <div>
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-emerald-400/80">Test criteria</h3>
          <p class="mt-1 text-xs text-zinc-500">Tests the AI should implement or satisfy before this card is done.${total ? ` · ${done}/${total} passing` : ""}</p>
        </div>
      </div>
      <div class="mt-3 space-y-2">${items || `<p class="text-sm text-zinc-500">No tests defined. Add unit, integration, or manual verification steps.</p>`}</div>
      <form data-action="submit->scrum#addTestCriterion" data-card-id="${escapeHTML(card.id)}" class="mt-3 flex gap-2">
        <input data-scrum-field="testCriterionText" type="text" placeholder="e.g. go test ./internal/api passes" class="min-w-0 flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-emerald-300/40" />
        <button type="submit" class="rounded-md border border-emerald-400/30 bg-emerald-400/10 px-3 py-2 text-xs font-semibold text-emerald-100 hover:bg-emerald-400/20">Add</button>
      </form>
    </section>
  `;
}

export function renderScrumCoachTags(card: ScrumCard): string {
  const tags = card.tags ?? [];
  if (!tags.length) return "";
  return `<div class="flex flex-wrap gap-1.5">${tags.map((tag) => `<span class="rounded-full border border-cyan-300/25 bg-cyan-300/10 px-2 py-0.5 text-[10px] font-medium text-cyan-100">${escapeHTML(tag)}</span>`).join("")}</div>`;
}

export function renderScrumCoachToasts(suggestions: ScrumCoachSuggestion[] = []): string {
  if (!suggestions.length) {
    return `<p class="text-xs text-zinc-600">Coach suggestions appear here as you edit.</p>`;
  }
  return suggestions.map((item) => {
    const tone = item.level === "warn" ? "border-amber-300/30 bg-amber-300/10 text-amber-100" : item.level === "tip" ? "border-violet-300/30 bg-violet-300/10 text-violet-100" : "border-cyan-300/25 bg-cyan-300/10 text-cyan-100";
    return `<div class="rounded-md border px-3 py-2 text-xs leading-5 ${tone}">${escapeHTML(item.text)}</div>`;
  }).join("");
}

function coachConfig(card: ScrumCard): ScrumCoachConfig {
  return {
    enabled: card.coach_config?.enabled !== false,
    auto_scan: card.coach_config?.auto_scan !== false,
    model: card.coach_config?.model || "qwen3:4b-thinking",
  };
}

export function renderScrumCoachChat(card: ScrumCard): string {
  const messages = (card.planning_chat ?? []).map((msg) => {
    const shell = msg.role === "user" ? "border-cyan-300/25 bg-cyan-300/10" : "border-white/10 bg-zinc-900/70";
    return `<div class="rounded-md border ${shell} px-3 py-2"><div class="text-[10px] uppercase tracking-wide text-zinc-500">${escapeHTML(msg.role)}</div><div class="mt-1 whitespace-pre-wrap text-xs leading-5 text-zinc-200">${escapeHTML(msg.content)}</div></div>`;
  }).join("");
  return messages || `<p class="text-xs text-zinc-500">Ask the coach to refine scope, split work, or draft a card ticket prompt.</p>`;
}

export function renderScrumCoachPanel(card: ScrumCard): string {
  const cfg = coachConfig(card);
  return `
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Card coach</h3>
          <p class="mt-1 text-[11px] leading-5 text-zinc-500">Meta-planning for this card only. Try <span class="font-mono text-zinc-400">/plan</span> <span class="font-mono text-zinc-400">/research</span> <span class="font-mono text-zinc-400">/card</span> <span class="font-mono text-zinc-400">/scan</span></p>
        </div>
        <label class="flex items-center gap-2 text-xs text-zinc-300"><input type="checkbox" data-scrum-field="coachEnabled" class="rounded border-white/20 bg-zinc-900 text-cyan-300"${cfg.enabled ? " checked" : ""} /> On</label>
      </div>
      <div class="mt-3 grid gap-2 sm:grid-cols-2">
        <label class="flex items-center gap-2 text-xs text-zinc-400"><input type="checkbox" data-scrum-field="coachAutoScan" class="rounded border-white/20 bg-zinc-900 text-cyan-300"${cfg.auto_scan ? " checked" : ""} /> Auto-scan while editing</label>
        <label class="block text-xs text-zinc-500">Model<input data-scrum-field="coachModel" type="text" value="${escapeHTML(cfg.model || "")}" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-2 py-1.5 font-mono text-[11px] text-zinc-100 outline-none focus:border-cyan-300/40" /></label>
      </div>
      <button type="button" data-action="scrum#saveCoachConfig" data-card-id="${escapeHTML(card.id)}" class="mt-2 rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200 hover:border-cyan-300/40">Save coach settings</button>
      <div class="scrollbar mt-3 max-h-36 space-y-2 overflow-y-auto pr-1" data-recyclr-sink="scrum-coach-toasts"><p class="text-xs text-zinc-600">Coach suggestions appear here as you edit.</p></div>
      <div class="scrollbar mt-3 max-h-52 space-y-2 overflow-y-auto pr-1" data-recyclr-sink="scrum-coach-chat">${renderScrumCoachChat(card)}</div>
      <form data-action="submit->scrum#sendCoach" data-card-id="${escapeHTML(card.id)}" class="mt-3 flex gap-2">
        <textarea data-scrum-field="coachMessage" rows="2" placeholder="Talk to the coach… /plan /research /card" class="scrollbar min-w-0 flex-1 resize-none rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40"></textarea>
        <button type="submit" class="self-end rounded-md bg-cyan-300 px-3 py-2 text-xs font-semibold text-zinc-950 hover:bg-cyan-200">Send</button>
      </form>
    </section>
  `;
}

export function renderScrumModalTestsTab(card: ScrumCard): string {
  return `
    <div class="mx-auto max-w-3xl space-y-4">
      <p class="text-sm leading-6 text-zinc-400">Define what “done” means for this card. Play and channel runs include these criteria in agent context; check them off as the agent satisfies each one.</p>
      ${renderScrumModalTestCriteria(card)}
    </div>
  `;
}

export function renderScrumModalCardTab(card: ScrumCard, files: string[] = []): string {
  return `
    <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(280px,360px)]">
      <div class="space-y-4">
        ${renderScrumModalDetails(card)}
        ${renderScrumModalChecklist(card)}
        ${renderScrumModalRefFiles(card, files)}
        <div data-recyclr-sink="scrum-card-ticket">${renderScrumModalCardTicket(card)}</div>
      </div>
      <div class="space-y-4 xl:sticky xl:top-0 xl:self-start">
        ${renderScrumModalTagsPanel(card)}
        ${renderScrumCoachPanel(card)}
      </div>
    </div>
  `;
}

export function renderScrumModalConfigTab(
  card: ScrumCard,
  modelFields: ModelFieldDefinition[] = [],
  resolvedModelSource = "env",
  agentFields: AgentFieldDefinition[] = [],
  resolvedAgentSource = "env",
  resolvedAgentSystem = "omnidex",
): string {
  const usingCursor = resolvedAgentSystem === "cursor" || card.agent_config?.agent_system === "cursor";
  const usingOmnidex = resolvedAgentSystem === "omnidex" || card.agent_config?.agent_system === "omnidex";
  return `
    <div class="space-y-4">
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Execution layer</h3>
        <p class="mt-2 text-sm leading-6 text-zinc-400">Play runs the resolved agent (card → project → env) with full card context: title, description, checklist, card ticket draft, ref files, and recipe. A programmatic manager reads <span class="font-mono text-zinc-300">SCRUM_STATUS:</span> from agent output to move the card to review, blocked, or back to assigned.</p>
        <div class="mt-3 flex flex-wrap items-center gap-2">
          <button type="button" data-action="scrum#quickSetAgent" data-card-id="${escapeHTML(card.id)}" data-agent-system="cursor" class="rounded-md border ${usingCursor ? "border-cyan-300/40 bg-cyan-300/10 text-cyan-100" : "border-white/10 text-zinc-200 hover:border-cyan-300/40"} px-3 py-1.5 text-xs font-semibold">Use Cursor</button>
          <button type="button" data-action="scrum#quickSetAgent" data-card-id="${escapeHTML(card.id)}" data-agent-system="codex" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200 hover:border-cyan-300/40">Use Codex</button>
          <button type="button" data-action="scrum#quickSetAgent" data-card-id="${escapeHTML(card.id)}" data-agent-system="omnidex" class="inline-flex items-center gap-2 rounded-md border ${usingOmnidex ? "border-cyan-300/40 bg-cyan-300/10 text-cyan-100" : "border-white/10 text-zinc-200 hover:border-cyan-300/40"} px-3 py-1.5 text-xs font-semibold">Use Omnidex ${renderPreAlphaBadge()}</button>
        </div>
        <p class="mt-3 text-[11px] leading-5 text-zinc-600">Cursor/Codex also need an API key — set under <span class="font-semibold text-zinc-500">Admin → API secrets</span> (DB, preferred) or <span class="font-mono">CURSOR_API_KEY</span> / <span class="font-mono">CODEX_API_KEY</span> in env. Project agent choice overrides env defaults.</p>
      </section>
      <p class="text-xs text-zinc-500">Overrides inherit project → environment.</p>
      ${modelFields.length ? renderModelConfigSection(modelFields, card.model_config ?? {}, resolvedModelSource, "card", card.id) : `<p class="text-sm text-zinc-500">Model config unavailable.</p>`}
      ${agentFields.length ? renderAgentConfigSection(agentFields, card.agent_config ?? {}, resolvedAgentSource, resolvedAgentSystem, "card", card.id) : ""}
    </div>
  `;
}

export function renderScrumModalRecipeTab(
  card: ScrumCard,
  recipes: RecipeCatalogItem[] = [],
  projectRecipeId = "",
  projectRecipe: Record<string, unknown> = {},
): string {
  const effectiveRecipeId = card.recipe_id?.trim() || projectRecipeId;
  const effectiveRecipe = Object.keys(card.recipe ?? {}).length ? card.recipe : projectRecipe;
  const recipeOptions = recipes
    .map((recipe) => {
      const selected = recipe.id === effectiveRecipeId ? " selected" : "";
      return `<option value="${escapeHTML(recipe.id)}"${selected}>${escapeHTML(recipe.id)} — ${escapeHTML(recipe.description)}</option>`;
    })
    .join("");
  const recipeJSON = JSON.stringify(effectiveRecipe ?? {}, null, 2);
  const inherited = !card.recipe_id?.trim() && !Object.keys(card.recipe ?? {}).length && (projectRecipeId || Object.keys(projectRecipe).length);
  return `
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Omnidex recipe</h3>
          <p class="mt-1 text-xs text-zinc-500">${inherited ? "Inheriting project recipe until you save card overrides." : "Card-specific recipe used when this card plays."}</p>
        </div>
        <select data-scrum-field="recipeId" class="max-w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none">
          <option value="">No catalog recipe</option>
          ${recipeOptions}
        </select>
      </div>
      <textarea data-scrum-field="recipeJson" rows="18" class="scrollbar mt-4 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs leading-5 text-zinc-100 outline-none focus:border-cyan-300/40">${escapeHTML(recipeJSON)}</textarea>
      <div class="mt-3 flex flex-wrap gap-2">
        <button type="button" data-action="scrum#loadCatalogRecipe" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-white/10 px-3 py-2 text-xs text-zinc-200 hover:border-cyan-300/40 hover:bg-cyan-300/10">Load catalog template</button>
        <button type="button" data-action="scrum#saveRecipe" data-card-id="${escapeHTML(card.id)}" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Save recipe</button>
      </div>
    </section>
  `;
}

function channelLiveBadge(card: ScrumCard): { label: string; tone: string } {
  if (card.play_state === "running") {
    return { label: "streaming", tone: "border-amber-300/30 bg-amber-300/10 text-amber-100" };
  }
  if (card.play_state === "queued") {
    return { label: "queued", tone: "border-violet-300/30 bg-violet-300/10 text-violet-100" };
  }
  if (card.play_state === "paused") {
    return { label: "paused", tone: "border-zinc-400/30 bg-zinc-400/10 text-zinc-300" };
  }
  if (card.job_id?.trim()) {
    return { label: "has job", tone: "border-cyan-300/25 bg-cyan-300/10 text-cyan-100" };
  }
  return { label: "idle", tone: "border-white/10 bg-white/[.04] text-zinc-400" };
}

export function renderScrumModalChannelTab(
  card: ScrumCard,
  playQueue?: ScrumBoardResponse["play_queue"],
  options?: { pilotPending?: boolean; agentRunning?: boolean },
): string {
  const messages = scrumMessagesToChat(card.chat ?? []);
  const isLive = card.play_state === "running" || card.play_state === "queued";
  const isRunning = card.play_state === "running";
  const pilotPending = Boolean(options?.pilotPending);
  const showPending = pilotPending;
  const pendingLabel = "Sending…";
  const status = channelSessionStatus(card, playQueue);
  const liveBadge = channelLiveBadge(card);
  const interrupt = isRunning
    ? `<button type="button" data-action="scrum#pausePlay" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-rose-400/35 bg-rose-400/10 px-3 py-1.5 text-xs font-semibold text-rose-100 transition hover:bg-rose-400/20">Interrupt</button>`
    : "";
  const sync = card.job_id?.trim() && !isRunning
    ? `<button type="button" data-action="scrum#syncJob" data-card-id="${escapeHTML(card.id)}" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200 transition hover:border-cyan-300/40">Sync job</button>`
    : "";
  const jobLine = card.job_id?.trim()
    ? `<span class="font-mono text-[11px] text-cyan-200/90">Job #${escapeHTML(card.job_id)}</span>`
    : "";
  const messageHtml =
    messages.length > 0 || showPending
      ? renderChannelChatMessages(messages, {
          pending: showPending,
          pendingLabel,
        })
      : `<div class="flex h-full min-h-[12rem] items-center justify-center px-4 py-8 text-center text-sm text-zinc-500">Play this card to watch the agent work — commands, file edits, diffs, thinking, and replies stream here in real time.</div>`;

  return renderChannelSurface({
    eyebrow: "Card channel",
    title: card.title,
    statusHtml: status,
    actionsHtml: `${jobLine}${interrupt}${sync}`,
    badgeHtml: `<span class="rounded-full border px-3 py-1 text-xs font-medium ${liveBadge.tone}">${escapeHTML(liveBadge.label)}</span>`,
    messagesHtml: messageHtml,
    messagesAttrs: "data-scrum-channel-messages",
    messagesClass: "scrum-channel-scroll scrollbar min-h-0 flex-1 overflow-y-auto overflow-x-hidden flex flex-col-reverse gap-1.5 px-3 py-3 md:px-4",
    composerHtml: renderChatComposer({
        formAction: "submit->scrum#sendChat",
        keydownAction: "scrum#channelComposerKeydown",
        cardId: card.id,
        placeholder: isLive ? "Steer the running agent…" : "Message uses this card's Config tab agent and models…",
        disabled: pilotPending,
        submitLabel: pilotPending ? "Sending…" : "Send",
      }),
  });
}

function channelSessionStatus(card: ScrumCard, playQueue?: ScrumBoardResponse["play_queue"]): string {
  if (card.play_state === "running") {
    return `<span class="rounded-full border border-amber-300/30 bg-amber-300/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-200">Live</span>`;
  }
  if (card.play_state === "queued") {
    const position = playQueue?.queued_card_ids?.indexOf(card.id);
    const label = position != null && position >= 0 ? `#${position + 1} in queue` : "Queued";
    return `<span class="rounded-full border border-violet-300/30 bg-violet-300/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-violet-100">${escapeHTML(label)}</span>`;
  }
  if (card.play_state === "paused") {
    return `<span class="rounded-full border border-zinc-400/30 bg-zinc-400/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-zinc-300">Paused</span>`;
  }
  if (card.job_id?.trim()) {
    return `<span class="rounded-full border border-cyan-300/25 bg-cyan-300/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-cyan-100">Has job</span>`;
  }
  return "";
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
  playQueue?: ScrumBoardResponse["play_queue"],
  activeTab: ScrumCardTab = "card",
  recipes: RecipeCatalogItem[] = [],
  projectRecipeId = "",
  projectRecipe: Record<string, unknown> = {},
): string {
  const liveTarget = `scrum-modal-card-live-${escapeHTML(card.id)}`;
  return `
    <div class="shrink-0 border-b border-white/10 p-4 md:p-5" data-scrum-modal-card-id="${escapeHTML(card.id)}">
      <span data-recyclr-sink="${liveTarget}" class="sr-only"></span>
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div class="font-mono text-xs text-cyan-200">${escapeHTML(card.id)}</div>
          <h2 class="mt-1 text-2xl font-semibold tracking-tight text-zinc-100">${escapeHTML(card.title)}</h2>
        </div>
        <button type="button" data-action="scrum#closeModal" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300">Close</button>
      </div>
      <p data-scrum-modal-feedback class="mt-3 hidden rounded-md border px-3 py-2 text-xs leading-5" role="status" aria-live="polite"></p>
    </div>
    <div class="shrink-0" data-recyclr-sink="scrum-modal-toolbar">${renderScrumModalToolbar(card, board, playQueue)}</div>
    <div class="shrink-0 border-b border-white/10 px-4 py-3 md:px-5" data-recyclr-sink="scrum-modal-tabs">
      <nav class="flex flex-wrap gap-2" aria-label="Card sections">${renderScrumModalTabNav(card, activeTab)}</nav>
    </div>
    <div class="omni-modal-body scrollbar p-4 md:p-5">
      <div data-scrum-tab-panel="card" class="${tabPanelClass("card", activeTab)}" data-recyclr-sink="scrum-modal-card">${renderScrumModalCardTab(card, files)}</div>
      <div data-scrum-tab-panel="tests" class="${tabPanelClass("tests", activeTab)}" data-recyclr-sink="scrum-modal-tests">${renderScrumModalTestsTab(card)}</div>
      <div data-scrum-tab-panel="config" class="${tabPanelClass("config", activeTab)}" data-recyclr-sink="scrum-modal-config">${renderScrumModalConfigTab(card, modelFields, resolvedModelSource, agentFields, resolvedAgentSource, resolvedAgentSystem)}</div>
      <div data-scrum-tab-panel="recipe" class="${tabPanelClass("recipe", activeTab)}" data-recyclr-sink="scrum-modal-recipe">${renderScrumModalRecipeTab(card, recipes, projectRecipeId, projectRecipe)}</div>
      <div data-scrum-tab-panel="channel" class="${tabPanelClass("channel", activeTab)}" data-recyclr-sink="scrum-modal-channel">${renderScrumModalChannelTab(card, playQueue)}</div>
    </div>
  `;
}

export function renderScrumCreateCardModal(defaultColumn = "backlog"): string {
  const columnOptions = SCRUM_COLUMNS.map((col) => {
    const selected = col === defaultColumn ? " selected" : "";
    return `<option value="${escapeHTML(col)}"${selected}>${escapeHTML(COLUMN_LABELS[col] ?? col)}</option>`;
  }).join("");
  return `
    <div class="border-b border-white/10 p-4 md:p-5">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p class="text-xs uppercase tracking-[.20em] text-cyan-200/80">Scrum</p>
          <h2 class="mt-1 text-2xl font-semibold tracking-tight text-zinc-100">New card</h2>
        </div>
        <button type="button" data-action="scrum#closeModal" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300">Cancel</button>
      </div>
    </div>
    <form data-action="submit->scrum#createCard" class="omni-modal-body scrollbar space-y-4 p-4 md:p-5">
      <label class="block">
        <span class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Title</span>
        <input data-scrum-field="newTitle" type="text" required autofocus placeholder="What needs doing?" class="mt-2 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
      </label>
      <label class="block">
        <span class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Description</span>
        <textarea data-scrum-field="newDesc" rows="4" placeholder="Optional context for Omnidex" class="scrollbar mt-2 w-full resize-y rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm leading-6 text-zinc-100 outline-none focus:border-cyan-300/40"></textarea>
      </label>
      <label class="block">
        <span class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Column</span>
        <select data-scrum-field="newColumn" class="mt-2 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none">${columnOptions}</select>
      </label>
      <div class="flex justify-end gap-2 border-t border-white/10 pt-4">
        <button type="button" data-action="scrum#closeModal" class="rounded-md border border-white/10 px-4 py-2 text-sm text-zinc-300">Cancel</button>
        <button type="submit" data-scrum-submit="create" class="inline-flex items-center justify-center gap-2 rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200 disabled:cursor-not-allowed disabled:opacity-60">Create card</button>
      </div>
    </form>
  `;
}
