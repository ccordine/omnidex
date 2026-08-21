import { fetchChatTimelinePage } from "../lib/chat_component_api";
import {
  ChatActivityIndicator,
  type ChatTransportActivityState,
} from "../lib/chat_activity_indicator";
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
  private turnActive = false;
  private activityIndicator: ChatActivityIndicator | null = null;

  protected initializeViewState(): void {
    this.seenProgress = new Set();
    this.busy = false;
    this.transcriptRenderPending = false;
    this.roleplayComposerAvailable = true;
    this.turnActive = false;
    this.systemActivity().acknowledge();
    this.syncTypingIndicatorState();
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
    this.systemActivity().pulse();
  }

  async renderJobState(bundle: string): Promise<void> {
    await this.renderComponentBundle(bundle);
  }

  setBusy(value: boolean): void {
    this.busy = value;
    this.syncComposerAvailability();
    if (this.hasSpinnerTarget) this.spinnerTarget.classList.toggle("hidden", !value);
    if (!value) this.activityLabel = "";
    if (value) this.systemActivity().begin("request");
    else this.systemActivity().end("request");
    this.syncTranscriptLoadingState();
    this.syncTypingIndicatorState();
  }

  setTurnActive(value: boolean): void {
    this.turnActive = value;
    if (value) this.systemActivity().begin("turn");
    else this.systemActivity().end("turn");
    this.syncComposerAvailability();
    this.syncTranscriptLoadingState();
    this.syncTypingIndicatorState();
  }

  isTurnActive(): boolean {
    return this.turnActive;
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
      this.sendTarget.disabled = disabled || this.turnActive;
      this.sendTarget.textContent = this.busy
        ? "Sending…"
        : this.turnActive
          ? "Replying…"
          : this.roleplayComposerAvailable
            ? "Send"
            : "Configure simulation";
    }
  }

  private syncTranscriptLoadingState(): void {
    const loading = this.busy || this.transcriptRenderPending;
    if (this.hasMessagesTarget) this.messagesTarget.setAttribute("aria-busy", String(loading || this.turnActive));
    if (this.hasTranscriptLoadingTarget) this.transcriptLoadingTarget.classList.toggle("hidden", !loading);
  }

  private syncTypingIndicatorState(): void {
    if (!this.hasTypingIndicatorTarget) return;
    const visible = this.turnActive || (this.busy && this.activityLabel.trim() !== "");
    this.typingIndicatorTarget.classList.toggle("hidden", !visible);
    this.typingIndicatorTarget.setAttribute("aria-hidden", String(!visible));
    if (visible && this.hasMessagesTarget) {
      this.messagesTarget.scrollTop = this.messagesTarget.scrollHeight;
    }
  }

  setStatus(text: string, mode: StatusTone): void {
    if (this.hasStatusTarget) this.statusTarget.textContent = text;
    this.systemActivity().reportStatus(text, mode);
  }

  acknowledgeActivityProblems(event: Event): void {
    event.preventDefault();
    this.systemActivity().acknowledge();
  }

  protected pulseSystemActivity(): void {
    this.systemActivity().pulse();
  }

  protected reportSystemProblem(message: string): void {
    this.systemActivity().problem(message);
  }

  protected reportTransportActivity(state: ChatTransportActivityState, message: string): void {
    this.systemActivity().reportTransport(state, message);
  }

  protected refreshSystemActivity(): void {
    this.systemActivity().refresh();
  }

  protected disconnectSystemActivity(): void {
    this.activityIndicator?.disconnect();
  }

  private systemActivity(): ChatActivityIndicator {
    this.activityIndicator ??= new ChatActivityIndicator({
      hasIndicator: () => this.hasLiveBadgeTarget,
      indicator: () => this.liveBadgeTarget,
      text: () => this.activityTextTarget,
      dot: () => this.activityDotTarget,
      problems: () => this.activityProblemsTarget,
    });
    return this.activityIndicator;
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
