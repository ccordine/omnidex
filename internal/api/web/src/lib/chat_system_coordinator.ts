import { readJSON } from "./api";
import { requireServerComponentBundle } from "./chat_component_api";
import { errorMessage, toastError, toastFromError, toastOk } from "./feedback";

export interface ChatSystemHost {
  queueEnabled(): boolean;
  setQueueEnabled(enabled: boolean): void;
  hasStatusOutput(): boolean;
  statusOutput(): HTMLElement;
  hasHostBridgeStatus(): boolean;
  hostBridgeStatus(): HTMLElement;
  hasResearchStatus(): boolean;
  researchStatus(): HTMLElement;
  hasMetrics(): boolean;
  metrics(): HTMLElement;
  updateTransportLabel(): void;
  renderComponentBundle(bundle: string): Promise<void>;
  addEvent(type: string, details?: Record<string, unknown>, full?: unknown): void;
}

export class ChatSystemCoordinator {
  constructor(private readonly host: ChatSystemHost) {}

  async loadStatus(): Promise<void> {
    try {
      const payload = await readJSON(await fetch("/healthz"));
      if (this.host.hasStatusOutput()) this.setText(this.host.statusOutput(), JSON.stringify(payload, null, 2));
      this.host.setQueueEnabled(Boolean(payload.queue_enabled));
      this.host.updateTransportLabel();
      this.host.addEvent("status_loaded", payload);
    } catch (error) {
      const message = errorMessage(error);
      if (this.host.hasStatusOutput()) this.setText(this.host.statusOutput(), `Error loading /healthz: ${message}`);
      this.host.addEvent("status_load_failed", { error: message });
    }
    await Promise.all([this.loadResearchStatus(), this.loadHostBridgeStatus()]);
  }

  async loadHostBridgeStatus(): Promise<void> {
    if (!this.host.hasHostBridgeStatus()) return;
    const target = this.host.hostBridgeStatus();
    this.setLoading(target, "Loading host bridge status…");
    try {
      const payload = await readJSON<Record<string, unknown>>(await fetch("/v1/host/status"));
      await this.host.renderComponentBundle(requireServerComponentBundle(payload, "Host bridge"));
      this.clearLoading(target);
      this.host.addEvent("host_bridge_status_loaded", {
        configured: Boolean(payload.configured),
        reachable: Boolean(payload.reachable),
        picker_ready: Boolean(payload.picker_ready),
      }, payload);
      document.dispatchEvent(new CustomEvent("omni:host-bridge-status", { detail: payload }));
    } catch (error) {
      const message = errorMessage(error);
      this.setText(target, `Host bridge status unavailable: ${message}`);
      this.host.addEvent("host_bridge_status_failed", { error: message });
    }
  }

  async loadResearchStatus(): Promise<void> {
    if (!this.host.hasResearchStatus()) return;
    const target = this.host.researchStatus();
    this.setLoading(target, "Loading research status…");
    try {
      const payload = await readJSON<Record<string, unknown>>(await fetch("/v1/status/research"));
      await this.host.renderComponentBundle(requireServerComponentBundle(payload, "Research status"));
      this.clearLoading(target);
      const web = payload.web_search as Record<string, unknown> | undefined;
      this.host.addEvent("research_status_loaded", {
        web_configured: Boolean(web?.enabled),
      }, payload);
    } catch (error) {
      const message = errorMessage(error);
      this.setText(target, `Research status unavailable: ${message}`);
      this.host.addEvent("research_status_failed", { error: message });
    }
  }

  async loadMetrics(options: { strict?: boolean } = {}): Promise<void> {
    if (!this.host.hasMetrics()) return;
    const target = this.host.metrics();
    if (!this.host.queueEnabled()) {
      this.setText(target, "Metrics require repository mode.");
      return;
    }
    this.setLoading(target, "Loading metrics…");
    try {
      const payload = await readJSON<Record<string, unknown>>(await fetch("/v1/ui/chat/metrics"));
      await this.host.renderComponentBundle(requireServerComponentBundle(payload, "Chat metrics"));
      this.clearLoading(target);
      this.host.addEvent("metrics_loaded", {}, payload);
    } catch (error) {
      const message = errorMessage(error);
      this.setText(target, `Metrics unavailable: ${message}`);
      this.host.addEvent("metrics_failed", { error: message });
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

  private setLoading(target: HTMLElement, message: string): void {
    target.setAttribute("aria-busy", "true");
    this.setText(target, message, true);
  }

  private clearLoading(target: HTMLElement): void {
    target.setAttribute("aria-busy", "false");
  }

  private setText(target: HTMLElement, message: string, preserveBusy = false): void {
    target.textContent = message;
    if (!preserveBusy) target.setAttribute("aria-busy", "false");
  }
}
