import { Controller } from "@hotwired/stimulus";
import { createRecyclrGX, createRecyclrRealtimeStream, type RecyclrGX, type RecyclrStream } from "../lib/recyclr";
import { RecyclrBundleQueue } from "../lib/recyclr_bundle_queue";
import { requestRealtimeSync } from "../lib/realtime_sync";
import { showToast, type ToastTone } from "../lib/toast";
import { t } from "../lib/i18n";

type RealtimeState = "connecting" | "syncing" | "live" | "reconnecting" | "error";

export default class RecyclrController extends Controller<HTMLElement> {
  static targets = ["status", "indicator"];
  static values = { scope: String };

  declare readonly statusTarget: HTMLElement;
  declare readonly hasStatusTarget: boolean;
  declare readonly indicatorTarget: HTMLElement;
  declare readonly hasIndicatorTarget: boolean;
  declare readonly scopeValue: string;

  gx: RecyclrGX | null = null;
  private stream: RecyclrStream | null = null;
  private bundles: RecyclrBundleQueue | null = null;
  private monitorTimer: number | null = null;
  private messageChain = Promise.resolve();
  private lastAppliedID = 0;
  private replayRemaining = 0;
  private state: RealtimeState = "connecting";

  connect(): void {
    if (this.element !== document.body || this.scopeValue !== "page") {
      throw new Error('Recyclr must be mounted once on <body> with scope "page".');
    }
    if (this.gx) throw new Error("Duplicate page-scoped Recyclr connection attempted.");
    this.gx = createRecyclrGX();
    this.bundles = new RecyclrBundleQueue((events) => this.requireGX().render(events));
    (window as Window & { omniRecyclr?: RecyclrController }).omniRecyclr = this;
    this.setRealtimeState("connecting");
    this.stream = createRecyclrRealtimeStream((message) => this.enqueueRealtimeMessage(message));
    this.stream.start();
    this.monitorTimer = window.setInterval(() => this.monitorConnection(), 750);
  }

  disconnect(): void {
    this.stream?.stop();
    this.stream = null;
    if (this.monitorTimer != null) window.clearInterval(this.monitorTimer);
    this.monitorTimer = null;
    const owner = window as Window & { omniRecyclr?: RecyclrController };
    if (owner.omniRecyclr === this) delete owner.omniRecyclr;
    this.bundles = null;
    this.gx = null;
  }

  pushRoute(url: string): void {
    if (!this.requireGX().history) return;
    history.pushState(null, document.title, url);
  }

  renderBundle(html: string): Promise<void> {
    if (!this.bundles) throw new Error("Recyclr bundle queue is unavailable.");
    return this.bundles.enqueue(html);
  }

  private requireGX(): RecyclrGX {
    if (!this.gx) throw new Error("Page-scoped Recyclr GX is unavailable.");
    return this.gx;
  }

  private enqueueRealtimeMessage(message: Record<string, unknown>): void {
    this.messageChain = this.messageChain
      .then(() => this.applyRealtimeMessage(message))
      .catch((error) => this.handleRealtimeFailure(error));
  }

  private async applyRealtimeMessage(message: Record<string, unknown>): Promise<void> {
    if (message.eventName === "realtime-connected") {
      await this.handleConnectedMessage(message);
      return;
    }
    if (message.snapshot === true) {
      const html = String(message.html ?? "").trim();
      if (!html) throw new Error("Realtime snapshot did not include a server bundle.");
      await this.renderBundle(html);
      this.dispatchRealtimeMessage(message);
      return;
    }
    const messageID = Number(message.id ?? 0);
    if (!Number.isSafeInteger(messageID) || messageID <= 0) {
      throw new Error(`Realtime event ${JSON.stringify(message.eventName ?? "unknown")} has no valid id.`);
    }
    if (messageID <= this.lastAppliedID) return;
    if (this.lastAppliedID > 0 && messageID !== this.lastAppliedID + 1) {
      await this.requestAuthoritativeSync("message_gap", messageID);
    }
    const html = String(message.html ?? "").trim();
    if (html) await this.renderBundle(html);
    this.dispatchRealtimeMessage(message);
    this.lastAppliedID = messageID;
    if (this.replayRemaining > 0) this.replayRemaining--;
    this.setRealtimeState(this.replayRemaining > 0 ? "syncing" : "live");
  }

