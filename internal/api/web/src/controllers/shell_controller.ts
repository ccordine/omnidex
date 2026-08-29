import { Controller } from "@hotwired/stimulus";
import { readJSON, jsonRequest } from "../lib/api";
import { errorMessage, reportError, toastError, toastOk } from "../lib/feedback";
import { getLocale, isLocaleCode, t, tf } from "../lib/i18n";
import {
  aiControlDegradedMessage,
  validateAIControlMutationPayload,
  validateAIControlStatePayload,
  type AIControlMutationPayload,
  type AIControlStatePayload,
} from "../lib/ai_control_feedback";

type UISessionLocalePayload = {
  locale: string;
  state: Record<string, unknown>;
};

/** Keeps slide drawers open on tap/click (touch) until dismissed. Hover still works via CSS. */
export default class ShellController extends Controller {
  static targets = ["leftDrawer", "rightDrawer", "localeSelect", "aiControlButton", "aiControlStatus"];

  declare readonly leftDrawerTarget: HTMLElement;
  declare readonly rightDrawerTarget: HTMLElement;
  declare readonly hasLeftDrawerTarget: boolean;
  declare readonly hasRightDrawerTarget: boolean;
  declare readonly hasLocaleSelectTarget: boolean;
  declare readonly localeSelectTarget: HTMLSelectElement;
  declare readonly hasAiControlButtonTarget: boolean;
  declare readonly aiControlButtonTarget: HTMLButtonElement;
  declare readonly hasAiControlStatusTarget: boolean;
  declare readonly aiControlStatusTarget: HTMLElement;

  private pinnedSide: "left" | "right" | null = null;
  private aiPaused = false;
  private aiControlBusy = false;
  private aiControlLoad: Promise<void> | null = null;
  private aiControlReloadPending = false;
  private aiControlUpdatedHandler: ((event: Event) => void) | null = null;
  private jobProgressHandler: ((event: Event) => void) | null = null;
  private realtimeStatusHandler: ((event: Event) => void) | null = null;
  private lastAIControlUpdatedAt = 0;

  connect() {
    this.aiControlUpdatedHandler = (event) => this.handleAIControlUpdate(event);
    this.jobProgressHandler = (event) => this.handleJobProgress(event);
    this.realtimeStatusHandler = (event) => this.handleRealtimeStatus(event);
    document.addEventListener("omni:ai-control-updated", this.aiControlUpdatedHandler);
    document.addEventListener("omni:job-progress", this.jobProgressHandler);
    document.addEventListener("omni:realtime-status", this.realtimeStatusHandler);
    void this.loadAIControl();
  }

  disconnect() {
    if (this.aiControlUpdatedHandler) document.removeEventListener("omni:ai-control-updated", this.aiControlUpdatedHandler);
    if (this.jobProgressHandler) document.removeEventListener("omni:job-progress", this.jobProgressHandler);
    if (this.realtimeStatusHandler) document.removeEventListener("omni:realtime-status", this.realtimeStatusHandler);
    this.aiControlUpdatedHandler = null;
    this.jobProgressHandler = null;
    this.realtimeStatusHandler = null;
  }

  async setLocale(event: Event) {
    const select = event.currentTarget as HTMLSelectElement;
    const value = select.value;
    const previous = getLocale();
    if (value === previous) return;
    if (!isLocaleCode(value)) {
      select.value = previous;
      throw new Error(`Locale selector submitted unsupported locale ${JSON.stringify(value)}.`);
    }
    select.disabled = true;
    toastOk(t("locale.changing"));
    try {
      const payload = await readJSON<UISessionLocalePayload>(await fetch("/v1/ui/session", {
        ...jsonRequest({ state: { locale: value } }),
        method: "PATCH",
      }));
      if (payload.locale !== value || payload.state?.locale !== value) {
        throw new Error(`Server did not confirm requested locale ${JSON.stringify(value)}.`);
      }
      const location = new URL(window.location.href);
      location.searchParams.set("locale", value);
      window.location.assign(`${location.pathname}${location.search}${location.hash}`);
    } catch (error) {
      select.value = previous;
      select.disabled = false;
      console.error("UI locale update failed", error);
      toastError(errorMessage(error));
    }
  }

  toggleLeft(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    this.togglePin("left");
  }

  toggleRight(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    this.togglePin("right");
  }

  private togglePin(side: "left" | "right") {
    if (this.pinnedSide === side) {
      this.pinnedSide = null;
    } else {
      this.pinnedSide = side;
    }
    this.applyPinned();
  }

  private applyPinned() {
    if (this.hasLeftDrawerTarget) {
      this.leftDrawerTarget.classList.toggle("is-open", this.pinnedSide === "left");
    }
    if (this.hasRightDrawerTarget) {
      this.rightDrawerTarget.classList.toggle("is-open", this.pinnedSide === "right");
    }
  }

  dismissDrawers(event: Event) {
    const target = event.target as HTMLElement;
    if (target.closest(".slide-drawer")) return;
    if (this.pinnedSide == null) return;
    this.pinnedSide = null;
    this.applyPinned();
  }

