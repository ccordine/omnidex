import { readJSON } from "../lib/api";
import { TranscriptStore } from "../lib/transcript_store";
import { renderChatMessages } from "../lib/chat_render";
import {
  renderEventModal,
  renderContextModal,
} from "../lib/render";
import type { ChatMessage, TimelineEvent, JobContext } from "../lib/types";
import { closeModalShell, openModalShell } from "../lib/modal";
import type RecyclrController from "./recyclr_controller";
import { toastFromError } from "../lib/feedback";
import { renderRecyclrBundle, type RecyclrSinkMode } from "../lib/recyclr";
import { getLocale, t } from "../lib/i18n";
import { parsePanelFromLocation, type OmniPanel } from "../lib/panel_routing";
import type { RealtimeSyncDetail } from "../lib/realtime_sync";
import {
  badgeClass,
  escapeHTML,
  formatTime,
  statusPillClass,
} from "../lib/dom";
import { ChatChannelCoordinator } from "../lib/chat_channel_coordinator";
import { ChatJobsCoordinator } from "../lib/chat_jobs_coordinator";
import { ChatMemoryCoordinator } from "../lib/chat_memory_coordinator";
import { ChatSystemCoordinator } from "../lib/chat_system_coordinator";
import { recordChatJobProgress } from "../lib/chat_job_progress";
import { renderChatProgress, renderChatProgressActivity } from "../lib/chat_progress_view";
import { ChatExecutionCoordinator } from "../lib/chat_execution_coordinator";
import { ChatPanelCoordinator } from "../lib/chat_panel_coordinator";
import { ChatTargetsController } from "./chat_targets_controller";

export default class ChatController extends ChatTargetsController {

  recyclrController: RecyclrController | null = null;
  store!: TranscriptStore;
  messages: ChatMessage[] = [];
  events: TimelineEvent[] = [];
  eventSequence = 0;
  eventIndex = new Map<string, TimelineEvent>();
  contextIndex = new Map<string, JobContext>();
  seenProgress = new Set<string>();
  busy = false;
  queueEnabled = false;
  memoryChangedHandler: ((event: Event) => void) | null = null;
  networkSettingsHandler: ((event: Event) => void) | null = null;
  openedProjectID: number | null = null;
  openedProjectLocation: string | null = null;
  projectOpenedHandler: ((event: Event) => void) | null = null;
  projectClosedHandler: ((event: Event) => void) | null = null;
  private channel!: ChatChannelCoordinator;
  private jobs!: ChatJobsCoordinator;
  private memory!: ChatMemoryCoordinator;
  private system!: ChatSystemCoordinator;
  private execution!: ChatExecutionCoordinator;
  private panels!: ChatPanelCoordinator;
  activityLabel = "";
  private metricsGlanceHandler: ((event: Event) => void) | null = null;
  private readonly jobProgressHandler = (event: Event) => {
    this.execution.handleProgress(event);
  };
  private readonly realtimeSyncHandler = (event: Event) => {
    const detail = (event as CustomEvent<RealtimeSyncDetail>).detail;
    if (!detail || typeof detail.waitUntil !== "function") {
      throw new Error("Realtime synchronization event is missing waitUntil().");
    }
    detail.waitUntil(this.synchronizeRealtimeState());
  };

  private async synchronizeRealtimeState(): Promise<void> {
    const tasks: Promise<unknown>[] = [this.loadGlobalActivity({ quiet: true, strict: true })];
    if (this.execution.currentJobID() !== null) tasks.push(this.execution.refreshCurrent());
    if (this.panels.isCurrent("jobs")) tasks.push(this.loadJobs({ quiet: true, strict: true }));
    if (this.panels.isCurrent("metrics")) tasks.push(this.loadMetrics({ strict: true }));
    await Promise.all(tasks);
  }

  

