import { Controller } from "@hotwired/stimulus";
import { fetchScreenMonitorsComponent } from "../lib/operational_component_api";
import { renderServerBundle } from "../lib/server_component_api";
import type RecyclrController from "./recyclr_controller";

export default class ScreenController extends Controller {
  static targets = ["frame", "stream", "placeholder", "status", "monitorSelect", "fpsSelect", "scaleSelect", "streamUrl", "fullscreenButton"];

  declare readonly frameTarget: HTMLElement;
  declare readonly streamTarget: HTMLImageElement;
  declare readonly placeholderTarget: HTMLElement;
  declare readonly statusTarget: HTMLElement;
  declare readonly monitorSelectTarget: HTMLSelectElement;
  declare readonly fpsSelectTarget: HTMLSelectElement;
  declare readonly scaleSelectTarget: HTMLSelectElement;
  declare readonly streamUrlTarget: HTMLInputElement;
  declare readonly fullscreenButtonTarget: HTMLButtonElement;

  private projectID: number | null = null;
  private activeTab = "";
  private monitorsLoaded = false;
  private monitorOffset = 0;
  private streamNonce = 0;
  private onProjectOpened = (event: Event) => this.handleProjectOpened(event);
  private onProjectClosed = () => this.handleProjectClosed();
  private onProjectTab = (event: Event) => this.handleProjectTab(event);
  private onFullscreenChange = () => this.handleFullscreenChange();

  connect() {
    document.addEventListener("omni:project-opened", this.onProjectOpened);
    document.addEventListener("omni:project-closed", this.onProjectClosed);
    document.addEventListener("omni:project-tab", this.onProjectTab);
    document.addEventListener("fullscreenchange", this.onFullscreenChange);
  }

  disconnect() {
    document.removeEventListener("omni:project-opened", this.onProjectOpened);
    document.removeEventListener("omni:project-closed", this.onProjectClosed);
    document.removeEventListener("omni:project-tab", this.onProjectTab);
    document.removeEventListener("fullscreenchange", this.onFullscreenChange);
    void this.exitImmersive();
    this.stopStream();
  }

  reconnect(event: Event) {
    event.preventDefault();
    void this.startStream(true);
  }

  changeMonitor(event: Event) {
    event.preventDefault();
    void this.startStream(true);
  }

  changeQuality(event: Event) {
    event.preventDefault();
    void this.startStream(true);
  }

  async loadMonitorPage(event: Event) {
    event.preventDefault();
    const offset = Number((event.currentTarget as HTMLElement).dataset.pageOffset ?? -1);
    if (!Number.isSafeInteger(offset) || offset < 0) {
      throw new Error("Monitor page offset is invalid.");
    }
    this.monitorOffset = offset;
    this.monitorsLoaded = false;
    this.stopStream();
    this.setStatus("Loading monitors…", "busy");
    try {
      await this.loadMonitors();
      await this.startStream(true);
    } catch (error) {
      this.stopStream();
      this.setStatus(error instanceof Error ? error.message : "Monitor page unavailable", "error");
    }
  }

  async toggleFullscreen(event: Event): Promise<void> {
    event.preventDefault();
    try {
      if (this.isImmersive()) {
        await this.exitImmersive();
        return;
      }
      await this.enterImmersive();
    } catch (error) {
      const message = error instanceof Error ? error.message : "Native fullscreen failed";
      this.setStatus(message, "error");
      throw error instanceof Error ? error : new Error(message);
    }
  }

  private isImmersive(): boolean {
    return document.fullscreenElement === this.frameTarget;
  }

  private async enterImmersive() {
    if (!window.isSecureContext) {
      throw new Error("Native fullscreen requires a secure browser context.");
    }
    if (!document.fullscreenEnabled || typeof this.frameTarget.requestFullscreen !== "function") {
      throw new Error("Native fullscreen is unavailable in this browser.");
    }
    try {
      await this.frameTarget.requestFullscreen();
    } catch (error) {
      const detail = error instanceof Error ? error.message : String(error);
      throw new Error(`Native fullscreen request failed: ${detail}`);
    }
    this.syncFullscreenButton();
  }

