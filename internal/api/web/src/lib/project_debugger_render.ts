import { escapeHTML, formatDateTime } from "./dom";
import type { DebuggerLastRun } from "./project_types";

export type DebuggerModalState = {
  projectID: number;
  projectName: string;
  agentSystem: string;
  agentSource: string;
  lastRun: DebuggerLastRun | null;
  running: boolean;
  statusText: string;
};

function agentLabel(system: string): string {
  switch (system) {
    case "cursor":
      return "Cursor";
    case "codex":
      return "Codex";
    default:
      return "Omnidex";
  }
}

function statusTone(status: string): string {
  switch (status) {
    case "completed":
      return "border-emerald-400/30 bg-emerald-400/10 text-emerald-200";
    case "failed":
    case "canceled":
      return "border-rose-400/30 bg-rose-400/10 text-rose-200";
    case "running":
    case "pending":
      return "border-cyan-300/30 bg-cyan-300/10 text-cyan-100";
    default:
      return "border-white/10 bg-zinc-900/60 text-zinc-300";
  }
}

function renderCardsCreated(lastRun: DebuggerLastRun | null): string {
  const cards = lastRun?.cards_created ?? [];
  if (!cards.length) {
    return `<p class="text-sm text-zinc-500">No bug tickets created yet.</p>`;
  }
  return cards
    .map((card) => {
      const severity = card.severity
        ? `<span class="rounded-full border border-amber-300/30 bg-amber-300/10 px-2 py-0.5 text-[10px] uppercase tracking-wide text-amber-200">${escapeHTML(card.severity)}</span>`
        : "";
      return `
        <div class="rounded-lg border border-white/10 bg-zinc-950/60 px-3 py-2">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm font-medium text-zinc-100">${escapeHTML(card.title)}</span>
            ${severity}
          </div>
          <p class="mt-1 font-mono text-[11px] text-zinc-500">${escapeHTML(card.id)}</p>
        </div>
      `;
    })
    .join("");
}

function renderSuggestions(lastRun: DebuggerLastRun | null): string {
  const items = lastRun?.suggestions ?? [];
  if (!items.length) return "";
  return `
    <section class="space-y-2">
      <h4 class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Suggestions</h4>
      <ul class="list-disc space-y-1 pl-5 text-sm text-zinc-300">
        ${items.map((item) => `<li>${escapeHTML(item)}</li>`).join("")}
      </ul>
    </section>
  `;
}

export function renderProjectDebuggerModal(state: DebuggerModalState): string {
  const lastRun = state.lastRun;
  const status = state.running ? "running" : lastRun?.status || "idle";
  const summary = state.statusText || lastRun?.summary || "Run a scan to have your project agent review the codebase map and backlog for bugs.";
  const runLabel = state.running ? "Scanning…" : "Run debugger";
  const completedAt = lastRun?.completed_at ? formatDateTime(lastRun.completed_at) : "";
  const jobLine = lastRun?.job_id ? `Job #${lastRun.job_id}` : "No runs yet";

  return `
    <div class="border-b border-white/10 p-4 md:p-5">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p class="text-xs uppercase tracking-[.20em] text-cyan-200/80">Project debugger</p>
          <h2 class="mt-1 text-2xl font-semibold tracking-tight text-zinc-100">${escapeHTML(state.projectName)}</h2>
          <p class="mt-1 text-sm text-zinc-500">Scan for bugs and quality issues, then auto-create scrum bug tickets on the backlog.</p>
        </div>
        <button type="button" data-action="projects#closeDebuggerModal" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300">Close</button>
      </div>
    </div>
    <div class="omni-modal-body scrollbar space-y-5 p-4 md:p-5">
      <section class="grid gap-3 md:grid-cols-3">
        <div class="rounded-xl border border-white/10 bg-zinc-950/60 p-4">
          <p class="text-[11px] uppercase tracking-[.16em] text-zinc-500">Agent</p>
          <p class="mt-2 text-sm font-semibold text-zinc-100">${escapeHTML(agentLabel(state.agentSystem))}</p>
          <p class="mt-1 text-xs text-zinc-500">Source: ${escapeHTML(state.agentSource)}</p>
        </div>
        <div class="rounded-xl border border-white/10 bg-zinc-950/60 p-4">
          <p class="text-[11px] uppercase tracking-[.16em] text-zinc-500">Status</p>
          <p class="mt-2"><span class="rounded-full border px-2.5 py-1 text-[11px] font-semibold uppercase tracking-wide ${statusTone(status)}">${escapeHTML(status)}</span></p>
          <p class="mt-2 text-xs text-zinc-500">${escapeHTML(jobLine)}</p>
        </div>
        <div class="rounded-xl border border-white/10 bg-zinc-950/60 p-4">
          <p class="text-[11px] uppercase tracking-[.16em] text-zinc-500">Findings</p>
          <p class="mt-2 text-2xl font-semibold text-zinc-100">${lastRun?.findings_count ?? 0}</p>
          <p class="mt-1 text-xs text-zinc-500">${lastRun?.cards_created?.length ?? 0} ticket(s) created</p>
        </div>
      </section>

      <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-4">
        <h3 class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Scan summary</h3>
        <p data-debugger-summary class="mt-3 whitespace-pre-wrap text-sm leading-6 text-zinc-300">${escapeHTML(summary)}</p>
        ${completedAt ? `<p class="mt-2 text-xs text-zinc-500">Completed ${escapeHTML(completedAt)}</p>` : ""}
        ${lastRun?.error ? `<p class="mt-2 text-sm text-rose-300">${escapeHTML(lastRun.error)}</p>` : ""}
      </section>

      ${renderSuggestions(lastRun)}

      <section class="space-y-3">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <h3 class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Created bug tickets</h3>
          <button
            type="button"
            data-action="projects#runDebugger"
            data-project-id="${state.projectID}"
            data-debugger-run
            class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200 disabled:cursor-not-allowed disabled:opacity-60"
            ${state.running ? "disabled" : ""}
          >${escapeHTML(runLabel)}</button>
        </div>
        <div data-debugger-cards class="space-y-2">${renderCardsCreated(lastRun)}</div>
      </section>

      <p class="text-xs text-zinc-500">Uses the project codebase map, scrum board, and your configured execution agent. New tickets are tagged <span class="font-mono text-zinc-400">bug</span> and <span class="font-mono text-zinc-400">debugger</span>.</p>
    </div>
  `;
}
