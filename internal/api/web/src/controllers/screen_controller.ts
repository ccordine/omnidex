import { Controller } from "@hotwired/stimulus";

type ScreenMonitor = {
  id: string;
  name: string;
  width: number;
  height: number;
  primary?: boolean;
};

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
  private monitors: ScreenMonitor[] = [];
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

  toggleFullscreen(event: Event) {
    event.preventDefault();
    if (this.isImmersive()) {
      void this.exitImmersive();
      return;
    }
    void this.enterImmersive();
  }

  private immersive = false;
  private immersiveKeydown: ((event: KeyboardEvent) => void) | null = null;
  private immersiveRestore: { parent: HTMLElement; next: ChildNode | null } | null = null;

  private isImmersive(): boolean {
    return document.fullscreenElement === this.frameTarget || this.immersive;
  }

  private canUseNativeFullscreen(): boolean {
    if (!window.isSecureContext) return false;
    const enabled = document.fullscreenEnabled ?? (document as Document & { webkitFullscreenEnabled?: boolean }).webkitFullscreenEnabled;
    return enabled !== false;
  }

  private async enterImmersive() {
    if (this.canUseNativeFullscreen()) {
      try {
        await this.frameTarget.requestFullscreen();
        this.syncFullscreenButton();
        return;
      } catch {
        // Fall back to fixed overlay on LAN http:// or blocked API.
      }
    }
    this.enableImmersiveFallback();
  }

  private async exitImmersive() {
    if (document.fullscreenElement === this.frameTarget) {
      try {
        await document.exitFullscreen();
      } catch {
        // ignore
      }
    }
    this.disableImmersiveFallback();
  }

  private enableImmersiveFallback() {
    this.immersive = true;
    const frame = this.frameTarget;
    const parent = frame.parentElement;
    if (parent) {
      this.immersiveRestore = { parent, next: frame.nextSibling };
      document.body.appendChild(frame);
    }
    frame.classList.add("screen-fullscreen-fallback");
    document.body.classList.add("screen-fullscreen-active");
    this.immersiveKeydown = (event: KeyboardEvent) => {
      if (event.key === "Escape") void this.exitImmersive();
    };
    document.addEventListener("keydown", this.immersiveKeydown);
    this.syncFullscreenButton();
  }

  private disableImmersiveFallback() {
    this.immersive = false;
    const frame = this.frameTarget;
    frame.classList.remove("screen-fullscreen-fallback");
    document.body.classList.remove("screen-fullscreen-active");
    if (this.immersiveRestore) {
      const { parent, next } = this.immersiveRestore;
      if (next && next.parentElement === parent) parent.insertBefore(frame, next);
      else parent.appendChild(frame);
      this.immersiveRestore = null;
    }
    if (this.immersiveKeydown) {
      document.removeEventListener("keydown", this.immersiveKeydown);
      this.immersiveKeydown = null;
    }
    this.syncFullscreenButton();
  }

  private handleFullscreenChange() {
    if (document.fullscreenElement !== this.frameTarget && this.immersive) {
      this.disableImmersiveFallback();
      return;
    }
    this.syncFullscreenButton();
  }

  private handleProjectOpened(event: Event) {
    const detail = (event as CustomEvent<{ project_id?: number }>).detail;
    this.projectID = detail?.project_id ?? null;
    if (this.activeTab === "screen") {
      void this.prepareScreen(false);
    }
  }

  private handleProjectClosed() {
    this.projectID = null;
    this.monitors = [];
    this.stopStream();
    this.resetMonitorSelect();
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
      if (force || this.monitors.length === 0) {
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
    const response = await fetch(`/v1/host/screen/monitors?project_id=${this.projectID}`);
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(typeof payload.error === "string" ? payload.error : "Failed to load monitors");
    }
    const monitors = Array.isArray(payload.monitors) ? payload.monitors : [];
    this.monitors = monitors.map((item: Record<string, unknown>) => ({
      id: String(item.id ?? item.name ?? ""),
      name: String(item.name ?? item.id ?? "Monitor"),
      width: Number(item.width ?? 0),
      height: Number(item.height ?? 0),
      primary: Boolean(item.primary),
    }));
    this.renderMonitorOptions();
  }

  private renderMonitorOptions() {
    const select = this.monitorSelectTarget;
    if (!this.monitors.length) {
      select.innerHTML = `<option value="">No monitors found</option>`;
      return;
    }
    select.innerHTML = this.monitors
      .map((monitor) => {
        const suffix = monitor.primary ? " · primary" : "";
        const size = monitor.width && monitor.height ? ` (${monitor.width}×${monitor.height})` : "";
        return `<option value="${monitor.id}">${monitor.name}${size}${suffix}</option>`;
      })
      .join("");
    const primary = this.monitors.find((monitor) => monitor.primary);
    select.value = primary?.id ?? this.monitors[0]?.id ?? "";
  }

  private resetMonitorSelect() {
    this.monitorSelectTarget.innerHTML = `<option value="">Loading…</option>`;
  }

  private async startStream(force: boolean) {
    if (!this.projectID) return;
    if (!this.monitors.length) {
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
    this.frameTarget.classList.toggle("screen-fullscreen-active", active);
  }
}
