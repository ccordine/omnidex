import { Controller } from "@hotwired/stimulus";
import {
  chatScrumCard,
  createScrumCard,
  deleteScrumCard,
  doneScrumCard,
  fetchScrumBoard,
  fetchScrumFiles,
  moveScrumCard,
  pauseScrumCard,
  patchScrumCard,
  playScrumCard,
  syncScrumBoard,
  updateScrumBoard,
} from "../lib/scrum_api";
import { renderRecyclrBundle } from "../lib/recyclr";
import { fetchModelDefaults } from "../lib/model_config_api";
import { fetchAgentDefaults } from "../lib/agent_config_api";
import { collectModelFieldValues, clearModelFieldInputs } from "../lib/model_config_render";
import { collectAgentFieldValues, clearAgentFieldInputs } from "../lib/agent_config_render";
import type { ResolvedModelConfig } from "../lib/model_config_types";
import type { ResolvedAgentConfig } from "../lib/agent_config_types";
import { renderScrumBoard, renderScrumEmptyState } from "../lib/scrum_render";
import {
  renderScrumCardModal,
  renderScrumModalChat,
  renderScrumModalChecklist,
  renderScrumModalDetails,
  renderScrumModalSidebar,
} from "../lib/scrum_modal_render";
import { nextColumn, prevColumn, type ScrumBoard, type ScrumBoardResponse, type ScrumCard, type ScrumChecklistItem } from "../lib/scrum_types";
import { fetchWorkspace } from "../lib/project_api";
import type GxController from "./gx_controller";

export default class ScrumController extends Controller {
  static targets = [
    "board",
    "status",
    "projectDir",
    "boardName",
    "newTitle",
    "newDesc",
    "newColumn",
    "createForm",
  ];

  declare readonly boardTarget: HTMLElement;
  declare readonly statusTarget: HTMLElement;
  declare readonly projectDirTarget: HTMLInputElement;
  declare readonly boardNameTarget: HTMLElement;
  declare readonly newTitleTarget: HTMLInputElement;
  declare readonly newDescTarget: HTMLTextAreaElement;
  declare readonly newColumnTarget: HTMLSelectElement;
  declare readonly createFormTarget: HTMLFormElement;

  private board: ScrumBoard | null = null;
  private busy = false;
  private activeCardID: string | null = null;
  private projectFiles: string[] = [];
  private activeProjectID: number | null = null;
  private cardModelConfig: ResolvedModelConfig | null = null;
  private cardAgentConfig: ResolvedAgentConfig | null = null;
  private panelShownHandler: ((event: Event) => void) | null = null;
  private modalClosedHandler: ((event: Event) => void) | null = null;
  private projectActivatedHandler: ((event: Event) => void) | null = null;
  private pollTimer: number | null = null;
  private playQueue: ScrumBoardResponse["play_queue"] | null = null;

  connect() {
    this.panelShownHandler = (event: Event) => {
      const detail = (event as CustomEvent<{ panel?: string }>).detail;
      if (detail?.panel === "scrum") {
        void this.syncActiveProject().then(() => this.load());
        this.startPolling();
        return;
      }
      this.stopPolling();
    };
    document.addEventListener("omni:panel-shown", this.panelShownHandler);

    this.modalClosedHandler = () => this.resetModalShell();
    document.addEventListener("omni:modal-closed", this.modalClosedHandler);

    this.projectActivatedHandler = (event: Event) => {
      const detail = (event as CustomEvent<{ project_id?: number }>).detail;
      this.activeProjectID = detail?.project_id && detail.project_id > 0 ? detail.project_id : null;
    };
    document.addEventListener("omni:project-activated", this.projectActivatedHandler);
    void this.syncActiveProject();
  }

  disconnect() {
    if (this.panelShownHandler) {
      document.removeEventListener("omni:panel-shown", this.panelShownHandler);
    }
    if (this.modalClosedHandler) {
      document.removeEventListener("omni:modal-closed", this.modalClosedHandler);
    }
    if (this.projectActivatedHandler) {
      document.removeEventListener("omni:project-activated", this.projectActivatedHandler);
    }
    this.stopPolling();
  }

  private startPolling() {
    this.stopPolling();
    this.pollTimer = window.setInterval(() => {
      if (this.shouldPoll()) void this.pollBoard();
    }, 2500);
  }

  private stopPolling() {
    if (this.pollTimer != null) {
      window.clearInterval(this.pollTimer);
      this.pollTimer = null;
    }
  }

