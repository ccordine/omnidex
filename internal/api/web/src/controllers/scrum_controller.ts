import { Controller } from "@hotwired/stimulus";
import {
  createScrumCard,
  fetchScrumBoard,
  moveScrumCard,
  pauseScrumCard,
  playScrumCard,
} from "../lib/scrum_api";
import { openModalShell } from "../lib/modal";
import { fetchScrumCreateCardComponent } from "../lib/operational_component_api";
import { renderServerBundle } from "../lib/server_component_api";
import type { ScrumBoard, ScrumBoardResponse, ScrumCard } from "../lib/scrum_types";
import { ScrumBoardDrag, type ScrumDragDropResult } from "../lib/scrum_drag";
import { debounce, yieldToMain } from "../lib/main_thread";
import type RecyclrController from "./recyclr_controller";
import { showToast } from "../lib/toast";
import type { RealtimeSyncDetail } from "../lib/realtime_sync";
import { ScrumBoardTracker } from "../lib/scrum_board_tracker";
import { ScrumCardModalHost } from "../lib/scrum_card_modal_host";
import { ScrumWorkingState } from "../lib/scrum_working_state";
import { ScrumFeedback } from "../lib/scrum_feedback";

export default class ScrumController extends Controller {
  static targets = ["board", "status", "boardOverlay", "boardOverlayMessage"];

  declare readonly boardTarget: HTMLElement;
  declare readonly statusTarget: HTMLElement;
  declare readonly hasBoardTarget: boolean;
  declare readonly hasStatusTarget: boolean;
  declare readonly boardOverlayTarget: HTMLElement;
  declare readonly boardOverlayMessageTarget: HTMLElement;

  private board: ScrumBoard | null = null;
  private busy = false;
  private projectID: number | null = null;
  private modalClosedHandler: ((event: Event) => void) | null = null;
  private projectOpenedHandler: ((event: Event) => void) | null = null;
  private projectClosedHandler: ((event: Event) => void) | null = null;
  private projectTabHandler: ((event: Event) => void) | null = null;
  private reconcileInFlight = false;
  private readonly boardTracker = new ScrumBoardTracker();
  private scrumTabActive = true;
  private playQueue: ScrumBoardResponse["play_queue"] | null = null;
  private autoWorkEnabled = false;
  private activeColumn = "assigned";
  private cardOffset = 0;
  private boardDrag = new ScrumBoardDrag();
  private readonly cardModal = new ScrumCardModalHost(
    () => this.projectID,
    (cardID) => Boolean(this.findCard(cardID)),
  );
  private readonly workingState = new ScrumWorkingState(() => ({
    overlay: this.boardOverlayTarget,
    message: this.boardOverlayMessageTarget,
  }));
  private readonly feedback = new ScrumFeedback(() => (this.hasStatusTarget ? this.statusTarget : null));
  private boardAbortController: AbortController | null = null;
  private readonly scheduleBoardRefresh = debounce(() => {
    if (this.projectID) void this.reconcileBoardFromServer();
  }, 150);
  private scrumRefreshHandler = (event: Event) => {
    const detail = (event as CustomEvent<{ project_id?: number; projectID?: number }>).detail;
    const eventProjectID = detail?.projectID ?? detail?.project_id;
    if (eventProjectID && this.projectID && eventProjectID !== this.projectID) return;
    this.scheduleBoardRefresh();
  };
  private scrumCardRealtimeHandler = (event: Event) => {
    const detail = (event as CustomEvent<{ projectID?: number; reason?: string }>).detail;
    if (detail?.projectID && this.projectID && detail.projectID !== this.projectID) return;
    if (detail?.reason !== "agent output") this.scheduleBoardRefresh();
  };
  private realtimeSyncHandler = (event: Event) => {
    const detail = (event as CustomEvent<RealtimeSyncDetail>).detail;
    if (!detail || typeof detail.waitUntil !== "function") {
      throw new Error("Realtime synchronization event is missing waitUntil().");
    }
    detail.waitUntil(this.synchronizeRealtimeBoard());
  };
  private reactCardModalTabHandler = (event: Event) => {
    const detail = (event as CustomEvent<{ card_id?: string; tab?: string }>).detail;
    this.cardModal.handleTabChanged(detail?.card_id, detail?.tab);
  };

