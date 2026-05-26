import { Application, Controller } from "https://unpkg.com/@hotwired/stimulus@3.2.2/dist/stimulus.js";
import RecyclrModule from "https://esm.sh/recyclrjs@1.0.1?bundle";

const RecyclrGX = RecyclrModule?.GX || RecyclrModule?.default || RecyclrModule;

class GXController extends Controller {
  connect() {
    if (this.gx) return;
    this.gx = new RecyclrGX({
      url: location.href,
      method: "get",
      selection: "[data-recyclr-target]",
      history: false,
      dispatch: true,
      debug: false,
    });
    window.omniRecyclr = this;
  }

  renderBundle(html) {
    const doc = new DOMParser().parseFromString(String(html || ""), "text/html");
    const events = [...doc.querySelectorAll("[data-recyclr-target]")].map((node) => ({
      selector: `[data-recyclr-sink="${cssEscape(node.dataset.recyclrTarget)}"]`,
      location: "innerHTML",
      selection: node.innerHTML,
    }));
    if (events.length > 0) {
      this.gx.render(events);
      this.element.dispatchEvent(new CustomEvent("omni:recycled", { detail: { events: events.length } }));
    }
  }
}

class TranscriptStore {
  constructor() {
    this.key = "omni.chat.transcript.v1";
  }

  load() {
    try {
      return JSON.parse(localStorage.getItem(this.key) || "[]");
    } catch (_) {
      return [];
    }
  }

  save(messages) {
    const compact = messages.slice(-80);
    localStorage.setItem(this.key, JSON.stringify(compact));
  }

  clear() {
    localStorage.removeItem(this.key);
  }
}

class ChatController extends Controller {
  static targets = [
    "messages",
    "timeline",
    "input",
    "send",
    "status",
    "transport",
    "job",
    "liveBadge",
    "eventCount",
    "panel",
    "jobFilter",
    "jobsList",
    "jobDetails",
    "memoryCandidates",
    "memoryList",
    "memoryKind",
    "memoryTags",
    "memoryContent",
    "personaMode",
    "personaModel",
    "personaSystem",
    "personaPrompt",
    "personaOutput",
    "statusOutput",
    "researchStatusOutput",
    "metricsOutput",
    "progress",
    "progressState",
    "spinner",
    "modal",
    "modalPanel",
  ];

  static values = {
    pollMs: Number,
  };

  async connect() {
    this.gxController = this.application.getControllerForElementAndIdentifier(this.element, "gx");
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
    if (this.messages.length === 0) {
      this.addMessage("system", "Omni UI is ready. Queue mode uses the Go job API; direct mode uses /v1/instruct.");
    }
  }

  disconnect() {
    if (this.activityTimer) window.clearInterval(this.activityTimer);
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
    if (name === "admin") this.loadStatus();
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
    const requestBody = {
      instruction: prompt,
      pipeline: "chat",
      metadata: { source: "omni-web-chat", ui: "stimulus-tailwind-recyclr" },
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
    const [payload, memoryPayload] = await Promise.all([
      readJSON(await fetch("/v1/memory-candidates?limit=50")),
      readJSON(await fetch("/v1/memory?limit=50")),
    ]);
    this.renderMemoryList(memoryPayload.memories || []);
    this.renderMemoryCandidates(payload.memory_candidates || []);
    this.addEvent("memory_loaded", {
      memories: (memoryPayload.memories || []).length,
      candidates: (payload.memory_candidates || []).length,
    }, { memories: memoryPayload, candidates: payload });
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

  recycle(target, html) {
    const bundle = `<template data-recyclr-target="${escapeHTML(target)}">${html}</template>`;
    const controller = this.gxController || window.omniRecyclr;
    if (controller && typeof controller.renderBundle === "function") {
      controller.renderBundle(bundle);
      return;
    }
    const sink = this.element.querySelector(`[data-recyclr-sink="${target}"]`);
    if (sink) sink.innerHTML = html;
  }
}

async function readJSON(response) {
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.error || payload.message || `HTTP ${response.status}`);
  }
  return payload;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function trimText(value, max) {
  value = String(value || "").trim();
  return value.length > max ? `${value.slice(0, max)}...` : value;
}

function hashText(value) {
  value = String(value || "");
  let hash = 0;
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 31 + value.charCodeAt(index)) >>> 0;
  }
  return hash.toString(36);
}

function formatTime(value) {
  if (!value) return "";
  return new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit" }).format(new Date(value));
}

