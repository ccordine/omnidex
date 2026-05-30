export type ScrumDragDropResult = {
  cardID: string;
  column: string;
  beforeCardID: string | null;
  sourceColumn: string;
};

export type ScrumDragDropHandler = (result: ScrumDragDropResult) => void | Promise<void>;

const DRAG_THRESHOLD_PX = 8;
const SCROLL_INTENT_PX = 6;
const EDGE_SCROLL_PX = 56;
const EDGE_SCROLL_MAX = 18;

type ScrollLockEntry = {
  el: HTMLElement;
  overflow: string;
  overscrollBehavior: string;
  touchAction: string;
};

type TouchProbe = {
  cardID: string;
  sourceColumn: string;
  pointerId: number;
  startX: number;
  startY: number;
  cardEl: HTMLElement;
  dropzone: HTMLElement;
  sourceIndex: number;
  onMove: (event: PointerEvent) => void;
  onUp: (event: PointerEvent) => void;
};

type DropTarget = {
  column: string;
  beforeCardID: string | null;
};

type DragSession = {
  cardID: string;
  sourceColumn: string;
  pointerId: number;
  pointerType: string;
  startX: number;
  startY: number;
  lastX: number;
  lastY: number;
  dragging: boolean;
  ghost: HTMLElement | null;
  placeholder: HTMLElement | null;
  cardEl: HTMLElement;
  dropTarget: DropTarget | null;
  sourceIndex: number;
};

export class ScrumBoardDrag {
  private board: HTMLElement | null = null;
  private onDrop: ScrumDragDropHandler | null = null;
  private session: DragSession | null = null;
  private suppressClickUntil = 0;
  private scrollLockEntries: ScrollLockEntry[] = [];
  private edgeScrollFrame = 0;
  private touchProbe: TouchProbe | null = null;
  private boundPointerDown = (event: PointerEvent) => this.onPointerDown(event);
  private boundPendingPointerMove = (event: PointerEvent) => this.onPendingPointerMove(event);
  private boundPointerMove = (event: PointerEvent) => this.onPointerMove(event);
  private boundPointerUp = (event: PointerEvent) => this.onPointerUp(event);
  private boundTouchMove = (event: TouchEvent) => this.onTouchMove(event);
  private boundTickEdgeScroll = () => this.tickEdgeScroll();

  wire(board: HTMLElement, onDrop: ScrumDragDropHandler) {
    this.unwire();
    this.board = board;
    this.onDrop = onDrop;
    board.addEventListener("pointerdown", this.boundPointerDown);
  }

  unwire() {
    if (this.board) {
      this.board.removeEventListener("pointerdown", this.boundPointerDown);
    }
    this.cancelTouchProbe();
    this.detachDocumentListeners();
    this.cleanupSession(false);
    this.board = null;
    this.onDrop = null;
  }

  shouldSuppressClick(): boolean {
    return Date.now() < this.suppressClickUntil;
  }

  isActive(): boolean {
    return this.session?.dragging === true;
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

    if (event.pointerType === "touch") {
      this.cancelTouchProbe();
      this.cancelPendingSession();
      this.startTouchProbe(event, cardEl, cardID, sourceColumn);
      return;
    }

    this.session = {
      cardID,
      sourceColumn,
      pointerId: event.pointerId,
      pointerType: event.pointerType,
      startX: event.clientX,
      startY: event.clientY,
      lastX: event.clientX,
      lastY: event.clientY,
      dragging: false,
      ghost: null,
      placeholder: null,
      cardEl,
      dropTarget: null,
      sourceIndex: this.cardIndex(cardEl),
    };

    this.attachPendingListeners();
  }

  private onPendingPointerMove(event: PointerEvent) {
    const session = this.session;
    if (!session || session.dragging || event.pointerId !== session.pointerId) return;

    session.lastX = event.clientX;
    session.lastY = event.clientY;

    const dx = event.clientX - session.startX;
    const dy = event.clientY - session.startY;

    if (session.pointerType === "touch" && this.isVerticalScrollIntent(dx, dy)) {
      this.cancelPendingSession();
      return;
    }

    if (Math.hypot(dx, dy) < DRAG_THRESHOLD_PX) return;

    if (session.pointerType === "touch" && Math.abs(dy) > Math.abs(dx)) {
      this.cancelPendingSession();
      return;
    }

    this.detachPendingListeners();
    this.beginDrag(session, event);
    this.attachDragListeners();
  }

