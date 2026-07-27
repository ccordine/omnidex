import { jsonRequest, readJSON } from "./api";
import { emptyState, escapeHTML, formatDateTime, formatTime, statusPillClass, trimText } from "./dom";
import { toastError, toastFromError, toastOk } from "./feedback";

export interface ChatMemoryHost {
  queueEnabled(): boolean;
  hasMemoryList(): boolean;
  memoryKind(): HTMLSelectElement;
  memoryKindFilter(): HTMLSelectElement;
  memoryTags(): HTMLInputElement;
  memoryContent(): HTMLTextAreaElement;
  recycle(target: string, html: string): void;
  addEvent(type: string, details?: Record<string, unknown>, full?: unknown): void;
  addObservedEvent(key: string, type: string, details?: Record<string, unknown>, full?: unknown): void;
}

export class ChatMemoryCoordinator {
  constructor(private readonly host: ChatMemoryHost) {}

  async load(): Promise<void> {
    if (!this.host.queueEnabled()) {
      this.host.recycle("memory-candidates", emptyState("Memory routes require repository mode."));
      if (this.host.hasMemoryList()) this.host.recycle("memory-list", emptyState("Memory routes require repository mode."));
      return;
    }
    const kind = this.host.memoryKindFilter().value.trim();
    const memoryQuery = new URLSearchParams({ limit: "50" });
    if (kind) memoryQuery.set("kind", kind);
    const [payload, memoryPayload] = await Promise.all([
      readJSON(await fetch("/v1/memory-candidates?limit=50")),
      readJSON(await fetch(`/v1/memory?${memoryQuery.toString()}`)),
    ]);
    this.renderList(memoryPayload.memories || []);
    this.renderCandidates(payload.memory_candidates || []);
    this.host.addEvent("memory_loaded", {
      memories: (memoryPayload.memories || []).length,
      candidates: (payload.memory_candidates || []).length,
    }, { memories: memoryPayload, candidates: payload });
  }