  async connect() {
    this.recyclrController = this.application.getControllerForElementAndIdentifier(this.element, "recyclr") as RecyclrController | null;
    if (!this.recyclrController) throw new Error("The page-scoped Recyclr controller is unavailable.");
    this.store = new TranscriptStore();
    this.messages = this.store.load();
    this.events = [];
    this.eventSequence = 0;
    this.eventIndex = new Map();
    this.contextIndex = new Map();
    this.seenProgress = new Set();
    this.busy = false;
    this.channel = new ChatChannelCoordinator({
      hasNetworkURL: () => this.hasNetworkUrlTarget,
      networkURL: () => this.networkUrlTarget,
      hasTransport: () => this.hasTransportTarget,
      transport: () => this.transportTarget,
      hasChannelSelect: () => this.hasChannelSelectTarget,
      channelSelect: () => this.channelSelectTarget,
      queueEnabled: () => this.queueEnabled,
      setQueueEnabled: (enabled) => { this.queueEnabled = enabled; },
      setStatus: (text, mode) => this.setStatus(text, mode),
      addEvent: (type, details, full) => this.addEvent(type, details, full),
      addMessage: (role, content) => this.addMessage(role, content),
      replaceMessages: (messages) => {
        this.messages = messages;
        this.renderMessages();
      },
      restorePipelineTranscript: () => {
        this.messages = this.store.load();
        this.renderMessages();
        if (this.messages.length === 0) this.addMessage("system", "Agent pipeline — queue jobs or direct instruct.");
      },
      setActivityLabel: (label) => { this.activityLabel = label; },
      renderProgressActivity: (label) => this.renderProgressActivity(label),
      setBusy: (value) => this.setBusy(value),
    });
    this.execution = new ChatExecutionCoordinator({
      openedProjectID: () => this.openedProjectID,
      openedProjectLocation: () => this.openedProjectLocation,
      currentPanel: () => this.panels.current(),
      hasJobBadge: () => this.hasJobTarget,
      jobBadge: () => this.jobTarget,
      setActivityLabel: (label) => { this.activityLabel = label; },
      setStatus: (text, mode) => this.setStatus(text, mode),
      renderProgressActivity: (label) => this.renderProgressActivity(label),
      recordJobProgress: (details) => recordChatJobProgress(this, details),
      renderMessages: () => this.renderMessages(),
      renderJobDetails: (details) => this.renderJobDetails(details),
      addEvent: (type, details, full) => this.addEvent(type, details, full),
      addMessage: (role, content) => this.addMessage(role, content),
      setBusy: (value) => this.setBusy(value),
      loadJobs: (options) => this.loadJobs(options),
      loadGlobalActivity: (options) => this.loadGlobalActivity(options),
      reportError: (error) => toastFromError(error),
    });
    this.jobs = new ChatJobsCoordinator({
      queueEnabled: () => this.queueEnabled,
      jobFilter: () => this.jobFilterTarget,
      hasJobDetails: () => this.hasJobDetailsTarget,
      jobDetails: () => this.jobDetailsTarget,
      hasJobBadge: () => this.hasJobTarget,
      jobBadge: () => this.jobTarget,
      setCurrentJobID: (id) => this.execution.setCurrentJobID(id),
      recycle: (target, html) => this.recycle(target, html),
      indexContexts: (contexts) => this.indexContexts(contexts),
      addEvent: (type, details, full) => this.addEvent(type, details, full),
    });
    this.panels = new ChatPanelCoordinator({
      root: () => this.element,
      locale: () => getLocale(),
      renderPanel: (html) => renderRecyclrBundle(this.recyclrController, "app-panel", html),
      loadPanelData: (panel) => this.loadPanelData(panel),
      pushRoute: (path) => {
        if (!this.recyclrController) throw new Error("The page-scoped Recyclr controller is unavailable.");
        this.recyclrController.pushRoute(path);
      },
      addEvent: (type, details, full) => this.addEvent(type, details, full),
      reportError: (error) => toastFromError(error),
    });
    this.memory = new ChatMemoryCoordinator({
      queueEnabled: () => this.queueEnabled,
      hasMemoryList: () => this.hasMemoryListTarget,
      memoryKind: () => this.memoryKindTarget,
      memoryKindFilter: () => this.memoryKindFilterTarget,
      memoryTags: () => this.memoryTagsTarget,
      memoryContent: () => this.memoryContentTarget,
      recycle: (target, html) => this.recycle(target, html),
      addEvent: (type, details, full) => this.addEvent(type, details, full),
      addObservedEvent: (key, type, details, full) => this.addObservedEvent(key, type, details, full),
    });
    this.system = new ChatSystemCoordinator({
      queueEnabled: () => this.queueEnabled,
      setQueueEnabled: (enabled) => { this.queueEnabled = enabled; },
      personaMode: () => this.personaModeTarget,
      personaModel: () => this.personaModelTarget,
      personaSystem: () => this.personaSystemTarget,
      personaPrompt: () => this.personaPromptTarget,
      hasHostBridgeStatus: () => this.hasHostBridgeStatusOutputTarget,
      hasResearchStatus: () => this.hasResearchStatusOutputTarget,
      hasMetrics: () => this.hasMetricsOutputTarget,
      updateTransportLabel: () => this.channel.updateTransportLabel(),
      recycle: (target, html, mode) => this.recycle(target, html, mode),
      addEvent: (type, details, full) => this.addEvent(type, details, full),
    });
    this.renderProgress();
    this.renderMessages();
    this.renderTimeline();
    await this.channel.detectTransport();
    const initialPanel = parsePanelFromLocation();
    await this.activatePanel(initialPanel, { pushHistory: false });
    await this.loadStatus();
    await this.channel.loadChannels();
    await this.loadGlobalActivity();
    this.memoryChangedHandler = () => void this.loadMemoryCandidates();
    document.addEventListener("omni:memory-changed", this.memoryChangedHandler);
    this.networkSettingsHandler = (event: Event) => {
      const detail = (event as CustomEvent<{ core_url?: string }>).detail;
      if (detail?.core_url) this.channel.setNetworkURL(detail.core_url);
    };
    document.addEventListener("omni:network-settings", this.networkSettingsHandler);
    this.projectOpenedHandler = (event: Event) => {
      const detail = (event as CustomEvent<{ project_id?: number; location?: string }>).detail;
      this.openedProjectID = detail?.project_id && detail.project_id > 0 ? detail.project_id : null;
      this.openedProjectLocation = detail?.location?.trim() || null;
    };
    document.addEventListener("omni:project-opened", this.projectOpenedHandler);
    this.projectClosedHandler = () => {
      this.openedProjectID = null;
      this.openedProjectLocation = null;
    };
    document.addEventListener("omni:project-closed", this.projectClosedHandler);
    if (this.messages.length === 0) {
      this.addMessage("system", t("panel.chat.ready"));
    }
    this.metricsGlanceHandler = () => {
      if (this.panels.isCurrent("metrics")) void this.loadMetrics();
    };
    document.addEventListener("omni:metrics-glance", this.metricsGlanceHandler);
    document.addEventListener("omni:job-progress", this.jobProgressHandler);
    document.addEventListener("omni:realtime-sync-required", this.realtimeSyncHandler);
  }

