import { Controller } from "@hotwired/stimulus";
import { readJSON, jsonRequest } from "../lib/api";
import { TranscriptStore } from "../lib/transcript_store";
import { escapeHTML, trimText, hashText, formatTime, formatDateTime, badgeClass, statusPillClass, emptyState, sleep } from "../lib/dom";
import {
  renderStep,
  renderStepSummary,
  renderContext,
  renderEventModal,
  renderContextModal,
  renderResearchStatus,
  renderMetricsDashboard,
  contextEventType,
} from "../lib/render";
import type { ChatMessage, TimelineEvent, JobDetails, JobContext, MemoryRecord, MemoryCandidate } from "../lib/types";
import { fetchWorkspace } from "../lib/project_api";
import type GxController from "./gx_controller";

export default class ChatController extends Controller {
  static targets = [
    "messages","timeline","input","send","status","transport","job","liveBadge","eventCount","panel",
    "jobFilter","jobsList","jobDetails","memoryCandidates","memoryList","memoryKind","memoryKindFilter","memoryTags","memoryContent",
    "personaMode","personaModel","personaSystem","personaPrompt","personaOutput","statusOutput","researchStatusOutput",
    "metricsOutput","progress","progressState","spinner","modal","modalPanel",
  ];
  static values = { pollMs: Number };

  declare readonly messagesTarget: HTMLElement;
  declare readonly timelineTarget: HTMLElement;
  declare readonly inputTarget: HTMLTextAreaElement;
  declare readonly sendTarget: HTMLButtonElement;
  declare readonly statusTarget: HTMLElement;
  declare readonly transportTarget: HTMLElement;
  declare readonly jobTarget: HTMLElement;
  declare readonly liveBadgeTarget: HTMLElement;
  declare readonly eventCountTarget: HTMLElement;
  declare readonly panelTargets: HTMLElement[];
  declare readonly jobFilterTarget: HTMLSelectElement;
  declare readonly jobsListTarget: HTMLElement;
  declare readonly jobDetailsTarget: HTMLElement;
  declare readonly memoryCandidatesTarget: HTMLElement;
  declare readonly memoryListTarget: HTMLElement;
  declare readonly memoryKindTarget: HTMLSelectElement;
  declare readonly memoryKindFilterTarget: HTMLSelectElement;
  declare readonly memoryTagsTarget: HTMLInputElement;
  declare readonly memoryContentTarget: HTMLTextAreaElement;
  declare readonly personaModeTarget: HTMLSelectElement;
  declare readonly personaModelTarget: HTMLInputElement;
  declare readonly personaSystemTarget: HTMLTextAreaElement;
  declare readonly personaPromptTarget: HTMLTextAreaElement;
  declare readonly personaOutputTarget: HTMLElement;
  declare readonly statusOutputTarget: HTMLElement;
  declare readonly researchStatusOutputTarget: HTMLElement;
  declare readonly metricsOutputTarget: HTMLElement;
  declare readonly progressTarget: HTMLElement;
  declare readonly progressStateTarget: HTMLElement;
  declare readonly spinnerTarget: HTMLElement;
  declare readonly modalTarget: HTMLElement;
  declare readonly modalPanelTarget: HTMLElement;
  declare readonly hasMemoryListTarget: boolean;
  declare readonly hasResearchStatusOutputTarget: boolean;
  declare readonly hasMetricsOutputTarget: boolean;
  declare readonly hasProgressStateTarget: boolean;
  declare readonly hasModalTarget: boolean;
  declare readonly hasSpinnerTarget: boolean;
  declare readonly pollMsValue: number;

  gxController: GxController | null = null;
  store!: TranscriptStore;
  messages: ChatMessage[] = [];
  events: TimelineEvent[] = [];
  eventSequence = 0;
  eventIndex = new Map<string, TimelineEvent>();
  contextIndex = new Map<string, JobContext>();
  seenProgress = new Set<string>();
  currentJobID: number | string | null = null;
  busy = false;
  queueEnabled = false;
  activityTimer: number | null = null;
  memoryChangedHandler: ((event: Event) => void) | null = null;

  

  

