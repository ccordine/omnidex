import { reportError, reportErrorMessage, reportOk } from "./feedback";

export type ScrumStatusTone = "idle" | "busy" | "error" | "ok";

const TONE_CLASSES: Record<ScrumStatusTone, string> = {
  idle: "text-zinc-400",
  busy: "text-cyan-200",
  error: "text-rose-300",
  ok: "text-emerald-300",
};

type StatusTargetProvider = () => HTMLElement | null;

export class ScrumFeedback {
  constructor(private readonly statusTarget: StatusTargetProvider) {}

  set(message: string, tone: ScrumStatusTone = "idle"): void {
    const text = message.trim();
    if (!text) throw new Error("Scrum status requires a message.");
    const target = this.statusTarget();
    if (!target) return;
    target.textContent = text;
    target.className = `text-xs ${TONE_CLASSES[tone]}`;
  }

  ok(message: string): void {
    reportOk(this.set.bind(this), message);
  }

  fail(error: unknown): void {
    reportError(this.set.bind(this), error);
  }

  failMessage(message: string): void {
    reportErrorMessage(this.set.bind(this), message);
  }
}
