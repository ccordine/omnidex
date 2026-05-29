import { Controller } from "@hotwired/stimulus";
import {
  chatScrumCard,
  coachScrumCard,
  createScrumCard,
  deleteScrumCard,
  doneScrumCard,
  fetchScrumBoard,
  fetchScrumFiles,
  fetchScrumTags,
  jiraScrumCard,
  moveScrumCard,
  pauseScrumCard,
  patchScrumCard,
  playScrumCard,
  syncScrumBoard,
  suggestScrumTags,
  updateScrumCoachConfig,
} from "../lib/scrum_api";
import { fetchProject, fetchRecipes } from "../lib/project_api";
import type { RecipeCatalogItem } from "../lib/project_types";
import { renderRecyclrBundle } from "../lib/recyclr";
import { closeModalShell, openModalShell, resetModalPanelWidth } from "../lib/modal";
import { fetchModelDefaults } from "../lib/model_config_api";
import { fetchAgentDefaults } from "../lib/agent_config_api";
import { collectModelFieldValues, clearModelFieldInputs } from "../lib/model_config_render";
import { collectAgentFieldValues, clearAgentFieldInputs } from "../lib/agent_config_render";
import type { ResolvedModelConfig } from "../lib/model_config_types";
import type { ResolvedAgentConfig } from "../lib/agent_config_types";
import { renderScrumBoard, renderScrumEmptyState } from "../lib/scrum_render";
import {
  renderScrumCardModal,
  renderScrumModalCardTab,
  renderScrumTagPills,
  renderScrumTagSuggestions,
  renderScrumCoachChat,
  renderScrumCoachPanel,
  renderScrumCoachToasts,
  renderScrumModalChannelTab,
  renderScrumModalConfigTab,
  renderScrumModalRecipeTab,
  renderScrumModalToolbar,
  renderScrumModalTabNav,
  renderScrumCreateCardModal,
  type ScrumCardTab,
} from "../lib/scrum_modal_render";
import { nextColumn, prevColumn, type ScrumBoard, type ScrumBoardResponse, type ScrumCard, type ScrumChecklistItem, type ScrumTestCriterion } from "../lib/scrum_types";
import type GxController from "./gx_controller";

export default class ScrumController extends Controller {
  static targets = ["board", "status"];

  declare readonly boardTarget: HTMLElement;
  declare readonly statusTarget: HTMLElement;
  declare readonly hasBoardTarget: boolean;
  declare readonly hasStatusTarget: boolean;

  private board: ScrumBoard | null = null;
  private busy = false;
  private activeCardID: string | null = null;
  private projectFiles: string[] = [];
  private projectID: number | null = null;
  private cardModelConfig: ResolvedModelConfig | null = null;
  private cardAgentConfig: ResolvedAgentConfig | null = null;
  private modalClosedHandler: ((event: Event) => void) | null = null;
  private projectOpenedHandler: ((event: Event) => void) | null = null;
  private projectClosedHandler: ((event: Event) => void) | null = null;
  private pollTimer: number | null = null;
  private playQueue: ScrumBoardResponse["play_queue"] | null = null;
  private activeCardTab: ScrumCardTab = "card";
  private recipes: RecipeCatalogItem[] = [];
  private projectRecipeId = "";
  private projectRecipe: Record<string, unknown> = {};
  private coachScanTimer: number | null = null;
  private tagSearchTimer: number | null = null;

  connect() {
    this.modalClosedHandler = () => this.resetModalShell();
    document.addEventListener("omni:modal-closed", this.modalClosedHandler);

    this.projectOpenedHandler = (event: Event) => {
      const detail = (event as CustomEvent<{ project_id?: number }>).detail;
      this.projectID = detail?.project_id && detail.project_id > 0 ? detail.project_id : null;
      void this.load();
    };
    document.addEventListener("omni:project-opened", this.projectOpenedHandler);

    this.projectClosedHandler = () => {
      this.projectID = null;
      this.board = null;
      this.stopPolling();
      if (this.hasBoardTarget) {
        this.boardTarget.innerHTML = renderScrumEmptyState("Open a project to view its scrum board.");
      }
      this.setStatus("No project open", "idle");
    };
    document.addEventListener("omni:project-closed", this.projectClosedHandler);
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
    this.stopPolling();
    if (this.coachScanTimer != null) {
      window.clearTimeout(this.coachScanTimer);
      this.coachScanTimer = null;
    }
    if (this.tagSearchTimer != null) {
      window.clearTimeout(this.tagSearchTimer);
      this.tagSearchTimer = null;
    }
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
    if (!this.projectID || !this.board) return false;
    return this.board.cards.some((card) => card.play_state === "running" || card.play_state === "queued");
  }