  async connect() {
    this.gxController = this.application.getControllerForElementAndIdentifier(this.element, "gx") as GxController | null;
    this.store = new TranscriptStore();
    this.messages = this.store.load();
    this.events = [];
    this.eventSequence = 0;
    this.eventIndex = new Map();
    this.contextIndex = new Map();
    this.seenProgress = new Set();
    this.currentJobID = null;
    this.busy = false;
    this.renderProgress();
    this.renderMessages();
    this.renderTimeline();
    await this.detectTransport();
    await this.loadStatus();
    await this.loadGlobalActivity();
    this.activityTimer = window.setInterval(() => this.loadGlobalActivity({ quiet: true }), 5000);
    this.memoryChangedHandler = () => void this.loadMemoryCandidates();
    document.addEventListener("omni:memory-changed", this.memoryChangedHandler);
    if (this.messages.length === 0) {
      this.addMessage("system", "Omni UI is ready. Queue mode uses the Go job API; direct mode uses /v1/instruct.");
    }
  }

  disconnect() {
    if (this.activityTimer) window.clearInterval(this.activityTimer);
    if (this.memoryChangedHandler) document.removeEventListener("omni:memory-changed", this.memoryChangedHandler);
  }

  async detectTransport() {
    try {
      const response = await fetch("/healthz");
      const health = await response.json();
      this.queueEnabled = Boolean(health.queue_enabled);
      this.transportTarget.textContent = this.queueEnabled ? "queue jobs" : "direct instruct";
      this.setStatus("ready", "ready");
      this.addEvent("health", health);
    } catch (error) {
      this.queueEnabled = false;
      this.transportTarget.textContent = "offline";
      this.setStatus("offline", "error");
    }
  }

  showPanel(event) {
    const name = event.currentTarget?.dataset?.panel || "chat";
    for (const panel of this.panelTargets) {
      const active = panel.dataset.panelName === name;
      panel.classList.toggle("hidden", !active);
      panel.classList.toggle("flex", active);
    }
    for (const button of this.element.querySelectorAll(".nav-button")) {
      const active = button.dataset.panel === name;
      button.classList.toggle("is-active", active);
      button.classList.toggle("bg-white/[.06]", active);
      button.classList.toggle("text-zinc-100", active);
      button.classList.toggle("text-zinc-300", !active);
    }
    if (name === "jobs") this.loadJobs();
    if (name === "memory") this.loadMemoryCandidates();
    if (name === "metrics") this.loadMetrics();
    if (name === "admin") {
      this.loadStatus();
      this.loadResearchStatus();
    }
    document.dispatchEvent(new CustomEvent("omni:panel-shown", { detail: { panel: name } }));
  }

