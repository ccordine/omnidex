export type ScrumDragMoveHandler = (cardID: string, column: string) => void | Promise<void>;

const DRAG_THRESHOLD_PX = 8;

type DragSession = {
  cardID: string;
  sourceColumn: string;
  pointerId: number;
  startX: number;
  startY: number;
  dragging: boolean;
  ghost: HTMLElement | null;
  cardEl: HTMLElement;
};

export class ScrumBoardDrag {
  private board: HTMLElement | null = null;
  private onMove: ScrumDragMoveHandler | null = null;
  private session: DragSession | null = null;
  private suppressClickUntil = 0;
  private boundPointerDown = (event: PointerEvent) => this.onPointerDown(event);
  private boundPointerMove = (event: PointerEvent) => this.onPointerMove(event);
  private boundPointerUp = (event: PointerEvent) => this.onPointerUp(event);

  wire(board: HTMLElement, onMove: ScrumDragMoveHandler) {
    this.unwire();
    this.board = board;
    this.onMove = onMove;
    board.addEventListener("pointerdown", this.boundPointerDown);
  }

  unwire() {
    if (this.board) {
      this.board.removeEventListener("pointerdown", this.boundPointerDown);
    }
    this.detachDocumentListeners();
    this.cleanupSession(false);
    this.board = null;
    this.onMove = null;
  }

  shouldSuppressClick(): boolean {
    return Date.now() < this.suppressClickUntil;
  }

  private onPointerDown(event: PointerEvent) {
    if (!this.board || event.button !== 0) return;
    const target = event.target as HTMLElement;
    if (target.closest("button, select, option, a, textarea, input, label")) return;

    const cardEl = target.closest(".scrum-card[data-card-id]") as HTMLElement | null;
    if (!cardEl || !this.board.contains(cardEl)) return;

    const cardID = cardEl.dataset.cardId?.trim() ?? "";
    const sourceColumn = cardEl.dataset.scrumColumn?.trim() ?? "";
    if (!cardID || !sourceColumn) return;

    this.session = {
      cardID,
      sourceColumn,
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      dragging: false,
      ghost: null,
      cardEl,
    };

    cardEl.setPointerCapture(event.pointerId);
    this.attachDocumentListeners();
  }

  private onPointerMove(event: PointerEvent) {
    const session = this.session;
    if (!session || event.pointerId !== session.pointerId) return;

    if (!session.dragging) {
      const dx = event.clientX - session.startX;
      const dy = event.clientY - session.startY;
      if (Math.hypot(dx, dy) < DRAG_THRESHOLD_PX) return;
      this.beginDrag(session, event);
    }

    event.preventDefault();
    this.moveGhost(session, event.clientX, event.clientY);
    this.highlightDropTarget(event.clientX, event.clientY);
  }

  private onPointerUp(event: PointerEvent) {
    const session = this.session;
    if (!session || event.pointerId !== session.pointerId) return;

    if (session.dragging) {
      event.preventDefault();
      const column = this.columnAtPoint(event.clientX, event.clientY);
      if (column && column !== session.sourceColumn) {
        this.suppressClickUntil = Date.now() + 400;
        void this.onMove?.(session.cardID, column);
      }
    }

    try {
      session.cardEl.releasePointerCapture(event.pointerId);
    } catch {
      // pointer may already be released
    }
    this.cleanupSession(session.dragging);
    this.detachDocumentListeners();
    this.session = null;
  }

  private beginDrag(session: DragSession, event: PointerEvent) {
    session.dragging = true;
    document.body.classList.add("scrum-drag-active");
    document.body.style.touchAction = "none";
    session.cardEl.classList.add("scrum-card-dragging");

    const rect = session.cardEl.getBoundingClientRect();
    const ghost = session.cardEl.cloneNode(true) as HTMLElement;
    ghost.classList.add("scrum-drag-ghost");
    ghost.style.width = `${rect.width}px`;
    ghost.removeAttribute("data-action");
    document.body.appendChild(ghost);
    session.ghost = ghost;
    this.moveGhost(session, event.clientX, event.clientY);
  }

  private moveGhost(session: DragSession, x: number, y: number) {
    if (!session.ghost) return;
    session.ghost.style.left = `${x}px`;
    session.ghost.style.top = `${y}px`;
  }

  private columnAtPoint(x: number, y: number): string | null {
    if (!this.board) return null;
    const previous = this.board.style.pointerEvents;
    this.board.style.pointerEvents = "none";
    const el = document.elementFromPoint(x, y);
    this.board.style.pointerEvents = previous;
    const zone = el?.closest("[data-scrum-dropzone]") as HTMLElement | null;
    return zone?.dataset.scrumDropzone?.trim() || null;
  }

  private highlightDropTarget(x: number, y: number) {
    if (!this.board) return;
    const column = this.columnAtPoint(x, y);
    for (const zone of this.board.querySelectorAll("[data-scrum-dropzone]")) {
      zone.classList.toggle("scrum-drop-target", (zone as HTMLElement).dataset.scrumDropzone === column);
    }
  }

  private cleanupSession(wasDragging: boolean) {
    const session = this.session;
    if (!session) return;

    session.cardEl.classList.remove("scrum-card-dragging");
    session.ghost?.remove();
    session.ghost = null;
    document.body.classList.remove("scrum-drag-active");
    document.body.style.touchAction = "";
    if (this.board) {
      for (const zone of this.board.querySelectorAll("[data-scrum-dropzone]")) {
        zone.classList.remove("scrum-drop-target");
      }
    }
    if (wasDragging) {
      this.suppressClickUntil = Date.now() + 400;
    }
  }

  private attachDocumentListeners() {
    document.addEventListener("pointermove", this.boundPointerMove, { passive: false });
    document.addEventListener("pointerup", this.boundPointerUp);
    document.addEventListener("pointercancel", this.boundPointerUp);
  }

  private detachDocumentListeners() {
    document.removeEventListener("pointermove", this.boundPointerMove);
    document.removeEventListener("pointerup", this.boundPointerUp);
    document.removeEventListener("pointercancel", this.boundPointerUp);
  }
}