  private isVerticalScrollIntent(dx: number, dy: number): boolean {
    return Math.abs(dy) >= SCROLL_INTENT_PX && Math.abs(dy) > Math.abs(dx);
  }

  private cancelPendingSession() {
    this.detachPendingListeners();
    this.session = null;
  }

  private startTouchProbe(event: PointerEvent, cardEl: HTMLElement, cardID: string, sourceColumn: string) {
    const dropzone = cardEl.closest(".scrum-column-dropzone") as HTMLElement | null;
    if (!dropzone) return;

    const probe: TouchProbe = {
      cardID,
      sourceColumn,
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      cardEl,
      dropzone,
      sourceIndex: this.cardIndex(cardEl),
      onMove: () => {},
      onUp: () => {},
    };

    probe.onMove = (moveEvent: PointerEvent) => {
      if (moveEvent.pointerId !== probe.pointerId) return;

      const dx = moveEvent.clientX - probe.startX;
      const dy = moveEvent.clientY - probe.startY;

      if (this.isVerticalScrollIntent(dx, dy)) {
        this.cancelTouchProbe();
        return;
      }

      if (Math.hypot(dx, dy) < DRAG_THRESHOLD_PX) return;

      if (Math.abs(dy) > Math.abs(dx)) {
        this.cancelTouchProbe();
        return;
      }

      this.cancelTouchProbe();
      this.session = {
        cardID: probe.cardID,
        sourceColumn: probe.sourceColumn,
        pointerId: probe.pointerId,
        pointerType: "touch",
        startX: probe.startX,
        startY: probe.startY,
        lastX: moveEvent.clientX,
        lastY: moveEvent.clientY,
        dragging: false,
        ghost: null,
        placeholder: null,
        cardEl: probe.cardEl,
        dropTarget: null,
        sourceIndex: probe.sourceIndex,
      };
      this.beginDrag(this.session, moveEvent);
      this.attachDragListeners();
    };

    probe.onUp = (upEvent: PointerEvent) => {
      if (upEvent.pointerId !== probe.pointerId) return;
      this.cancelTouchProbe();
    };

    this.touchProbe = probe;
    dropzone.addEventListener("pointermove", probe.onMove, { passive: true });
    dropzone.addEventListener("pointerup", probe.onUp);
    dropzone.addEventListener("pointercancel", probe.onUp);
  }

  private cancelTouchProbe() {
    const probe = this.touchProbe;
    if (!probe) return;
    probe.dropzone.removeEventListener("pointermove", probe.onMove);
    probe.dropzone.removeEventListener("pointerup", probe.onUp);
    probe.dropzone.removeEventListener("pointercancel", probe.onUp);
    this.touchProbe = null;
  }

  private onPointerMove(event: PointerEvent) {
    const session = this.session;
    if (!session || event.pointerId !== session.pointerId) return;

    if (session.dragging) {
      event.preventDefault();
    }

    session.lastX = event.clientX;
    session.lastY = event.clientY;

    this.moveGhost(session, event.clientX, event.clientY);
    session.dropTarget = this.dropTargetAt(event.clientX, event.clientY, session);
    this.updatePlaceholder(session);
    this.highlightDropTarget(session.dropTarget);
  }

  private onTouchMove(event: TouchEvent) {
    if (!this.session?.dragging) return;
    event.preventDefault();
  }

  private onPointerUp(event: PointerEvent) {
    const session = this.session;
    if (!session || event.pointerId !== session.pointerId) return;

    if (session.dragging) {
      event.preventDefault();
      session.ghost?.remove();
      session.ghost = null;
      const target = session.dropTarget ?? this.dropTargetAt(session.lastX, session.lastY, session);
      if (target && this.shouldCommitDrop(session, target)) {
        this.commitDrop(session, target);
        this.suppressClickUntil = Date.now() + 400;
        void this.onDrop?.({
          cardID: session.cardID,
          column: target.column,
          beforeCardID: target.beforeCardID,
          sourceColumn: session.sourceColumn,
        });
      } else if (session.placeholder) {
        session.placeholder.replaceWith(session.cardEl);
      }
    }

    try {
      session.cardEl.releasePointerCapture(event.pointerId);
    } catch {
      // pointer may already be released
    }
    this.cleanupSession(session.dragging);
    this.detachDragListeners();
    this.detachPendingListeners();
    this.session = null;
  }

