import { Controller } from "@hotwired/stimulus";
import { createScrumCard } from "../lib/scrum_api";
import {
  fetchOllamaModels,
  fetchProjectPlanningChat,
  sendProjectPlanningChat,
  updateProjectPlanningChatConfig,
  type ProjectPlanningCardDraft,
  type ProjectPlanningChatConfig,
  type ProjectPlanningSuggestion,
} from "../lib/project_chat_api";
import {
  renderProjectPlanningCardDrafts,
  renderProjectPlanningChatMessages,
  renderProjectPlanningSuggestions,
} from "../lib/project_chat_render";
import { reportError } from "../lib/feedback";
import type { ScrumChatMessage } from "../lib/scrum_types";

export default class ProjectChatController extends Controller {
  static targets = ["messages", "input", "status", "modelSelect", "suggestions", "drafts"];

  declare readonly messagesTarget: HTMLElement;
  declare readonly inputTarget: HTMLTextAreaElement;
  declare readonly statusTarget: HTMLElement;
  declare readonly modelSelectTarget: HTMLSelectElement;
  declare readonly suggestionsTarget: HTMLElement;
  declare readonly draftsTarget: HTMLElement;

  private projectID: number | null = null;
  private activeTab = "";
  private busy = false;
  private chat: ScrumChatMessage[] = [];
  private config: ProjectPlanningChatConfig = { reasoning_mode: "instant" };
  private cardDrafts: ProjectPlanningCardDraft[] = [];
  private modelOptions: string[] = [];
  private onProjectOpened = (event: Event) => this.handleProjectOpened(event);
  private onProjectClosed = () => this.handleProjectClosed();
  private onProjectTab = (event: Event) => this.handleProjectTab(event);

  connect() {
    document.addEventListener("omni:project-opened", this.onProjectOpened);
    document.addEventListener("omni:project-closed", this.onProjectClosed);
    document.addEventListener("omni:project-tab", this.onProjectTab);
  }

  disconnect() {
    document.removeEventListener("omni:project-opened", this.onProjectOpened);
    document.removeEventListener("omni:project-closed", this.onProjectClosed);
    document.removeEventListener("omni:project-tab", this.onProjectTab);
  }

  private handleProjectOpened(event: Event) {
    const detail = (event as CustomEvent<{ project_id?: number }>).detail;
    this.projectID = detail?.project_id ?? null;
    if (this.activeTab === "chat" && this.projectID) {
      void this.loadChat();
    }
  }

  private handleProjectClosed() {
    this.projectID = null;
    this.chat = [];
    this.cardDrafts = [];
    this.renderMessages();
    this.renderSidePanels([], []);
  }

  private handleProjectTab(event: Event) {
    const detail = (event as CustomEvent<{ tab?: string; project_id?: number | null }>).detail;
    this.activeTab = detail?.tab ?? "";
    if (detail?.project_id) {
      this.projectID = detail.project_id;
    }
    if (this.activeTab === "chat" && this.projectID) {
      void this.loadChat();
    }
  }

  private setStatus(message: string, tone: "idle" | "busy" | "error" | "ok" = "idle") {
    const classes = { idle: "text-zinc-500", busy: "text-cyan-200", error: "text-rose-300", ok: "text-emerald-300" };
    this.statusTarget.textContent = message;
    this.statusTarget.className = `text-xs ${classes[tone] ?? classes.idle}`;
  }

  private currentConfig(): ProjectPlanningChatConfig {
    return {
      model: this.modelSelectTarget?.value?.trim() || "",
      reasoning_mode: this.config.reasoning_mode || "instant",
    };
  }

  private async loadChat() {
    if (!this.projectID || this.busy) return;
    this.setStatus("Loading chat…", "busy");
    try {
      await this.ensureModels();
      const payload = await fetchProjectPlanningChat(this.projectID);
      this.chat = payload.chat ?? [];
      this.config = payload.config ?? { reasoning_mode: "instant" };
      this.syncModelSelect();
      this.renderMessages();
      this.setStatus("Ready", "ok");
    } catch (error) {
      reportError(this.setStatus.bind(this), error);
    }
  }

  private async ensureModels() {
    if (this.modelOptions.length) return;
    try {
      const payload = await fetchOllamaModels();
      this.modelOptions = (payload.models ?? []).map((item) => item.name).filter(Boolean);
      this.syncModelSelect();
    } catch {
      this.modelOptions = [];
    }
  }