  composerKeydown(event) {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault();
      this.submit(event);
    }
  }

  async submit(event) {
    event.preventDefault();
    const prompt = this.inputTarget.value.trim();
    if (!prompt || this.busy) return;

    this.inputTarget.value = "";
    this.addMessage("user", prompt);
    this.setBusy(true);

    try {
      if (this.queueEnabled) {
        await this.submitJob(prompt);
      } else {
        await this.submitDirect(prompt);
      }
    } catch (error) {
      this.addMessage("error", error.message || String(error));
      this.addEvent("request_failed", { error: error.message || String(error) });
      this.setBusy(false);
      this.setStatus("failed", "error");
    }
  }

  async submitJob(prompt) {
    this.setStatus("queued", "active");
    const metadata: Record<string, unknown> = {
      source: "omni-web-chat",
      ui: "stimulus-tailwind-recyclr",
    };
    try {
      const workspace = await fetchWorkspace();
      if (workspace.active_project_id > 0) {
        metadata.project_id = workspace.active_project_id;
      }
      if (workspace.project?.location) {
        metadata.client_cwd = workspace.project.location;
        metadata.project_directory = workspace.project.location;
      }
    } catch {
      // workspace API unavailable without database
    }
    const requestBody = {
      instruction: prompt,
      pipeline: "chat",
      metadata,
    };
    const response = await fetch("/v1/jobs", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(requestBody),
    });
    const payload = await readJSON(response);
    const job = payload.job;
    this.currentJobID = job.id;
    this.jobTarget.textContent = `#${job.id}`;
    this.addEvent("job_created", { id: job.id, status: job.status }, { request: requestBody, response: payload, job });
    await this.pollJob(job.id);
  }

  async pollJob(jobID) {
    this.setStatus("running", "active");
    let lastSignature = "";
    for (;;) {
      await sleep(this.pollMsValue || 1600);
      const response = await fetch(`/v1/jobs/${jobID}`);
      const details = await readJSON(response);
      const signature = JSON.stringify({
        status: details.job?.status,
        result: details.job?.result,
        error: details.job?.error,
        steps: (details.steps || []).map((step) => [step.id, step.status, step.output, step.error]),
        contexts: (details.contexts || []).length,
      });
      if (signature !== lastSignature) {
        this.renderJobProgress(details);
        lastSignature = signature;
      }
      const status = details.job?.status;
      if (status === "completed") {
        this.addMessage("assistant", details.job.result || "Completed.");
        this.setStatus("completed", "ready");
        this.setBusy(false);
        return;
      }
      if (status === "failed" || status === "canceled") {
        this.addMessage("error", details.job.error || `Job ${status}.`);
        this.setStatus(status, "error");
        this.setBusy(false);
        return;
      }
    }
  }

  async loadJobs() {
    if (!this.queueEnabled) {
      this.jobsListTarget.innerHTML = emptyState("Queue routes are disabled in wrapper-only mode.");
      this.jobDetailsTarget.textContent = "Start the core server with DATABASE_URL and WRAPPER_ONLY=false to use job controls.";
      return;
    }
    const status = this.jobFilterTarget.value;
    const query = new URLSearchParams({ limit: "30" });
    if (status) query.set("status", status);
    const payload = await readJSON(await fetch(`/v1/jobs?${query}`));
    this.renderJobs(payload.jobs || []);
    this.addEvent("jobs_loaded", { count: (payload.jobs || []).length, status: status || "all" });
  }

  renderJobs(jobs) {
    if (jobs.length === 0) {
      this.recycle("jobs-list", emptyState("No jobs matched this filter."));
      return;
    }
    this.recycle(
      "jobs-list",
      jobs
        .map(
        (job) => `
          <button data-action="chat#selectJob" data-job-id="${job.id}" class="w-full rounded-lg border border-white/10 bg-zinc-950/50 p-3 text-left transition hover:border-cyan-300/40 hover:bg-cyan-300/10">
            <div class="flex items-start justify-between gap-3">
              <div>
                <div class="font-mono text-xs text-cyan-200">#${job.id}</div>
                <div class="mt-1 line-clamp-2 text-sm font-medium text-zinc-100">${escapeHTML(job.instruction)}</div>
              </div>
              <span class="${statusPillClass(job.status)}">${escapeHTML(job.status)}</span>
            </div>
            <div class="mt-2 text-xs text-zinc-500">${escapeHTML(job.pipeline || "assistant")} · ${formatDateTime(job.updated_at)}</div>
          </button>
        `,
        )
        .join(""),
    );
  }

  async selectJob(event) {
    const id = event.currentTarget.dataset.jobId;
    const details = await readJSON(await fetch(`/v1/jobs/${id}`));
    this.currentJobID = details.job?.id;
    this.jobTarget.textContent = `#${details.job?.id}`;
    this.renderJobDetails(details);
  }

  renderJobDetails(details) {
    const job = details.job || {};
    const steps = details.steps || [];
    const contexts = details.contexts || [];
    this.indexContexts(contexts);
    this.recycle("job-details", `
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
        <div class="flex flex-wrap items-center justify-between gap-3">
          <h4 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Steps</h4>
          ${renderStepSummary(steps)}
        </div>
        <div class="mt-3 space-y-3">${steps.map(renderStep).join("") || emptyState("No steps yet.")}</div>
      </section>
      <section class="mt-5">
        <h4 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Contexts</h4>
        <div class="mt-3 space-y-2">${contexts.slice(-12).map(renderContext).join("") || emptyState("No context records yet.")}</div>
      </section>
    `);
  }

  async interruptJob(event) {
    await this.postJobControl(event.currentTarget.dataset.jobId, "interrupt", "Interrupt with what instruction?");
  }

  async replanJob(event) {
    await this.postJobControl(event.currentTarget.dataset.jobId, "replan", "What should Omni change in the plan?");
  }

  async cancelJob(event) {
    const reason = window.prompt("Cancel reason?", "Canceled from Omni UI");
    if (!reason) return;
    await readJSON(await fetch(`/v1/jobs/${event.currentTarget.dataset.jobId}/cancel`, jsonRequest({ reason })));
    await this.loadJobs();
    this.addEvent("job_canceled", { id: event.currentTarget.dataset.jobId });
  }

  async postJobControl(id, action, question) {
    const feedback = window.prompt(question);
    if (!feedback) return;
    await readJSON(await fetch(`/v1/jobs/${id}/${action}`, jsonRequest({ feedback })));
    const details = await readJSON(await fetch(`/v1/jobs/${id}`));
    this.renderJobDetails(details);
    this.addEvent(`job_${action}`, { id });
  }

  async loadMemoryCandidates() {
    if (!this.queueEnabled) {
      this.recycle("memory-candidates", emptyState("Memory routes require repository mode."));
      if (this.hasMemoryListTarget) this.recycle("memory-list", emptyState("Memory routes require repository mode."));
      return;
    }
    const kind = this.memoryKindFilterTarget?.value?.trim() ?? "";
    const memoryQuery = new URLSearchParams({ limit: "200" });
    if (kind) memoryQuery.set("kind", kind);
    const [payload, memoryPayload] = await Promise.all([
      readJSON(await fetch("/v1/memory-candidates?limit=200")),
      readJSON(await fetch(`/v1/memory?${memoryQuery.toString()}`)),
    ]);
    this.renderMemoryList(memoryPayload.memories || []);
    this.renderMemoryCandidates(payload.memory_candidates || []);
    this.addEvent("memory_loaded", {
      memories: (memoryPayload.memories || []).length,
      candidates: (payload.memory_candidates || []).length,
    }, { memories: memoryPayload, candidates: payload });
  }

  async deleteMemory(event) {
    event.preventDefault();
    const id = Number(event.currentTarget.dataset.memoryId || 0);
    if (!id || !window.confirm(`Delete memory #${id}?`)) return;
    await readJSON(await fetch(`/v1/memory/${id}`, { method: "DELETE" }));
    await this.loadMemoryCandidates();
    this.addEvent("memory_deleted", { id });
  }

  async deleteMemoryCandidate(event) {
    event.preventDefault();
    const id = Number(event.currentTarget.dataset.candidateId || 0);
    if (!id || !window.confirm(`Delete candidate #${id}?`)) return;
    await readJSON(await fetch(`/v1/memory-candidates/${id}`, { method: "DELETE" }));
    await this.loadMemoryCandidates();
    this.addEvent("memory_candidate_deleted", { id });
  }

  renderMemoryList(items) {
    if (!this.hasMemoryListTarget) return;
    if (items.length === 0) {
      this.recycle("memory-list", emptyState("No durable memory chunks found."));
      return;
    }
    this.recycle(
      "memory-list",
      items
        .map(
        (item) => `
          <article class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div class="font-mono text-xs text-cyan-200">memory #${item.id}</div>
              <span class="${statusPillClass(item.kind || "memory")}">${escapeHTML(item.kind || "memory")}</span>
            </div>
            <div class="mt-2 text-xs text-zinc-500">${escapeHTML(item.source || "unknown")} · ${formatDateTime(item.created_at)}</div>
            <p class="mt-2 whitespace-pre-wrap text-sm leading-6 text-zinc-200">${escapeHTML(trimText(item.content || "", 900))}</p>
            ${(item.tags || []).length ? `<div class="mt-3 flex flex-wrap gap-1">${(item.tags || []).slice(0, 12).map((tag) => `<span class="rounded bg-white/[.06] px-2 py-1 font-mono text-[11px] text-zinc-400">${escapeHTML(tag)}</span>`).join("")}</div>` : ""}
            <div class="mt-4">
              <button data-action="chat#deleteMemory" data-memory-id="${item.id}" class="rounded-md border border-rose-300/30 px-3 py-1.5 text-xs font-semibold text-rose-200 hover:bg-rose-400/10">Remove</button>
            </div>
          </article>
        `,
        )
        .join(""),
    );
  }

  async loadGlobalActivity(options = {}) {
    if (!this.queueEnabled) return;
    try {
      const payload = await readJSON(await fetch("/v1/activity?limit=60"));
      for (const job of payload.jobs || []) {
        this.addObservedEvent(`global-job:${job.id}:${job.status}:${job.updated_at}`, "global_job", {
          id: job.id,
          status: job.status,
          pipeline: job.pipeline || "job",
          updated: formatTime(job.updated_at),
        }, { job });
      }
      for (const event of payload.telemetry_events || []) {
        this.addObservedEvent(`telemetry:${event.id}`, `run:${event.event_type}`, {
          run: trimText(event.run_id || "", 8),
          step: event.step ?? "",
          at: formatTime(event.created_at),
        }, { telemetry_event: event });
      }
      for (const memory of payload.memories || []) {
        this.addObservedEvent(`memory:${memory.id}`, "memory_chunk", {
          id: memory.id,
          kind: memory.kind || "memory",
          source: trimText(memory.source || "", 40),
        }, { memory });
      }
      if (this.hasMemoryListTarget) this.renderMemoryList(payload.memories || []);
      if (!options.quiet) {
        this.addObservedEvent(`activity-sync:${Date.now()}`, "global_activity_synced", {
          jobs: (payload.jobs || []).length,
          events: (payload.telemetry_events || []).length,
          memories: (payload.memories || []).length,
        }, payload);
      }
    } catch (error) {
      if (!options.quiet) this.addEvent("global_activity_failed", { error: error.message || String(error) });
    }
  }

  renderMemoryCandidates(items) {
    if (items.length === 0) {
      this.recycle("memory-candidates", emptyState("No memory candidates found."));
      return;
    }
    this.recycle(
      "memory-candidates",
      items
        .map(
        (item) => `
          <article class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div class="font-mono text-xs text-cyan-200">candidate #${item.id}</div>
              <span class="${statusPillClass(item.status)}">${escapeHTML(item.status || "candidate")}</span>
            </div>
            <div class="mt-2 text-xs uppercase tracking-[.16em] text-zinc-500">${escapeHTML(item.candidate_kind || "memory")}</div>
            <p class="mt-2 whitespace-pre-wrap text-sm leading-6 text-zinc-200">${escapeHTML(item.content || "")}</p>
            <div class="mt-4 flex flex-wrap gap-2">
              <button data-action="chat#promoteMemory" data-candidate-id="${item.id}" data-tier="approved" class="rounded-md border border-cyan-300/30 bg-cyan-300/10 px-3 py-2 text-xs font-semibold text-cyan-100">Approve</button>
              <button data-action="chat#promoteMemory" data-candidate-id="${item.id}" data-tier="durable" class="rounded-md border border-emerald-300/30 bg-emerald-300/10 px-3 py-2 text-xs font-semibold text-emerald-100">Durable</button>
              <button data-action="chat#rejectMemory" data-candidate-id="${item.id}" class="rounded-md border border-rose-300/30 bg-rose-300/10 px-3 py-2 text-xs font-semibold text-rose-100">Reject</button>
              <button data-action="chat#deleteMemoryCandidate" data-candidate-id="${item.id}" class="rounded-md border border-white/10 px-3 py-2 text-xs text-zinc-300 hover:bg-white/[.04]">Delete</button>
            </div>
          </article>
        `,
        )
        .join(""),
    );
  }

  async promoteMemory(event) {
    const id = event.currentTarget.dataset.candidateId;
    const tier = event.currentTarget.dataset.tier || "approved";
    await readJSON(await fetch(`/v1/memory-candidates/${id}/promote`, jsonRequest({ tier })));
    await this.loadMemoryCandidates();
    this.addEvent("memory_promoted", { id, tier });
  }

  async rejectMemory(event) {
    const id = event.currentTarget.dataset.candidateId;
    await readJSON(await fetch(`/v1/memory-candidates/${id}/reject`, jsonRequest({})));
    await this.loadMemoryCandidates();
    this.addEvent("memory_rejected", { id });
  }

  async addMemory(event) {
    event.preventDefault();
    if (!this.queueEnabled) {
      this.addEvent("memory_unavailable", { reason: "repository disabled" });
      return;
    }
    const content = this.memoryContentTarget.value.trim();
    if (!content) return;
    const tags = this.memoryTagsTarget.value.split(",").map((tag) => tag.trim()).filter(Boolean);
    await readJSON(
      await fetch(
        "/v1/memory",
        jsonRequest({ source: "omni-web-ui", kind: this.memoryKindTarget.value, content, tags }),
      ),
    );
    this.memoryContentTarget.value = "";
    this.memoryTagsTarget.value = "";
    await this.loadMemoryCandidates();
    this.addEvent("memory_added", { kind: this.memoryKindTarget.value, tags: tags.join(",") || "none" });
  }

  async runPersona(event) {
    event.preventDefault();
    const mode = this.personaModeTarget.value;
    const prompt = this.personaPromptTarget.value.trim();
    if (!prompt) return;
    this.recycle("persona-output", escapeHTML("Running..."));
    const body = {
      prompt,
      model: this.personaModelTarget.value.trim(),
      system: this.personaSystemTarget.value.trim(),
      context: { source: "omni-web-ui", mode },
    };
    const payload = await readJSON(await fetch(`/v1/${mode}`, jsonRequest(body)));
    this.recycle("persona-output", escapeHTML(JSON.stringify(payload, null, 2)));
    this.addEvent("persona_run", { mode, model: payload.model || "default", latency_ms: payload.latency_ms });
  }

  async loadStatus() {
    const payload = await readJSON(await fetch("/healthz"));
    this.recycle("status-output", escapeHTML(JSON.stringify(payload, null, 2)));
    this.queueEnabled = Boolean(payload.queue_enabled);
    this.transportTarget.textContent = this.queueEnabled ? "queue jobs" : "direct instruct";
    this.addEvent("status_loaded", payload);
    await this.loadResearchStatus();
  }

  async loadResearchStatus() {
    if (!this.hasResearchStatusOutputTarget) return;
    try {
      const payload = await readJSON(await fetch("/v1/status/research"));
      this.recycle("research-status-output", renderResearchStatus(payload));
      this.addEvent("research_status_loaded", {
        provider: payload.generation_provider?.provider || "unknown",
        runnable: Boolean(payload.research_runnable),
        ollama_reachable: Boolean(payload.ollama?.reachable),
        web_reachable: Boolean(payload.web_search?.reachable_provider),
      }, payload);
    } catch (error) {
      this.recycle("research-status-output", `<div class="rounded border border-rose-300/30 bg-rose-400/10 p-3 text-rose-100">${escapeHTML(error.message || String(error))}</div>`);
      this.addEvent("research_status_failed", { error: error.message || String(error) });
    }
  }

  async loadMetrics() {
    if (!this.queueEnabled) {
      this.recycle("metrics-output", emptyState("Metrics require repository mode."));
      return;
    }
    if (this.hasMetricsOutputTarget) this.recycle("metrics-output", emptyState("Loading metrics..."));
    try {
      const [live, models, playbooks, benchmarks] = await Promise.all([
        readJSON(await fetch("/v1/metrics/live")),
        readJSON(await fetch("/v1/metrics/models")),
        readJSON(await fetch("/v1/metrics/playbooks")),
        readJSON(await fetch("/v1/metrics/benchmarks")),
      ]);
      this.recycle("metrics-output", renderMetricsDashboard(live, models.models || [], playbooks.playbooks || [], benchmarks.benchmarks || []));
      this.addEvent("metrics_loaded", {
        live_runs: (live.live_runs || []).length,
        recent_runs: (live.recent_runs || []).length,
        models: (models.models || []).length,
        playbooks: (playbooks.playbooks || []).length,
        benchmarks: (benchmarks.benchmarks || []).length,
      }, { live, models, playbooks, benchmarks });
    } catch (error) {
      this.recycle("metrics-output", `<div class="rounded border border-rose-300/30 bg-rose-400/10 p-3 text-rose-100">${escapeHTML(error.message || String(error))}</div>`);
      this.addEvent("metrics_failed", { error: error.message || String(error) });
    }
  }

  async migrateFresh() {
    if (!this.queueEnabled) {
      this.addEvent("admin_unavailable", { reason: "repository disabled" });
      return;
    }
    if (!window.confirm("This will reset repository data. Continue?")) return;
    await readJSON(await fetch("/v1/admin/migrate-fresh", { method: "POST" }));
    this.addEvent("admin_migrate_fresh", { status: "ok" });
    await this.loadStatus();
  }

  renderJobProgress(details) {
    this.renderProgress(details);
    this.indexContexts(details.contexts || []);
    this.addEvent("job_update", {
      id: details.job?.id,
      status: details.job?.status,
      steps: (details.steps || []).length,
      contexts: (details.contexts || []).length,
    }, details);
    for (const step of details.steps || []) {
      const outputKey = `step-output:${step.id}:${hashText(step.output || "")}`;
      if (step.output && !this.seenProgress.has(outputKey)) {
        this.seenProgress.add(outputKey);
        this.addEvent("step_output", { step: step.id, status: step.status, output: trimText(step.output, 280) }, { step });
      }
      const errorKey = `step-error:${step.id}:${hashText(step.error || "")}`;
      if (step.error && !this.seenProgress.has(errorKey)) {
        this.seenProgress.add(errorKey);
        this.addEvent("step_error", { step: step.id, status: step.status, error: trimText(step.error, 280) }, { step });
      }
    }
    for (const context of details.contexts || []) {
      const key = `context:${context.id || `${context.step_id}:${context.key}`}`;
      if (this.seenProgress.has(key)) continue;
      this.seenProgress.add(key);
      const type = contextEventType(context.key);
      this.addEvent(type, {
        context_id: context.id,
        step: context.step_id,
        key: context.key || "context",
        value: trimText(context.value || "", 220),
      }, { job: details.job, context });
    }
  }

  async submitDirect(prompt) {
    this.setStatus("thinking", "active");
    const requestBody = {
      prompt,
      system: "You are Omni chat. Be concise, useful, and grounded.",
      context: { source: "omni-web-chat", mode: "direct" },
      history: this.messages
        .filter((message) => message.role === "user" || message.role === "assistant")
        .slice(-12)
        .map((message) => ({ role: message.role, content: message.content })),
    };
    const response = await fetch("/v1/instruct", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(requestBody),
    });
    const payload = await readJSON(response);
    this.addEvent("direct_response", { model: payload.model, latency_ms: payload.latency_ms }, { request: requestBody, response: payload });
    this.addMessage("assistant", payload.output || "(empty response)");
    this.setStatus("ready", "ready");
    this.setBusy(false);
  }

  newThread() {
    this.currentJobID = null;
    this.jobTarget.textContent = "none";
    this.events = [];
    this.eventIndex = new Map();
    this.contextIndex = new Map();
    this.seenProgress = new Set();
    this.messages = [];
    this.store.save(this.messages);
    this.renderProgress();
    this.renderMessages();
    this.renderTimeline();
    this.addMessage("system", "New local thread started.");
  }

  clearTranscript() {
    this.store.clear();
    this.messages = [];
    this.renderMessages();
    this.addMessage("system", "Local transcript cleared.");
  }

  addMessage(role, content) {
    this.messages.push({ role, content, at: new Date().toISOString() });
    this.store.save(this.messages);
    this.renderMessages();
  }

  addEvent(type, details = {}, full = null) {
    const id = `evt_${String(++this.eventSequence).padStart(6, "0")}`;
    const event = { id, type, details, full: full || details, at: new Date().toISOString() };
    this.events.push(event);
    this.events = this.events.slice(-120);
    this.eventIndex.set(id, event);
    for (const oldID of [...this.eventIndex.keys()]) {
      if (!this.events.some((item) => item.id === oldID)) this.eventIndex.delete(oldID);
    }
    this.renderTimeline();
  }

  addObservedEvent(key, type, details = {}, full = null) {
    if (!key || this.seenProgress.has(key)) return;
    this.seenProgress.add(key);
    this.addEvent(type, details, full);
  }

  renderMessages() {
    const html = this.messages
      .map(
        (message) => `
      <article class="message-grid message-${message.role}">
        <div class="message-shell">
          <div class="mb-2 flex items-center justify-between gap-3 text-xs text-zinc-500">
            <span class="font-semibold uppercase tracking-[.16em]">${escapeHTML(message.role)}</span>
            <time>${formatTime(message.at)}</time>
          </div>
          <div class="message-body text-sm leading-6 text-zinc-100">${escapeHTML(message.content)}</div>
        </div>
      </article>
    `,
      )
      .join("");
    this.recycle("messages", html);
    this.messagesTarget.scrollTop = this.messagesTarget.scrollHeight;
  }

  renderTimeline() {
    this.eventCountTarget.textContent = `${this.events.length} events`;
    const html = this.events
      .slice()
      .reverse()
      .map((event) => {
      const detailRows = Object.entries(event.details || {})
        .map(([key, value]) => `<div><span class="timeline-key">${escapeHTML(key)}</span><span>${escapeHTML(String(value))}</span></div>`)
        .join("");
      return `
      <button type="button" data-action="chat#openTimelineItem" data-event-id="${escapeHTML(event.id)}" class="timeline-card block w-full text-left transition hover:border-cyan-300/40 hover:bg-cyan-300/10">
        <div class="flex items-start justify-between gap-3">
          <div>
            <h3 class="text-sm font-semibold text-zinc-100">${escapeHTML(event.type)}</h3>
            <div class="mt-1 font-mono text-[11px] text-zinc-600">${escapeHTML(event.id)}</div>
          </div>
          <time class="font-mono text-[11px] text-zinc-500">${formatTime(event.at)}</time>
        </div>
        <div class="mt-2 space-y-1 font-mono text-xs text-zinc-300">${detailRows}</div>
      </button>
    `;
      })
      .join("");
    this.recycle("timeline", html);
  }

  renderProgress(details = null) {
    if (!details || !details.job) {
      if (this.hasProgressStateTarget) this.progressStateTarget.textContent = "idle";
      this.recycle("progress", `<div class="text-sm text-zinc-500">No active job.</div>`);
      return;
    }
    const job = details.job || {};
    const steps = details.steps || [];
    const contexts = details.contexts || [];
    const latestStep = [...steps].reverse().find((step) => step.status) || steps[steps.length - 1] || {};
    const latestContext = contexts[contexts.length - 1] || {};
    if (this.hasProgressStateTarget) this.progressStateTarget.textContent = job.status || "running";
    this.recycle("progress", `
      <div class="space-y-3">
        <div class="flex items-center justify-between gap-3">
          <span class="font-mono text-xs text-cyan-200">#${escapeHTML(job.id || "")}</span>
          <span class="${statusPillClass(job.status)}">${escapeHTML(job.status || "running")}</span>
        </div>
        <div class="grid grid-cols-3 gap-2 text-center text-xs">
          <div class="rounded border border-white/10 bg-white/[.03] p-2"><div class="font-mono text-zinc-100">${steps.length}</div><div class="mt-1 text-zinc-500">steps</div></div>
          <div class="rounded border border-white/10 bg-white/[.03] p-2"><div class="font-mono text-zinc-100">${contexts.length}</div><div class="mt-1 text-zinc-500">contexts</div></div>
          <div class="rounded border border-white/10 bg-white/[.03] p-2"><div class="font-mono text-zinc-100">${formatTime(job.updated_at || new Date().toISOString())}</div><div class="mt-1 text-zinc-500">updated</div></div>
        </div>
        <div class="rounded border border-white/10 bg-white/[.03] p-3">
          <div class="text-xs uppercase tracking-[.16em] text-zinc-500">Current step</div>
          <div class="mt-1 text-sm text-zinc-200">${escapeHTML(latestStep.action || latestStep.status || "waiting for updates")}</div>
        </div>
        <button type="button" data-action="chat#openContextItem" data-context-id="${escapeHTML(latestContext.id || "")}" class="w-full rounded border border-white/10 bg-white/[.03] p-3 text-left transition hover:border-cyan-300/40 hover:bg-cyan-300/10 ${latestContext.id ? "" : "pointer-events-none opacity-60"}">
          <div class="text-xs uppercase tracking-[.16em] text-zinc-500">Latest context</div>
          <div class="mt-1 font-mono text-xs text-zinc-300">${escapeHTML(latestContext.key || "none")}</div>
        </button>
      </div>
    `);
  }

  indexContexts(contexts) {
    for (const context of contexts || []) {
      if (context && context.id != null) this.contextIndex.set(String(context.id), context);
    }
  }

  openTimelineItem(event) {
    const id = event.currentTarget.dataset.eventId;
    const item = this.eventIndex.get(id);
    if (!item) return;
    this.openModal(renderEventModal(item));
  }

  openContextItem(event) {
    const id = event.currentTarget.dataset.contextId;
    const context = this.contextIndex.get(String(id));
    if (!context) return;
    this.openModal(renderContextModal(context));
  }

  closeModal() {
    if (!this.hasModalTarget) return;
    this.modalTarget.classList.add("hidden");
    this.modalTarget.classList.remove("grid");
    document.dispatchEvent(new CustomEvent("omni:modal-closed"));
  }

  closeModalBackdrop(event) {
    if (event.target === this.modalTarget) this.closeModal();
  }

  openModal(html) {
    this.recycle("modal", html);
    this.modalTarget.classList.remove("hidden");
    this.modalTarget.classList.add("grid");
  }

  setBusy(value) {
    this.busy = value;
    this.sendTarget.disabled = value;
    this.sendTarget.textContent = value ? "Working" : "Send";
    if (this.hasSpinnerTarget) this.spinnerTarget.classList.toggle("hidden", !value);
  }

  setStatus(text, mode) {
    this.statusTarget.textContent = text;
    this.liveBadgeTarget.textContent = text;
    this.liveBadgeTarget.className = badgeClass(mode);
  }

  recycle(target: string, html: string): void {
    const bundle = `<template data-recyclr-target="${escapeHTML(target)}">${html}</template>`;
    const controller = this.gxController ?? (window as Window & { omniRecyclr?: GxController }).omniRecyclr ?? null;
    if (controller && typeof controller.renderBundle === "function") {
      controller.renderBundle(bundle);
      return;
    }
    const sink = this.element.querySelector(`[data-recyclr-sink="${target}"]`);
    if (sink) sink.innerHTML = html;
  }
}