  private beginDrag(session: DragSession, event: PointerEvent) {
    session.dragging = true;
    document.body.classList.add("scrum-drag-active");
    this.lockScrolling();
    try {
      session.cardEl.setPointerCapture(event.pointerId);
    } catch {
      // pointer may already be released
    }
    session.cardEl.classList.add("scrum-card-dragging");

    const rect = session.cardEl.getBoundingClientRect();
    const ghost = session.cardEl.cloneNode(true) as HTMLElement;
    ghost.classList.add("scrum-drag-ghost");
    ghost.style.width = `${rect.width}px`;
    ghost.removeAttribute("data-action");
    document.body.appendChild(ghost);
    session.ghost = ghost;

    session.placeholder = document.createElement("div");
    session.placeholder.className = "scrum-drop-placeholder";
    session.placeholder.style.height = `${rect.height}px`;
    session.cardEl.replaceWith(session.placeholder);

    this.moveGhost(session, event.clientX, event.clientY);
    session.dropTarget = this.dropTargetAt(event.clientX, event.clientY, session);
    this.updatePlaceholder(session);
    this.highlightDropTarget(session.dropTarget);
    this.startEdgeScroll();
  }

  private moveGhost(session: DragSession, x: number, y: number) {
    if (!session.ghost) return;
    session.ghost.style.left = `${x}px`;
    session.ghost.style.top = `${y}px`;
  }

  private dropzoneForColumn(column: string): HTMLElement | null {
    if (!this.board) return null;
    const columnEl = this.board.querySelector(`[data-scrum-dropzone="${CSS.escape(column)}"]`);
    return columnEl?.querySelector(".scrum-column-dropzone") as HTMLElement | null;
  }

  private dropTargetAt(x: number, y: number, session: DragSession): DropTarget | null {
    const column = this.columnAtPoint(x, y);
    if (!column) return null;

    const dropzone = this.dropzoneForColumn(column);
    if (!dropzone) return null;

    const cards = [...dropzone.querySelectorAll(".scrum-card[data-card-id]")].filter(
      (el) => (el as HTMLElement).dataset.cardId !== session.cardID,
    ) as HTMLElement[];

    for (const card of cards) {
      const rect = card.getBoundingClientRect();
      const midY = rect.top + rect.height / 2;
      if (y < midY) {
        return { column, beforeCardID: card.dataset.cardId?.trim() || null };
      }
    }

    return { column, beforeCardID: null };
  }

  private updatePlaceholder(session: DragSession) {
    if (!session.placeholder || !session.dropTarget) return;
    const dropzone = this.dropzoneForColumn(session.dropTarget.column);
    if (!dropzone) return;

    dropzone.querySelector(".scrum-column-empty")?.remove();

    const beforeEl = session.dropTarget.beforeCardID
      ? dropzone.querySelector(`.scrum-card[data-card-id="${CSS.escape(session.dropTarget.beforeCardID)}"]`)
      : null;

    if (beforeEl) {
      if (session.placeholder.nextElementSibling !== beforeEl) {
        dropzone.insertBefore(session.placeholder, beforeEl);
      }
      return;
    }

    if (dropzone.lastElementChild !== session.placeholder) {
      dropzone.appendChild(session.placeholder);
    }
  }

  private commitDrop(session: DragSession, target: DropTarget) {
    const dropzone = this.dropzoneForColumn(target.column);
    if (!dropzone || !session.placeholder) return;

    dropzone.querySelector(".scrum-column-empty")?.remove();
    session.cardEl.classList.remove("scrum-card-dragging");
    session.cardEl.dataset.scrumColumn = target.column;

    const beforeEl = target.beforeCardID
      ? dropzone.querySelector(`.scrum-card[data-card-id="${CSS.escape(target.beforeCardID)}"]`)
      : null;

    if (beforeEl) {
      dropzone.insertBefore(session.cardEl, beforeEl);
    } else {
      dropzone.appendChild(session.cardEl);
    }
    session.placeholder.remove();
    session.placeholder = null;
  }