  private syncModelSelect() {
    if (!this.hasModelSelectTarget) return;
    const current = this.config.model || "";
    const options = [
      `<option value="">Auto (${this.config.reasoning_mode === "thinking" ? "thinking" : "instant"})</option>`,
      ...this.modelOptions.map((name) => `<option value="${name}"${current === name ? " selected" : ""}>${name}</option>`),
    ];
    this.modelSelectTarget.innerHTML = options.join("");
  }

  private renderMessages() {
    this.messagesTarget.innerHTML = renderProjectPlanningChatMessages(this.chat, {
      pending: this.busy,
      pendingLabel: "Planning…",
    });
    this.messagesTarget.scrollTop = this.messagesTarget.scrollHeight;
  }

  private renderSidePanels(suggestions: ProjectPlanningSuggestion[], drafts: ProjectPlanningCardDraft[]) {
    this.cardDrafts = drafts;
    this.suggestionsTarget.innerHTML =
      renderProjectPlanningSuggestions(suggestions) || `<p class="text-xs text-zinc-600">Tips from the planner appear here.</p>`;
    this.draftsTarget.innerHTML =
      renderProjectPlanningCardDrafts(drafts) || `<p class="text-xs text-zinc-600">Draft cards suggested by the planner.</p>`;
  }

  async changeModel() {
    if (!this.projectID) return;
    this.config = this.currentConfig();
    try {
      await updateProjectPlanningChatConfig(this.projectID, this.config);
      this.setStatus(`Model: ${this.config.model || "auto"}`, "ok");
    } catch (error) {
      reportError(this.setStatus.bind(this), error);
    }
  }

  async setReasoningMode(event: Event) {
    event.preventDefault();
    const button = event.currentTarget as HTMLElement;
    const mode = button.dataset.reasoningMode as "instant" | "thinking" | undefined;
    if (!mode || !this.projectID) return;
    this.config = { ...this.currentConfig(), reasoning_mode: mode };
    this.syncModelSelect();
    try {
      await updateProjectPlanningChatConfig(this.projectID, this.config);
      this.setStatus(mode === "thinking" ? "Thinking mode" : "Instant mode", "ok");
    } catch (error) {
      reportError(this.setStatus.bind(this), error);
    }
  }

  async scanBoard(event: Event) {
    event.preventDefault();
    await this.postChat({ mode: "scan", message: "" });
  }

  async runResearch(event: Event) {
    event.preventDefault();
    const query = this.inputTarget.value.trim();
    if (!query) {
      this.setStatus("Enter a research topic first", "error");
      this.inputTarget.focus();
      return;
    }
    await this.postChat({ mode: "research", message: query });
  }

  async sendMessage(event: Event) {
    event.preventDefault();
    const message = this.inputTarget.value.trim();
    if (!message) return;
    await this.postChat({ message });
  }

  private async postChat(input: { message?: string; mode?: string }) {
    if (!this.projectID || this.busy) return;
    this.busy = true;
    this.config = this.currentConfig();
    this.renderMessages();
    this.setStatus(input.mode === "research" ? "Researching…" : "Planning…", "busy");
    try {
      const payload = await sendProjectPlanningChat(this.projectID, {
        ...input,
        config: this.config,
      });
      this.chat = payload.chat ?? this.chat;
      this.config = payload.config ?? this.config;
      this.renderMessages();
      this.renderSidePanels(payload.suggestions ?? [], payload.card_drafts ?? []);
      if (input.message && input.mode !== "scan") {
        this.inputTarget.value = "";
      }
      const modelLabel = payload.model ? ` · ${payload.model}` : "";
      const researchLabel = payload.research_used ? " · research" : "";
      this.setStatus(`Ready${modelLabel}${researchLabel}`, "ok");
    } catch (error) {
      this.busy = false;
      this.renderMessages();
      reportError(this.setStatus.bind(this), error);
      return;
    }
    this.busy = false;
    this.renderMessages();
  }

  async createDraftCard(event: Event) {
    event.preventDefault();
    if (!this.projectID) return;
    const button = event.currentTarget as HTMLElement;
    const index = Number(button.dataset.draftIndex);
    const draft = this.cardDrafts[index];
    if (!draft?.title?.trim()) return;
    this.setStatus("Creating card…", "busy");
    try {
      const description = [draft.description?.trim(), draft.checklist?.length ? `Checklist:\n${draft.checklist.map((item) => `- ${item}`).join("\n")}` : ""]
        .filter(Boolean)
        .join("\n\n");
      await createScrumCard(draft.title.trim(), description, draft.column?.trim() || "backlog", this.projectID);
      this.setStatus(`Added “${draft.title.trim()}” to board`, "ok");
      document.dispatchEvent(new CustomEvent("omni:scrum-refresh"));
    } catch (error) {
      reportError(this.setStatus.bind(this), error);
    }
  }
}