  private async pollBoard() {
    if (!this.projectID) return;
    try {
      const payload = await fetchScrumBoard(this.projectID);
      this.applyBoardPayload(payload, false);
      if (this.activeCardID) await this.refreshModalSections(this.activeCardID);
    } catch {
      // keep last good board state during transient poll failures
    }
  }

  private applyBoardPayload(payload: ScrumBoardResponse, updateStatus = true) {
    if (!this.hasBoardTarget) return;
    this.board = payload.board;
    this.playQueue = payload.play_queue ?? null;
    this.boardTarget.innerHTML = renderScrumBoard(payload.board, payload.cards_by_col, payload.play_queue);
    if (updateStatus && this.shouldPoll()) {
      const queued = payload.play_queue?.queued_count ?? 0;
      const running = payload.play_queue?.running_card_id ? "running" : "idle";
      this.setStatus(`Play queue: ${running}${queued ? `, ${queued} queued` : ""}`, "ok");
    }
  }

  private resetModalShell() {
    this.activeCardID = null;
    this.activeCardTab = "card";
    resetModalPanelWidth();
  }

  showCardTab(event: Event) {
    event.preventDefault();
    const tab = (event.currentTarget as HTMLElement).dataset.scrumTab as ScrumCardTab | undefined;
    if (!tab) return;
    this.activeCardTab = tab;
    this.applyCardTabState();
  }