  private cardIndex(cardEl: HTMLElement): number {
    const dropzone = cardEl.closest(".scrum-column-dropzone");
    if (!dropzone) return 0;
    return [...dropzone.querySelectorAll(".scrum-card[data-card-id]")].indexOf(cardEl);
  }

  private targetIndex(dropzone: HTMLElement, target: DropTarget, session: DragSession): number {
    const cards = [...dropzone.querySelectorAll(".scrum-card[data-card-id]")].filter(
      (el) => (el as HTMLElement).dataset.cardId !== session.cardID,
    ) as HTMLElement[];
    if (!target.beforeCardID) return cards.length;
    const index = cards.findIndex((card) => card.dataset.cardId === target.beforeCardID);
    return index >= 0 ? index : cards.length;
  }

  private shouldCommitDrop(session: DragSession, target: DropTarget): boolean {
    if (target.column !== session.sourceColumn) return true;
    const dropzone = this.dropzoneForColumn(target.column);
    if (!dropzone) return false;
    const nextIndex = this.targetIndex(dropzone, target, session);
    return nextIndex !== session.sourceIndex && nextIndex !== session.sourceIndex + 1;
  }

  private columnAtPoint(x: number, y: number): string | null {
    if (!this.board) return null;
    let match: string | null = null;
    for (const zone of this.board.querySelectorAll("[data-scrum-dropzone]")) {
      const el = zone as HTMLElement;
      const rect = el.getBoundingClientRect();
      if (x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom) {
        match = el.dataset.scrumDropzone?.trim() || null;
      }
    }
    return match;
  }

  private highlightDropTarget(target: DropTarget | null) {
    if (!this.board) return;
    for (const zone of this.board.querySelectorAll("[data-scrum-dropzone]")) {
      zone.classList.toggle("scrum-drop-target", target != null && (zone as HTMLElement).dataset.scrumDropzone === target.column);
    }
  }

  private getBoardScrollEl(): HTMLElement | null {
    if (!this.board) return null;
    return (this.board.closest("[data-scrum-board-scroll]") as HTMLElement | null) ?? this.board.parentElement;
  }

  private scrollableAncestors(): HTMLElement[] {
    const found: HTMLElement[] = [];
    if (!this.board) return found;

    let el: HTMLElement | null = this.board;
    while (el) {
      const style = getComputedStyle(el);
      const scrollable =
        /(auto|scroll)/.test(style.overflowY) ||
        /(auto|scroll)/.test(style.overflowX) ||
        /(auto|scroll)/.test(style.overflow);
      if (scrollable) found.push(el);
      el = el.parentElement;
    }
    return found;
  }

  private lockScrolling() {
    this.unlockScrolling();
    for (const el of this.scrollableAncestors()) {
      this.scrollLockEntries.push({
        el,
        overflow: el.style.overflow,
        overscrollBehavior: el.style.overscrollBehavior,
        touchAction: el.style.touchAction,
      });
      el.style.overscrollBehavior = "none";
      el.style.touchAction = "none";
    }
    document.body.style.touchAction = "none";
    document.body.style.overscrollBehavior = "none";
  }

  private unlockScrolling() {
    for (const entry of this.scrollLockEntries) {
      entry.el.style.overflow = entry.overflow;
      entry.el.style.overscrollBehavior = entry.overscrollBehavior;
      entry.el.style.touchAction = entry.touchAction;
    }
    this.scrollLockEntries = [];
    document.body.style.touchAction = "";
    document.body.style.overscrollBehavior = "";
  }

  private edgeScrollDelta(pointer: number, start: number, end: number): number {
    if (pointer < start + EDGE_SCROLL_PX) {
      const t = Math.max(0, 1 - (pointer - start) / EDGE_SCROLL_PX);
      return -Math.ceil(EDGE_SCROLL_MAX * t);
    }
    if (pointer > end - EDGE_SCROLL_PX) {
      const t = Math.max(0, 1 - (end - pointer) / EDGE_SCROLL_PX);
      return Math.ceil(EDGE_SCROLL_MAX * t);
    }
    return 0;
  }

