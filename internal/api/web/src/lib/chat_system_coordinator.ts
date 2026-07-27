import { jsonRequest, readJSON } from "./api";
import { emptyState, escapeHTML } from "./dom";
import { errorMessage, toastError, toastFromError, toastOk } from "./feedback";
import { renderHostBridgeStatus, renderMetricsDashboard } from "./render";

export interface ChatSystemHost {
  queueEnabled(): boolean;
  setQueueEnabled(enabled: boolean): void;
  personaMode(): HTMLSelectElement;
  personaModel(): HTMLInputElement;
  personaSystem(): HTMLTextAreaElement;
  personaPrompt(): HTMLTextAreaElement;
  hasHostBridgeStatus(): boolean;
  hasResearchStatus(): boolean;
  hasMetrics(): boolean;
  updateTransportLabel(): void;
  recycle(target: string, html: string, mode?: "html" | "text"): void;
  addEvent(type: string, details?: Record<string, unknown>, full?: unknown): void;
}

export class ChatSystemCoordinator {
  constructor(private readonly host: ChatSystemHost) {}

  async runPersona(event: Event): Promise<void> {
    event.preventDefault();
    const mode = this.host.personaMode().value;
    const prompt = this.host.personaPrompt().value.trim();
    if (!prompt) {
      toastError("Enter a prompt first");
      return;
    }
    this.host.recycle("persona-output", "Running…", "text");
    try {
      const body = {
        prompt,
        model: this.host.personaModel().value.trim(),
        system: this.host.personaSystem().value.trim(),
        context: { source: "omni-web-ui", mode },
      };
      const payload = await readJSON(await fetch(`/v1/${mode}`, jsonRequest(body)));
      this.host.recycle("persona-output", JSON.stringify(payload, null, 2), "text");
      this.host.addEvent("persona_run", { mode, model: payload.model || "default", latency_ms: payload.latency_ms });
      toastOk("Persona run completed");
    } catch (error) {
      toastFromError(error);
    }
  }

  async loadStatus(): Promise<void> {
    try {
      const payload = await readJSON(await fetch("/healthz"));
      this.host.recycle("status-output", JSON.stringify(payload, null, 2), "text");
      this.host.setQueueEnabled(Boolean(payload.queue_enabled));
      this.host.updateTransportLabel();
      this.host.addEvent("status_loaded", payload);
    } catch (error) {
      const message = errorMessage(error);
      this.host.recycle("status-output", `Error loading /healthz: ${message}`, "text");
      this.host.addEvent("status_load_failed", { error: message });
    }
    await Promise.all([this.loadResearchStatus(), this.loadHostBridgeStatus()]);
  }

  async loadHostBridgeStatus(): Promise<void> {
    if (!this.host.hasHostBridgeStatus()) return;
    try {
      const payload = await readJSON(await fetch("/v1/host/status"));
      this.host.recycle("host-bridge-status-output", renderHostBridgeStatus(payload));
      this.host.addEvent("host_bridge_status_loaded", {
        configured: Boolean(payload.configured),
        reachable: Boolean(payload.reachable),
        picker_ready: Boolean(payload.picker_ready),
      }, payload);
      document.dispatchEvent(new CustomEvent("omni:host-bridge-status", { detail: payload }));
    } catch (error) {
      const message = errorMessage(error);
      this.host.recycle("host-bridge-status-output", errorPanel(message));
      this.host.addEvent("host_bridge_status_failed", { error: message });
    }
  }