  private applyCardTabState() {
    const panel = document.querySelector('[data-chat-target="modalPanel"]');
    if (!panel) return;
    panel.querySelectorAll("[data-scrum-tab-panel]").forEach((element) => {
      const tab = (element as HTMLElement).dataset.scrumTabPanel;
      element.classList.toggle("hidden", tab !== this.activeCardTab);
    });
    panel.querySelectorAll("[data-scrum-tab]").forEach((button) => {
      const tab = (button as HTMLElement).dataset.scrumTab;
      const active = tab === this.activeCardTab;
      button.classList.toggle("border-cyan-300/40", active);
      button.classList.toggle("bg-cyan-300/10", active);
      button.classList.toggle("text-cyan-100", active);
      button.classList.toggle("border-white/10", !active);
      button.classList.toggle("text-zinc-400", !active);
    });
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

  private modalPanelField(name: string): string {
    const panel = document.querySelector('[data-chat-target="modalPanel"]');
    const field = panel?.querySelector(`[data-scrum-field="${name}"]`) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null;
    return field?.value?.trim() ?? "";
  }

  openCreateCardModal(event?: Event) {
    event?.preventDefault();
    event?.stopPropagation();
    const column = (event?.currentTarget as HTMLElement | null)?.dataset?.column || "backlog";
    this.activeCardID = null;
    this.openModal(renderScrumCreateCardModal(column));
  }

  setStatus(message: string, tone: "idle" | "busy" | "error" | "ok" = "idle") {
    if (!this.hasStatusTarget) return;
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
    openModalShell({ wide: true });
  }

  closeModal() {
    this.activeCardID = null;
    closeModalShell();
  }

  private upsertCard(card: ScrumCard) {
    if (!this.board) return;
    const index = this.board.cards.findIndex((entry) => entry.id === card.id);
    if (index >= 0) this.board.cards[index] = card;
    else this.board.cards.push(card);
  }

  private async reloadBoard(cardID?: string | null): Promise<ScrumCard | null> {
    const payload = await fetchScrumBoard(this.projectID);
    this.applyBoardPayload(payload);
    const id = cardID ?? this.activeCardID;
    if (!id) return null;
    return this.findCard(id);
  }

  private async loadProjectFiles(): Promise<string[]> {
    try {
      const payload = await fetchScrumFiles(this.projectID);
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
        fetchModelDefaults(this.projectID, cardID),
        fetchAgentDefaults(this.projectID, cardID),
      ]);
      this.cardModelConfig = modelPayload.resolved ?? null;
      this.cardAgentConfig = agentPayload.resolved ?? null;
    } catch {
      this.cardModelConfig = null;
      this.cardAgentConfig = null;
    }
  }

  async loadCardContext() {
    if (!this.projectID) return;
    try {
      const [projectPayload, recipePayload] = await Promise.all([
        fetchProject(this.projectID).catch(() => null),
        fetchRecipes().catch(() => ({ recipes: [], root: "" })),
      ]);
      this.recipes = recipePayload.recipes ?? [];
      this.projectRecipeId = projectPayload?.project.recipe_id ?? "";
      this.projectRecipe = projectPayload?.project.recipe ?? {};
    } catch {
      this.recipes = [];
      this.projectRecipeId = "";
      this.projectRecipe = {};
    }
  }

  async refreshModalSections(cardID: string) {
    const card = this.findCard(cardID);
    if (!card || !this.board) return;
    const files = this.projectFiles.length ? this.projectFiles : await this.loadProjectFiles();
    await this.loadCardConfigs(cardID);
    this.recycle("scrum-modal-toolbar", renderScrumModalToolbar(card, this.board, this.playQueue ?? undefined));
    this.recycle("scrum-modal-tabs", `<nav class="flex flex-wrap gap-2" aria-label="Card sections">${renderScrumModalTabNav(card, this.activeCardTab)}</nav>`);
    this.recycle("scrum-modal-card", renderScrumModalCardTab(card, files));
    this.recycle("scrum-modal-config", renderScrumModalConfigTab(
      card,
      this.cardModelConfig?.fields ?? [],
      this.cardModelConfig?.source ?? "env",
      this.cardAgentConfig?.fields ?? [],
      this.cardAgentConfig?.source ?? "env",
      this.cardAgentConfig?.system ?? "omnidex",
    ));
    this.recycle("scrum-modal-recipe", renderScrumModalRecipeTab(card, this.recipes, this.projectRecipeId, this.projectRecipe));
    this.recycle("scrum-modal-channel", renderScrumModalChannelTab(card));
    this.applyCardTabState();
    this.wireCoachAutoScan();
  }

  private wireCoachAutoScan() {
    const panel = document.querySelector('[data-chat-target="modalPanel"]');
    if (!panel) return;
    const card = this.activeCardID ? this.findCard(this.activeCardID) : null;
    if (!card || card.coach_config?.enabled === false || card.coach_config?.auto_scan === false) return;
    const handler = () => {
      if (this.coachScanTimer != null) window.clearTimeout(this.coachScanTimer);
      this.coachScanTimer = window.setTimeout(() => {
        if (this.activeCardID) void this.runCoachScan(this.activeCardID);
      }, 2000);
    };
    panel.querySelectorAll('[data-scrum-field="title"], [data-scrum-field="description"]').forEach((el) => {
      el.removeEventListener("input", handler);
      el.addEventListener("input", handler);
    });
  }

  private coachSnapshot(): Record<string, string> {
    const panel = document.querySelector('[data-chat-target="modalPanel"]');
    return {
      title: (panel?.querySelector('[data-scrum-field="title"]') as HTMLInputElement | null)?.value?.trim() ?? "",
      description: (panel?.querySelector('[data-scrum-field="description"]') as HTMLTextAreaElement | null)?.value ?? "",
    };
  }

  private async runCoachScan(cardID: string) {
    const card = this.findCard(cardID);
    if (!card || card.coach_config?.enabled === false) return;
    try {
      const payload = await coachScrumCard(cardID, { mode: "scan", snapshot: this.coachSnapshot() }, this.projectID);
      this.upsertCard(payload.card);
      this.recycle("scrum-coach-toasts", renderScrumCoachToasts(payload.suggestions ?? []));
      this.recycle("scrum-card-tags", renderScrumTagPills(payload.card));
      if (payload.jira_prompt) {
        const draft = document.querySelector('[data-scrum-field="jiraPromptDraft"]') as HTMLTextAreaElement | null;
        if (draft) draft.value = payload.jira_prompt;
      }
      this.recycle("scrum-coach-chat", renderScrumCoachChat(payload.card));
    } catch {
      // ignore transient coach scan failures
    }
  }

  async saveCoachConfig(event: Event) {
    event.preventDefault();
    const cardID = this.cardID(event);
    if (!cardID) return;
    const panel = document.querySelector('[data-chat-target="modalPanel"]');
    const enabled = (panel?.querySelector('[data-scrum-field="coachEnabled"]') as HTMLInputElement | null)?.checked ?? true;
    const autoScan = (panel?.querySelector('[data-scrum-field="coachAutoScan"]') as HTMLInputElement | null)?.checked ?? true;
    const model = (panel?.querySelector('[data-scrum-field="coachModel"]') as HTMLInputElement | null)?.value?.trim() || "qwen3:4b-thinking";
    this.setStatus("Saving coach settings…", "busy");
    try {
      const payload = await updateScrumCoachConfig(cardID, { enabled, auto_scan: autoScan, model }, this.projectID);
      this.upsertCard(payload.card);
      await this.refreshModalSections(cardID);
      this.setStatus("Coach settings saved", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async sendCoach(event: Event) {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    const cardID = form.dataset.cardId || "";
    const message = (form.querySelector('[data-scrum-field="coachMessage"]') as HTMLTextAreaElement | null)?.value.trim();
    if (!cardID || !message) return;
    this.setStatus("Coach thinking…", "busy");
    try {
      const payload = await coachScrumCard(cardID, { message, snapshot: this.coachSnapshot() }, this.projectID);
      this.upsertCard(payload.card);
      this.recycle("scrum-coach-toasts", renderScrumCoachToasts(payload.suggestions ?? []));
      this.recycle("scrum-card-tags", renderScrumTagPills(payload.card));
      this.recycle("scrum-coach-chat", renderScrumCoachChat(payload.card));
      if (payload.jira_prompt) {
        const draft = document.querySelector('[data-scrum-field="jiraPromptDraft"]') as HTMLTextAreaElement | null;
        if (draft) draft.value = payload.jira_prompt;
      }
      const input = form.querySelector('[data-scrum-field="coachMessage"]') as HTMLTextAreaElement | null;
      if (input) input.value = "";
      await this.reloadBoard(cardID);
      this.setStatus("Coach replied", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
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
    this.activeCardTab = "card";
    const files = await this.loadProjectFiles();
    await Promise.all([this.loadCardConfigs(cardID), this.loadCardContext()]);
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
        this.playQueue ?? undefined,
        this.activeCardTab,
        this.recipes,
        this.projectRecipeId,
        this.projectRecipe,
      ),
    );
    this.wireCoachAutoScan();
  }

  async load() {
    if (this.busy || !this.projectID || !this.hasBoardTarget) return;
    this.busy = true;
    this.setStatus("Loading board…", "busy");
    try {
      const payload = await fetchScrumBoard(this.projectID);
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

  async createCard(event: Event) {
    event.preventDefault();
    const title = this.modalField(event, "newTitle") || this.modalPanelField("newTitle");
    if (!title) return;

    const description = this.modalField(event, "newDesc") || this.modalPanelField("newDesc");
    const column = this.modalField(event, "newColumn") || this.modalPanelField("newColumn") || "backlog";

    this.setStatus("Creating card…", "busy");
    try {
      await createScrumCard(title, description, column, this.projectID);
      this.closeModal();
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
    void this.withCardAction(cardID, () => pauseScrumCard(cardID, this.projectID), "Pausing play");
  }

  async withPlayAction(cardID: string, pivot: boolean) {
    if (!cardID) return;
    this.setStatus(pivot ? "Pivoting play…" : "Queueing play…", "busy");
    try {
      const result = await playScrumCard(cardID, this.projectID, { pivot });
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
    void syncScrumBoard(this.projectID)
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
    void this.withCardAction(cardID, () => doneScrumCard(cardID, this.projectID), "Marking done");
  }

  advance(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const cardID = this.cardID(event);
    const card = this.findCard(cardID);
    if (!card) return;
    const column = nextColumn(card.column);
    if (!column) return;
    void this.withCardAction(cardID, () => moveScrumCard(cardID, column, this.projectID), "Moving card");
  }

  retreat(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const cardID = this.cardID(event);
    const card = this.findCard(cardID);
    if (!card) return;
    const column = prevColumn(card.column);
    if (!column) return;
    void this.withCardAction(cardID, () => moveScrumCard(cardID, column, this.projectID), "Moving card");
  }

  moveSelect(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const target = event.currentTarget as HTMLSelectElement;
    const cardID = target.dataset.cardId || "";
    const column = target.value;
    if (!cardID || !column) return;
    void this.withCardAction(cardID, () => moveScrumCard(cardID, column, this.projectID), "Moving card");
  }

  modalMoveSelect(event: Event) {
    this.moveSelect(event);
  }

  assignCard(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const cardID = this.cardID(event);
    void this.withCardAction(cardID, () => moveScrumCard(cardID, "assigned", this.projectID), "Moving to Assigned");
  }

  async saveDetails(event: Event) {
    event.preventDefault();
    const cardID = this.cardID(event);
    if (!cardID) return;

    const title = this.modalField(event, "title");
    const description = (document.querySelector('[data-chat-target="modalPanel"]')
      ?.querySelector('[data-scrum-field="description"]') as HTMLTextAreaElement | null);
    if (!title) return;

    this.setStatus("Saving card…", "busy");
    try {
      const card = await patchScrumCard(cardID, {
        title,
        description: description?.value ?? "",
      }, this.projectID);
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
      const updated = await patchScrumCard(cardID, { checklist }, this.projectID);
      this.upsertCard(updated);
      this.recycle("scrum-modal-card", renderScrumModalCardTab(updated, this.projectFiles));
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
      const updated = await patchScrumCard(cardID, { checklist }, this.projectID);
      this.upsertCard(updated);
      this.recycle("scrum-modal-card", renderScrumModalCardTab(updated, this.projectFiles));
      await this.reloadBoard(cardID);
      this.setStatus("Checklist item added", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async removeChecklistItem(event: Event) {
    event.preventDefault();
    const target = event.currentTarget as HTMLElement;
    const cardID = target.dataset.cardId || "";
    const itemID = target.dataset.itemId || "";
    const card = this.findCard(cardID);
    if (!card || !itemID) return;

    const checklist = (card.checklist ?? []).filter((item) => item.id !== itemID);
    this.setStatus("Updating checklist…", "busy");
    try {
      const updated = await patchScrumCard(cardID, { checklist }, this.projectID);
      this.upsertCard(updated);
      await this.refreshModalSections(cardID);
      await this.reloadBoard(cardID);
      this.setStatus("Checklist updated", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  filterTagSuggestions(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const query = input.value.trim();
    if (this.tagSearchTimer != null) window.clearTimeout(this.tagSearchTimer);
    this.tagSearchTimer = window.setTimeout(async () => {
      try {
        const tags = await fetchScrumTags(query, this.projectID);
        this.recycle("scrum-tag-suggestions", renderScrumTagSuggestions(tags));
      } catch {
        // ignore transient tag search failures
      }
    }, 250);
  }

  private normalizeTag(value: string): string {
    return value.trim().toLowerCase().replace(/\s+/g, "-");
  }

  private async patchCardTags(cardID: string, tags: string[]) {
    const normalized = [...new Set(tags.map((tag) => this.normalizeTag(tag)).filter(Boolean))];
    const updated = await patchScrumCard(cardID, { tags: normalized }, this.projectID);
    this.upsertCard(updated);
    this.recycle("scrum-card-tags", renderScrumTagPills(updated));
    await this.reloadBoard(cardID);
    return updated;
  }

  async addCardTag(event: Event) {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    const cardID = form.dataset.cardId || "";
    const raw = (form.querySelector('[data-scrum-field="tagInput"]') as HTMLInputElement | null)?.value ?? "";
    const tag = this.normalizeTag(raw);
    if (!cardID || !tag) return;

    const card = this.findCard(cardID);
    if (!card) return;
    if ((card.tags ?? []).includes(tag)) return;

    this.setStatus("Adding tag…", "busy");
    try {
      await this.patchCardTags(cardID, [...(card.tags ?? []), tag]);
      const input = form.querySelector('[data-scrum-field="tagInput"]') as HTMLInputElement | null;
      if (input) input.value = "";
      this.setStatus("Tag added", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async removeCardTag(event: Event) {
    event.preventDefault();
    const target = event.currentTarget as HTMLElement;
    const cardID = target.dataset.cardId || "";
    const tag = target.dataset.tag || "";
    const card = this.findCard(cardID);
    if (!card || !tag) return;

    this.setStatus("Removing tag…", "busy");
    try {
      await this.patchCardTags(cardID, (card.tags ?? []).filter((item) => item !== tag));
      this.setStatus("Tag removed", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async suggestCardTags(event: Event) {
    event.preventDefault();
    const cardID = this.cardID(event);
    if (!cardID) return;

    this.setStatus("Suggesting tags…", "busy");
    try {
      const payload = await suggestScrumTags(cardID, this.projectID);
      this.upsertCard(payload.card);
      this.recycle("scrum-card-tags", renderScrumTagPills(payload.card));
      await this.reloadBoard(cardID);
      this.setStatus(payload.notes ? `Tags suggested — ${payload.notes}` : "Tags suggested", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async toggleTestCriterion(event: Event) {
    const target = event.currentTarget as HTMLInputElement;
    const cardID = target.dataset.cardId || "";
    const itemID = target.dataset.itemId || "";
    const card = this.findCard(cardID);
    if (!card || !itemID) return;

    const testCriteria = (card.test_criteria ?? []).map((item) =>
      item.id === itemID ? { ...item, done: target.checked } : item,
    );

    this.setStatus("Updating test…", "busy");
    try {
      const updated = await patchScrumCard(cardID, { test_criteria: testCriteria }, this.projectID);
      this.upsertCard(updated);
      this.recycle("scrum-modal-card", renderScrumModalCardTab(updated, this.projectFiles));
      await this.reloadBoard(cardID);
      this.setStatus("Test updated", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async addTestCriterion(event: Event) {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    const cardID = form.dataset.cardId || "";
    const text = (form.querySelector('[data-scrum-field="testCriterionText"]') as HTMLInputElement | null)?.value.trim();
    if (!cardID || !text) return;

    const card = this.findCard(cardID);
    if (!card) return;

    const testCriteria: ScrumTestCriterion[] = [
      ...(card.test_criteria ?? []),
      { id: `test_${Date.now()}`, text, done: false },
    ];

    this.setStatus("Adding test…", "busy");
    try {
      const updated = await patchScrumCard(cardID, { test_criteria: testCriteria }, this.projectID);
      this.upsertCard(updated);
      this.recycle("scrum-modal-card", renderScrumModalCardTab(updated, this.projectFiles));
      await this.reloadBoard(cardID);
      this.setStatus("Test added", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async removeTestCriterion(event: Event) {
    event.preventDefault();
    const target = event.currentTarget as HTMLElement;
    const cardID = target.dataset.cardId || "";
    const itemID = target.dataset.itemId || "";
    const card = this.findCard(cardID);
    if (!card || !itemID) return;

    const testCriteria = (card.test_criteria ?? []).filter((item) => item.id !== itemID);
    this.setStatus("Updating tests…", "busy");
    try {
      const updated = await patchScrumCard(cardID, { test_criteria: testCriteria }, this.projectID);
      this.upsertCard(updated);
      await this.refreshModalSections(cardID);
      await this.reloadBoard(cardID);
      this.setStatus("Test removed", "ok");
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
      const payload = await chatScrumCard(cardID, message, this.projectID);
      this.upsertCard(payload.card);
      this.recycle("scrum-modal-channel", renderScrumModalChannelTab(payload.card));
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
      const updated = await patchScrumCard(cardID, { ref_files: refFiles }, this.projectID);
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
      const updated = await patchScrumCard(cardID, { ref_files: refFiles }, this.projectID);
      this.upsertCard(updated);
      await this.refreshModalSections(cardID);
      await this.reloadBoard(cardID);
      this.setStatus("Reference removed", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async generateJira(event: Event) {
    event.preventDefault();
    const cardID = this.cardID(event);
    if (!cardID) return;
    const prompt = this.modalField(event, "jiraPromptDraft") || this.modalPanelField("jiraPromptDraft");
    this.setStatus("Generating Jira ticket…", "busy");
    try {
      const payload = await jiraScrumCard(cardID, { prompt }, this.projectID);
      this.upsertCard(payload.card);
      await this.refreshModalSections(cardID);
      await this.reloadBoard(cardID);
      this.setStatus("Jira draft generated", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  async saveJira(event: Event) {
    event.preventDefault();
    const cardID = this.cardID(event);
    if (!cardID) return;
    const ticket = this.modalField(event, "jiraTicket") || this.modalPanelField("jiraTicket");
    this.setStatus("Saving Jira draft…", "busy");
    try {
      const payload = await jiraScrumCard(cardID, { ticket }, this.projectID);
      this.upsertCard(payload.card);
      await this.refreshModalSections(cardID);
      this.setStatus("Jira draft saved", "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  loadCatalogRecipe(event: Event) {
    event.preventDefault();
    const recipeID = this.modalField(event, "recipeId") || this.modalPanelField("recipeId");
    const recipe = this.recipes.find((entry) => entry.id === recipeID);
    const panel = document.querySelector('[data-chat-target="modalPanel"]');
    const editor = panel?.querySelector('[data-scrum-field="recipeJson"]') as HTMLTextAreaElement | null;
    if (recipe && editor) editor.value = JSON.stringify(recipe, null, 2);
  }

  async saveRecipe(event: Event) {
    event.preventDefault();
    const cardID = this.cardID(event);
    if (!cardID) return;
    const panel = document.querySelector('[data-chat-target="modalPanel"]');
    const raw = (panel?.querySelector('[data-scrum-field="recipeJson"]') as HTMLTextAreaElement | null)?.value ?? "{}";
    let recipe: Record<string, unknown>;
    try {
      recipe = JSON.parse(raw) as Record<string, unknown>;
    } catch {
      this.setStatus("Recipe JSON is invalid", "error");
      return;
    }
    const recipeID = this.modalField(event, "recipeId") || this.modalPanelField("recipeId");
    this.setStatus("Saving card recipe…", "busy");
    try {
      const updated = await patchScrumCard(cardID, { recipe_id: recipeID, recipe }, this.projectID);
      this.upsertCard(updated);
      await this.refreshModalSections(cardID);
      await this.reloadBoard(cardID);
      this.setStatus("Card recipe saved", "ok");
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
      await deleteScrumCard(cardID, this.projectID);
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
    const modal = document.querySelector('[data-recyclr-sink="scrum-modal-config"]');
    if (!modal) return;
    this.setStatus("Saving card model settings…", "busy");
    try {
      const updated = await patchScrumCard(
        cardID,
        { model_config: collectModelFieldValues(modal, "card") },
        this.projectID,
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
    const modal = document.querySelector('[data-recyclr-sink="scrum-modal-config"]');
    if (modal) clearModelFieldInputs(modal, "card");
    this.setStatus("Clearing card model overrides…", "busy");
    try {
      const updated = await patchScrumCard(cardID, { model_config: {} }, this.projectID);
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
    await this.saveAgentConfigForCard(cardID);
  }

  async quickSetAgent(event: Event) {
    event.preventDefault();
    const target = event.currentTarget as HTMLElement;
    const cardID = target.dataset.cardId || this.activeCardID || "";
    const system = target.dataset.agentSystem || "omnidex";
    if (!cardID) return;
    this.setStatus(`Setting agent to ${system}…`, "busy");
    try {
      const patch: Record<string, string> = { agent_system: system };
      if (system === "cursor" || system === "codex") {
        patch.agent_strict = "true";
      }
      const updated = await patchScrumCard(cardID, { agent_config: patch }, this.projectID);
      this.upsertCard(updated);
      await this.refreshModalSections(cardID);
      await this.reloadBoard(cardID);
      this.setStatus(`Agent set to ${system}`, "ok");
    } catch (error) {
      this.setStatus(error instanceof Error ? error.message : String(error), "error");
    }
  }

  private async saveAgentConfigForCard(cardID: string) {
    const modal = document.querySelector('[data-recyclr-sink="scrum-modal-config"]');
    if (!modal) return;
    this.setStatus("Saving card agent settings…", "busy");
    try {
      const updated = await patchScrumCard(
        cardID,
        { agent_config: collectAgentFieldValues(modal, "card") },
        this.projectID,
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
    const modal = document.querySelector('[data-recyclr-sink="scrum-modal-config"]');
    if (modal) clearAgentFieldInputs(modal, "card");
    this.setStatus("Clearing card agent overrides…", "busy");
    try {
      const updated = await patchScrumCard(cardID, { agent_config: {} }, this.projectID);
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
