import { ChatChannelCoordinator } from "../lib/chat_channel_coordinator";
import { ChatChannelCreationCoordinator } from "../lib/chat_channel_creation_coordinator";
import { ChatDataSourceCoordinator } from "../lib/chat_data_source_coordinator";
import { ChatExecutionCoordinator } from "../lib/chat_execution_coordinator";
import { ChatJobsCoordinator } from "../lib/chat_jobs_coordinator";
import { ChatMemoryCoordinator } from "../lib/chat_memory_coordinator";
import { handleOllamaDownload } from "../lib/chat_ollama_download";
import { ChatPanelCoordinator } from "../lib/chat_panel_coordinator";
import { ChatRoleplayCoordinator } from "../lib/chat_roleplay_coordinator";
import { RoleplayCharacterEditorCoordinator } from "../lib/roleplay_character_editor_coordinator";
import { RoleplayWorkspaceCoordinator } from "../lib/roleplay_workspace_coordinator";
import { RoleplayWorkspaceDialogs } from "../lib/roleplay_workspace_dialogs";
import { ChatSlashPaletteCoordinator } from "../lib/chat_slash_palette_coordinator";
import { ChatSystemCoordinator } from "../lib/chat_system_coordinator";
import { toastFromError } from "../lib/feedback";
import { getLocale } from "../lib/i18n";
import { parsePanelFromLocation } from "../lib/panel_routing";
import type { RealtimeSyncDetail } from "../lib/realtime_sync";
import type RecyclrController from "./recyclr_controller";
import { ChatSynchronizationController } from "./chat_synchronization_controller";
export abstract class ChatRuntimeController extends ChatSynchronizationController {
  private memoryChangedHandler: (() => void) | null = null;
  private networkSettingsHandler: ((event: Event) => void) | null = null;
  private projectOpenedHandler: ((event: Event) => void) | null = null;
  private projectClosedHandler: (() => void) | null = null;
  private metricsGlanceHandler: (() => void) | null = null;
  private readonly ollamaDownloadHandler = (event: Event) => handleOllamaDownload(event, {
    roleplayIsCurrent: () => this.panels.isCurrent("roleplay"),
    hasSelectedChannel: () => this.channel.hasSelection(),
    setStatus: (message, tone) => this.setStatus(message, tone),
    refreshRoleplay: async () => { await Promise.all([this.roleplay.refresh(), this.roleplayCharacterEditor.refreshIfOpen()]); },
    addEvent: (type, details) => this.addEvent(type, details),
    reportError: (error) => this.reportRuntimeError(error),
  });
  private readonly jobProgressHandler = (event: Event) => this.execution.handleProgress(event);
  private readonly realtimeSyncHandler = (event: Event) => {
    const detail = (event as CustomEvent<RealtimeSyncDetail>).detail;
    if (!detail || typeof detail.waitUntil !== "function") {
      throw new Error("Realtime synchronization event is missing waitUntil().");
    }
    detail.waitUntil(this.synchronizeRealtimeState());
  };
  private readonly realtimeActivityHandler = (event: Event) => {
    const detail = (event as CustomEvent<Record<string, unknown>>).detail;
    if (!detail || typeof detail !== "object" || Array.isArray(detail)) {
      throw new Error("Realtime activity event is missing typed detail.");
    }
    this.pulseSystemActivity();
    if (detail.toastTone === "error") {
      const problem = typeof detail.toast === "string" ? detail.toast.trim() : "";
      if (!problem) throw new Error("Realtime error activity is missing a problem message.");
      this.reportSystemProblem(problem);
    }
  };
  private readonly realtimeStatusHandler = (event: Event) => {
    const detail = (event as CustomEvent<Record<string, unknown>>).detail;
    if (!detail || typeof detail !== "object" || Array.isArray(detail)) {
      throw new Error("Realtime status event is missing typed detail.");
    }
    const state = detail.state;
    if (state !== "connecting" && state !== "syncing" && state !== "live" &&
        state !== "reconnecting" && state !== "error") {
      throw new Error(`Realtime status has unregistered state ${JSON.stringify(state)}.`);
    }
    const message = typeof detail.message === "string" ? detail.message.trim() : "";
    if (!message) throw new Error("Realtime status is missing a nonblank message.");
    this.reportTransportActivity(state, message);
  };