  async loadResearchStatus(): Promise<void> {
    if (!this.host.hasResearchStatus()) return;
    try {
      const payload = await readJSON(await fetch("/v1/status/research"));
      if (typeof payload.html !== "string" || !payload.html.trim()) {
        throw new Error("Research status response did not include its required server-rendered fragment.");
      }
      this.host.recycle("research-status-output", payload.html);
      this.host.addEvent("research_status_loaded", {
        provider: payload.generation_provider?.provider || "unknown",
        provider_state: payload.generation_provider?.state || "unknown",
        embedding_provider: payload.embedding_provider?.provider || "unknown",
        embedding_state: payload.embedding_provider?.state || "unknown",
        runnable: Boolean(payload.research_runnable),
        ollama_reachable: Boolean(payload.ollama?.reachable),
        web_reachable: Boolean(payload.web_search?.reachable_provider),
      }, payload);
    } catch (error) {
      const message = errorMessage(error);
      this.host.recycle("research-status-output", errorPanel(message));
      this.host.addEvent("research_status_failed", { error: message });
    }
  }

  async loadMetrics(options: { strict?: boolean } = {}): Promise<void> {
    if (!this.host.queueEnabled()) {
      this.setMetricsOutput(emptyState("Metrics require repository mode."));
      return;
    }
    this.setMetricsOutput(emptyState("Loading metrics…"));
    try {
      const [live, models, playbooks, benchmarks, contextShrink, contextUsage, operations] = await Promise.all([
        fetchMetric("/v1/metrics/live"),
        fetchMetric("/v1/metrics/models"),
        fetchMetric("/v1/metrics/playbooks"),
        fetchMetric("/v1/metrics/benchmarks"),
        fetchMetric("/v1/metrics/context-shrink?limit=100"),
        fetchMetric("/v1/metrics/context-usage?limit=100"),
        fetchMetric("/v1/metrics/operations"),
      ]);
      this.setMetricsOutput(renderMetricsDashboard(
        live,
        models.models || [],
        playbooks.playbooks || [],
        benchmarks.benchmarks || [],
        contextShrink,
        contextUsage,
        operations,
      ));
      this.host.addEvent("metrics_loaded", {
        live_runs: (live.live_runs || []).length,
        recent_runs: (live.recent_runs || []).length,
        models: (models.models || []).length,
        playbooks: (playbooks.playbooks || []).length,
        benchmarks: (benchmarks.benchmarks || []).length,
        context_shrink_events: Number(contextShrink?.summary?.requests || 0),
        context_shrink_avg_saved_pct: Number(contextShrink?.summary?.avg_saved_pct || 0),
        context_usage_events: Number(contextUsage?.summary?.requests || 0),
        context_overloads: Number(contextUsage?.summary?.overload_events || 0),
        llm_failures: Number(contextUsage?.summary?.failure_events || operations?.llm_failures || 0),
        failure_events: (operations?.failure_counts || []).reduce(
          (sum: number, item: { count?: number }) => sum + Number(item.count || 0),
          0,
        ),
      }, { live, models, playbooks, benchmarks, contextShrink, contextUsage, operations });
    } catch (error) {
      this.setMetricsOutput(errorPanel(errorMessage(error)));
      this.host.addEvent("metrics_failed", { error: errorMessage(error) });
      if (options.strict) throw error;
    }
  }

  async migrateFresh(): Promise<void> {
    if (!this.host.queueEnabled()) {
      toastError("Migrate fresh requires repository mode");
      this.host.addEvent("admin_unavailable", { reason: "repository disabled" });
      return;
    }
    if (!window.confirm("This will reset repository data. Continue?")) return;
    try {
      await readJSON(await fetch("/v1/admin/migrate-fresh", { method: "POST" }));
      this.host.addEvent("admin_migrate_fresh", { status: "ok" });
      toastOk("Database migrated fresh");
      await this.loadStatus();
    } catch (error) {
      toastFromError(error);
    }
  }

  private setMetricsOutput(html: string): void {
    if (this.host.hasMetrics()) this.host.recycle("metrics-output", html);
  }
}

async function fetchMetric(path: string): Promise<any> {
  const response = await fetch(path);
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `${response.status} ${response.statusText}`);
  }
  return readJSON(response);
}

function errorPanel(message: string): string {
  return `<div class="rounded border border-rose-300/30 bg-rose-400/10 p-3 text-rose-100">${escapeHTML(message)}</div>`;
}
