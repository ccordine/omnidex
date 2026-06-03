import { Controller } from "@hotwired/stimulus";
import { readJSON, jsonRequest } from "../lib/api";
import { reportError, toastOk } from "../lib/feedback";
import { applyI18n, initI18n, LOCALE_OPTIONS, applyDocumentLocale, getLocale, setLocale, t, type LocaleCode } from "../lib/i18n";

type AIControlPayload = {
  paused: boolean;
  canceled_jobs?: number;
  counts?: Record<string, number>;
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
  private localeChangedHandler: ((event: Event) => void) | null = null;
  private aiPaused = false;
  private aiControlBusy = false;
  private aiControlTimer: number | null = null;

  connect() {
    initI18n();
    this.populateLocaleSelect();
    this.localeChangedHandler = () => this.populateLocaleSelect();
    document.addEventListener("omni:locale-changed", this.localeChangedHandler);
    void this.loadAIControl();
    this.aiControlTimer = window.setInterval(() => void this.loadAIControl(), 8000);
  }

  disconnect() {
    if (this.localeChangedHandler) {
      document.removeEventListener("omni:locale-changed", this.localeChangedHandler);
    }
    if (this.aiControlTimer) {
      window.clearInterval(this.aiControlTimer);
      this.aiControlTimer = null;
    }
  }

  setLocale(event: Event) {
    const value = (event.currentTarget as HTMLSelectElement).value as LocaleCode;
    if (!value || value === getLocale()) return;
    setLocale(value);
    applyDocumentLocale();
    applyI18n(document);
    document.dispatchEvent(new CustomEvent("omni:locale-changed", { detail: { locale: value } }));
    toastOk(t("locale.changed"));
  }

  private populateLocaleSelect() {
    if (!this.hasLocaleSelectTarget) return;
    const current = getLocale();
    this.localeSelectTarget.innerHTML = LOCALE_OPTIONS.map(
      (option) => `<option value="${option.code}"${option.code === current ? " selected" : ""}>${option.nativeLabel}</option>`,
    ).join("");
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
    this.renderAIControl(this.aiPaused, "working");
    const action = this.aiPaused ? "resume" : "pause";
    try {
      const payload = await readJSON<AIControlPayload>(await fetch("/v1/ai/control", jsonRequest({ action })));
      this.aiPaused = Boolean(payload.paused);
      this.renderAIControl(this.aiPaused);
      const canceled = Number(payload.canceled_jobs || 0);
      toastOk(this.aiPaused ? `AI paused${canceled ? `; canceled ${canceled} active jobs` : ""}` : "AI resumed");
      document.dispatchEvent(new CustomEvent("omni:scrum-refresh", { detail: {} }));
    } catch (error) {
      reportError((message) => this.renderAIControl(this.aiPaused, message), error);
    } finally {
      this.aiControlBusy = false;
      this.renderAIControl(this.aiPaused);
    }
  }

  private async loadAIControl() {
    if (this.aiControlBusy) return;
    try {
      const payload = await readJSON<AIControlPayload>(await fetch("/v1/ai/control"));
      this.aiPaused = Boolean(payload.paused);
      this.renderAIControl(this.aiPaused, undefined, payload.counts);
    } catch {
      this.renderAIControl(this.aiPaused, "unavailable");
    }
  }

  private renderAIControl(paused: boolean, status?: string, counts?: Record<string, number>) {
    if (this.hasAiControlButtonTarget) {
      const button = this.aiControlButtonTarget;
      button.textContent = paused ? "▶" : "⏸";
      button.title = paused ? "Resume AI" : "Pause all AI";
      button.setAttribute("aria-label", paused ? "Resume AI" : "Pause all AI");
      button.disabled = this.aiControlBusy;
      button.className = paused
        ? "grid h-9 w-9 place-items-center rounded-md border border-emerald-300/30 bg-emerald-300/10 text-sm font-semibold text-emerald-100 transition hover:bg-emerald-300/20 disabled:opacity-60"
        : "grid h-9 w-9 place-items-center rounded-md border border-amber-300/30 bg-amber-300/10 text-sm font-semibold text-amber-100 transition hover:bg-amber-300/20 disabled:opacity-60";
    }
    if (!this.hasAiControlStatusTarget) return;
    const running = Number(counts?.running || 0);
    const pending = Number(counts?.pending || 0);
    const waiting = Number(counts?.waiting_input || 0);
    this.aiControlStatusTarget.textContent =
      status || (paused ? `paused · ${pending} queued` : `live · ${running} running${waiting ? ` · ${waiting} waiting` : ""}`);
    this.aiControlStatusTarget.className = paused ? "text-xs text-amber-200" : "text-xs text-emerald-200";
  }
}
