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
import { renderScrumBoard, renderScrumEmptyState, renderScrumFocusBar, renderScrumFlowSummary } from "../lib/scrum_render";
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
import { COLUMN_LABELS, nextColumn, prevColumn, groupCardsByColumn, type ScrumBoard, type ScrumBoardResponse, type ScrumCard, type ScrumChecklistItem, type ScrumTestCriterion } from "../lib/scrum_types";
import { ScrumBoardDrag, type ScrumDragDropResult } from "../lib/scrum_drag";
import type GxController from "./gx_controller";
import { reportError, reportErrorMessage, reportOk } from "../lib/feedback";
import { showToast } from "../lib/toast";

export default class ScrumController extends Controller {
  static targets = ["board", "status", "focus", "boardOverlay", "boardOverlayMessage", "flowSummary"];

  declare readonly flowSummaryTarget: HTMLElement;
  declare readonly hasFlowSummaryTarget: boolean;

  declare readonly boardTarget: HTMLElement;
  declare readonly statusTarget: HTMLElement;
  declare readonly hasFocusTarget: boolean;
  declare readonly focusTarget: HTMLElement;
  declare readonly hasBoardTarget: boolean;
  declare readonly hasStatusTarget: boolean;
  declare readonly hasBoardOverlayTarget: boolean;
  declare readonly boardOverlayTarget: HTMLElement;
  declare readonly hasBoardOverlayMessageTarget: boolean;
  declare readonly boardOverlayMessageTarget: HTMLElement;