function formatDateTime(value) {
  if (!value) return "";
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function escapeHTML(value) {
  return String(value || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function cssEscape(value) {
  if (window.CSS && typeof window.CSS.escape === "function") {
    return window.CSS.escape(String(value));
  }
  return String(value).replaceAll('"', '\\"');
}

function badgeClass(mode) {
  const base = "rounded-full border px-3 py-1 text-xs font-medium";
  if (mode === "error") return `${base} border-rose-300/35 bg-rose-400/10 text-rose-100`;
  if (mode === "active") return `${base} border-cyan-300/35 bg-cyan-300/10 text-cyan-100`;
  return `${base} border-emerald-300/35 bg-emerald-300/10 text-emerald-100`;
}

function statusPillClass(status) {
  const base = "rounded px-2 py-1 text-[11px] font-semibold uppercase tracking-[.14em]";
  switch (status) {
    case "completed":
    case "approved":
    case "durable":
      return `${base} bg-emerald-300/15 text-emerald-200`;
    case "running":
      return `${base} bg-cyan-300/15 text-cyan-200`;
    case "waiting_input":
    case "pending":
    case "candidate":
      return `${base} bg-amber-300/15 text-amber-200`;
    case "failed":
    case "canceled":
    case "rejected":
      return `${base} bg-rose-300/15 text-rose-200`;
    default:
      return `${base} bg-zinc-300/10 text-zinc-300`;
  }
}

function jsonRequest(body) {
  return {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body || {}),
  };
}

function emptyState(text) {
  return `<div class="rounded-lg border border-dashed border-white/10 bg-white/[.03] p-5 text-sm leading-6 text-zinc-500">${escapeHTML(text)}</div>`;
}

function renderStep(step) {
  const state = stepVisualState(step.status);
  return `
    <article class="${stepCardClass(state)}">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex min-w-0 items-center gap-2">
          <span class="${stepMarkerClass(state)}">${escapeHTML(stepMarkerText(state))}</span>
          <div class="font-mono text-xs text-cyan-200">step #${step.id}</div>
        </div>
        <span class="${statusPillClass(step.status)}">${escapeHTML(step.status || "unknown")}</span>
      </div>
      <div class="mt-2 text-sm font-medium text-zinc-100">${escapeHTML(step.action || "step")}</div>
      ${step.output ? `<pre class="mt-3 max-h-44 overflow-auto whitespace-pre-wrap rounded bg-zinc-950/70 p-3 text-xs leading-5 text-zinc-300">${escapeHTML(step.output)}</pre>` : ""}
      ${step.error ? `<pre class="mt-3 max-h-44 overflow-auto whitespace-pre-wrap rounded bg-rose-400/10 p-3 text-xs leading-5 text-rose-100">${escapeHTML(step.error)}</pre>` : ""}
    </article>
  `;
}

function renderStepSummary(steps) {
  let completed = 0;
  let incomplete = 0;
  let failed = 0;
  const active = steps.find((step) => stepVisualState(step.status) === "active");
  for (const step of steps) {
    const state = stepVisualState(step.status);
    if (state === "done") {
      completed += 1;
    } else {
      incomplete += 1;
      if (state === "failed") failed += 1;
    }
  }
  const activeText = active ? `active #${active.id}${active.action ? ` ${active.action}` : ""}` : "no active step";
  return `
    <div class="flex flex-wrap items-center gap-2 text-[11px] font-medium">
      <span class="rounded border border-cyan-300/30 bg-cyan-300/10 px-2 py-1 text-cyan-100">${escapeHTML(activeText)}</span>
      <span class="rounded border border-emerald-300/25 bg-emerald-300/10 px-2 py-1 text-emerald-100">done ${completed}</span>
      <span class="rounded border border-amber-300/25 bg-amber-300/10 px-2 py-1 text-amber-100">incomplete ${incomplete}</span>
      ${failed ? `<span class="rounded border border-rose-300/25 bg-rose-300/10 px-2 py-1 text-rose-100">failed ${failed}</span>` : ""}
    </div>
  `;
}

function stepVisualState(status) {
  switch (String(status || "").toLowerCase()) {
    case "running":
    case "waiting_input":
      return "active";
    case "completed":
      return "done";
    case "failed":
    case "canceled":
      return "failed";
    case "pending":
      return "pending";
    default:
      return "unknown";
  }
}

function stepCardClass(state) {
  const base = "rounded-md border p-3 transition";
  switch (state) {
    case "active":
      return `${base} active-step-card border-cyan-300/60 bg-cyan-300/10`;
    case "done":
      return `${base} border-emerald-300/25 bg-emerald-300/[.06]`;
    case "failed":
      return `${base} border-rose-300/35 bg-rose-400/[.08]`;
    case "pending":
      return `${base} border-white/10 bg-white/[.025] opacity-80`;
    default:
      return `${base} border-white/10 bg-white/[.035]`;
  }
}

function stepMarkerClass(state) {
  const base = "rounded px-1.5 py-0.5 font-mono text-[10px] font-bold uppercase tracking-[.14em]";
  switch (state) {
    case "active":
      return `${base} bg-cyan-300 text-zinc-950`;
    case "done":
      return `${base} bg-emerald-300/20 text-emerald-100`;
    case "failed":
      return `${base} bg-rose-300/20 text-rose-100`;
    case "pending":
      return `${base} bg-zinc-300/10 text-zinc-400`;
    default:
      return `${base} bg-zinc-300/10 text-zinc-300`;
  }
}

function stepMarkerText(state) {
  switch (state) {
    case "active":
      return "active";
    case "done":
      return "done";
    case "failed":
      return "stop";
    case "pending":
      return "todo";
    default:
      return "step";
  }
}

function renderContext(context) {
  return `
    <button type="button" data-action="chat#openContextItem" data-context-id="${escapeHTML(context.id || "")}" class="block w-full rounded-md border border-white/10 bg-white/[.03] p-3 text-left transition hover:border-cyan-300/40 hover:bg-cyan-300/10">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="font-mono text-xs text-zinc-400">${escapeHTML(context.key || "context")}</span>
        <span class="font-mono text-[11px] text-zinc-600">ctx ${escapeHTML(context.id || "")} · step ${escapeHTML(context.step_id || "")}</span>
      </div>
      <pre class="mt-2 max-h-40 overflow-auto whitespace-pre-wrap text-xs leading-5 text-zinc-300">${escapeHTML(trimText(context.value || "", 1200))}</pre>
    </button>
  `;
}

function contextEventType(key) {
  key = String(key || "").toLowerCase();
  if (key.includes("prompt")) return "llm_prompt";
  if (key.includes("response") || key.includes("completion")) return "llm_response";
  if (key.includes("context")) return "llm_context";
  return "context_recorded";
}

function renderEventModal(event) {
  const full = event.full || {};
  const context = full.context;
  const step = full.step;
  const job = full.job || full.job_snapshot;
  return `
    <div class="border-b border-white/10 p-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div class="font-mono text-xs text-cyan-200">${escapeHTML(event.id)}</div>
          <h2 class="mt-1 text-xl font-semibold text-zinc-100">${escapeHTML(event.type)}</h2>
          <p class="mt-1 text-xs text-zinc-500">${formatDateTime(event.at)}</p>
        </div>
        <button type="button" data-action="chat#closeModal" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300 transition hover:border-cyan-300/40 hover:bg-cyan-300/10">Close</button>
      </div>
    </div>
    <div class="grid gap-4 p-4 lg:grid-cols-[320px_minmax(0,1fr)]">
      <section class="rounded-md border border-white/10 bg-white/[.03] p-4">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Details</h3>
        <div class="mt-3 space-y-2 font-mono text-xs text-zinc-300">${renderDetailRows(event.details)}</div>
        ${job ? `<div class="mt-4 rounded border border-white/10 bg-zinc-950/60 p-3"><div class="text-xs uppercase tracking-[.16em] text-zinc-500">Job</div><div class="mt-1 font-mono text-xs text-cyan-200">#${escapeHTML(job.id || "")}</div><div class="mt-1 text-xs text-zinc-300">${escapeHTML(job.status || "")}</div></div>` : ""}
        ${step ? `<div class="mt-4 rounded border border-white/10 bg-zinc-950/60 p-3"><div class="text-xs uppercase tracking-[.16em] text-zinc-500">Step</div><div class="mt-1 font-mono text-xs text-cyan-200">#${escapeHTML(step.id || "")}</div><div class="mt-1 text-xs text-zinc-300">${escapeHTML(step.action || step.status || "")}</div></div>` : ""}
      </section>
      <section class="min-w-0 rounded-md border border-white/10 bg-white/[.03] p-4">
        ${context ? renderContextPayload(context) : ""}
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Full payload</h3>
        <pre class="scrollbar mt-3 max-h-[58vh] overflow-auto whitespace-pre-wrap rounded bg-zinc-950/70 p-3 text-xs leading-5 text-zinc-300">${escapeHTML(JSON.stringify(full, null, 2))}</pre>
      </section>
    </div>
  `;
}

function renderContextModal(context) {
  return `
    <div class="border-b border-white/10 p-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div class="font-mono text-xs text-cyan-200">ctx ${escapeHTML(context.id || "")}</div>
          <h2 class="mt-1 text-xl font-semibold text-zinc-100">${escapeHTML(context.key || "context")}</h2>
          <p class="mt-1 text-xs text-zinc-500">step ${escapeHTML(context.step_id || "")}</p>
        </div>
        <button type="button" data-action="chat#closeModal" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300 transition hover:border-cyan-300/40 hover:bg-cyan-300/10">Close</button>
      </div>
    </div>
    <div class="p-4">
      ${renderContextPayload(context)}
      <h3 class="mt-4 text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Raw context</h3>
      <pre class="scrollbar mt-3 max-h-[58vh] overflow-auto whitespace-pre-wrap rounded bg-zinc-950/70 p-3 text-xs leading-5 text-zinc-300">${escapeHTML(JSON.stringify(context, null, 2))}</pre>
    </div>
  `;
}

function renderContextPayload(context) {
  return `
    <div class="mb-4 rounded-md border border-cyan-300/20 bg-cyan-300/5 p-4">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-cyan-200">Captured context</h3>
        <span class="font-mono text-[11px] text-zinc-500">step ${escapeHTML(context.step_id || "")}</span>
      </div>
      <pre class="scrollbar mt-3 max-h-[44vh] overflow-auto whitespace-pre-wrap text-xs leading-5 text-zinc-200">${escapeHTML(context.value || "")}</pre>
    </div>
  `;
}

function renderResearchStatus(payload) {
  const generation = payload.generation_provider || {};
  const ollama = payload.ollama || {};
  const web = payload.web_search || {};
  const warnings = payload.warnings || [];
  const probes = web.probes || [];
  return `
    <div class="space-y-3">
      <div class="grid grid-cols-2 gap-2 text-xs">
        ${metricTile("Runnable", payload.research_runnable ? "yes" : "no", payload.research_runnable ? "ok" : "bad")}
        ${metricTile("Provider", generation.provider || "unknown", generation.reachable ? "ok" : "bad")}
        ${metricTile("Ollama", ollama.reachable ? "reachable" : "down", ollama.reachable ? "ok" : "bad")}
        ${metricTile("Web", web.enabled ? (web.reachable_provider ? "reachable" : "degraded") : "disabled", web.enabled && web.reachable_provider ? "ok" : "warn")}
      </div>
      <div class="rounded border border-white/10 bg-white/[.03] p-3">
        <div class="text-xs uppercase tracking-[.16em] text-zinc-500">Ollama</div>
        <dl class="mt-2 space-y-1 font-mono text-xs text-zinc-300">
          <div><span class="text-zinc-500">base_url</span> ${escapeHTML(ollama.base_url || "n/a")}</div>
          <div><span class="text-zinc-500">configured</span> ${escapeHTML((ollama.configured_models || []).join(", ") || "none")}</div>
          <div><span class="text-zinc-500">missing</span> ${escapeHTML((ollama.missing_models || []).join(", ") || "none")}</div>
          <div><span class="text-zinc-500">embedding</span> ${escapeHTML(ollama.embedding_model || "n/a")} ${ollama.embedding_available ? "(available)" : "(not found)"}</div>
          ${ollama.last_provider_error ? `<div class="text-rose-200"><span class="text-rose-300">error</span> ${escapeHTML(ollama.last_provider_error)}</div>` : ""}
        </dl>
      </div>
      <div class="rounded border border-white/10 bg-white/[.03] p-3">
        <div class="text-xs uppercase tracking-[.16em] text-zinc-500">Web Providers</div>
        <div class="mt-2 space-y-2">
          ${probes.map((probe) => `
            <div class="rounded border border-white/10 bg-zinc-950/40 p-2 font-mono text-xs">
              <div class="flex items-center justify-between gap-2">
                <span class="text-zinc-200">${escapeHTML(probe.provider || "provider")}</span>
                <span class="${probe.reachable ? "text-emerald-200" : "text-rose-200"}">${probe.reachable ? "ok" : "failed"}</span>
              </div>
              <div class="mt-1 truncate text-zinc-500">${escapeHTML(probe.target_url || "")}</div>
              ${probe.error ? `<div class="mt-1 text-rose-200">${escapeHTML(probe.error)}</div>` : `<div class="mt-1 text-zinc-400">status=${escapeHTML(probe.status_code || "")}</div>`}
            </div>
          `).join("") || `<div class="text-zinc-500">No provider probes.</div>`}
        </div>
      </div>
      ${warnings.length ? `<div class="rounded border border-amber-300/30 bg-amber-300/10 p-3 text-sm leading-6 text-amber-100">${warnings.map(escapeHTML).join("<br>")}</div>` : ""}
    </div>
  `;
}

function renderMetricsDashboard(live, models, playbooks, benchmarks) {
  const statusCounts = live.status_counts || {};
  const liveRuns = live.live_runs || [];
  const recentRuns = live.recent_runs || [];
  const blockers = live.common_blockers || [];
  const completed = Number(statusCounts.completed || 0);
  const failed = Number(statusCounts.failed || 0);
  const cancelled = Number(statusCounts.canceled || statusCounts.cancelled || 0);
  const totalTerminal = completed + failed + cancelled;
  const successRate = totalTerminal > 0 ? `${Math.round((completed / totalTerminal) * 100)}%` : "n/a";
  return `
    <div class="grid gap-4 xl:grid-cols-4">
      ${metricTile("Live Runs", String(liveRuns.length), liveRuns.length ? "warn" : "ok")}
      ${metricTile("Recent Runs", String(recentRuns.length), "ok")}
      ${metricTile("Success Rate", successRate, completed >= failed ? "ok" : "warn")}
      ${metricTile("Blocker Types", String(blockers.length), blockers.length ? "warn" : "ok")}
    </div>
    <div class="grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <div class="flex items-center justify-between gap-3">
          <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Live Run Timeline</h3>
          <span class="font-mono text-xs text-zinc-500">${escapeHTML(liveRuns.length)} active</span>
        </div>
        <div class="mt-3 space-y-3">${liveRuns.map(renderMetricRun).join("") || emptyState("No active telemetry runs.")}</div>
      </section>
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Run Health</h3>
        <div class="mt-3 grid grid-cols-2 gap-2 text-xs">
          ${Object.entries(statusCounts).map(([key, value]) => metricTile(key, String(value), key === "completed" ? "ok" : key === "failed" ? "bad" : "warn")).join("") || emptyState("No status counts yet.")}
        </div>
      </section>
    </div>
    <div class="grid gap-4 xl:grid-cols-3">
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Recent Outcomes</h3>
        <div class="mt-3 space-y-2">${recentRuns.slice(0, 8).map(renderMetricRun).join("") || emptyState("No telemetry runs yet.")}</div>
      </section>
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Common Blockers</h3>
        <div class="mt-3 space-y-2">${blockers.map(renderMetricCount).join("") || emptyState("No blocker metrics yet.")}</div>
      </section>
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Model Performance</h3>
        <div class="mt-3 space-y-2">${models.slice(0, 8).map(renderMetricModel).join("") || emptyState("No model metrics yet.")}</div>
      </section>
    </div>
    <div class="grid gap-4 xl:grid-cols-2">
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Playbook Leverage</h3>
        <div class="mt-3 space-y-2">${playbooks.slice(0, 8).map(renderMetricPlaybook).join("") || emptyState("No playbook usage metrics yet.")}</div>
      </section>
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">Benchmarks</h3>
        <div class="mt-3 space-y-2">${benchmarks.slice(0, 8).map(renderMetricBenchmark).join("") || emptyState("No benchmark metrics yet.")}</div>
      </section>
    </div>
  `;
}

function renderMetricRun(run) {
  return `
    <div class="rounded border border-white/10 bg-white/[.03] p-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="font-mono text-xs text-cyan-200">${escapeHTML((run.id || "").slice(0, 8) || "run")}</span>
        <span class="${statusPillClass(run.status)}">${escapeHTML(run.status || "unknown")}</span>
      </div>
      <div class="mt-2 text-sm text-zinc-200">${escapeHTML(run.task_kind || run.project_type || "unclassified task")}</div>
      <div class="mt-1 font-mono text-xs text-zinc-500">${escapeHTML(run.workspace_id || "workspace n/a")} · ${escapeHTML(formatDurationMS(run.duration_ms))}</div>
    </div>
  `;
}

function renderMetricCount(item) {
  return `
    <div class="flex items-center justify-between gap-3 rounded border border-white/10 bg-white/[.03] p-3">
      <span class="font-mono text-xs text-zinc-300">${escapeHTML(item.key || "unknown")}</span>
      <span class="font-mono text-xs text-amber-200">${escapeHTML(item.count || 0)}</span>
    </div>
  `;
}

function renderMetricModel(model) {
  const calls = Number(model.calls || 0);
  const successes = Number(model.successes || 0);
  const rate = calls > 0 ? `${Math.round((successes / calls) * 100)}%` : "n/a";
  return `
    <div class="rounded border border-white/10 bg-white/[.03] p-3">
      <div class="flex items-center justify-between gap-2">
        <span class="truncate text-sm text-zinc-200">${escapeHTML(model.role || "role")}</span>
        <span class="font-mono text-xs text-emerald-200">${escapeHTML(rate)}</span>
      </div>
      <div class="mt-1 truncate font-mono text-xs text-zinc-500">${escapeHTML(model.provider || "provider")} / ${escapeHTML(model.model || "model")}</div>
      <div class="mt-2 grid grid-cols-3 gap-2 text-center font-mono text-xs text-zinc-300">
        <span>calls ${escapeHTML(calls)}</span>
        <span>bad ${escapeHTML(model.failures || 0)}</span>
        <span>${escapeHTML(Math.round(model.avg_latency_ms || 0))}ms</span>
      </div>
    </div>
  `;
}

function renderMetricPlaybook(playbook) {
  return `
    <div class="rounded border border-white/10 bg-white/[.03] p-3">
      <div class="truncate font-mono text-xs text-cyan-200">${escapeHTML(playbook.playbook_id || "playbook")}</div>
      <div class="mt-2 grid grid-cols-4 gap-2 text-center font-mono text-xs text-zinc-300">
        <span>uses ${escapeHTML(playbook.uses || 0)}</span>
        <span>reused ${escapeHTML(playbook.reused || 0)}</span>
        <span>ok ${escapeHTML(playbook.successes || 0)}</span>
        <span>bad ${escapeHTML(playbook.failures || 0)}</span>
      </div>
    </div>
  `;
}

function renderMetricBenchmark(item) {
  return `
    <div class="rounded border border-white/10 bg-white/[.03] p-3">
      <div class="flex items-center justify-between gap-2">
        <span class="truncate font-mono text-xs text-cyan-200">${escapeHTML(item.benchmark_id || "benchmark")}</span>
        <span class="font-mono text-xs text-zinc-500">${escapeHTML(item.runs || 0)} runs</span>
      </div>
      <div class="mt-2 grid grid-cols-3 gap-2 text-center font-mono text-xs text-zinc-300">
        <span>ok ${escapeHTML(item.successes || 0)}</span>
        <span>bad ${escapeHTML(item.failures || 0)}</span>
        <span>${escapeHTML(formatDurationMS(item.avg_duration_ms))}</span>
      </div>
    </div>
  `;
}

function metricTile(label, value, mode) {
  const tone = mode === "ok" ? "text-emerald-200" : mode === "bad" ? "text-rose-200" : "text-amber-200";
  return `
    <div class="rounded border border-white/10 bg-white/[.03] p-3">
      <div class="text-[11px] uppercase tracking-[.16em] text-zinc-500">${escapeHTML(label)}</div>
      <div class="mt-1 truncate font-mono text-xs ${tone}">${escapeHTML(value)}</div>
    </div>
  `;
}

function formatDurationMS(value) {
  const ms = Number(value || 0);
  if (!Number.isFinite(ms) || ms <= 0) return "n/a";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  if (ms < 60000) return `${Math.round(ms / 1000)}s`;
  return `${Math.round(ms / 60000)}m`;
}

function renderDetailRows(details) {
  const entries = Object.entries(details || {});
  if (entries.length === 0) return `<div class="text-zinc-500">No details.</div>`;
  return entries
    .map(([key, value]) => `
      <div class="grid grid-cols-[96px_minmax(0,1fr)] gap-3">
        <span class="text-zinc-500">${escapeHTML(key)}</span>
        <span class="break-words text-zinc-200">${escapeHTML(String(value))}</span>
      </div>
    `)
    .join("");
}

const application = Application.start();
application.register("gx", GXController);
application.register("chat", ChatController);
