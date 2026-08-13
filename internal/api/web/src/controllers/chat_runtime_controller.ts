import { ChatChannelCoordinator } from "../lib/chat_channel_coordinator";
import { ChatExecutionCoordinator } from "../lib/chat_execution_coordinator";
import { ChatJobsCoordinator } from "../lib/chat_jobs_coordinator";
import { ChatMemoryCoordinator } from "../lib/chat_memory_coordinator";
import { ChatPanelCoordinator } from "../lib/chat_panel_coordinator";
import { ChatSystemCoordinator } from "../lib/chat_system_coordinator";
import { toastFromError } from "../lib/feedback";
import { getLocale } from "../lib/i18n";
import { parsePanelFromLocation, type OmniPanel } from "../lib/panel_routing";
import type { RealtimeSyncDetail } from "../lib/realtime_sync";
import type RecyclrController from "./recyclr_controller";
import { ChatViewController } from "./chat_view_controller";

export abstract class ChatRuntimeController extends ChatViewController {
  protected channel!: ChatChannelCoordinator;
  protected jobs!: ChatJobsCoordinator;
  protected memory!: ChatMemoryCoordinator;
  protected system!: ChatSystemCoordinator;
  protected execution!: ChatExecutionCoordinator;
  protected panels!: ChatPanelCoordinator;

  private memoryChangedHandler: (() => void) | null = null;
  private networkSettingsHandler: ((event: Event) => void) | null = null;
  private projectOpenedHandler: ((event: Event) => void) | null = null;
  private projectClosedHandler: (() => void) | null = null;
  private metricsGlanceHandler: (() => void) | null = null;
  private readonly jobProgressHandler = (event: Event) => this.execution.handleProgress(event);
  private readonly realtimeSyncHandler = (event: Event) => {
    const detail = (event as CustomEvent<RealtimeSyncDetail>).detail;
    if (!detail || typeof detail.waitUntil !== "function") {
      throw new Error("Realtime synchronization event is missing waitUntil().");
    }
    detail.waitUntil(this.synchronizeRealtimeState());
  };

  async connect(): Promise<void> {
    this.recyclrController = this.application.getControllerForElementAndIdentifier(
      this.element,
      "recyclr",
    ) as RecyclrController | null;
    if (!this.recyclrController) throw new Error("The page-scoped Recyclr controller is unavailable.");
    this.initializeViewState();
    this.wireCoordinators();
    await this.channel.detectTransport();
    await this.panels.activate(parsePanelFromLocation(), { pushHistory: false });
    await this.system.loadStatus();
    await this.channel.loadChannels();
    await this.memory.loadGlobalActivity();
    this.bindDocumentEvents();
  }

  disconnect(): void {
    if (this.memoryChangedHandler) document.removeEventListener("omni:memory-changed", this.memoryChangedHandler);
    if (this.networkSettingsHandler) document.removeEventListener("omni:network-settings", this.networkSettingsHandler);
    if (this.projectOpenedHandler) document.removeEventListener("omni:project-opened", this.projectOpenedHandler);
    if (this.projectClosedHandler) document.removeEventListener("omni:project-closed", this.projectClosedHandler);
    if (this.metricsGlanceHandler) document.removeEventListener("omni:metrics-glance", this.metricsGlanceHandler);
    document.removeEventListener("omni:job-progress", this.jobProgressHandler);
    document.removeEventListener("omni:realtime-sync-required", this.realtimeSyncHandler);
    this.execution.disconnect();
  }

