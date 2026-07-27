import { jsonRequest, readJSON } from "./api";
import { emptyState, escapeHTML, formatDateTime, statusPillClass } from "./dom";
import { renderContext, renderJobsPanel, renderStep, renderStepSummary } from "./render";
import type { JobContext } from "./types";

export interface ChatJobsHost {
  queueEnabled(): boolean;
  jobFilter(): HTMLSelectElement;
  hasJobDetails(): boolean;
  jobDetails(): HTMLElement;
  hasJobBadge(): boolean;
  jobBadge(): HTMLElement;
  setCurrentJobID(id: number | string | null): void;
  recycle(target: string, html: string): void;
  indexContexts(contexts: JobContext[]): void;
  addEvent(type: string, details?: Record<string, unknown>, full?: unknown): void;
}

export function authoritativeControlJobID(payload: unknown): string {
  if (!payload || typeof payload !== "object") throw new Error("Job control response must be an object.");
  const job = (payload as { job?: unknown }).job;
  if (!job || typeof job !== "object") throw new Error("Job control response is missing the authoritative job.");
  const id = (job as { id?: unknown }).id;
  if (typeof id !== "number" || !Number.isSafeInteger(id) || id <= 0) {
    throw new Error("Job control response contains an invalid authoritative job id.");
  }
  return String(id);
}

export class ChatJobsCoordinator {
  constructor(private readonly host: ChatJobsHost) {}

  async load(options: { quiet?: boolean; strict?: boolean } = {}): Promise<void> {
    if (!this.host.queueEnabled()) {
      this.setListOutput(emptyState("Queue routes are disabled in wrapper-only mode."));
      if (this.host.hasJobDetails()) {
        this.host.jobDetails().textContent = "Start the core server with DATABASE_URL and WRAPPER_ONLY=false to use job controls.";
      }
      return;
    }
    if (!options.quiet) this.setListOutput(emptyState("Loading jobs…"));
    try {
      const status = this.host.jobFilter().value;
      const query = new URLSearchParams({ limit: "30" });
      if (status) query.set("status", status);
      const [jobsPayload, activityPayload] = await Promise.all([
        readJSON(await fetch(`/v1/jobs?${query}`)),
        readJSON(await fetch("/v1/activity?limit=30")),
      ]);
      const jobs = jobsPayload.jobs || [];
      const activity = activityPayload.llm_activity || [];
      this.setListOutput(renderJobsPanel(jobs, activity));
      this.host.addEvent("jobs_loaded", { count: jobs.length, llm_activity: activity.length, status: status || "all" });
    } catch (error) {
      this.setListOutput(errorPanel(error));
      if (!options.quiet) this.host.addEvent("jobs_failed", { error: errorMessage(error) });
      if (options.strict) throw error;
    }
  }

  render(jobs: unknown[]): void {
    this.setListOutput(renderJobsPanel(jobs, []));
  }

  async select(event: Event): Promise<void> {
    const id = (event.currentTarget as HTMLElement).dataset.jobId;
    if (!id) throw new Error("Selected job did not include its id.");
    const details = await readJSON(await fetch(`/v1/jobs/${id}`));
    const jobID = details.job?.id;
    if (!jobID) throw new Error(`Job response for ${id} did not include a job record.`);
    this.host.setCurrentJobID(jobID);
    if (this.host.hasJobBadge()) this.host.jobBadge().textContent = `#${jobID}`;
    this.renderDetails(details);
  }