  connect() {
    this.modalClosedHandler = () => this.cardModal.reset();
    document.addEventListener("omni:modal-closed", this.modalClosedHandler);

    this.projectOpenedHandler = (event: Event) => {
      const detail = (event as CustomEvent<{ project_id?: number }>).detail;
      const nextProjectID = detail?.project_id && detail.project_id > 0 ? detail.project_id : null;
      if (nextProjectID !== this.projectID) {
        this.cardModal.close();
        this.boardAbortController?.abort();
        this.busy = false;
        this.board = null;
				this.cardOffset = 0;
        this.boardTracker.reset();
      }
      this.projectID = nextProjectID;
      if (this.scrumTabActive) void this.load();
    };
    document.addEventListener("omni:project-opened", this.projectOpenedHandler);

    this.projectClosedHandler = () => {
      this.cardModal.close();
      this.projectID = null;
      this.board = null;
      this.boardTracker.reset();
      this.boardAbortController?.abort();
      this.boardAbortController = null;
      this.busy = false;
      this.scrumTabActive = true;
      this.activeColumn = "assigned";
      this.cardOffset = 0;
      if (this.hasBoardTarget) {
        this.boardTarget.replaceChildren();
        this.boardTarget.textContent = "Select a project to view its scrum board.";
      }
      this.feedback.set("No project open", "idle");
    };
    document.addEventListener("omni:project-closed", this.projectClosedHandler);

    this.projectTabHandler = (event: Event) => {
      const detail = (event as CustomEvent<{ tab?: string; project_id?: number }>).detail;
      if (detail?.project_id && detail.project_id !== this.projectID) return;
      this.scrumTabActive = detail?.tab === "scrum";
      if (this.scrumTabActive && this.projectID && this.board) {
        this.scheduleBoardRefresh();
      }
    };
    document.addEventListener("omni:project-tab", this.projectTabHandler);
    document.addEventListener("omni:scrum-refresh", this.scrumRefreshHandler);
    document.addEventListener("omni:scrum-card-updated", this.scrumCardRealtimeHandler);
    document.addEventListener("omni:realtime-sync-required", this.realtimeSyncHandler);
    document.addEventListener("omni:card-modal-tab-changed", this.reactCardModalTabHandler);
  }

  disconnect() {
    if (this.modalClosedHandler) {
      document.removeEventListener("omni:modal-closed", this.modalClosedHandler);
    }
    if (this.projectOpenedHandler) {
      document.removeEventListener("omni:project-opened", this.projectOpenedHandler);
    }
    if (this.projectClosedHandler) {
      document.removeEventListener("omni:project-closed", this.projectClosedHandler);
    }
    if (this.projectTabHandler) {
      document.removeEventListener("omni:project-tab", this.projectTabHandler);
    }
    document.removeEventListener("omni:scrum-refresh", this.scrumRefreshHandler);
    document.removeEventListener("omni:scrum-card-updated", this.scrumCardRealtimeHandler);
    document.removeEventListener("omni:realtime-sync-required", this.realtimeSyncHandler);
    document.removeEventListener("omni:card-modal-tab-changed", this.reactCardModalTabHandler);
    this.boardDrag.unwire();
  }

  private isPlayActive(): boolean {
    if (this.hasLivePlayRunner()) return true;
    if (this.autoWorkEnabled) return true;
    return false;
  }

  private hasLivePlayRunner(): boolean {
    return (
      this.board?.cards.some(
        (card) =>
			card.play_state === "running" || card.play_state === "queued",
      ) ?? false
    );
  }

  private columnSessionKey(): string {
    return `omni.scrum.column.${this.projectID ?? "global"}`;
  }

  private resolveActiveColumn(): string {
    const params = new URLSearchParams(window.location.search);
    const fromURL = params.get("scrum_column")?.trim();
    if (fromURL) return fromURL;
    return sessionStorage.getItem(this.columnSessionKey()) || this.activeColumn || "assigned";
  }

  private setActiveColumn(column: string, updateURL = true) {
    const next = column.trim() || "assigned";
    this.activeColumn = next;
    sessionStorage.setItem(this.columnSessionKey(), next);
    if (!updateURL) return;
    const url = new URL(window.location.href);
    url.searchParams.set("scrum_column", next);
    history.replaceState(null, document.title, `${url.pathname}${url.search}${url.hash}`);
  }

  private fetchBoardViewport(): Promise<ScrumBoardResponse> {
    this.boardAbortController?.abort();
    const controller = new AbortController();
    this.boardAbortController = controller;
    return fetchScrumBoard(this.projectID, { column: this.activeColumn, cardOffset: this.cardOffset }, controller.signal);
  }