  private shouldPoll(): boolean {
    if (!this.board) return false;
    return this.board.cards.some((card) => card.play_state === "running" || card.play_state === "queued");
  }

  private async pollBoard() {
    try {
      const payload = await fetchScrumBoard(this.activeProjectID);
      this.applyBoardPayload(payload, false);
      if (this.activeCardID) await this.refreshModalSections(this.activeCardID);
    } catch {
      // keep last good board state during transient poll failures
    }
  }

  private applyBoardPayload(payload: ScrumBoardResponse, updateStatus = true) {
    this.board = payload.board;
    this.playQueue = payload.play_queue ?? null;
    this.projectDirTarget.value = payload.board.project_directory || "";
    if (this.hasBoardNameTarget) {
      this.boardNameTarget.textContent = payload.board.name || "Default board";
    }
    this.boardTarget.innerHTML = renderScrumBoard(payload.board, payload.cards_by_col, payload.play_queue);
    if (updateStatus && this.shouldPoll()) {
      const queued = payload.play_queue?.queued_count ?? 0;
      const running = payload.play_queue?.running_card_id ? "running" : "idle";
      this.setStatus(`Play queue: ${running}${queued ? `, ${queued} queued` : ""}`, "ok");
    }
  }

  private async syncActiveProject() {
    try {
      const workspace = await fetchWorkspace();
      this.activeProjectID = workspace.active_project_id > 0 ? workspace.active_project_id : null;
    } catch {
      this.activeProjectID = null;
    }
  }