  renderDetails(details: Record<string, any>): void {
    const job = details.job || {};
    const steps = details.steps || [];
    const contexts = details.contexts || [];
    this.host.indexContexts(contexts);
    this.host.recycle("job-details", `
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div class="font-mono text-xs text-cyan-200">#${job.id || ""}</div>
          <h3 class="mt-1 text-lg font-semibold text-zinc-100">${escapeHTML(job.instruction || "Untitled job")}</h3>
          <p class="mt-1 text-xs text-zinc-500">${escapeHTML(job.pipeline || "")} · ${formatDateTime(job.created_at)}</p>
        </div>
        <span class="${statusPillClass(job.status)}">${escapeHTML(job.status || "unknown")}</span>
      </div>
      <div class="mt-4 flex flex-wrap gap-2">
        <button data-action="chat#interruptJob" data-job-id="${job.id}" class="rounded-md border border-amber-300/30 bg-amber-300/10 px-3 py-2 text-xs font-semibold text-amber-100">Interrupt</button>
        <button data-action="chat#replanJob" data-job-id="${job.id}" class="rounded-md border border-cyan-300/30 bg-cyan-300/10 px-3 py-2 text-xs font-semibold text-cyan-100">Replan</button>
        <button data-action="chat#cancelJob" data-job-id="${job.id}" class="rounded-md border border-rose-300/30 bg-rose-300/10 px-3 py-2 text-xs font-semibold text-rose-100">Cancel</button>
      </div>
      ${job.result ? `<section class="mt-5"><h4 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Result</h4><pre class="mt-2 whitespace-pre-wrap rounded-md bg-white/[.04] p-3 text-sm text-zinc-200">${escapeHTML(job.result)}</pre></section>` : ""}
      ${job.error ? `<section class="mt-5"><h4 class="text-xs font-semibold uppercase tracking-[.18em] text-rose-300">Error</h4><pre class="mt-2 whitespace-pre-wrap rounded-md bg-rose-400/10 p-3 text-sm text-rose-100">${escapeHTML(job.error)}</pre></section>` : ""}
      <section class="mt-5">
        <div class="flex flex-wrap items-center justify-between gap-3"><h4 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Steps</h4>${renderStepSummary(steps)}</div>
        <div class="mt-3 space-y-3">${steps.map(renderStep).join("") || emptyState("No steps yet.")}</div>
      </section>
      <section class="mt-5"><h4 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Contexts</h4><div class="mt-3 space-y-2">${contexts.slice(-12).map(renderContext).join("") || emptyState("No context records yet.")}</div></section>
    `);
  }

  async interrupt(event: Event): Promise<void> {
    await this.postControl(jobIDFromEvent(event), "interrupt", "Interrupt with what instruction?");
  }

  async replan(event: Event): Promise<void> {
    await this.postControl(jobIDFromEvent(event), "replan", "What should Omni change in the plan?");
  }

  async cancel(event: Event): Promise<void> {
    const reason = window.prompt("Cancel reason?", "Canceled from Omni UI");
    if (!reason) return;
    const id = jobIDFromEvent(event);
    await readJSON(await fetch(`/v1/jobs/${id}/cancel`, jsonRequest({ reason })));
    await this.load();
    this.host.addEvent("job_canceled", { id });
  }

  private async postControl(id: string, action: string, question: string): Promise<void> {
    const feedback = window.prompt(question);
    if (!feedback) return;
    const control = await readJSON(await fetch(`/v1/jobs/${id}/${action}`, jsonRequest({ feedback })));
    const authoritativeID = authoritativeControlJobID(control);
    const details = await readJSON(await fetch(`/v1/jobs/${authoritativeID}`));
    this.renderDetails(details);
    this.host.addEvent(`job_${action}`, { id: authoritativeID, superseded_id: authoritativeID === id ? undefined : id });
  }

  private setListOutput(html: string): void {
    this.host.recycle("jobs-list", html);
  }
}

function jobIDFromEvent(event: Event): string {
  const id = (event.currentTarget as HTMLElement).dataset.jobId;
  if (!id) throw new Error("Job control is missing its job id.");
  return id;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function errorPanel(error: unknown): string {
  return `<div class="rounded border border-rose-300/30 bg-rose-400/10 p-3 text-rose-100">${escapeHTML(errorMessage(error))}</div>`;
}
