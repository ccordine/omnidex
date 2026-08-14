import { debounce } from "../lib/main_thread";
import { SCRUM_CARD_REALTIME_REASON } from "../lib/scrum_types";
import type { RealtimeSyncDetail } from "../lib/realtime_sync";
import { ScrumActionController } from "./scrum_controller_actions";

export default class ScrumController extends ScrumActionController {
  static targets = ["board", "status", "boardOverlay", "boardOverlayMessage"];

  private modalClosedHandler: (() => void) | null = null;
  private projectOpenedHandler: ((event: Event) => void) | null = null;
  private projectClosedHandler: (() => void) | null = null;
  private projectTabHandler: ((event: Event) => void) | null = null;
  private readonly scheduleBoardRefresh = debounce(() => {
    if (this.projectID) void this.reconcileBoardFromServer();
  }, 150);
  private readonly scrumRefreshHandler = (event: Event) => {
    const detail = (event as CustomEvent<{ project_id?: number; projectID?: number }>).detail;
    const eventProjectID = detail?.projectID ?? detail?.project_id;
    if (eventProjectID && this.projectID && eventProjectID !== this.projectID) return;
    this.scheduleBoardRefresh();
  };
  private readonly scrumCardRealtimeHandler = (event: Event) => {
    const detail = (event as CustomEvent<{ projectID?: number; reason?: string }>).detail;
    if (detail?.projectID && this.projectID && detail.projectID !== this.projectID) return;
    if (detail?.reason !== SCRUM_CARD_REALTIME_REASON.jobProgress) this.scheduleBoardRefresh();
  };
  private readonly realtimeSyncHandler = (event: Event) => {
    const detail = (event as CustomEvent<RealtimeSyncDetail>).detail;
    if (!detail || typeof detail.waitUntil !== "function") {
      throw new Error("Realtime synchronization event is missing waitUntil().");
    }
    detail.waitUntil(this.synchronizeRealtimeBoard());
  };
  private readonly reactCardModalTabHandler = (event: Event) => {
    const detail = (event as CustomEvent<{ card_id?: string; tab?: string }>).detail;
    this.cardModal.handleTabChanged(detail?.card_id, detail?.tab);
  };

  connect(): void {
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
      if (this.scrumTabActive && this.projectID && this.board) this.scheduleBoardRefresh();
    };
    document.addEventListener("omni:project-tab", this.projectTabHandler);
    document.addEventListener("omni:scrum-refresh", this.scrumRefreshHandler);
    document.addEventListener("omni:scrum-card-updated", this.scrumCardRealtimeHandler);
    document.addEventListener("omni:realtime-sync-required", this.realtimeSyncHandler);
    document.addEventListener("omni:card-modal-tab-changed", this.reactCardModalTabHandler);
  }

  disconnect(): void {
    if (this.modalClosedHandler) document.removeEventListener("omni:modal-closed", this.modalClosedHandler);
    if (this.projectOpenedHandler) document.removeEventListener("omni:project-opened", this.projectOpenedHandler);
    if (this.projectClosedHandler) document.removeEventListener("omni:project-closed", this.projectClosedHandler);
    if (this.projectTabHandler) document.removeEventListener("omni:project-tab", this.projectTabHandler);
    document.removeEventListener("omni:scrum-refresh", this.scrumRefreshHandler);
    document.removeEventListener("omni:scrum-card-updated", this.scrumCardRealtimeHandler);
    document.removeEventListener("omni:realtime-sync-required", this.realtimeSyncHandler);
    document.removeEventListener("omni:card-modal-tab-changed", this.reactCardModalTabHandler);
    this.boardDrag.unwire();
  }
}
