import type { StatusTone } from "./types";

export type ChatActivitySource = "request" | "turn" | "transport" | "pulse";
export type ChatTransportActivityState = "connecting" | "syncing" | "live" | "reconnecting" | "error";

export interface ChatActivityIndicatorHost {
  hasIndicator(): boolean;
  indicator(): HTMLButtonElement;
  text(): HTMLElement;
  dot(): HTMLElement;
  problems(): HTMLElement;
}

export class ChatActivityIndicator {
  private readonly activeSources = new Set<ChatActivitySource>();
  private problemCount = 0;
  private latestProblem = "";
  private pulseTimer: number | null = null;
  private lastProblemAt = 0;

  constructor(private readonly host: ChatActivityIndicatorHost) {}

  begin(source: ChatActivitySource): void {
    this.activeSources.add(source);
    this.render();
  }

  end(source: ChatActivitySource): void {
    this.activeSources.delete(source);
    this.render();
  }

  pulse(): void {
    this.begin("pulse");
    if (this.pulseTimer !== null) window.clearTimeout(this.pulseTimer);
    this.pulseTimer = window.setTimeout(() => {
      this.pulseTimer = null;
      this.end("pulse");
    }, 1200);
  }

  reportStatus(message: string, tone: StatusTone): void {
    if (tone === "active") this.pulse();
    if (tone === "error") this.problem(message);
  }

  reportTransport(state: ChatTransportActivityState, message: string): void {
    if (state === "connecting" || state === "syncing" || state === "reconnecting") {
      this.begin("transport");
      return;
    }
    this.end("transport");
    if (state === "error") this.problem(message);
  }

  problem(message: string): void {
    const normalized = message.trim();
    if (!normalized) throw new Error("System activity problems require a nonblank message.");
    const now = Date.now();
    if (normalized !== this.latestProblem || now - this.lastProblemAt > 1000) this.problemCount += 1;
    this.latestProblem = normalized;
    this.lastProblemAt = now;
    this.render();
  }

  acknowledge(): void {
    this.problemCount = 0;
    this.latestProblem = "";
    this.lastProblemAt = 0;
    this.render();
  }

  refresh(): void {
    this.render();
  }

  disconnect(): void {
    if (this.pulseTimer !== null) window.clearTimeout(this.pulseTimer);
    this.pulseTimer = null;
    this.activeSources.clear();
  }

  private render(): void {
    if (!this.host.hasIndicator()) return;
    const indicator = this.host.indicator();
    const active = this.activeSources.size > 0;
    const state = active ? "active" : this.problemCount > 0 ? "problem" : "ready";
    const label = state === "active" ? "Active" : state === "problem" ? "Problem" : "Ready";
    indicator.dataset.state = state;
    indicator.title = this.problemCount > 0
      ? `${this.problemCount} unacknowledged problem${this.problemCount === 1 ? "" : "s"}. Latest: ${this.latestProblem}`
      : active ? "Omnidex is active." : "Omnidex is ready. No reported problems.";
    indicator.setAttribute("aria-label", `System activity: ${label}. ${indicator.title}`);
    this.host.text().textContent = label;
    this.host.dot().setAttribute("aria-hidden", "true");
    const problems = this.host.problems();
    problems.textContent = String(this.problemCount);
    problems.classList.toggle("hidden", this.problemCount === 0);
  }
}