  private async handleConnectedMessage(message: Record<string, unknown>): Promise<void> {
    const latestID = Number(message.latestID ?? 0);
    const replayCount = Number(message.replayCount ?? 0);
    if (!Number.isSafeInteger(latestID) || latestID < 0 || !Number.isSafeInteger(replayCount) || replayCount < 0) {
      throw new Error("Realtime connection metadata is invalid.");
    }
    this.replayRemaining = replayCount;
    if (message.syncRequired === true) {
      this.lastAppliedID = latestID;
      this.replayRemaining = 0;
      await this.requestAuthoritativeSync("replay_gap", latestID);
    } else if (replayCount === 0) {
      if (latestID > this.lastAppliedID) {
        this.lastAppliedID = latestID;
        await this.requestAuthoritativeSync("transport_ahead", latestID);
      } else {
        this.lastAppliedID = Math.max(this.lastAppliedID, latestID);
      }
    }
    this.setRealtimeState(replayCount > 0 ? "syncing" : "live");
  }

  private dispatchRealtimeMessage(message: Record<string, unknown>): void {
    const toast = String(message.toast ?? "").trim();
    if (toast) {
      const tone = String(message.toastTone ?? "info").trim() as ToastTone;
      showToast(toast, tone === "error" || tone === "ok" || tone === "busy" ? tone : "info");
    }
    const eventName = String(message.eventName ?? "");
    const eventMap: Record<string, string> = {
      "metrics-glance": "omni:metrics-glance",
      "scrum-card-updated": "omni:scrum-card-updated",
      "scrum-board-refresh": "omni:scrum-refresh",
      "job-progress": "omni:job-progress",
      "ai-control-updated": "omni:ai-control-updated",
    };
    const browserEvent = eventMap[eventName];
    if (browserEvent) document.dispatchEvent(new CustomEvent(browserEvent, { detail: message }));
    document.dispatchEvent(new CustomEvent("omni:realtime-activity", { detail: message }));
  }

  private async requestAuthoritativeSync(reason: string, latestID: number): Promise<void> {
    this.setRealtimeState("syncing");
    await requestRealtimeSync(reason, latestID);
    this.setRealtimeState("live");
  }

  private monitorConnection(): void {
    if (!this.stream?.isConnected()) {
      if (this.state !== "error") this.setRealtimeState("reconnecting");
      return;
    }
    if (this.state === "connecting" || this.state === "reconnecting") this.setRealtimeState("syncing");
  }

  private async handleRealtimeFailure(error: unknown): Promise<void> {
    const message = error instanceof Error ? error.message : String(error);
    console.error("Realtime update failed", error);
    this.setRealtimeState("error", message);
    showToast(t("realtime.resyncing"), "error");
    try {
      await this.requestAuthoritativeSync("render_failure", this.lastAppliedID);
    } catch (syncError) {
      const syncMessage = syncError instanceof Error ? syncError.message : String(syncError);
      console.error("Realtime server synchronization failed", syncError);
      this.setRealtimeState("error", `${message} ${syncMessage}`);
      showToast(t("realtime.syncFailed"), "error");
    }
  }

  private setRealtimeState(state: RealtimeState, detail = ""): void {
    if (this.state === state && !detail) return;
    this.state = state;
    const transport = this.stream?.transport();
    const labels: Record<RealtimeState, string> = {
      connecting: t("realtime.connecting"),
      syncing: t("realtime.syncing"),
      live: transport === "ws" ? t("realtime.liveWebSocket") : t("realtime.live"),
      reconnecting: t("realtime.reconnecting"),
      error: t("realtime.error"),
    };
    if (this.hasStatusTarget) {
      this.statusTarget.textContent = labels[state];
      this.statusTarget.dataset.state = state;
      this.statusTarget.title = detail || labels[state];
    }
    if (this.hasIndicatorTarget) {
      this.indicatorTarget.className = state === "error"
        ? "inline-block h-2 w-2 rounded-full bg-rose-400"
        : state === "live"
          ? "inline-block h-2 w-2 rounded-full bg-cyan-300 shadow-[0_0_8px_rgba(103,232,249,.8)]"
          : "inline-block h-2 w-2 animate-pulse rounded-full bg-amber-300";
    }
    document.dispatchEvent(new CustomEvent("omni:realtime-status", { detail: { state, transport, message: labels[state] } }));
  }
}