  async deleteMemory(event: Event): Promise<void> {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.memoryId || 0);
    if (!id || !window.confirm(`Delete memory #${id}?`)) return;
    await readJSON(await fetch(`/v1/memory/${id}`, { method: "DELETE" }));
    await this.load();
    this.host.addEvent("memory_deleted", { id });
  }

  async deleteCandidate(event: Event): Promise<void> {
    event.preventDefault();
    const id = Number((event.currentTarget as HTMLElement).dataset.candidateId || 0);
    if (!id || !window.confirm(`Delete candidate #${id}?`)) return;
    await readJSON(await fetch(`/v1/memory-candidates/${id}`, { method: "DELETE" }));
    await this.load();
    this.host.addEvent("memory_candidate_deleted", { id });
  }

  renderList(items: Array<Record<string, any>>): void {
    if (!this.host.hasMemoryList()) return;
    if (items.length === 0) {
      this.host.recycle("memory-list", emptyState("No durable memory chunks found."));
      return;
    }
    this.host.recycle("memory-list", items.map((item) => `
      <article class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <div class="flex flex-wrap items-center justify-between gap-3"><div class="font-mono text-xs text-cyan-200">memory #${item.id}</div><span class="${statusPillClass(item.kind || "memory")}">${escapeHTML(item.kind || "memory")}</span></div>
        <div class="mt-2 text-xs text-zinc-500">${escapeHTML(item.source || "unknown")} · ${formatDateTime(item.created_at)}</div>
        <p class="mt-2 whitespace-pre-wrap text-sm leading-6 text-zinc-200">${escapeHTML(trimText(item.content || "", 900))}</p>
        ${(item.tags || []).length ? `<div class="mt-3 flex flex-wrap gap-1">${(item.tags || []).slice(0, 12).map((tag: string) => `<span class="rounded bg-white/[.06] px-2 py-1 font-mono text-[11px] text-zinc-400">${escapeHTML(tag)}</span>`).join("")}</div>` : ""}
        <div class="mt-4"><button data-action="chat#deleteMemory" data-memory-id="${item.id}" class="rounded-md border border-rose-300/30 px-3 py-1.5 text-xs font-semibold text-rose-200 hover:bg-rose-400/10">Remove</button></div>
      </article>
    `).join(""));
  }

  async loadGlobalActivity(options: { quiet?: boolean; strict?: boolean } = {}): Promise<void> {
    if (!this.host.queueEnabled()) return;
    try {
      const payload = await readJSON(await fetch("/v1/activity?limit=60"));
      for (const job of payload.jobs || []) {
        this.host.addObservedEvent(`global-job:${job.id}:${job.status}:${job.updated_at}`, "global_job", {
          id: job.id, status: job.status, pipeline: job.pipeline || "job", updated: formatTime(job.updated_at),
        }, { job });
      }
      for (const event of payload.telemetry_events || []) {
        this.host.addObservedEvent(`telemetry:${event.id}`, `run:${event.event_type}`, {
          run: trimText(event.run_id || "", 8), step: event.step ?? "", at: formatTime(event.created_at),
        }, { telemetry_event: event });
      }
      for (const memory of payload.memories || []) {
        this.host.addObservedEvent(`memory:${memory.id}`, "memory_chunk", {
          id: memory.id, kind: memory.kind || "memory", source: trimText(memory.source || "", 40),
        }, { memory });
      }
      for (const entry of payload.llm_activity || []) {
        this.host.addObservedEvent(`llm:${entry.id}`, `llm:${entry.source}`, {
          source: entry.source || "llm", chars: entry.sent_chars || 0,
          ok: entry.success !== false, at: formatTime(entry.created_at),
        }, { llm_activity: entry });
      }
      if (this.host.hasMemoryList()) this.renderList(payload.memories || []);
      if (!options.quiet) {
        this.host.addObservedEvent(`activity-sync:${Date.now()}`, "global_activity_synced", {
          jobs: (payload.jobs || []).length,
          events: (payload.telemetry_events || []).length,
          memories: (payload.memories || []).length,
        }, payload);
      }
    } catch (error) {
      if (!options.quiet) this.host.addEvent("global_activity_failed", { error: errorMessage(error) });
      if (options.strict) throw error;
    }
  }

  renderCandidates(items: Array<Record<string, any>>): void {
    if (items.length === 0) {
      this.host.recycle("memory-candidates", emptyState("No memory candidates found."));
      return;
    }
    this.host.recycle("memory-candidates", items.map((item) => `
      <article class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <div class="flex flex-wrap items-center justify-between gap-3"><div class="font-mono text-xs text-cyan-200">candidate #${item.id}</div><span class="${statusPillClass(item.status)}">${escapeHTML(item.status || "candidate")}</span></div>
        <div class="mt-2 text-xs uppercase tracking-[.16em] text-zinc-500">${escapeHTML(item.candidate_kind || "memory")}</div>
        <p class="mt-2 whitespace-pre-wrap text-sm leading-6 text-zinc-200">${escapeHTML(item.content || "")}</p>
        <div class="mt-4 flex flex-wrap gap-2">
          <button data-action="chat#promoteMemory" data-candidate-id="${item.id}" data-tier="approved" class="rounded-md border border-cyan-300/30 bg-cyan-300/10 px-3 py-2 text-xs font-semibold text-cyan-100">Approve</button>
          <button data-action="chat#promoteMemory" data-candidate-id="${item.id}" data-tier="durable" class="rounded-md border border-emerald-300/30 bg-emerald-300/10 px-3 py-2 text-xs font-semibold text-emerald-100">Durable</button>
          <button data-action="chat#rejectMemory" data-candidate-id="${item.id}" class="rounded-md border border-rose-300/30 bg-rose-300/10 px-3 py-2 text-xs font-semibold text-rose-100">Reject</button>
          <button data-action="chat#deleteMemoryCandidate" data-candidate-id="${item.id}" class="rounded-md border border-white/10 px-3 py-2 text-xs text-zinc-300 hover:bg-white/[.04]">Delete</button>
        </div>
      </article>
    `).join(""));
  }

  async promote(event: Event): Promise<void> {
    const target = event.currentTarget as HTMLElement;
    const id = target.dataset.candidateId;
    const tier = target.dataset.tier || "approved";
    try {
      await readJSON(await fetch(`/v1/memory-candidates/${id}/promote`, jsonRequest({ tier })));
      await this.load();
      this.host.addEvent("memory_promoted", { id, tier });
      toastOk("Memory promoted");
    } catch (error) {
      toastFromError(error);
    }
  }

  async reject(event: Event): Promise<void> {
    const id = (event.currentTarget as HTMLElement).dataset.candidateId;
    try {
      await readJSON(await fetch(`/v1/memory-candidates/${id}/reject`, jsonRequest({})));
      await this.load();
      this.host.addEvent("memory_rejected", { id });
      toastOk("Memory candidate rejected");
    } catch (error) {
      toastFromError(error);
    }
  }

  async add(event: Event): Promise<void> {
    event.preventDefault();
    if (!this.host.queueEnabled()) {
      toastError("Memory requires repository mode");
      this.host.addEvent("memory_unavailable", { reason: "repository disabled" });
      return;
    }
    const content = this.host.memoryContent().value.trim();
    if (!content) {
      toastError("Memory content is required");
      return;
    }
    const tags = this.host.memoryTags().value.split(",").map((tag) => tag.trim()).filter(Boolean);
    try {
      await readJSON(await fetch("/v1/memory", jsonRequest({
        source: "omni-web-ui", kind: this.host.memoryKind().value, content, tags,
      })));
      this.host.memoryContent().value = "";
      this.host.memoryTags().value = "";
      await this.load();
      this.host.addEvent("memory_added", { kind: this.host.memoryKind().value, tags: tags.join(",") || "none" });
      toastOk("Memory saved");
    } catch (error) {
      toastFromError(error);
    }
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