  private async exitImmersive() {
    if (document.fullscreenElement !== this.frameTarget) return;
    try {
      await document.exitFullscreen();
    } catch (error) {
      const detail = error instanceof Error ? error.message : String(error);
      throw new Error(`Native fullscreen exit failed: ${detail}`);
    }
    this.syncFullscreenButton();
  }

  private handleFullscreenChange() {
    this.syncFullscreenButton();
  }

  private handleProjectOpened(event: Event) {
    const detail = (event as CustomEvent<{ project_id?: number }>).detail;
    if (detail?.project_id !== this.projectID) {
      this.monitorOffset = 0;
      this.monitorsLoaded = false;
    }
    this.projectID = detail?.project_id ?? null;
    if (this.activeTab === "screen") {
      void this.prepareScreen(false);
    }
  }

  private handleProjectClosed() {
    this.projectID = null;
    this.monitorsLoaded = false;
    this.monitorOffset = 0;
    this.stopStream();
    this.setStatus("Idle", "idle");
  }

  private handleProjectTab(event: Event) {
    const detail = (event as CustomEvent<{ tab?: string; project_id?: number | null }>).detail;
    this.activeTab = detail?.tab ?? "";
    if (detail?.project_id) {
      this.projectID = detail.project_id;
    }
    if (this.activeTab === "screen" && this.projectID) {
      void this.prepareScreen(false);
      return;
    }
    this.stopStream();
  }

  private async prepareScreen(force: boolean) {
    if (!this.projectID) return;
    this.setStatus("Loading monitors…", "busy");
    try {
      if (force || !this.monitorsLoaded) {
        await this.loadMonitors();
      }
      await this.startStream(force);
    } catch (error) {
      this.stopStream();
      this.setStatus(error instanceof Error ? error.message : "Screen unavailable", "error");
    }
  }

  private async loadMonitors() {
    if (!this.projectID) return;
    const payload = await fetchScreenMonitorsComponent(this.projectID, this.monitorOffset);
    await renderServerBundle(this.recyclrController(), payload, "Screen monitors");
    if (!Number.isSafeInteger(payload.offset) || payload.offset < 0 || payload.offset !== this.monitorOffset) {
      throw new Error("Monitor component returned an invalid authoritative offset.");
    }
    this.monitorOffset = payload.offset;
    this.monitorsLoaded = Boolean(payload.monitor_id);
  }

  private async startStream(force: boolean) {
    if (!this.projectID) return;
    if (!this.monitorsLoaded) {
      throw new Error("No monitors available on the host");
    }
    const monitor = this.monitorSelectTarget.value.trim();
    if (!monitor) {
      throw new Error("Select a monitor");
    }

    const fps = this.fpsSelectTarget.value || "12";
    const scale = this.scaleSelectTarget.value || "75";
    const params = new URLSearchParams({
      project_id: String(this.projectID),
      monitor,
      fps,
      scale,
      quality: "5",
      t: String(force ? Date.now() : ++this.streamNonce),
    });

    const url = `/v1/host/screen/mjpeg?${params.toString()}`;
    this.streamUrlTarget.value = `${window.location.origin}${url}`;
    this.streamTarget.src = url;
    this.streamTarget.onerror = () => {
      this.setStatus("Stream error — check host bridge, ffmpeg, and grim", "error");
    };
    this.placeholderTarget.classList.add("hidden");
    this.setStatus("Streaming", "ok");
  }

  private recyclrController(): RecyclrController {
    const controller = this.application.getControllerForElementAndIdentifier(document.body, "recyclr") as RecyclrController | null;
    if (!controller) throw new Error("The page-scoped Recyclr controller is unavailable.");
    return controller;
  }

  private stopStream() {
    this.streamTarget.removeAttribute("src");
    this.streamUrlTarget.value = "";
    this.placeholderTarget.classList.remove("hidden");
  }

  private setStatus(message: string, tone: "idle" | "busy" | "error" | "ok") {
    const classes = {
      idle: "text-zinc-500",
      busy: "text-cyan-200",
      error: "text-rose-300",
      ok: "text-emerald-300",
    };
    this.statusTarget.textContent = message;
    this.statusTarget.className = `text-xs ${classes[tone] ?? classes.idle}`;
  }

  private syncFullscreenButton() {
    const active = this.isImmersive();
    this.fullscreenButtonTarget.textContent = active ? "Exit fullscreen" : "Fullscreen";
  }
}