  disconnect() {
    if (this.memoryChangedHandler) document.removeEventListener("omni:memory-changed", this.memoryChangedHandler);
    if (this.networkSettingsHandler) document.removeEventListener("omni:network-settings", this.networkSettingsHandler);
    if (this.projectOpenedHandler) document.removeEventListener("omni:project-opened", this.projectOpenedHandler);
    if (this.projectClosedHandler) document.removeEventListener("omni:project-closed", this.projectClosedHandler);
    if (this.metricsGlanceHandler) document.removeEventListener("omni:metrics-glance", this.metricsGlanceHandler);
    document.removeEventListener("omni:job-progress", this.jobProgressHandler);
    document.removeEventListener("omni:realtime-sync-required", this.realtimeSyncHandler);
    this.execution.disconnect();
  }

  async selectChannel(event: Event) {
    await this.channel.select(event);
  }

  async createChannel(event: Event) {
    await this.channel.create(event);
  }

  async submitChannel(prompt: string) {
    await this.channel.submit(prompt);
  }

  async showPanel(event: Event) {
    await this.panels.show(event);
  }

  async activatePanel(name: OmniPanel, options: { pushHistory?: boolean } = {}) {
    await this.panels.activate(name, options);
  }

  private loadPanelData(panel: OmniPanel): void {
    let task: Promise<void> | null = null;
    if (panel === "jobs") task = this.loadJobs({ strict: true });
    if (panel === "memory") task = this.loadMemoryCandidates();
    if (panel === "metrics") task = this.loadMetrics({ strict: true });
    if (!task) return;
    void task.catch((error) => {
      this.addEvent("ui_panel_data_error", { panel, error: error instanceof Error ? error.message : String(error) });
      toastFromError(error);
    });
  }