  private resetModalShell() {
    this.activeCardID = null;
    const panel = document.querySelector('[data-chat-target="modalPanel"]') as HTMLElement | null;
    panel?.classList.remove("max-w-6xl");
    panel?.classList.add("max-w-5xl");
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

  setStatus(message: string, tone: "idle" | "busy" | "error" | "ok" = "idle") {
    const classes: Record<string, string> = {
      idle: "text-zinc-400",
      busy: "text-cyan-200",
      error: "text-rose-300",
      ok: "text-emerald-300",
    };
    this.statusTarget.textContent = message;
    this.statusTarget.className = `text-xs ${classes[tone] ?? classes.idle}`;
  }

  private gxHost(): GxController | null {
    return (window as Window & { omniRecyclr?: GxController }).omniRecyclr ?? null;
  }

  recycle(target: string, html: string): void {
    renderRecyclrBundle(this.gxHost(), target, html);
  }

  openModal(html: string): void {
    this.recycle("modal", html);
    const modal = document.querySelector('[data-chat-target="modal"]') as HTMLElement | null;
    modal?.classList.remove("hidden");
    modal?.classList.add("grid");
    const panel = document.querySelector('[data-chat-target="modalPanel"]') as HTMLElement | null;
    panel?.classList.remove("max-w-5xl");
    panel?.classList.add("max-w-6xl");
  }

  closeModal() {
    this.activeCardID = null;
    const modal = document.querySelector('[data-chat-target="modal"]') as HTMLElement | null;
    modal?.classList.add("hidden");
    modal?.classList.remove("grid");
    document.dispatchEvent(new CustomEvent("omni:modal-closed"));
  }

  private upsertCard(card: ScrumCard) {
    if (!this.board) return;
    const index = this.board.cards.findIndex((entry) => entry.id === card.id);
    if (index >= 0) this.board.cards[index] = card;
    else this.board.cards.push(card);
  }

  private async reloadBoard(cardID?: string | null): Promise<ScrumCard | null> {
    const payload = await fetchScrumBoard(this.activeProjectID);
    this.applyBoardPayload(payload);
    const id = cardID ?? this.activeCardID;
    if (!id) return null;
    return this.findCard(id);
  }

  private async loadProjectFiles(): Promise<string[]> {
    try {
      const payload = await fetchScrumFiles(this.activeProjectID);
      this.projectFiles = payload.files ?? [];
      return this.projectFiles;
    } catch {
      this.projectFiles = [];
      return [];
    }
  }

  async loadCardConfigs(cardID: string) {
    try {
      const [modelPayload, agentPayload] = await Promise.all([
        fetchModelDefaults(this.activeProjectID, cardID),
        fetchAgentDefaults(this.activeProjectID, cardID),
      ]);
      this.cardModelConfig = modelPayload.resolved ?? null;
      this.cardAgentConfig = agentPayload.resolved ?? null;
    } catch {
      this.cardModelConfig = null;
      this.cardAgentConfig = null;
    }
  }

  async refreshModalSections(cardID: string) {
    const card = this.findCard(cardID);
    if (!card || !this.board) return;
    const files = this.projectFiles.length ? this.projectFiles : await this.loadProjectFiles();
    await this.loadCardConfigs(cardID);
    this.recycle("scrum-modal-details", renderScrumModalDetails(card));
    this.recycle("scrum-modal-checklist", renderScrumModalChecklist(card));
    this.recycle("scrum-modal-chat", renderScrumModalChat(card));
    this.recycle(
      "scrum-modal-sidebar",
      renderScrumModalSidebar(
        card,
        this.board,
        files,
        this.cardModelConfig?.fields ?? [],
        this.cardModelConfig?.source ?? "env",
        this.cardAgentConfig?.fields ?? [],
        this.cardAgentConfig?.source ?? "env",
        this.cardAgentConfig?.system ?? "omnidex",
        this.playQueue ?? undefined,
      ),
    );
  }

  async openCard(event: Event) {
    const target = event.target as HTMLElement;
    if (target.closest("button, select, option, a, textarea, input, label")) return;

    const article = target.closest("[data-card-id]") as HTMLElement | null;
    const cardID = article?.dataset.cardId;
    if (!cardID) return;

    const card = this.findCard(cardID);
    if (!card || !this.board) return;

    this.activeCardID = cardID;
    const files = await this.loadProjectFiles();
    await this.loadCardConfigs(cardID);
    this.openModal(
      renderScrumCardModal(
        card,
        this.board,
        files,
        this.cardModelConfig?.fields ?? [],
        this.cardModelConfig?.source ?? "env",
        this.cardAgentConfig?.fields ?? [],
        this.cardAgentConfig?.source ?? "env",
        this.cardAgentConfig?.system ?? "omnidex",
      ),
    );
  }

  async load() {
    if (this.busy) return;
    this.busy = true;
    this.setStatus("Loading board…", "busy");
    try {
      const payload = await fetchScrumBoard(this.activeProjectID);
      this.applyBoardPayload(payload);
      if (this.activeCardID) await this.refreshModalSections(this.activeCardID);
      if (this.shouldPoll()) this.startPolling();
      this.setStatus(`Updated ${new Date().toLocaleTimeString()}`, "ok");
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      this.boardTarget.innerHTML = renderScrumEmptyState(`Failed to load scrum board: ${message}`);
      this.setStatus(message, "error");
    } finally {
      this.busy = false;
    }
  }

  refresh(event?: Event) {
    event?.preventDefault();
    void this.load();
  }

  async saveProjectDir(event: Event) {
    event.preventDefault();
    if (!this.board) {
      await this.load();
      if (!this.board) return;
    }
    this.setStatus("Saving project directory…", "busy");
    try {
      const updated = await updateScrumBoard(this.board.name, this.projectDirTarget.value.trim(), this.activeProjectID);
      this.board = updated;
      this.setStatus("Project directory saved", "ok");
      if (this.activeCardID) await this.refreshModalSections(this.activeCardID);
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async createCard(event: Event) {
    event.preventDefault();
    const title = this.newTitleTarget.value.trim();
    if (!title) return;

    const description = this.newDescTarget.value.trim();
    const column = this.newColumnTarget.value || "backlog";

    this.setStatus("Creating card…", "busy");
    try {
      await createScrumCard(title, description, column, this.activeProjectID);
      this.createFormTarget.reset();
      this.newColumnTarget.value = "backlog";
      await this.load();
      this.setStatus("Card created", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async withCardAction(cardID: string, action: () => Promise<ScrumCard>, label: string) {
    if (!cardID) return;
    this.setStatus(`${label}…`, "busy");
    try {
      const card = await action();
      this.upsertCard(card);
      await this.reloadBoard(cardID);
      if (this.activeCardID === cardID) await this.refreshModalSections(cardID);
      this.setStatus(`${label} complete`, "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
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
    void this.withCardAction(cardID, () => pauseScrumCard(cardID, this.activeProjectID), "Pausing play");
  }

  async withPlayAction(cardID: string, pivot: boolean) {
    if (!cardID) return;
    this.setStatus(pivot ? "Pivoting play…" : "Queueing play…", "busy");
    try {
      const result = await playScrumCard(cardID, this.activeProjectID, { pivot });
      this.upsertCard(result);
      await this.reloadBoard(cardID);
      if (this.activeCardID === cardID) await this.refreshModalSections(cardID);
      this.startPolling();
      this.setStatus(result.message || (pivot ? "Pivoted" : "Play updated"), "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  syncJob(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    this.setStatus("Refreshing play queue…", "busy");
    void syncScrumBoard(this.activeProjectID)
      .then((payload) => {
        this.applyBoardPayload(payload);
        this.setStatus("Play queue refreshed", "ok");
      })
      .catch((error) => {
        this.setStatus(error instanceof Error ? error.message : String(error), "error");
      });
  }

  markDone(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const cardID = this.cardID(event);
    void this.withCardAction(cardID, () => doneScrumCard(cardID, this.activeProjectID), "Marking done");
  }

  advance(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const cardID = this.cardID(event);
    const card = this.findCard(cardID);
    if (!card) return;
    const column = nextColumn(card.column);
    if (!column) return;
    void this.withCardAction(cardID, () => moveScrumCard(cardID, column, this.activeProjectID), "Moving card");
  }

  retreat(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const cardID = this.cardID(event);
    const card = this.findCard(cardID);
    if (!card) return;
    const column = prevColumn(card.column);
    if (!column) return;
    void this.withCardAction(cardID, () => moveScrumCard(cardID, column, this.activeProjectID), "Moving card");
  }

  moveSelect(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const target = event.currentTarget as HTMLSelectElement;
    const cardID = target.dataset.cardId || "";
    const column = target.value;
    if (!cardID || !column) return;
    void this.withCardAction(cardID, () => moveScrumCard(cardID, column, this.activeProjectID), "Moving card");
  }

  modalMoveSelect(event: Event) {
    this.moveSelect(event);
  }

  async saveDetails(event: Event) {
    event.preventDefault();
    const cardID = this.cardID(event);
    if (!cardID) return;

    const title = this.modalField(event, "title");
    const description = (event.currentTarget as HTMLElement)
      .closest("[data-recyclr-sink='scrum-modal-details']")
      ?.querySelector('[data-scrum-field="description"]') as HTMLTextAreaElement | null;
    if (!title) return;

    this.setStatus("Saving card…", "busy");
    try {
      const card = await patchScrumCard(cardID, {
        title,
        description: description?.value ?? "",
      }, this.activeProjectID);
      this.upsertCard(card);
      await this.reloadBoard(cardID);
      await this.refreshModalSections(cardID);
      this.setStatus("Card saved", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async toggleChecklistItem(event: Event) {
    const target = event.currentTarget as HTMLInputElement;
    const cardID = target.dataset.cardId || "";
    const itemID = target.dataset.itemId || "";
    const card = this.findCard(cardID);
    if (!card || !itemID) return;

    const checklist = (card.checklist ?? []).map((item) =>
      item.id === itemID ? { ...item, done: target.checked } : item,
    );

    this.setStatus("Updating checklist…", "busy");
    try {
      const updated = await patchScrumCard(cardID, { checklist }, this.activeProjectID);
      this.upsertCard(updated);
      this.recycle("scrum-modal-checklist", renderScrumModalChecklist(updated));
      await this.reloadBoard(cardID);
      this.setStatus("Checklist updated", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async addChecklistItem(event: Event) {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    const cardID = form.dataset.cardId || "";
    const text = (form.querySelector('[data-scrum-field="checklistText"]') as HTMLInputElement | null)?.value.trim();
    if (!cardID || !text) return;

    const card = this.findCard(cardID);
    if (!card) return;

    const checklist: ScrumChecklistItem[] = [
      ...(card.checklist ?? []),
      { id: `chk_${Date.now()}`, text, done: false },
    ];

    this.setStatus("Adding checklist item…", "busy");
    try {
      const updated = await patchScrumCard(cardID, { checklist }, this.activeProjectID);
      this.upsertCard(updated);
      this.recycle("scrum-modal-checklist", renderScrumModalChecklist(updated));
      await this.reloadBoard(cardID);
      this.setStatus("Checklist item added", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async sendChat(event: Event) {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    const cardID = form.dataset.cardId || "";
    const message = (form.querySelector('[data-scrum-field="chatMessage"]') as HTMLTextAreaElement | null)?.value.trim();
    if (!cardID || !message) return;

    this.setStatus("Thinking…", "busy");
    try {
      const payload = await chatScrumCard(cardID, message, this.activeProjectID);
      this.upsertCard(payload.card);
      this.recycle("scrum-modal-chat", renderScrumModalChat(payload.card));
      await this.reloadBoard(cardID);
      this.setStatus("Reply received", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async addRefFile(event: Event) {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    const cardID = form.dataset.cardId || "";
    const file = (form.querySelector('[data-scrum-field="refFile"]') as HTMLSelectElement | null)?.value.trim();
    if (!cardID || !file) return;

    const card = this.findCard(cardID);
    if (!card) return;
    const refFiles = [...(card.ref_files ?? [])];
    if (refFiles.includes(file)) return;
    refFiles.push(file);

    this.setStatus("Attaching file…", "busy");
    try {
      const updated = await patchScrumCard(cardID, { ref_files: refFiles }, this.activeProjectID);
      this.upsertCard(updated);
      await this.refreshModalSections(cardID);
      await this.reloadBoard(cardID);
      this.setStatus("Reference attached", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async removeRefFile(event: Event) {
    event.preventDefault();
    const target = event.currentTarget as HTMLElement;
    const cardID = target.dataset.cardId || "";
    const file = target.dataset.refFile || "";
    const card = this.findCard(cardID);
    if (!card || !file) return;

    const refFiles = (card.ref_files ?? []).filter((entry) => entry !== file);
    this.setStatus("Removing reference…", "busy");
    try {
      const updated = await patchScrumCard(cardID, { ref_files: refFiles }, this.activeProjectID);
      this.upsertCard(updated);
      await this.refreshModalSections(cardID);
      await this.reloadBoard(cardID);
      this.setStatus("Reference removed", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async deleteCard(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const cardID = this.cardID(event);
    if (!cardID) return;
    if (!window.confirm("Delete this scrum card?")) return;
    this.setStatus("Deleting card…", "busy");
    try {
      await deleteScrumCard(cardID, this.activeProjectID);
      if (this.activeCardID === cardID) this.closeModal();
      await this.load();
      this.setStatus("Card deleted", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async saveModelConfig(event: Event) {
    event.preventDefault();
    const cardID = (event.currentTarget as HTMLElement).dataset.cardId || this.activeCardID || "";
    if (!cardID) return;
    const modal = document.querySelector('[data-recyclr-sink="scrum-modal-sidebar"]');
    if (!modal) return;
    this.setStatus("Saving card model settings…", "busy");
    try {
      const updated = await patchScrumCard(
        cardID,
        { model_config: collectModelFieldValues(modal, "card") },
        this.activeProjectID,
      );
      this.upsertCard(updated);
      await this.refreshModalSections(cardID);
      await this.reloadBoard(cardID);
      this.setStatus("Card model settings saved", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async clearModelConfig(event: Event) {
    event.preventDefault();
    const cardID = (event.currentTarget as HTMLElement).dataset.cardId || this.activeCardID || "";
    if (!cardID) return;
    const modal = document.querySelector('[data-recyclr-sink="scrum-modal-sidebar"]');
    if (modal) clearModelFieldInputs(modal, "card");
    this.setStatus("Clearing card model overrides…", "busy");
    try {
      const updated = await patchScrumCard(cardID, { model_config: {} }, this.activeProjectID);
      this.upsertCard(updated);
      await this.refreshModalSections(cardID);
      await this.reloadBoard(cardID);
      this.setStatus("Card model overrides cleared", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async saveAgentConfig(event: Event) {
    event.preventDefault();
    const cardID = (event.currentTarget as HTMLElement).dataset.cardId || this.activeCardID || "";
    if (!cardID) return;
    const modal = document.querySelector('[data-recyclr-sink="scrum-modal-sidebar"]');
    if (!modal) return;
    this.setStatus("Saving card agent settings…", "busy");
    try {
      const updated = await patchScrumCard(
        cardID,
        { agent_config: collectAgentFieldValues(modal, "card") },
        this.activeProjectID,
      );
      this.upsertCard(updated);
      await this.refreshModalSections(cardID);
      await this.reloadBoard(cardID);
      this.setStatus("Card agent settings saved", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async clearAgentConfig(event: Event) {
    event.preventDefault();
    const cardID = (event.currentTarget as HTMLElement).dataset.cardId || this.activeCardID || "";
    if (!cardID) return;
    const modal = document.querySelector('[data-recyclr-sink="scrum-modal-sidebar"]');
    if (modal) clearAgentFieldInputs(modal, "card");
    this.setStatus("Clearing card agent overrides…", "busy");
    try {
      const updated = await patchScrumCard(cardID, { agent_config: {} }, this.activeProjectID);
      this.upsertCard(updated);
      await this.refreshModalSections(cardID);
      await this.reloadBoard(cardID);
      this.setStatus("Card agent overrides cleared", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  private findCard(cardID: string): ScrumCard | null {
    if (!this.board) return null;
    return this.board.cards.find((card) => card.id === cardID) ?? null;
  }
}