  async connect(): Promise<void> {
    this.recyclrController = this.application.getControllerForElementAndIdentifier(
      this.element,
      "recyclr",
    ) as RecyclrController | null;
    if (!this.recyclrController) throw new Error("The page-scoped Recyclr controller is unavailable.");
    this.initializeViewState();
    this.wireCoordinators();
    this.bindDocumentEvents();
    await Promise.all([
      this.channel.detectTransport(),
      this.panels.activate(parsePanelFromLocation(), { pushHistory: false }),
    ]);
    void Promise.all([this.system.loadStatus(), this.memory.loadGlobalActivity()]).catch((error) => {
      this.addEvent("chat_startup_synchronization_failed", {
        error: error instanceof Error ? error.message : String(error),
      });
      this.reportRuntimeError(error);
    });
  }

  disconnect(): void {
    if (this.memoryChangedHandler) document.removeEventListener("omni:memory-changed", this.memoryChangedHandler);
    if (this.networkSettingsHandler) document.removeEventListener("omni:network-settings", this.networkSettingsHandler);
    if (this.projectOpenedHandler) document.removeEventListener("omni:project-opened", this.projectOpenedHandler);
    if (this.projectClosedHandler) document.removeEventListener("omni:project-closed", this.projectClosedHandler);
    if (this.metricsGlanceHandler) document.removeEventListener("omni:metrics-glance", this.metricsGlanceHandler);
    document.removeEventListener("omni:ollama-download", this.ollamaDownloadHandler);
    document.removeEventListener("omni:job-progress", this.jobProgressHandler);
    document.removeEventListener("omni:realtime-sync-required", this.realtimeSyncHandler);
    document.removeEventListener("omni:realtime-activity", this.realtimeActivityHandler);
    document.removeEventListener("omni:realtime-status", this.realtimeStatusHandler);
    this.execution.disconnect();
    this.disconnectSystemActivity();
  }

