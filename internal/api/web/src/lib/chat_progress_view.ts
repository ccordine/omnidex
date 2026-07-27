import { escapeHTML, formatTime, statusPillClass } from "./dom";

export interface ChatProgressViewHost {
  hasProgressState(): boolean;
  progressState(): HTMLElement;
  recycle(target: string, html: string): void;
}

export function renderChatProgressActivity(host: ChatProgressViewHost, label: string): void {
  const text = label.trim() || "Working…";
  if (host.hasProgressState()) host.progressState().textContent = text;
  host.recycle(
    "progress",
    `<div class="flex items-center gap-2 text-sm text-cyan-100"><span class="inline-block h-2 w-2 animate-pulse rounded-full bg-cyan-300"></span><span>${escapeHTML(text)}</span></div>`,
  );
}

export function renderChatProgress(host: ChatProgressViewHost, details: Record<string, any> | null = null): void {
  if (!details?.job) {
    if (host.hasProgressState()) host.progressState().textContent = "idle";
    host.recycle("progress", `<div class="text-sm text-zinc-500">No active job.</div>`);
    return;
  }
  const job = details.job;
  const steps = details.steps || [];
  const contexts = details.contexts || [];
  const latestStep = [...steps].reverse().find((step) => step.status) || steps[steps.length - 1] || {};
  const runningStep = steps.find((step) => step.status === "running") || latestStep;
  const latestContext = contexts[contexts.length - 1] || {};
  if (host.hasProgressState()) host.progressState().textContent = job.status || "running";
  host.recycle("progress", `
    <div class="space-y-3">
      <div class="flex items-center justify-between gap-3"><span class="font-mono text-xs text-cyan-200">#${escapeHTML(job.id || "")}</span><span class="${statusPillClass(job.status)}">${escapeHTML(job.status || "running")}</span></div>
      <div class="grid grid-cols-3 gap-2 text-center text-xs">
        <div class="rounded border border-white/10 bg-white/[.03] p-2"><div class="font-mono text-zinc-100">${steps.length}</div><div class="mt-1 text-zinc-500">steps</div></div>
        <div class="rounded border border-white/10 bg-white/[.03] p-2"><div class="font-mono text-zinc-100">${contexts.length}</div><div class="mt-1 text-zinc-500">contexts</div></div>
        <div class="rounded border border-white/10 bg-white/[.03] p-2"><div class="font-mono text-zinc-100">${formatTime(job.updated_at || new Date().toISOString())}</div><div class="mt-1 text-zinc-500">updated</div></div>
      </div>
      <div class="rounded border border-white/10 bg-white/[.03] p-3">
        <div class="text-xs uppercase tracking-[.16em] text-zinc-500">Current step</div>
        <div class="mt-1 text-sm text-zinc-200">${escapeHTML(runningStep.action || runningStep.status || "waiting for updates")}</div>
        ${runningStep.status ? `<div class="mt-1 text-xs text-zinc-500">${escapeHTML(runningStep.status)}</div>` : ""}
      </div>
      <button type="button" data-action="chat#openContextItem" data-context-id="${escapeHTML(latestContext.id || "")}" class="w-full rounded border border-white/10 bg-white/[.03] p-3 text-left transition hover:border-cyan-300/40 hover:bg-cyan-300/10 ${latestContext.id ? "" : "pointer-events-none opacity-60"}">
        <div class="text-xs uppercase tracking-[.16em] text-zinc-500">Latest context</div>
        <div class="mt-1 font-mono text-xs text-zinc-300">${escapeHTML(latestContext.key || "none")}</div>
      </button>
    </div>
  `);
}