  private wireCoordinators(): void {
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
      renderComponentBundle: (bundle) => this.renderComponentBundle(bundle),
      renderTranscriptBundle: (bundle, preserveScroll) => this.renderTranscriptBundle(bundle, preserveScroll),
      workspaceRoot: () => this.openedProjectLocation,
      setActivityLabel: (label) => { this.activityLabel = label; },
      renderProgressActivity: (label) => this.renderProgressActivity(label),
      setBusy: (value) => this.setBusy(value),
      waitForJob: (id) => this.execution.waitForExistingJob(id),
    });
    this.execution = new ChatExecutionCoordinator({
      currentPanel: () => this.panels.current(),
      hasJobBadge: () => this.hasJobTarget,
      jobBadge: () => this.jobTarget,
      setActivityLabel: (label) => { this.activityLabel = label; },
      setStatus: (text, mode) => this.setStatus(text, mode),
      renderProgressActivity: (label) => this.renderProgressActivity(label),
      renderJobState: (details) => this.renderJobState(details),
      addEvent: (type, details, full) => this.addEvent(type, details, full),
      loadJobs: (options) => this.jobs.load(options),
      loadGlobalActivity: (options) => this.memory.loadGlobalActivity(options),
      reportError: (error) => toastFromError(error),
    });
    this.jobs = new ChatJobsCoordinator({
      queueEnabled: () => this.queueEnabled,
      jobFilter: () => this.jobFilterTarget,
      hasJobBadge: () => this.hasJobTarget,
      jobBadge: () => this.jobTarget,
      setCurrentJobID: (id) => this.execution.setCurrentJobID(id),
      renderComponentBundle: (bundle) => this.renderComponentBundle(bundle),
      addEvent: (type, details, full) => this.addEvent(type, details, full),
    });
    this.panels = new ChatPanelCoordinator({
      root: () => this.element,
      locale: () => getLocale(),
	  renderPanel: (bundle) => this.renderComponentBundle(bundle),
      loadPanelData: (panel) => this.loadPanelData(panel),
      pushRoute: (path) => this.recyclrController!.pushRoute(path),
      addEvent: (type, details, full) => this.addEvent(type, details, full),
      reportError: (error) => toastFromError(error),
    });
    this.wireMemoryAndSystem();
  }

  private wireMemoryAndSystem(): void {
    this.memory = new ChatMemoryCoordinator({
      queueEnabled: () => this.queueEnabled,
      hasMemoryList: () => this.hasMemoryListTarget,
      memoryKind: () => this.memoryKindTarget,
      memoryKindFilter: () => this.memoryKindFilterTarget,
      memoryTags: () => this.memoryTagsTarget,
      memoryContent: () => this.memoryContentTarget,
      renderComponentBundle: (bundle) => this.renderComponentBundle(bundle),
      loadTimeline: (options) => this.loadTimeline(options),
      addEvent: (type, details, full) => this.addEvent(type, details, full),
    });
    this.system = new ChatSystemCoordinator({
      queueEnabled: () => this.queueEnabled,
      setQueueEnabled: (enabled) => { this.queueEnabled = enabled; },
	  hasStatusOutput: () => this.hasStatusOutputTarget,
	  statusOutput: () => this.statusOutputTarget,
      hasHostBridgeStatus: () => this.hasHostBridgeStatusOutputTarget,
	  hostBridgeStatus: () => this.hostBridgeStatusOutputTarget,
      hasResearchStatus: () => this.hasResearchStatusOutputTarget,
	  researchStatus: () => this.researchStatusOutputTarget,
      hasMetrics: () => this.hasMetricsOutputTarget,
	  metrics: () => this.metricsOutputTarget,
      updateTransportLabel: () => this.channel.updateTransportLabel(),
	  renderComponentBundle: (bundle) => this.renderComponentBundle(bundle),
      addEvent: (type, details, full) => this.addEvent(type, details, full),
    });
  }

  private bindDocumentEvents(): void {
    this.memoryChangedHandler = () => { void this.memory.load(); };
    document.addEventListener("omni:memory-changed", this.memoryChangedHandler);
    this.networkSettingsHandler = (event) => {
      const detail = (event as CustomEvent<{ core_url?: string }>).detail;
      if (detail?.core_url) this.channel.setNetworkURL(detail.core_url);
    };
    document.addEventListener("omni:network-settings", this.networkSettingsHandler);
    this.projectOpenedHandler = (event) => {
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
    this.metricsGlanceHandler = () => {
      if (this.panels.isCurrent("metrics")) void this.system.loadMetrics();
    };
    document.addEventListener("omni:metrics-glance", this.metricsGlanceHandler);
    document.addEventListener("omni:job-progress", this.jobProgressHandler);
    document.addEventListener("omni:realtime-sync-required", this.realtimeSyncHandler);
  }

  private loadPanelData(panel: OmniPanel): void {
    let task: Promise<void> | null = null;
    if (panel === "chat" && this.channel.hasSelection()) {
      task = this.channel.loadTranscript(this.channel.selectedID());
    }
    if (panel === "jobs") task = this.jobs.load({ strict: true });
    if (panel === "memory") task = this.memory.load();
    if (panel === "metrics") task = this.system.loadMetrics({ strict: true });
    if (!task) return;
    void task.catch((error) => {
      this.addEvent("ui_panel_data_error", { panel, error: error instanceof Error ? error.message : String(error) });
      toastFromError(error);
    });
  }

  private async synchronizeRealtimeState(): Promise<void> {
    const tasks: Promise<unknown>[] = [this.memory.loadGlobalActivity({ quiet: true, strict: true })];
    if (this.execution.currentJobID() !== null) tasks.push(this.execution.refreshCurrent());
    if (this.channel.hasSelection()) tasks.push(this.channel.loadTranscript(this.channel.selectedID()));
    if (this.panels.isCurrent("jobs")) tasks.push(this.jobs.load({ quiet: true, strict: true }));
    if (this.panels.isCurrent("metrics")) tasks.push(this.system.loadMetrics({ strict: true }));
    await Promise.all(tasks);
  }
}