  private wireCoordinators(): void {
	this.slashPalette = new ChatSlashPaletteCoordinator({
		input: () => this.inputTarget,
		palette: () => this.slashPaletteTarget,
		options: () => this.slashOptionsTarget,
		renderComponentBundle: (bundle) => this.renderComponentBundle(bundle),
	});
	this.roleplay = new ChatRoleplayCoordinator({
		hasPanel: () => this.hasRoleplayPanelTarget,
		panel: () => this.roleplayPanelTarget,
		hasLoading: () => this.hasRoleplayLoadingTarget,
		loading: () => this.roleplayLoadingTarget,
		renderComponentBundle: async (bundle) => {
			await this.renderComponentBundle(bundle);
			this.roleplayWorkspaceDialogs.restoreSetupSection();
		},
		setComposerAvailable: (available) => this.setRoleplayComposerAvailable(available),
		setComposerText: (value) => this.setComposerText(value),
		focusComposer: () => this.focusComposer(),
		setStatus: (text, mode) => this.setStatus(text, mode),
		addEvent: (type, details) => this.addEvent(type, details),
			reportError: (error) => this.reportRuntimeError(error),
		refreshSlashCommands: () => this.slashPalette.refresh(),
	});
	this.roleplayWorkspace = new RoleplayWorkspaceCoordinator({
		hasLoading: () => this.hasRoleplayWorkspaceLoadingTarget,
		loading: () => this.roleplayWorkspaceLoadingTarget,
		renderComponentBundle: (bundle) => this.renderComponentBundle(bundle),
		selectedChannelID: () => this.channel.selectedID(),
		firstChannelID: () => this.channel.firstAvailableID(),
		selectChannelID: (id) => this.channel.selectID(id),
		createWorld: () => this.channel.createConversation(),
		refreshRoleplay: () => this.roleplay.refresh(),
		setStatus: (text, tone) => this.setStatus(text, tone),
		addEvent: (type, details) => this.addEvent(type, details),
		reportError: (error) => this.reportRuntimeError(error),
	});
	this.roleplayWorkspaceDialogs = new RoleplayWorkspaceDialogs({
		worldDialog: () => this.roleplayWorldDialogTarget,
		characterDialog: () => this.roleplayCharacterDialogTarget,
		setupDialog: () => this.roleplaySetupDialogTarget,
		characterEditorDialog: () => this.roleplayCharacterEditorDialogTarget,
	});
	this.roleplayCharacterEditor = new RoleplayCharacterEditorCoordinator({
		selectedChannelID: () => this.channel.selectedID(),
		renderComponentBundle: (bundle) => this.renderComponentBundle(bundle),
		refreshRoleplay: () => this.roleplay.refresh(),
		openDialog: () => this.roleplayWorkspaceDialogs.open("character-editor"),
		closeDialog: () => this.roleplayWorkspaceDialogs.close("character-editor"),
		closeDialogFromBackdrop: (event) => this.roleplayWorkspaceDialogs.closeFromBackdrop("character-editor", event),
		dialogIsOpen: () => this.roleplayCharacterEditorDialogTarget.open,
		setStatus: (text, tone) => this.setStatus(text, tone),
		addEvent: (type, details) => this.addEvent(type, details),
		reportError: (error) => this.reportRuntimeError(error),
	});
    this.creation = new ChatChannelCreationCoordinator({
      hasMode: () => this.hasNewChannelModeSelectTarget,
      mode: () => this.newChannelModeSelectTarget,
      hasRoleplayFields: () => this.hasNewChannelRoleplayFieldsTarget,
      roleplayFields: () => this.newChannelRoleplayFieldsTarget,
      hasWorldName: () => this.hasNewChannelRoleplayWorldNameTarget,
      worldName: () => this.newChannelRoleplayWorldNameTarget,
      hasViewpointName: () => this.hasNewChannelRoleplayViewpointNameTarget,
      viewpointName: () => this.newChannelRoleplayViewpointNameTarget,
    });
    this.dataSources = new ChatDataSourceCoordinator({
      hasSelect: () => this.hasNewChannelDataSourceSelectTarget,
      select: () => this.newChannelDataSourceSelectTarget,
      renderComponentBundle: (bundle) => this.renderComponentBundle(bundle),
      setStatus: (text, mode) => this.setStatus(text, mode),
      addEvent: (type, details) => this.addEvent(type, details),
    });
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
      newChannelDataSourceID: () => this.dataSources.selectedForCreation(),
      newChannelCreationContext: () => this.creation.parameters(),
      setActivityLabel: (label) => { this.activityLabel = label; },
      renderProgressActivity: (label) => this.renderProgressActivity(label),
      waitForJob: (id) => this.execution.waitForExistingJob(id),
			synchronizeRoleplay: (channelID, mode) => this.roleplay.activate(channelID, mode),
			roleplayConfigured: () => this.roleplay.isConfigured(),
			refreshRoleplay: () => this.roleplay.refresh(),
    });
    this.execution = new ChatExecutionCoordinator({
      currentPanel: () => this.panels.current(),
      hasJobBadge: () => this.hasJobTarget,
      jobBadge: () => this.jobTarget,
      setActivityLabel: (label) => { this.activityLabel = label; },
      setStatus: (text, mode) => this.setStatus(text, mode),
      renderProgressActivity: (label) => this.renderProgressActivity(label),
      renderJobState: (bundle) => this.renderJobState(bundle),
      addEvent: (type, details, full) => this.addEvent(type, details, full),
      loadJobs: (options) => this.jobs.load(options),
      loadGlobalActivity: (options) => this.memory.loadGlobalActivity(options),
      reportError: (error) => this.reportRuntimeError(error),
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
      reportError: (error) => this.reportRuntimeError(error),
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
    document.addEventListener("omni:ollama-download", this.ollamaDownloadHandler);
    document.addEventListener("omni:job-progress", this.jobProgressHandler);
    document.addEventListener("omni:realtime-sync-required", this.realtimeSyncHandler);
    document.addEventListener("omni:realtime-activity", this.realtimeActivityHandler);
    document.addEventListener("omni:realtime-status", this.realtimeStatusHandler);
  }

  private reportRuntimeError(error: unknown): void {
    const message = error instanceof Error ? error.message : String(error);
    this.reportSystemProblem(message);
    toastFromError(error);
  }
}
