import { createScrumCard, pauseScrumCard, playScrumCard } from "../lib/scrum_api";
import { openModalShell } from "../lib/modal";
import { fetchScrumCreateCardComponent } from "../lib/operational_component_api";
import { renderServerBundle } from "../lib/server_component_api";
import { yieldToMain } from "../lib/main_thread";
import { persistScrumViewportColumn, requireScrumViewportColumn, resolveScrumViewportColumn } from "../lib/scrum_viewport_state";
import { ScrumBoardController } from "./scrum_controller_board";

export abstract class ScrumActionController extends ScrumBoardController {
  stopCardClick(event: Event): void {
    event.stopPropagation();
  }

  cardID(event: Event): string {
    const cardID = (event.currentTarget as HTMLElement | null)?.dataset?.cardId;
    if (!cardID || cardID !== cardID.trim()) throw new Error("Scrum action control requires one exact card ID.");
    return cardID;
  }

  modalField(event: Event, name: string, preserveBytes = false): string {
    const root = (event.currentTarget as HTMLElement | null)?.closest("[data-recyclr-sink], [data-chat-target='modalPanel']");
    const field = root?.querySelector(`[data-scrum-field="${name}"]`) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null;
    if (!field) throw new Error(`Scrum modal field ${JSON.stringify(name)} is unavailable.`);
    return preserveBytes ? field.value : field.value.trim();
  }

  async openCreateCardModal(event?: Event): Promise<void> {
    event?.preventDefault();
    event?.stopPropagation();
    const clickedColumn = (event?.currentTarget as HTMLElement | null)?.dataset?.column;
    const column = clickedColumn === undefined ? "backlog" : requireScrumViewportColumn(clickedColumn, "Create-card column");
    const projectID = this.requiredProjectID();
    this.cardModal.clearCardRoute();
    try {
      const component = await fetchScrumCreateCardComponent(projectID, column);
      await renderServerBundle(this.recyclrHost(), component, "Scrum create-card component");
      openModalShell({ wide: true });
    } catch (error) {
      this.feedback.fail(error);
    }
  }

  private setModalSubmitting(submitting: boolean, label = "Create card"): void {
    const panel = document.querySelector('[data-chat-target="modalPanel"]');
    const button = panel?.querySelector('[data-scrum-submit="create"]') as HTMLButtonElement | null;
    if (!button) return;
    button.disabled = submitting;
    button.textContent = submitting ? `${label}…` : label;
  }

  protected async withBoardRefresh<T>(
    message: string,
    action: () => Promise<T>,
    options: { closeModal?: boolean } = {},
  ): Promise<T | undefined> {
    const finishWorking = this.workingState.start(message.endsWith("…") ? message : `${message}…`);
    this.feedback.set(message, "busy");
    try {
      const result = await action();
      if (this.projectID) await this.reloadBoard();
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

  closeModal(): void {
    this.cardModal.close();
  }

  async openCard(event: Event): Promise<void> {
    if (this.boardDrag.shouldSuppressClick()) return;
    const target = event.target as HTMLElement;
    if (target.closest("button, select, option, a, textarea, input, label")) return;
    const cardID = (target.closest("[data-card-id]") as HTMLElement | null)?.dataset.cardId;
    if (!cardID) return;
    try {
      await yieldToMain();
      this.cardModal.open(cardID);
    } catch (error) {
      this.feedback.fail(error);
    }
  }

  async load(): Promise<void> {
    if (this.busy || !this.projectID || !this.hasBoardTarget) return;
    this.busy = true;
    this.activeColumn = persistScrumViewportColumn(
      this.requiredProjectID(),
      resolveScrumViewportColumn(this.requiredProjectID(), this.activeColumn),
    );
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

  refresh(event?: Event): void {
    event?.preventDefault();
    void this.load();
  }

  selectColumn(event: Event): void {
    event.preventDefault();
    const column = requireScrumViewportColumn((event.currentTarget as HTMLElement | null)?.dataset.column, "Scrum column control");
    if (column === this.activeColumn) return;
    this.activeColumn = persistScrumViewportColumn(this.requiredProjectID(), column);
    this.cardOffset = 0;
    this.boardTracker.reset();
    void this.load();
  }

  loadCardPage(event: Event): void {
    event.preventDefault();
    const offset = Number((event.currentTarget as HTMLElement | null)?.dataset.cardOffset ?? -1);
    if (!Number.isSafeInteger(offset) || offset < 0) throw new Error("Scrum card page control is invalid.");
    this.cardOffset = offset;
    this.boardTracker.reset();
    void this.load();
  }

  async createCard(event: Event): Promise<void> {
    event.preventDefault();
    const title = this.modalField(event, "newTitle");
    if (!title) return;
    const description = this.modalField(event, "newDesc", true);
    const column = requireScrumViewportColumn(this.modalField(event, "newColumn"), "Create-card form column");
    this.setModalSubmitting(true, "Creating card");
    try {
      await this.withBoardRefresh(
        "Creating card…",
        () => createScrumCard(title, description, column, this.requiredProjectID()),
        { closeModal: true },
      );
    } finally {
      this.setModalSubmitting(false);
    }
  }

  play(event: Event): void {
    event.preventDefault();
    event.stopPropagation();
    void this.withPlayAction(this.cardID(event), false);
  }

  pivotPlay(event: Event): void {
    event.preventDefault();
    event.stopPropagation();
    void this.withPlayAction(this.cardID(event), true);
  }

  pausePlay(event: Event): void {
    event.preventDefault();
    event.stopPropagation();
    const cardID = this.cardID(event);
    const card = this.findCard(cardID);
    if (!card?.updated_at) throw new Error(`Scrum card ${cardID} lacks an observed server revision.`);
    void this.withBoardRefresh("Pausing play", () =>
      pauseScrumCard(cardID, card.updated_at, this.requiredProjectID()));
  }

  async withPlayAction(cardID: string, pivot: boolean): Promise<void> {
    const card = this.findCard(cardID);
    if (!card?.updated_at) throw new Error(`Scrum card ${cardID} lacks an observed server revision.`);
    await this.withBoardRefresh(
      pivot ? "Pivoting play…" : "Queueing play…",
      () => playScrumCard(cardID, card.updated_at, this.requiredProjectID(), { pivot }),
    );
  }
}
