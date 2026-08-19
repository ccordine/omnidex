import { badgeClass } from "../lib/dom";
import { fetchChatTimelinePage, requireServerComponentBundle } from "../lib/chat_component_api";
import type { StatusTone } from "../lib/types";
import type RecyclrController from "./recyclr_controller";
import { ChatTargetsController } from "./chat_targets_controller";

export abstract class ChatViewController extends ChatTargetsController {
  recyclrController: RecyclrController | null = null;
  seenProgress = new Set<string>();
  busy = false;
  queueEnabled = false;
  openedProjectID: number | null = null;
  openedProjectLocation: string | null = null;
  activityLabel = "";
  private transcriptRenderPending = false;
  private roleplayComposerAvailable = true;

  protected initializeViewState(): void {
    this.seenProgress = new Set();
    this.busy = false;
    this.transcriptRenderPending = false;
    this.roleplayComposerAvailable = true;
  }

  addEvent(type: string, details: Record<string, unknown> = {}, full: unknown = null): void {
    console.debug("Omni UI event", { type, details, full: full ?? details });
  }

  addObservedEvent(key: string, type: string, details: Record<string, unknown> = {}, full: unknown = null): void {
    if (!key || this.seenProgress.has(key)) return;
    this.seenProgress.add(key);
    this.addEvent(type, details, full);
  }

  protected async renderComponentBundle(bundle: string): Promise<void> {
    if (!bundle.trim()) throw new Error("The server component bundle is empty.");
    if (!this.recyclrController) throw new Error("The page Recyclr bridge is unavailable.");
    await this.recyclrController.renderBundle(bundle);
  }

  protected async renderTranscriptBundle(bundle: string, preserveScroll: boolean): Promise<void> {
    if (!this.hasMessagesTarget) throw new Error("The server transcript sink is unavailable.");
    const priorHeight = this.messagesTarget.scrollHeight;
    const priorTop = this.messagesTarget.scrollTop;
    this.transcriptRenderPending = true;
    this.syncTranscriptLoadingState();
    try {
      await this.renderComponentBundle(bundle);
    } finally {
      this.transcriptRenderPending = false;
      this.syncTranscriptLoadingState();
    }
    this.messagesTarget.scrollTop = preserveScroll
      ? priorTop + this.messagesTarget.scrollHeight - priorHeight
      : this.messagesTarget.scrollHeight;
  }

  async loadTimeline(options: { quiet?: boolean; strict?: boolean; offset?: number } = {}): Promise<void> {
    if (!this.queueEnabled) return;
    try {
      const page = await fetchChatTimelinePage(options.offset ?? 0);
      await this.renderComponentBundle(page.html.bundle);
      this.addEvent("timeline_loaded", { next_offset: page.next_offset ?? 0, has_more: page.has_more });
    } catch (error) {
      if (!options.quiet) this.addEvent("timeline_failed", { error: errorMessage(error) });
      if (options.strict) throw error;
    }
  }

  async loadMoreTimeline(event: Event): Promise<void> {
    const button = event.currentTarget as HTMLButtonElement;
    const offset = Number(button.dataset.nextOffset ?? "");
    if (button.dataset.pageSection !== "timeline" || !Number.isSafeInteger(offset) || offset < 1) {
      throw new Error("The server-rendered timeline cursor is invalid.");
    }
    const label = button.textContent;
    button.disabled = true;
    button.setAttribute("aria-busy", "true");
    button.textContent = "Loading activity…";
    try {
      await this.loadTimeline({ strict: true, offset });
    } catch (error) {
      button.disabled = false;
      button.setAttribute("aria-busy", "false");
      button.textContent = label;
      throw error;
    }
  }

  renderProgressActivity(label: string): void {
    const text = label.trim();
    if (!text) throw new Error("Progress activity requires a nonblank loading label.");
    if (this.hasProgressStateTarget) this.progressStateTarget.textContent = text;
    if (this.hasProgressLoadingTarget) {
      this.progressLoadingTarget.textContent = text;
      this.progressLoadingTarget.classList.remove("hidden");
    }
  }

  async renderJobState(details: unknown): Promise<void> {
    await this.renderComponentBundle(requireServerComponentBundle(details, "Job state"));
  }

  setBusy(value: boolean): void {
    this.busy = value;
    this.syncComposerAvailability();
    if (this.hasSpinnerTarget) this.spinnerTarget.classList.toggle("hidden", !value);
    if (!value) this.activityLabel = "";
    this.syncTranscriptLoadingState();
  }

  setRoleplayComposerAvailable(available: boolean): void {
    this.roleplayComposerAvailable = available;
    this.syncComposerAvailability();
  }

  setComposerText(value: string): void {
    if (!this.hasInputTarget) throw new Error("The canonical chat composer is unavailable.");
    this.inputTarget.value = value;
  }

  focusComposer(): void {
    if (!this.hasInputTarget) throw new Error("The canonical chat composer is unavailable.");
    this.inputTarget.focus();
  }

  private syncComposerAvailability(): void {
    const disabled = this.busy || !this.roleplayComposerAvailable;
    if (this.hasInputTarget) this.inputTarget.disabled = disabled;
    if (this.hasSendTarget) {
      this.sendTarget.disabled = disabled;
      this.sendTarget.textContent = this.busy ? "Working" : this.roleplayComposerAvailable ? "Send" : "Configure simulation";
    }
  }

  private syncTranscriptLoadingState(): void {
    const loading = this.busy || this.transcriptRenderPending;
    if (this.hasMessagesTarget) this.messagesTarget.setAttribute("aria-busy", String(loading));
    if (this.hasTranscriptLoadingTarget) this.transcriptLoadingTarget.classList.toggle("hidden", !loading);
  }

  setStatus(text: string, mode: StatusTone): void {
    if (this.hasStatusTarget) this.statusTarget.textContent = text;
    if (this.hasLiveBadgeTarget) {
      this.liveBadgeTarget.textContent = text;
      this.liveBadgeTarget.className = badgeClass(mode);
    }
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
