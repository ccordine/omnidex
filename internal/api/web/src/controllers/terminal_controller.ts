import { Controller } from "@hotwired/stimulus";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

export default class TerminalController extends Controller {
  static targets = ["mount", "frame", "status", "fullscreenButton"];

  declare readonly mountTarget: HTMLElement;
  declare readonly frameTarget: HTMLElement;
  declare readonly statusTarget: HTMLElement;
  declare readonly fullscreenButtonTarget: HTMLElement;

  private term: Terminal | null = null;
  private fitAddon: FitAddon | null = null;
  private socket: WebSocket | null = null;
  private projectID: number | null = null;
  private activeTab = "";
  private connected = false;
  private connecting = false;
  private resizeObserver: ResizeObserver | null = null;
  private onProjectOpened = (event: Event) => this.handleProjectOpened(event);
  private onProjectClosed = () => this.handleProjectClosed();
  private onProjectTab = (event: Event) => this.handleProjectTab(event);
  private onFullscreenChange = () => {
    this.syncFullscreenButton();
    this.fitTerminal();
    this.sendResize();
  };

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
    this.teardown();
  }

  reconnect(event: Event) {
    event.preventDefault();
    void this.connectTerminal(true);
  }

  toggleFullscreen(event: Event) {
    event.preventDefault();
    if (!this.frameTarget) return;
    if (document.fullscreenElement === this.frameTarget) {
      void document.exitFullscreen();
      return;
    }
    void this.frameTarget.requestFullscreen();
  }

  private handleProjectOpened(event: Event) {
    const detail = (event as CustomEvent<{ project_id?: number }>).detail;
    this.projectID = detail?.project_id ?? null;
    if (this.activeTab === "terminal") {
      void this.connectTerminal(false);
    }
  }

  private handleProjectClosed() {
    this.projectID = null;
    this.teardown();
  }

  private handleProjectTab(event: Event) {
    const detail = (event as CustomEvent<{ tab?: string; project_id?: number | null }>).detail;
    this.activeTab = detail?.tab ?? "";
    if (detail?.project_id) {
      this.projectID = detail.project_id;
    }
    if (this.activeTab === "terminal" && this.projectID) {
      void this.connectTerminal(false);
      return;
    }
    if (this.connected) {
      this.disconnectSocket();
    }
  }

  private async connectTerminal(force: boolean) {
    if (!this.projectID) return;
    if ((this.connected || this.connecting) && !force) return;

    this.disconnectSocket();
    this.ensureTerminal();
    this.connecting = true;
    this.setStatus("Connecting…", "busy");
    this.term?.reset();
    this.term?.focus();

    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    this.fitTerminal();

    const cols = Math.max(this.term?.cols ?? 0, 80);
    const rows = Math.max(this.term?.rows ?? 0, 24);
    const params = new URLSearchParams({
      project_id: String(this.projectID),
      cols: String(cols),
      rows: String(rows),
    });

    let wsURL: string;
    let mode = "proxy";
    try {
      const controller = new AbortController();
      const timeout = window.setTimeout(() => controller.abort(), 20000);
      const preflight = await fetch(`/v1/host/terminal/preflight?${params.toString()}`, {
        signal: controller.signal,
      });
      window.clearTimeout(timeout);
      const payload = (await preflight.json()) as {
        mode?: string;
        ws_url?: string;
        hint?: string;
        error?: string;
      };
      if (!preflight.ok) {
        throw new Error(payload.error?.trim() || `Preflight failed (${preflight.status})`);
      }
      wsURL = payload.ws_url?.trim() ?? "";
      mode = payload.mode?.trim() || "proxy";
      if (!wsURL) {
        throw new Error("Preflight did not return a terminal websocket URL");
      }
      if (payload.hint?.trim()) {
        console.info("[terminal]", payload.hint);
      }
    } catch (error) {
      this.connecting = false;
      const message =
        error instanceof DOMException && error.name === "AbortError"
          ? "Preflight timed out — is core reachable?"
          : error instanceof Error
            ? error.message
            : "Preflight failed";
      this.setStatus(message, "error");
      return;
    }

    const connectTimeout = window.setTimeout(() => {
      if (!this.connecting || this.socket?.readyState === WebSocket.OPEN) return;
      this.connecting = false;
      this.socket?.close();
      this.socket = null;
      const hint =
        mode === "direct"
          ? "Timed out connecting to host bridge (port 8091). Open port 8091 or unset OMNI_TERMINAL_DIRECT / HOST_BRIDGE_PUBLIC_WS_URL to proxy via core."
          : "Timed out connecting to terminal — check host bridge is running (`omni host serve --listen 0.0.0.0:8091`)";
      this.setStatus(hint, "error");
    }, 15000);

    const socket = new WebSocket(wsURL);
    socket.binaryType = "arraybuffer";
    this.socket = socket;

    socket.onopen = () => {
      window.clearTimeout(connectTimeout);
      this.connecting = false;
      this.connected = true;
      this.setStatus("Connected", "ok");
      this.fitTerminal();
      this.sendResize();
      this.term?.focus();
    };

    socket.onmessage = (message) => {
      if (!this.term) return;
      if (typeof message.data === "string") {
        this.term.write(message.data);
        return;
      }
      this.term.write(new Uint8Array(message.data));
    };

    socket.onerror = () => {
      window.clearTimeout(connectTimeout);
      this.connecting = false;
      if (this.connected) return;
      this.setStatus("Connection error", "error");
    };

    socket.onclose = (event) => {
      window.clearTimeout(connectTimeout);
      this.connecting = false;
      this.connected = false;
      if (this.socket === socket) {
        this.socket = null;
      }
      if (event.code === 1000) {
        this.setStatus("Shell exited", "idle");
        return;
      }
      const reason = event.reason?.trim();
      this.setStatus(reason ? `Disconnected: ${reason}` : `Disconnected (${event.code})`, "error");
    };
  }

  private ensureTerminal() {
    if (this.term) return;

    this.term = new Terminal({
      cursorBlink: true,
      fontFamily: '"JetBrains Mono", ui-monospace, SFMono-Regular, monospace',
      fontSize: 13,
      lineHeight: 1.25,
      theme: {
        background: "#09090b",
        foreground: "#e4e4e7",
        cursor: "#67e8f9",
        selectionBackground: "rgba(103, 232, 249, 0.25)",
        black: "#18181b",
        red: "#fb7185",
        green: "#34d399",
        yellow: "#fbbf24",
        blue: "#60a5fa",
        magenta: "#c084fc",
        cyan: "#22d3ee",
        white: "#f4f4f5",
        brightBlack: "#71717a",
        brightRed: "#fda4af",
        brightGreen: "#6ee7b7",
        brightYellow: "#fde68a",
        brightBlue: "#93c5fd",
        brightMagenta: "#d8b4fe",
        brightCyan: "#67e8f9",
        brightWhite: "#fafafa",
      },
      scrollback: 5000,
    });

    this.fitAddon = new FitAddon();
    this.term.loadAddon(this.fitAddon);
    this.term.open(this.mountTarget);
    this.term.onData((data) => {
      if (!this.socket || this.socket.readyState !== WebSocket.OPEN) return;
      this.socket.send(data);
    });

    this.resizeObserver = new ResizeObserver(() => {
      this.fitTerminal();
      this.sendResize();
    });
    this.resizeObserver.observe(this.frameTarget);
  }

  private fitTerminal() {
    if (!this.fitAddon || !this.term) return;
    try {
      this.fitAddon.fit();
    } catch {
      // mount may be hidden while another tab is active
    }
  }

  private sendResize() {
    if (!this.term || !this.socket || this.socket.readyState !== WebSocket.OPEN) return;
    this.socket.send(
      JSON.stringify({
        type: "resize",
        cols: this.term.cols,
        rows: this.term.rows,
      }),
    );
  }

  private disconnectSocket() {
    this.connected = false;
    this.connecting = false;
    if (this.socket) {
      this.socket.onopen = null;
      this.socket.onmessage = null;
      this.socket.onerror = null;
      this.socket.onclose = null;
      this.socket.close();
      this.socket = null;
    }
  }

  private teardown() {
    this.disconnectSocket();
    this.resizeObserver?.disconnect();
    this.resizeObserver = null;
    this.term?.dispose();
    this.term = null;
    this.fitAddon = null;
    this.mountTarget.innerHTML = "";
    this.setStatus("Idle", "idle");
    this.syncFullscreenButton();
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
    const active = document.fullscreenElement === this.frameTarget;
    this.fullscreenButtonTarget.textContent = active ? "Exit fullscreen" : "Fullscreen";
  }
}