  composerKeydown(event) {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault();
      this.submit(event);
    }
  }

  async submit(event) {
    event.preventDefault();
    if (!this.hasInputTarget) {
      await this.activatePanel("chat", { pushHistory: true });
      if (!this.hasInputTarget) return;
    }
    const prompt = this.inputTarget.value.trim();
    if (!prompt || this.busy) return;

    this.inputTarget.value = "";
    this.addMessage("user", prompt);
    this.activityLabel = "Sending…";
    this.setBusy(true);
    this.renderProgressActivity(this.activityLabel);

    try {
      if (this.channel.isChannelMode()) {
        await this.submitChannel(prompt);
      } else if (this.queueEnabled) {
        await this.submitJob(prompt);
      } else {
        await this.submitDirect(prompt);
      }
    } catch (error) {
      this.addMessage("error", error.message || String(error));
      this.addEvent("request_failed", { error: error.message || String(error) });
      this.setBusy(false);
      this.setStatus("failed", "error");
    }
  }

  async submitJob(prompt) {
    await this.execution.submit(prompt);
  }

  async loadJobs(options: { quiet?: boolean; strict?: boolean } = {}) {
    await this.jobs.load(options);
  }

  renderJobs(jobs) {
    this.jobs.render(jobs);
  }

  async selectJob(event) {
    await this.jobs.select(event);
  }

  renderJobDetails(details) {
    this.jobs.renderDetails(details);
  }

  async interruptJob(event) {
    await this.jobs.interrupt(event);
  }

  async replanJob(event) {
    await this.jobs.replan(event);
  }

  async cancelJob(event) {
    await this.jobs.cancel(event);
  }

  async loadMemoryCandidates() {
    await this.memory.load();
  }

  async deleteMemory(event) {
    await this.memory.deleteMemory(event);
  }

  async deleteMemoryCandidate(event) {
    await this.memory.deleteCandidate(event);
  }

  renderMemoryList(items) {
    this.memory.renderList(items);
  }

  async loadGlobalActivity(options: { quiet?: boolean; strict?: boolean } = {}) {
    await this.memory.loadGlobalActivity(options);
  }

  renderMemoryCandidates(items) {
    this.memory.renderCandidates(items);
  }

  async promoteMemory(event) {
    await this.memory.promote(event);
  }

  async rejectMemory(event) {
    await this.memory.reject(event);
  }

  async addMemory(event) {
    await this.memory.add(event);
  }

  async runPersona(event) {
    await this.system.runPersona(event);
  }

  async loadStatus() {
    await this.system.loadStatus();
  }

  async loadHostBridgeStatus() {
    await this.system.loadHostBridgeStatus();
  }

  async loadResearchStatus() {
    await this.system.loadResearchStatus();
  }

  async loadMetrics(options: { strict?: boolean } = {}) {
    await this.system.loadMetrics(options);
  }

  async migrateFresh() {
    await this.system.migrateFresh();
  }

  async submitDirect(prompt) {
    this.activityLabel = "Thinking…";
    this.setStatus("thinking", "active");
    this.renderProgressActivity(this.activityLabel);
    const requestBody = {
      prompt,
      system: "You are Omni chat. Be concise, useful, and grounded.",
      context: { source: "omni-web-chat", mode: "direct" },
      history: this.messages
        .filter((message) => message.role === "user" || message.role === "assistant")
        .slice(-12)
        .map((message) => ({ role: message.role, content: message.content })),
    };
    const response = await fetch("/v1/instruct", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(requestBody),
    });
    const payload = await readJSON(response);
    this.addEvent("direct_response", { model: payload.model, latency_ms: payload.latency_ms }, { request: requestBody, response: payload });
    this.addMessage("assistant", payload.output || "(empty response)");
    this.setStatus("ready", "ready");
    this.setBusy(false);
  }

  async newThread() {
    if (!this.panels.isCurrent("chat") || !this.hasMessagesTarget) {
      await this.activatePanel("chat", { pushHistory: true });
    }
    if (this.channel.isChannelMode()) {
      void this.channel.loadTranscript(this.channel.selectedID());
      this.addMessage("system", "Reloaded channel transcript from server.");
      return;
    }
    this.execution.setCurrentJobID(null);
    this.events = [];
    this.eventIndex = new Map();
    this.contextIndex = new Map();
    this.seenProgress = new Set();
    this.messages = [];
    this.store.save(this.messages);
    this.renderProgress();
    this.renderMessages();
    this.renderTimeline();
    this.addMessage("system", "New local thread started.");
  }

  clearTranscript() {
    this.store.clear();
    this.messages = [];
    this.renderMessages();
    this.addMessage("system", "Local transcript cleared.");
  }

  addMessage(role, content) {
    this.messages.push({ role, content, at: new Date().toISOString() });
    if (!this.channel.isChannelMode()) {
      this.store.save(this.messages);
    }
    this.renderMessages();
  }

  addEvent(type, details = {}, full = null) {
    const id = `evt_${String(++this.eventSequence).padStart(6, "0")}`;
    const event = { id, type, details, full: full || details, at: new Date().toISOString() };
    this.events.push(event);
    this.events = this.events.slice(-120);
    this.eventIndex.set(id, event);
    for (const oldID of [...this.eventIndex.keys()]) {
      if (!this.events.some((item) => item.id === oldID)) this.eventIndex.delete(oldID);
    }
    this.renderTimeline();
  }

  addObservedEvent(key, type, details = {}, full = null) {
    if (!key || this.seenProgress.has(key)) return;
    this.seenProgress.add(key);
    this.addEvent(type, details, full);
  }

  renderMessages() {
    if (!this.hasMessagesTarget) return;
    const html = renderChatMessages(this.messages, {
      pending: this.busy,
      pendingLabel: this.activityLabel || "Working…",
    });
    this.recycle("messages", html);
    this.messagesTarget.scrollTop = this.messagesTarget.scrollHeight;
  }

  renderTimeline() {
    if (!this.hasEventCountTarget) return;
    this.eventCountTarget.textContent = `${this.events.length} events`;
    const html = this.events
      .slice()
      .reverse()
      .map((event) => {
      const detailRows = Object.entries(event.details || {})
        .map(([key, value]) => `<div><span class="timeline-key">${escapeHTML(key)}</span><span>${escapeHTML(String(value))}</span></div>`)
        .join("");
      return `
      <button type="button" data-action="chat#openTimelineItem" data-event-id="${escapeHTML(event.id)}" class="timeline-card block w-full text-left transition hover:border-cyan-300/40 hover:bg-cyan-300/10">
        <div class="flex items-start justify-between gap-3">
          <div>
            <h3 class="text-sm font-semibold text-zinc-100">${escapeHTML(event.type)}</h3>
            <div class="mt-1 font-mono text-[11px] text-zinc-600">${escapeHTML(event.id)}</div>
          </div>
          <time class="font-mono text-[11px] text-zinc-500">${formatTime(event.at)}</time>
        </div>
        <div class="mt-2 space-y-1 font-mono text-xs text-zinc-300">${detailRows}</div>
      </button>
    `;
      })
      .join("");
    this.recycle("timeline", html);
  }

  renderProgressActivity(label: string) {
    renderChatProgressActivity(this.progressViewHost(), label);
  }

  renderProgress(details = null) {
    renderChatProgress(this.progressViewHost(), details);
  }

  private progressViewHost() {
    return {
      hasProgressState: () => this.hasProgressStateTarget,
      progressState: () => this.progressStateTarget,
      recycle: (target: string, html: string) => this.recycle(target, html),
    };
  }

  indexContexts(contexts) {
    for (const context of contexts || []) {
      if (context && context.id != null) this.contextIndex.set(String(context.id), context);
    }
  }

  openTimelineItem(event) {
    const id = event.currentTarget.dataset.eventId;
    const item = this.eventIndex.get(id);
    if (!item) return;
    this.openModal(renderEventModal(item));
  }

  openContextItem(event) {
    const id = event.currentTarget.dataset.contextId;
    const context = this.contextIndex.get(String(id));
    if (!context) return;
    this.openModal(renderContextModal(context));
  }

  closeModal() {
    if (!this.hasModalTarget) return;
    closeModalShell();
  }

  closeModalBackdrop(event) {
    if (event.target === this.modalTarget) this.closeModal();
  }

  openModal(html) {
    this.recycle("modal", html);
    openModalShell();
  }

  setBusy(value) {
    this.busy = value;
    if (this.hasSendTarget) {
      this.sendTarget.disabled = value;
      this.sendTarget.textContent = value ? "Working" : "Send";
    }
    if (this.hasSpinnerTarget) this.spinnerTarget.classList.toggle("hidden", !value);
    if (!value) this.activityLabel = "";
    this.renderMessages();
  }

  setStatus(text, mode) {
    if (this.hasStatusTarget) this.statusTarget.textContent = text;
    if (this.hasLiveBadgeTarget) {
      this.liveBadgeTarget.textContent = text;
      this.liveBadgeTarget.className = badgeClass(mode);
    }
  }

  recycle(target: string, html: string, mode: RecyclrSinkMode = "html"): void {
    const controller = this.recyclrController ?? (window as Window & { omniRecyclr?: RecyclrController }).omniRecyclr;
    if (!controller) throw new Error("The page-scoped Recyclr controller is unavailable.");
    void renderRecyclrBundle(controller, target, html, mode).catch((error) => {
      console.error(`Failed to render Recyclr sink ${JSON.stringify(target)}`, error);
      toastFromError(error);
    });
  }
}