  async toggleAIControl(event: Event) {
    event.preventDefault();
    if (this.aiControlBusy) return;
    this.aiControlBusy = true;
    if (this.hasAiControlButtonTarget) this.aiControlButtonTarget.setAttribute("aria-busy", "true");
    this.renderAIControl(this.aiPaused, t("status.working"));
	const action: "pause" | "resume" = this.aiPaused ? "resume" : "pause";
    let failure = "";
    try {
      const payload = validateAIControlMutationPayload(await readJSON<unknown>(await fetch("/v1/ai/control", jsonRequest({ action }))));
      this.applyAIControlPayload(payload);
      const canceled = payload.canceled_jobs;
		const degraded = aiControlDegradedMessage(payload);
		if (payload.commit_state === "committed_degraded" || degraded) {
			toastError(tf("ai.stateSavedDegraded", { error: degraded || "post-commit reconciliation failed" }));
		} else {
			toastOk(this.aiPaused ? (canceled ? tf("ai.actionPausedCanceled", { count: canceled }) : t("ai.actionPaused")) : t("ai.actionResumed"));
		}
    } catch (error) {
      failure = errorMessage(error);
      reportError((message) => this.renderAIControl(this.aiPaused, message), error);
    } finally {
      this.aiControlBusy = false;
      if (this.hasAiControlButtonTarget) this.aiControlButtonTarget.setAttribute("aria-busy", "false");
      this.renderAIControl(this.aiPaused, failure || t("status.checking"));
      if (this.aiControlReloadPending) {
        this.aiControlReloadPending = false;
        void this.loadAIControl();
      }
    }
  }

  private loadAIControl(): Promise<void> {
    if (this.aiControlBusy) {
      this.aiControlReloadPending = true;
      return Promise.resolve();
    }
    if (this.aiControlLoad) {
      this.aiControlReloadPending = true;
      return this.aiControlLoad;
    }
    this.aiControlLoad = (async () => {
      try {
        const payload = validateAIControlStatePayload(await readJSON<unknown>(await fetch("/v1/ai/control")));
        this.applyAIControlPayload(payload);
      } catch (error) {
        console.error("AI control synchronization failed", error);
        this.renderAIControl(this.aiPaused, t("ai.unavailable"));
      } finally {
        this.aiControlLoad = null;
        if (this.aiControlReloadPending && !this.aiControlBusy) {
          this.aiControlReloadPending = false;
          void this.loadAIControl();
        }
      }
    })();
    return this.aiControlLoad;
  }

  private handleAIControlUpdate(event: Event): void {
    try {
      const detail = (event as CustomEvent<Record<string, unknown>>).detail;
      const state = detail?.aiControl;
      if (!state || typeof state !== "object") throw new Error("AI control realtime event is missing state.");
      this.applyAIControlPayload(validateAIControlStatePayload(state));
    } catch (error) {
      console.error("AI control realtime update failed", error);
      this.renderAIControl(this.aiPaused, t("ai.syncFailed"));
      void this.loadAIControl();
    }
  }

  private handleJobProgress(event: Event): void {
    const detail = (event as CustomEvent<Record<string, unknown>>).detail;
    const phase = String(detail?.phase ?? "");
    if (phase === "queued" || phase === "state_changed" || phase === "finished") void this.loadAIControl();
  }

  private handleRealtimeStatus(event: Event): void {
    const detail = (event as CustomEvent<Record<string, unknown>>).detail;
    if (detail?.state === "live") void this.loadAIControl();
  }

  private applyAIControlPayload(payload: AIControlStatePayload | AIControlMutationPayload): void {
    if (typeof payload.paused !== "boolean") throw new Error("AI control payload.paused must be boolean.");
    const updatedAt = Date.parse(String(payload.updated_at ?? ""));
    if (!Number.isFinite(updatedAt)) throw new Error("AI control payload.updated_at must be a valid timestamp.");
    if (payload.counts != null && (typeof payload.counts !== "object" || Array.isArray(payload.counts))) {
      throw new Error("AI control payload.counts must be an object.");
    }
    for (const [status, count] of Object.entries(payload.counts ?? {})) {
      if (!Number.isSafeInteger(count) || count < 0) throw new Error(`AI control count ${status} must be a non-negative integer.`);
    }
    if (updatedAt < this.lastAIControlUpdatedAt) return;
    this.lastAIControlUpdatedAt = updatedAt;
    this.aiPaused = payload.paused;
    this.renderAIControl(this.aiPaused, undefined, payload.counts);
  }

  private renderAIControl(paused: boolean, status?: string, counts?: Record<string, number>) {
    if (this.hasAiControlButtonTarget) {
      const button = this.aiControlButtonTarget;
      button.textContent = paused ? "▶" : "⏸";
      button.title = paused ? t("ai.resumeAll") : t("ai.pauseAll");
      button.setAttribute("aria-label", paused ? t("ai.resumeAll") : t("ai.pauseAll"));
      button.disabled = this.aiControlBusy;
      button.className = paused
        ? "grid h-9 w-9 place-items-center rounded-md border border-emerald-300/30 bg-emerald-300/10 text-sm font-semibold text-emerald-100 transition hover:bg-emerald-300/20 disabled:opacity-60"
        : "grid h-9 w-9 place-items-center rounded-md border border-amber-300/30 bg-amber-300/10 text-sm font-semibold text-amber-100 transition hover:bg-amber-300/20 disabled:opacity-60";
    }
    if (!this.hasAiControlStatusTarget) return;
    if (counts === undefined && status === undefined) {
      throw new Error("AI control presentation requires authoritative counts or an explicit status.");
    }
    const running = counts?.running;
    const pending = counts?.pending;
    const waiting = counts?.waiting_input;
    this.aiControlStatusTarget.textContent = status || (paused
      ? tf("ai.pausedQueued", { pending: pending as number })
      : (waiting as number) > 0
        ? tf("ai.liveRunningWaiting", { running: running as number, waiting: waiting as number })
        : tf("ai.liveRunning", { running: running as number }));
    this.aiControlStatusTarget.className = paused ? "text-xs text-amber-200" : "text-xs text-emerald-200";
  }
}