  private startEdgeScroll() {
    if (this.edgeScrollFrame) return;
    this.edgeScrollFrame = requestAnimationFrame(this.boundTickEdgeScroll);
  }

  private stopEdgeScroll() {
    if (this.edgeScrollFrame) {
      cancelAnimationFrame(this.edgeScrollFrame);
      this.edgeScrollFrame = 0;
    }
  }

  private tickEdgeScroll() {
    const session = this.session;
    if (!session?.dragging) {
      this.stopEdgeScroll();
      return;
    }

    const x = session.lastX;
    const y = session.lastY;
    let scrolled = false;

    const boardScroll = this.getBoardScrollEl();
    if (boardScroll) {
      const rect = boardScroll.getBoundingClientRect();
      const dx = this.edgeScrollDelta(x, rect.left, rect.right);
      if (dx) {
        boardScroll.scrollLeft += dx;
        scrolled = true;
      }
    }

    const column = session.dropTarget?.column ?? this.columnAtPoint(x, y) ?? session.sourceColumn;
    const dropzone = this.dropzoneForColumn(column);
    if (dropzone) {
      const rect = dropzone.getBoundingClientRect();
      const dy = this.edgeScrollDelta(y, rect.top, rect.bottom);
      if (dy) {
        dropzone.scrollTop += dy;
        scrolled = true;
      }
    }

    for (const el of this.scrollableAncestors()) {
      if (el === boardScroll || el === dropzone || el.contains(dropzone ?? null)) continue;
      const style = getComputedStyle(el);
      if (!/(auto|scroll)/.test(style.overflowY)) continue;
      const rect = el.getBoundingClientRect();
      const dy = this.edgeScrollDelta(y, rect.top, rect.bottom);
      if (dy) {
        el.scrollTop += dy;
        scrolled = true;
      }
    }

    if (scrolled) {
      session.dropTarget = this.dropTargetAt(x, y, session);
      this.updatePlaceholder(session);
      this.highlightDropTarget(session.dropTarget);
    }

    this.edgeScrollFrame = requestAnimationFrame(this.boundTickEdgeScroll);
  }

  private cleanupSession(wasDragging: boolean) {
    const session = this.session;
    if (!session) return;

    this.cancelTouchProbe();
    this.stopEdgeScroll();
    this.unlockScrolling();
    session.cardEl.classList.remove("scrum-card-dragging");
    session.ghost?.remove();
    session.ghost = null;
    if (session.placeholder?.isConnected && !session.cardEl.isConnected) {
      session.placeholder.replaceWith(session.cardEl);
    }
    session.placeholder = null;
    document.body.classList.remove("scrum-drag-active");
    if (this.board) {
      for (const zone of this.board.querySelectorAll("[data-scrum-dropzone]")) {
        zone.classList.remove("scrum-drop-target");
      }
    }
    if (wasDragging) {
      this.suppressClickUntil = Date.now() + 400;
    }
  }

  private attachPendingListeners() {
    document.addEventListener("pointermove", this.boundPendingPointerMove, { passive: true });
    document.addEventListener("pointerup", this.boundPointerUp);
    document.addEventListener("pointercancel", this.boundPointerUp);
  }

  private detachPendingListeners() {
    document.removeEventListener("pointermove", this.boundPendingPointerMove);
    document.removeEventListener("pointerup", this.boundPointerUp);
    document.removeEventListener("pointercancel", this.boundPointerUp);
  }

  private attachDragListeners() {
    document.addEventListener("pointermove", this.boundPointerMove, { passive: false });
    document.addEventListener("pointerup", this.boundPointerUp);
    document.addEventListener("pointercancel", this.boundPointerUp);
    document.addEventListener("touchmove", this.boundTouchMove, { passive: false });
  }

  private detachDragListeners() {
    document.removeEventListener("pointermove", this.boundPointerMove);
    document.removeEventListener("pointerup", this.boundPointerUp);
    document.removeEventListener("pointercancel", this.boundPointerUp);
    document.removeEventListener("touchmove", this.boundTouchMove);
  }

  private detachDocumentListeners() {
    this.detachDragListeners();
    this.detachPendingListeners();
  }
}