  private board: ScrumBoard | null = null;
  private busy = false;
  private boardLoadingDepth = 0;
  private activeCardID: string | null = null;
  private projectFiles: string[] = [];
  private projectID: number | null = null;
  private cardModelConfig: ResolvedModelConfig | null = null;
  private cardAgentConfig: ResolvedAgentConfig | null = null;
  private modalClosedHandler: ((event: Event) => void) | null = null;
  private projectOpenedHandler: ((event: Event) => void) | null = null;
  private projectClosedHandler: ((event: Event) => void) | null = null;
  private projectTabHandler: ((event: Event) => void) | null = null;
  private pollTimer: number | null = null;
  private pollInFlight = false;
  private pollIntervalMs = 1500;
  private lastBoardUpdatedAt = "";
  private scrumTabActive = true;
  private playQueue: ScrumBoardResponse["play_queue"] | null = null;
  private flowSummary: ScrumBoardResponse["flow_summary"] | null = null;
  private activeCardTab: ScrumCardTab = "card";
  private channelPilotPendingCardID: string | null = null;
  private recipes: RecipeCatalogItem[] = [];
  private projectRecipeId = "";
  private projectRecipe: Record<string, unknown> = {};
  private coachScanTimer: number | null = null;
  private tagSearchTimer: number | null = null;
  private boardDrag = new ScrumBoardDrag();
  /** Previous card columns — used to toast agent-driven moves on poll. */
  private cardColumnSnapshot = new Map<string, string>();
  /** User drag / manual column changes — skip duplicate move toasts. */
  private skipMoveToastFor = new Set<string>();
  private scrumRefreshHandler = () => {
    if (this.projectID) void this.load();
  };

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
      this.lastBoardUpdatedAt = "";
      this.cardColumnSnapshot.clear();
      this.skipMoveToastFor.clear();
      this.scrumTabActive = true;
      this.stopPolling();
      if (this.hasBoardTarget) {
        this.boardTarget.innerHTML = renderScrumEmptyState("Open a project to view its scrum board.");
      }
      this.setStatus("No project open", "idle");
    };
    document.addEventListener("omni:project-closed", this.projectClosedHandler);

    this.projectTabHandler = (event: Event) => {
      const detail = (event as CustomEvent<{ tab?: string; project_id?: number }>).detail;
      if (detail?.project_id && detail.project_id !== this.projectID) return;
      this.scrumTabActive = detail?.tab === "scrum";
      if (this.scrumTabActive && this.projectID && this.board) {
        this.startPolling();
        void this.pollBoard();
      } else {
        this.stopPolling();
      }
    };
    document.addEventListener("omni:project-tab", this.projectTabHandler);
    document.addEventListener("omni:scrum-refresh", this.scrumRefreshHandler);
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
    this.stopPolling();
    if (this.coachScanTimer != null) {
      window.clearTimeout(this.coachScanTimer);
      this.coachScanTimer = null;
    }
    if (this.tagSearchTimer != null) {
      window.clearTimeout(this.tagSearchTimer);
      this.tagSearchTimer = null;
    }
    this.boardDrag.unwire();
  }

  private isPlayActive(): boolean {
    return this.board?.cards.some((card) => card.play_state === "running" || card.play_state === "queued") ?? false;
  }

  private isModalPlayLive(): boolean {
    if (!this.activeCardID) return false;
    const card = this.findCard(this.activeCardID);
    return card?.play_state === "running" || card?.play_state === "queued";
  }

  private isChannelLive(): boolean {
    if (!this.activeCardID || this.activeCardTab !== "channel") return false;
    const card = this.findCard(this.activeCardID);
    return card?.play_state === "running" || card?.play_state === "queued";
  }

  private desiredPollIntervalMs(): number {
    if (this.isChannelLive()) return 500;
    if (this.isModalPlayLive()) return 800;
    if (this.isPlayActive()) return 1000;
    return 1500;
  }

  private startPolling() {
    this.stopPolling();
    if (!this.shouldPoll()) return;
    this.pollIntervalMs = this.desiredPollIntervalMs();
    this.pollTimer = window.setInterval(() => {
      if (this.shouldPoll() && !this.boardDrag.isActive()) void this.pollBoard();
    }, this.pollIntervalMs);
  }

  private stopPolling() {
    if (this.pollTimer != null) {
      window.clearInterval(this.pollTimer);
      this.pollTimer = null;
    }
  }

  private shouldPoll(): boolean {
    return Boolean(this.projectID && this.board && this.scrumTabActive);
  }

  private syncPollInterval() {
    if (!this.shouldPoll()) return;
    const nextMs = this.desiredPollIntervalMs();
    if (nextMs !== this.pollIntervalMs) this.startPolling();
  }

  private boardLiveFingerprint(payload: ScrumBoardResponse): string {
    const active = this.activeCardID
      ? payload.board.cards.find((card) => card.id === this.activeCardID)
      : null;
    const activeSlice = active
      ? `${active.id}:${active.updated_at}:${active.column}:${active.play_state}:${(active.chat ?? []).length}:${active.chat?.at(-1)?.content?.length ?? 0}`
      : "";
    return `${payload.board.updated_at}|${payload.play_queue?.running_card_id ?? ""}|${payload.play_queue?.queued_count ?? 0}|${activeSlice}`;
  }

  private async pollBoard() {
    if (!this.projectID || this.pollInFlight || this.boardDrag.isActive()) return;
    if (this.channelPilotPendingCardID) return;
    this.pollInFlight = true;
    try {
      const payload = await fetchScrumBoard(this.projectID);
      const fingerprint = this.boardLiveFingerprint(payload);
      if (fingerprint === this.lastBoardUpdatedAt) return;
      this.lastBoardUpdatedAt = fingerprint;
      this.applyBoardPayload(payload, false);
      if (this.activeCardID) {
        if (this.activeCardTab === "channel" || this.isModalPlayLive()) {
          await this.refreshLiveChannel(this.activeCardID);
        } else {
          await this.refreshModalSections(this.activeCardID);
        }
      }
    } catch {
      // keep last good board state during transient poll failures
    } finally {
      this.pollInFlight = false;
    }
  }

  private renderFlowSummary() {
    if (!this.hasFlowSummaryTarget) return;
    const html = renderScrumFlowSummary(this.flowSummary);
    this.flowSummaryTarget.innerHTML = html;
    this.flowSummaryTarget.classList.toggle("hidden", !html);
  }

  private renderBoardFromLocal(updateStatus = true) {
    if (!this.board || !this.hasBoardTarget) return;
    const cardsByCol = groupCardsByColumn(this.board);
    this.boardTarget.innerHTML = renderScrumBoard(this.board, cardsByCol, this.playQueue ?? undefined);
    this.renderFlowSummary();
    if (this.hasFocusTarget) {
      this.focusTarget.innerHTML = renderScrumFocusBar(this.board, cardsByCol, this.playQueue ?? undefined);
    }
    if (updateStatus && this.isPlayActive()) {
      const queued = this.playQueue?.queued_count ?? 0;
      const running = this.playQueue?.running_card_id ? "running" : "idle";
      this.setStatus(`Play queue: ${running}${queued ? `, ${queued} queued` : ""}`, "ok");
    }
    this.wireBoardDragDrop();
  }

  private applyBoardPayload(payload: ScrumBoardResponse, updateStatus = true) {
    if (!this.hasBoardTarget) return;
    this.toastAgentColumnMoves(payload.board.cards);
    this.board = payload.board;
    this.playQueue = payload.play_queue ?? null;
    this.flowSummary = payload.flow_summary ?? null;
    if (!this.pollInFlight) {
      this.lastBoardUpdatedAt = this.boardLiveFingerprint(payload);
    }
    this.boardTarget.innerHTML = renderScrumBoard(payload.board, payload.cards_by_col, payload.play_queue);
    this.renderFlowSummary();
    if (this.hasFocusTarget) {
      this.focusTarget.innerHTML = renderScrumFocusBar(payload.board, payload.cards_by_col, payload.play_queue);
    }
    if (updateStatus && this.isPlayActive()) {
      const queued = payload.play_queue?.queued_count ?? 0;
      const running = payload.play_queue?.running_card_id ? "running" : "idle";
      this.setStatus(`Play queue: ${running}${queued ? `, ${queued} queued` : ""}`, "ok");
    }
    this.wireBoardDragDrop();
    this.syncPollInterval();
    this.syncCardColumnSnapshot(payload.board.cards);
  }

  private syncCardColumnSnapshot(cards: ScrumCard[]) {
    this.cardColumnSnapshot.clear();
    for (const card of cards) {
      this.cardColumnSnapshot.set(card.id, card.column);
    }
  }

  private toastAgentColumnMoves(cards: ScrumCard[]) {
    if (this.cardColumnSnapshot.size === 0) return;

    for (const card of cards) {
      const previousColumn = this.cardColumnSnapshot.get(card.id);
      if (!previousColumn || previousColumn === card.column) continue;
      if (this.skipMoveToastFor.delete(card.id)) continue;

      const label = COLUMN_LABELS[card.column] ?? card.column.replace(/_/g, " ");
      const title = this.cardMoveTitle(card.title);
      const verb = card.column === "in_progress" ? "Moving" : "Moved";
      showToast(`${verb} "${title}" to ${label}`, "info");
    }
  }

  private cardMoveTitle(title: string | undefined): string {
    const trimmed = String(title ?? "").trim() || "Card";
    return trimmed.length > 52 ? `${trimmed.slice(0, 49)}…` : trimmed;
  }

  private wireBoardDragDrop() {
    if (!this.hasBoardTarget) return;
    this.boardDrag.wire(this.boardTarget, (result) => {
      this.applyDragResult(result);
      void this.persistCardPlacement(result);
    });
  }

  private applyDragResult(result: ScrumDragDropResult) {
    this.skipMoveToastFor.add(result.cardID);
    const card = this.findCard(result.cardID);
    if (card) {
      card.column = result.column;
    }
  }

  private async persistCardPlacement(result: ScrumDragDropResult) {
    try {
      const card = await moveScrumCard(result.cardID, result.column, this.projectID, {
        before_card_id: result.beforeCardID,
      });
      this.upsertCard(card);
      const payload = await fetchScrumBoard(this.projectID);
      this.applyBoardPayload(payload, false);
    } catch (error) {
      this.actionFail(error);
      await this.load();
    }
  }

  private resetModalShell() {
    this.activeCardID = null;
    this.activeCardTab = "card";
    this.channelPilotPendingCardID = null;
    resetModalPanelWidth();
  }

  private cardTabStorageKey(cardID: string): string {
    return `omni.scrum.card-tab.${cardID}`;
  }

  private restoreCardTab(cardID: string): ScrumCardTab {
    const saved = sessionStorage.getItem(this.cardTabStorageKey(cardID));
    if (saved === "card" || saved === "config" || saved === "recipe" || saved === "channel") {
      return saved;
    }
    return "card";
  }

  private persistCardTab(tab: ScrumCardTab) {
    if (!this.activeCardID) return;
    sessionStorage.setItem(this.cardTabStorageKey(this.activeCardID), tab);
  }

  showCardTab(event: Event) {
    event.preventDefault();
    const tab = (event.currentTarget as HTMLElement).dataset.scrumTab as ScrumCardTab | undefined;
    if (!tab) return;
    this.activeCardTab = tab;
    this.persistCardTab(tab);
    this.applyCardTabState();
    this.syncPollInterval();
    if (tab === "channel" && this.activeCardID) {
      void this.refreshLiveChannel(this.activeCardID, true);
    }
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
    if (this.activeCardTab === "channel") {
      this.scrollChannelToLatest(true);
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

  private actionOk(message: string) {
    reportOk(this.setStatus.bind(this), message);
  }

  private actionFail(error: unknown) {
    reportError(this.setStatus.bind(this), error);
  }

  private actionFailMessage(message: string) {
    reportErrorMessage(this.setStatus.bind(this), message);
  }

  private setGlobalLoading(loading: boolean) {
    const spinner = document.querySelector('[data-chat-target="spinner"]');
    if (spinner) spinner.classList.toggle("hidden", !loading);
  }

  private setBoardLoading(loading: boolean, message = "Working…") {
    if (loading) {
      this.boardLoadingDepth += 1;
    } else {
      this.boardLoadingDepth = Math.max(0, this.boardLoadingDepth - 1);
    }
    const active = this.boardLoadingDepth > 0;
    this.setGlobalLoading(active);
    const overlay = this.hasBoardOverlayTarget
      ? this.boardOverlayTarget
      : (this.element.querySelector('[data-scrum-target="boardOverlay"]') as HTMLElement | null);
    const overlayMessage = this.hasBoardOverlayMessageTarget
      ? this.boardOverlayMessageTarget
      : (this.element.querySelector('[data-scrum-target="boardOverlayMessage"]') as HTMLElement | null);
    if (overlay) {
      overlay.classList.toggle("hidden", !active);
      overlay.classList.toggle("flex", active);
    }
    if (overlayMessage && message) {
      overlayMessage.textContent = message;
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
    options: { closeModal?: boolean; refreshCardID?: string | null } = {},
  ): Promise<T | undefined> {
    this.setBoardLoading(true, message.endsWith("…") ? message : `${message}…`);
    this.setStatus(message, "busy");
    try {
      const result = await action();
      if (this.projectID) {
        await this.reloadBoard(options.refreshCardID);
        if (options.refreshCardID) await this.refreshModalSections(options.refreshCardID);
        this.startPolling();
      }
      if (options.closeModal) this.closeModal();
      const doneLabel = message.endsWith("…") ? message.slice(0, -1) : message;
      this.actionOk(`${doneLabel} complete`);
      return result;
    } catch (error) {
      this.actionFail(error);
      return undefined;
    } finally {
      this.setBoardLoading(false);
    }
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
    this.applyBoardPayload(payload, false);
    const id = cardID ?? this.activeCardID;
    if (!id) return null;
    const card = this.findCard(id);
    if (card && this.activeCardID === id) {
      await this.refreshModalSections(id);
    }
    return card;
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

  private channelRenderOptions(cardID?: string | null) {
    const id = cardID ?? this.activeCardID;
    const card = id ? this.findCard(id) : null;
    return {
      pilotPending: Boolean(id && this.channelPilotPendingCardID === id),
      agentRunning: card?.play_state === "running",
    };
  }

  private refreshChannelUI(cardID?: string | null) {
    const id = cardID ?? this.activeCardID;
    if (!id) return;
    const card = this.findCard(id);
    if (!card || !this.board) return;
    this.recycle("scrum-modal-channel", renderScrumModalChannelTab(card, this.playQueue ?? undefined, this.channelRenderOptions(id)));
    this.applyCardTabState();
    this.scrollChannelToLatest(true);
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
    this.recycle("scrum-modal-channel", renderScrumModalChannelTab(card, this.playQueue ?? undefined, this.channelRenderOptions(cardID)));
    this.applyCardTabState();
    if (this.activeCardTab === "channel") {
      this.scrollChannelToLatest(true);
    }
    this.wireCoachAutoScan();
  }

  private async refreshLiveChannel(cardID: string, pinScroll = false) {
    const card = this.findCard(cardID);
    if (!card || !this.board) return;
    this.recycle("scrum-modal-toolbar", renderScrumModalToolbar(card, this.board, this.playQueue ?? undefined));
    this.recycle("scrum-modal-tabs", `<nav class="flex flex-wrap gap-2" aria-label="Card sections">${renderScrumModalTabNav(card, this.activeCardTab)}</nav>`);
    this.recycle("scrum-modal-channel", renderScrumModalChannelTab(card, this.playQueue ?? undefined, this.channelRenderOptions(cardID)));
    this.applyCardTabState();
    this.scrollChannelToLatest(pinScroll);
  }

  private channelStreamElement(): HTMLElement | null {
    const panel = document.querySelector('[data-chat-target="modalPanel"]');
    const stream = panel?.querySelector("[data-scrum-channel-messages]");
    return stream instanceof HTMLElement ? stream : null;
  }

  private shouldStickChannelScroll(stream: HTMLElement): boolean {
    // flex-col-reverse: scrollTop 0 = pinned to newest at the bottom
    return stream.scrollTop <= 64;
  }

  private scrollChannelToLatest(force = false) {
    const run = () => {
      const stream = this.channelStreamElement();
      if (!stream) return;
      if (!force && !this.shouldStickChannelScroll(stream)) return;
      stream.scrollTop = 0;
      stream.querySelector("[data-scrum-channel-anchor]")?.scrollIntoView({ block: "end" });
    };
    run();
    requestAnimationFrame(() => {
      run();
      requestAnimationFrame(run);
    });
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
      this.actionOk("Coach settings saved");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Coach replied");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async openCard(event: Event) {
    if (this.boardDrag.shouldSuppressClick()) return;
    const target = event.target as HTMLElement;
    if (target.closest("button, select, option, a, textarea, input, label")) return;

    const article = target.closest("[data-card-id]") as HTMLElement | null;
    const cardID = article?.dataset.cardId;
    if (!cardID) return;

    const card = this.findCard(cardID);
    if (!card || !this.board) return;

    this.activeCardID = cardID;
    this.activeCardTab = this.restoreCardTab(cardID);
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
    this.setBoardLoading(true, "Loading board…");
    this.setStatus("Loading board…", "busy");
    try {
      const payload = await fetchScrumBoard(this.projectID);
      this.applyBoardPayload(payload);
      if (this.activeCardID) await this.refreshModalSections(this.activeCardID);
      this.startPolling();
      this.setStatus(`Updated ${new Date().toLocaleTimeString()}`, "ok");
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      this.boardTarget.innerHTML = renderScrumEmptyState(`Failed to load scrum board: ${message}`);
      this.actionFailMessage(message);
    } finally {
      this.setBoardLoading(false);
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

  async withCardAction(cardID: string, action: () => Promise<ScrumCard>, label: string) {
    if (!cardID) return;
    await this.withBoardRefresh(label, async () => {
      const card = await action();
      this.upsertCard(card);
      return card;
    }, { refreshCardID: cardID });
  }

  private async withCardMove(cardID: string, column: string, label: string) {
    if (!cardID || !column) return;
    this.skipMoveToastFor.add(cardID);
    const card = this.findCard(cardID);
    const previousColumn = card?.column;
    if (card) {
      card.column = column;
      this.renderBoardFromLocal(false);
      if (this.activeCardID === cardID && this.board) {
        this.recycle("scrum-modal-toolbar", renderScrumModalToolbar(card, this.board, this.playQueue ?? undefined));
      }
    }
    this.setStatus(label, "busy");
    try {
      const updated = await moveScrumCard(cardID, column, this.projectID);
      this.upsertCard(updated);
      const payload = await fetchScrumBoard(this.projectID);
      this.applyBoardPayload(payload, false);
      if (this.activeCardID === cardID) await this.refreshModalSections(cardID);
      this.startPolling();
      this.actionOk(`${label} complete`);
    } catch (error) {
      if (card && previousColumn != null) {
        card.column = previousColumn;
        this.renderBoardFromLocal(false);
        if (this.activeCardID === cardID && this.board) {
          this.recycle("scrum-modal-toolbar", renderScrumModalToolbar(card, this.board, this.playQueue ?? undefined));
        }
      }
      this.actionFail(error);
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
    const modalOpen = this.activeCardID === cardID;
    if (modalOpen) this.activeCardTab = "channel";
    await this.withBoardRefresh(
      pivot ? "Pivoting play…" : "Queueing play…",
      async () => {
        const configSink = modalOpen ? document.querySelector('[data-recyclr-sink="scrum-modal-config"]') : null;
        const agentConfig = configSink ? collectAgentFieldValues(configSink, "card") : {};
        const result = await playScrumCard(cardID, this.projectID, {
          pivot,
          agentConfig: Object.keys(agentConfig).length > 0 ? agentConfig : undefined,
        });
        this.upsertCard(result);
        return result;
      },
      { refreshCardID: cardID },
    );
    if (modalOpen) {
      this.applyCardTabState();
      await this.refreshLiveChannel(cardID);
      this.syncPollInterval();
    }
  }

  syncJob(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    void this.withBoardRefresh("Refreshing play queue", () => syncScrumBoard(this.projectID));
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
    void this.withCardMove(cardID, column, "Moving card");
  }

  retreat(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const cardID = this.cardID(event);
    const card = this.findCard(cardID);
    if (!card) return;
    const column = prevColumn(card.column);
    if (!column) return;
    void this.withCardMove(cardID, column, "Moving card");
  }

  moveSelect(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const target = event.currentTarget as HTMLSelectElement;
    const cardID = target.dataset.cardId || "";
    const column = target.value;
    if (!cardID || !column) return;
    void this.withCardMove(cardID, column, "Moving card");
  }

  modalMoveSelect(event: Event) {
    this.moveSelect(event);
  }

  assignCard(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const cardID = this.cardID(event);
    void this.withCardMove(cardID, "assigned", "Moving to Assigned");
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
      this.actionOk("Card saved");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Checklist updated");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Checklist item added");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Checklist updated");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Tag added");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Tag removed");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk(payload.notes ? `Tags suggested — ` : "Tags suggested");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Test updated");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Test added");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Test removed");
    } catch (error) {
      this.actionFail(error);
    }
  }

  channelComposerKeydown(event: KeyboardEvent) {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault();
      void this.sendChat(event);
    }
  }

  async sendChat(event: Event) {
    event.preventDefault();
    const form = (event.currentTarget as HTMLElement | null)?.closest("form") ?? (event.currentTarget as HTMLFormElement | null);
    if (!form) return;
    const cardID = form.dataset.cardId || "";
    const input = form.querySelector('[data-scrum-field="chatMessage"]') as HTMLTextAreaElement | null;
    const message = input?.value.trim();
    if (!cardID || !message) return;
    if (this.channelPilotPendingCardID === cardID) {
      this.setStatus("Already sending…", "busy");
      return;
    }

    this.activeCardTab = "channel";
    this.persistCardTab("channel");
    this.applyCardTabState();

    const card = this.findCard(cardID);
    if (card) {
      const optimistic: ScrumCard = {
        ...card,
        chat: [...(card.chat ?? []), { role: "user", content: message, created_at: new Date().toISOString() }],
      };
      this.upsertCard(optimistic);
    }
    if (input) input.value = "";

    this.channelPilotPendingCardID = cardID;
    this.refreshChannelUI(cardID);
    this.setStatus("Sending…", "busy");

    try {
      const payload = await chatScrumCard(cardID, message, this.projectID);
      this.upsertCard(payload.card);
      this.channelPilotPendingCardID = null;
      this.refreshChannelUI(cardID);
      this.applyCardTabState();
      if (payload.error) {
        this.actionFailMessage(String(payload.error));
      } else if (payload.action === "steered" || payload.action === "feedback") {
        this.actionOk(`Sent to agent${payload.agent ? ` (${payload.agent})` : ""}`);
      } else if (payload.action === "started") {
        this.actionOk(`Agent started${payload.agent ? ` (${payload.agent})` : ""}`);
      } else {
        this.actionOk(payload.action === "saved" ? "Message saved" : "Message sent");
      }
    } catch (error) {
      this.channelPilotPendingCardID = null;
      this.refreshChannelUI(cardID);
      this.actionFail(error);
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
      this.actionOk("Reference attached");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Reference removed");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Jira draft generated");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Jira draft saved");
    } catch (error) {
      this.actionFail(error);
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
      this.actionFailMessage("Recipe JSON is invalid");
      return;
    }
    const recipeID = this.modalField(event, "recipeId") || this.modalPanelField("recipeId");
    this.setStatus("Saving card recipe…", "busy");
    try {
      const updated = await patchScrumCard(cardID, { recipe_id: recipeID, recipe }, this.projectID);
      this.upsertCard(updated);
      await this.refreshModalSections(cardID);
      await this.reloadBoard(cardID);
      this.actionOk("Card recipe saved");
    } catch (error) {
      this.actionFail(error);
    }
  }

  async deleteCard(event: Event) {
    event.preventDefault();
    event.stopPropagation();
    const cardID = this.cardID(event);
    if (!cardID) return;
    if (!window.confirm("Delete this scrum card?")) return;
    const closeModal = this.activeCardID === cardID;
    await this.withBoardRefresh(
      "Deleting card…",
      () => deleteScrumCard(cardID, this.projectID),
      { closeModal },
    );
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
      this.actionOk("Card model settings saved");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Card model overrides cleared");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk(`Agent set to ${system}`);
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Card agent settings saved");
    } catch (error) {
      this.actionFail(error);
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
      this.actionOk("Card agent overrides cleared");
    } catch (error) {
      this.actionFail(error);
    }
  }

  private findCard(cardID: string): ScrumCard | null {
    if (!this.board) return null;
    return this.board.cards.find((card) => card.id === cardID) ?? null;
  }
}
