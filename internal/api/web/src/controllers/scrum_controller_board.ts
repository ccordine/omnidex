import { Controller } from "@hotwired/stimulus";
import { fetchScrumBoard, moveScrumCard } from "../lib/scrum_api";
import { type ScrumBoard, type ScrumBoardResponse, type ScrumCard, type ScrumColumn } from "../lib/scrum_types";
import { ScrumBoardDrag, type ScrumDragDropResult } from "../lib/scrum_drag";
import { yieldToMain } from "../lib/main_thread";
import type RecyclrController from "./recyclr_controller";
import { showToast } from "../lib/toast";
import { ScrumBoardTracker } from "../lib/scrum_board_tracker";
import { ScrumCardModalHost } from "../lib/scrum_card_modal_host";
import { ScrumWorkingState } from "../lib/scrum_working_state";
import { ScrumFeedback } from "../lib/scrum_feedback";

export abstract class ScrumBoardController extends Controller {
  declare readonly boardTarget: HTMLElement;
  declare readonly statusTarget: HTMLElement;
  declare readonly hasBoardTarget: boolean;
  declare readonly hasStatusTarget: boolean;
  declare readonly boardOverlayTarget: HTMLElement;
  declare readonly boardOverlayMessageTarget: HTMLElement;

  protected board: ScrumBoard | null = null;
  protected busy = false;
  protected projectID: number | null = null;
  protected reconcileInFlight = false;
  protected readonly boardTracker = new ScrumBoardTracker();
  protected scrumTabActive = true;
  protected autoWorkEnabled = false;
  protected activeColumn: ScrumColumn = "assigned";
  protected cardOffset = 0;
  protected boardDrag = new ScrumBoardDrag();
  protected readonly cardModal = new ScrumCardModalHost(
    () => this.projectID,
    (cardID) => Boolean(this.findCard(cardID)),
  );
  protected readonly workingState = new ScrumWorkingState(() => ({
    overlay: this.boardOverlayTarget,
    message: this.boardOverlayMessageTarget,
  }));
  protected readonly feedback = new ScrumFeedback(() => (this.hasStatusTarget ? this.statusTarget : null));
  protected boardAbortController: AbortController | null = null;

  protected isPlayActive(): boolean {
    return this.hasLivePlayRunner() || this.autoWorkEnabled;
  }

  private hasLivePlayRunner(): boolean {
    return this.board?.cards.some((card) =>
      card.play_state === "running" || card.play_state === "queued",
    ) ?? false;
  }

  protected fetchBoardViewport(): Promise<ScrumBoardResponse> {
    this.boardAbortController?.abort();
    const controller = new AbortController();
    this.boardAbortController = controller;
    return fetchScrumBoard(
      this.requiredProjectID(),
      { column: this.activeColumn, cardOffset: this.cardOffset },
      controller.signal,
    );
  }

  protected async reconcileBoardFromServer(cardID?: string | null): Promise<ScrumCard | null> {
    if (!this.projectID || this.reconcileInFlight || this.boardDrag.isActive()) return null;
    this.reconcileInFlight = true;
    try {
      const payload = await this.fetchBoardViewport();
      await this.applyBoardPayload(payload, false);
      await yieldToMain();
      const id = cardID ?? this.cardModal.activeCardID();
      return id ? this.findCard(id) : null;
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return null;
      const message = error instanceof Error ? error.message : String(error);
      console.error("Scrum realtime reconciliation failed", error);
      this.feedback.set(`Live refresh failed: ${message}`, "error");
      return null;
    } finally {
      this.reconcileInFlight = false;
      this.boardAbortController = null;
    }
  }

  protected async synchronizeRealtimeBoard(): Promise<void> {
    if (!this.projectID || !this.scrumTabActive || !this.hasBoardTarget) return;
    try {
      const payload = await this.fetchBoardViewport();
      await this.applyBoardPayload(payload, false);
      await yieldToMain();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      console.error("Authoritative scrum realtime synchronization failed", error);
      this.feedback.set(`Live synchronization failed: ${message}`, "error");
      throw error;
    } finally {
      this.boardAbortController = null;
    }
  }

  private async applyServerBundle(bundle: string): Promise<void> {
    const html = bundle.trim();
    if (!html) throw new Error("Scrum board response did not include its required server-rendered Recyclr bundle.");
    await this.recyclrHost().renderBundle(html);
  }

  protected async applyBoardPayload(payload: ScrumBoardResponse, updateStatus = true): Promise<void> {
    if (!this.hasBoardTarget) throw new Error("Scrum board target is unavailable.");
    const transition = this.boardTracker.prepare(payload);
    if (transition.duplicate) return;
    this.board = payload.board;
    this.autoWorkEnabled = payload.auto_work.enabled;
    this.cardOffset = payload.card_offset;
    await this.applyServerBundle(payload.html.bundle);
    transition.commit();
    transition.notices.forEach((notice) => showToast(notice.message, notice.tone));
    if (updateStatus && this.isPlayActive()) {
      const queued = payload.play_queue.queued_count;
      const running = payload.play_queue.running_card_id ? "running" : "idle";
      const autoNote = this.autoWorkEnabled ? ", auto-work on" : "";
      this.feedback.set(`Play queue: ${running}${queued ? `, ${queued} queued` : ""}${autoNote}`, "ok");
    }
    this.wireBoardDragDrop();
  }

  private wireBoardDragDrop(): void {
    if (!this.hasBoardTarget) return;
    this.boardDrag.wire(this.boardTarget, (result) => void this.persistCardPlacement(result));
  }

  private async persistCardPlacement(result: ScrumDragDropResult): Promise<void> {
    this.boardTracker.markManualMove(result.cardID);
    const finishWorking = this.workingState.start("Moving card…");
    try {
      const revision = this.findCard(result.cardID)?.updated_at;
      if (!revision) throw new Error(`Scrum card ${result.cardID} has no authoritative revision in the current board page.`);
      const mutation = await moveScrumCard(result.cardID, result.column, revision, this.requiredProjectID(), {
        before_card_id: result.beforeCardID,
      });
      const payload = await this.fetchBoardViewport();
      await this.applyBoardPayload(payload, false);
      this.boardTracker.cancelManualMove(result.cardID);
      if (mutation.commit_state === "committed_degraded") {
        this.feedback.set(mutation.operation_error as string, "error");
      }
    } catch (error) {
      this.boardTracker.cancelManualMove(result.cardID);
      this.feedback.fail(error);
      await this.load();
    } finally {
      finishWorking();
    }
  }

  protected requiredProjectID(): number {
    if (!Number.isSafeInteger(this.projectID) || !this.projectID || this.projectID <= 0) {
      throw new Error("Scrum action requires one open server-authoritative project.");
    }
    return this.projectID;
  }

  protected recyclrHost(): RecyclrController {
    const controller = (window as Window & { omniRecyclr?: RecyclrController }).omniRecyclr;
    if (!controller) throw new Error("The page-scoped Recyclr controller is unavailable.");
    return controller;
  }

  protected async reloadBoard(): Promise<void> {
    const payload = await this.fetchBoardViewport();
    await this.applyBoardPayload(payload, false);
  }

  protected findCard(cardID: string): ScrumCard | null {
    return this.board?.cards.find((card) => card.id === cardID) ?? null;
  }

  abstract load(): Promise<void>;
}