  private async reconcileBoardFromServer(cardID?: string | null): Promise<ScrumCard | null> {
    if (!this.projectID || this.reconcileInFlight || this.boardDrag.isActive()) return null;
    this.reconcileInFlight = true;
    try {
      const payload = await this.fetchBoardViewport();
      await this.applyBoardPayload(payload, false);
      await yieldToMain();
      const id = cardID ?? this.cardModal.activeCardID();
      if (!id) return null;
      return this.findCard(id);
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

  private async synchronizeRealtimeBoard(): Promise<void> {
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

  private async applyServerBundle(bundle?: string | null): Promise<void> {
    const html = String(bundle ?? "").trim();
    if (!html) throw new Error("Scrum board response did not include its required server-rendered Recyclr bundle.");
    await this.recyclrHost().renderBundle(html);
  }

  private async applyBoardPayload(payload: ScrumBoardResponse, updateStatus = true): Promise<void> {
    if (!this.hasBoardTarget) throw new Error("Scrum board target is unavailable.");
    const transition = this.boardTracker.prepare(payload);
    if (transition.duplicate) return;
    this.board = payload.board;
    this.playQueue = payload.play_queue ?? null;
    this.autoWorkEnabled = Boolean(payload.auto_work?.enabled);
    this.cardOffset = payload.card_offset ?? 0;
    await this.applyServerBundle(payload.html?.bundle);
    transition.commit();
    transition.notices.forEach((notice) => {
      showToast(notice.message, notice.tone);
    });
    if (updateStatus && this.isPlayActive()) {
      const queued = payload.play_queue?.queued_count ?? 0;
      const running = payload.play_queue?.running_card_id ? "running" : "idle";
      const autoNote = this.autoWorkEnabled ? ", auto-work on" : "";
      this.feedback.set(`Play queue: ${running}${queued ? `, ${queued} queued` : ""}${autoNote}`, "ok");
    }
    this.wireBoardDragDrop();
  }

  private wireBoardDragDrop() {
    if (!this.hasBoardTarget) return;
    this.boardDrag.wire(this.boardTarget, (result) => {
      void this.persistCardPlacement(result);
    });
  }

  private async persistCardPlacement(result: ScrumDragDropResult) {
    this.boardTracker.markManualMove(result.cardID);
    const finishWorking = this.workingState.start("Moving card…");
    try {
      await moveScrumCard(result.cardID, result.column, this.projectID, {
        before_card_id: result.beforeCardID,
      });
      const payload = await this.fetchBoardViewport();
      await this.applyBoardPayload(payload, false);
      this.boardTracker.cancelManualMove(result.cardID);
    } catch (error) {
      this.boardTracker.cancelManualMove(result.cardID);
      this.feedback.fail(error);
      await this.load();
    } finally {
      finishWorking();
    }
  }

  stopCardClick(event: Event) {
    event.stopPropagation();
  }

  cardID(event: Event): string {
    const target = event.currentTarget as HTMLElement | null;
    return target?.dataset?.cardId || "";
  }

  modalField(event: Event, name: string): string {
    const root = (event.currentTarget as HTMLElement | null)?.closest("[data-recyclr-sink], [data-chat-target='modalPanel']");
    const field = root?.querySelector(`[data-scrum-field="${name}"]`) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null;
    return field?.value?.trim() ?? "";
  }

  async openCreateCardModal(event?: Event) {
    event?.preventDefault();
    event?.stopPropagation();
    const clickedColumn = (event?.currentTarget as HTMLElement | null)?.dataset?.column || "";
    const column = clickedColumn || "backlog";
    if (!this.projectID) throw new Error("Create card requires an open project.");
    this.cardModal.clearCardRoute();
    try {
      const component = await fetchScrumCreateCardComponent(this.projectID, column);
      await renderServerBundle(this.recyclrHost(), component, "Scrum create-card component");
      openModalShell({ wide: true });
    } catch (error) {
      this.feedback.fail(error);
    }
  }

  private setModalSubmitting(submitting: boolean, label = "Create card") {
    const panel = document.querySelector('[data-chat-target="modalPanel"]');
    const button = panel?.querySelector('[data-scrum-submit="create"]') as HTMLButtonElement | null;
    if (!button) return;
    button.disabled = submitting;
    button.textContent = submitting ? `${label}…` : label;
  }

  private async withBoardRefresh<T>(
    message: string,
    action: () => Promise<T>,
    options: { closeModal?: boolean } = {},
  ): Promise<T | undefined> {
    const finishWorking = this.workingState.start(message.endsWith("…") ? message : `${message}…`);
    this.feedback.set(message, "busy");
    try {
      const result = await action();
      if (this.projectID) {
        await this.reloadBoard();
      }
      if (options.closeModal) this.closeModal();
      const doneLabel = message.endsWith("…") ? message.slice(0, -1) : message;
      this.feedback.ok(`${doneLabel} complete`);
      return result;
    } catch (error) {
      this.feedback.fail(error);
      return undefined;
    } finally {
      finishWorking();
    }
  }

  private recyclrHost(): RecyclrController {
    const controller = (window as Window & { omniRecyclr?: RecyclrController }).omniRecyclr;
    if (!controller) throw new Error("The page-scoped Recyclr controller is unavailable.");
    return controller;
  }

  closeModal() {
    this.cardModal.close();
  }

  private async reloadBoard(): Promise<void> {
    const payload = await this.fetchBoardViewport();
    await this.applyBoardPayload(payload, false);
  }

  async openCard(event: Event) {
    if (this.boardDrag.shouldSuppressClick()) return;
    const target = event.target as HTMLElement;
    if (target.closest("button, select, option, a, textarea, input, label")) return;

    const article = target.closest("[data-card-id]") as HTMLElement | null;
    const cardID = article?.dataset.cardId;
    if (!cardID) return;

    try {
      await yieldToMain();
      this.cardModal.open(cardID);
    } catch (error) {
      this.feedback.fail(error);
    }
  }

  async load() {
    if (this.busy || !this.projectID || !this.hasBoardTarget) return;
    this.busy = true;
    this.setActiveColumn(this.resolveActiveColumn());
    const finishWorking = this.workingState.start("Loading board…");
    this.feedback.set("Loading board…", "busy");
    try {
      const payload = await this.fetchBoardViewport();
      await this.applyBoardPayload(payload);
      this.cardModal.openFromLocation();
      this.feedback.set(`Updated ${new Date().toLocaleTimeString()}`, "ok");
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      const message = error instanceof Error ? error.message : String(error);
      this.boardTarget.replaceChildren();
      this.boardTarget.textContent = `Failed to load scrum board: ${message}`;
      this.feedback.failMessage(message);
    } finally {
      finishWorking();
      this.busy = false;
      this.boardAbortController = null;
    }
  }

  refresh(event?: Event) {
    event?.preventDefault();
    void this.load();
  }

  selectColumn(event: Event) {
    event.preventDefault();
    const column = (event.currentTarget as HTMLElement | null)?.dataset.column?.trim();
    if (!column || column === this.activeColumn) return;
    this.setActiveColumn(column);
    this.cardOffset = 0;
    this.boardTracker.reset();
    void this.load();
  }

  loadCardPage(event: Event) {
    event.preventDefault();
    const offset = Number((event.currentTarget as HTMLElement | null)?.dataset.cardOffset ?? -1);
    if (!Number.isSafeInteger(offset) || offset < 0) {
      throw new Error("Scrum card page control is invalid.");
    }
    this.cardOffset = offset;
    this.boardTracker.reset();
    void this.load();
  }

  async createCard(event: Event) {
    event.preventDefault();
    const title = this.modalField(event, "newTitle");
    if (!title) return;

    const description = this.modalField(event, "newDesc");
    const column = this.modalField(event, "newColumn") || "backlog";

    this.setModalSubmitting(true, "Creating card");
    try {
      await this.withBoardRefresh(
        "Creating card…",
        () => createScrumCard(title, description, column, this.projectID),
        { closeModal: true },
      );
    } finally {
      this.setModalSubmitting(false);
    }
  }

  play(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const cardID = this.cardID(event);
    void this.withPlayAction(cardID, false);
  }

  pivotPlay(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const cardID = this.cardID(event);
    void this.withPlayAction(cardID, true);
  }

  pausePlay(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const cardID = this.cardID(event);
    if (!cardID) return;
    void this.withBoardRefresh("Pausing play", () => pauseScrumCard(cardID, this.projectID));
  }

  async withPlayAction(cardID: string, pivot: boolean) {
    if (!cardID) return;
    await this.withBoardRefresh(
      pivot ? "Pivoting play…" : "Queueing play…",
      () => playScrumCard(cardID, this.projectID, { pivot }),
    );
  }

  private findCard(cardID: string): ScrumCard | null {
    if (!this.board) return null;
    return this.board.cards.find((card) => card.id === cardID) ?? null;
  }
}
