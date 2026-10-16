import type { OmniPanel } from "../lib/panel_routing";
import type { ChatChannelTurnReceipt } from "../lib/chat_channel_turn";
import type { CodingMode } from "../lib/channel_api";
import { toastError } from "../lib/feedback";
import type { RoleplayTurnInput } from "../lib/roleplay_turn_input";
import { ChatRoleplayTurnController } from "./chat_roleplay_turn_controller";

export default class ChatController extends ChatRoleplayTurnController {
  static targets = ["codingMode", "agentQuestions", "agentQuestionText"];
  declare readonly codingModeTarget: HTMLSelectElement;
  declare readonly hasCodingModeTarget: boolean;
  declare readonly agentQuestionsTarget: HTMLElement;
  declare readonly hasAgentQuestionsTarget: boolean;
  declare readonly agentQuestionTextTarget: HTMLElement;
  private codingPreferenceBeforeActive?: CodingMode;
  private agentTurnWasActive = false;

  async connect(): Promise<void> {
    const ready = super.connect();
    this.channel?.onAgentTurnChange(() => this.renderCodingAgentState());
    await ready;
    this.renderCodingAgentState();
  }

  disconnect(): void {
    this.channel?.disconnectAgentTurns();
    super.disconnect();
  }


  async selectChannel(event: Event): Promise<void> {
    const wasBusy = this.busy;
    this.setBusy(true);
    try {
      await this.channel.select(event);
      await this.synchronizeSelectedSlashCommands();
      this.renderCodingAgentState();
    } finally {
      this.setBusy(wasBusy);
    }
  }

  async loadOlderChannelMessages(event: Event): Promise<void> { await this.channel.loadOlder(event); }

  async selectRoleplayWorld(event: Event): Promise<void> {
    const selected = await this.roleplayWorkspace.selectWorld(event);
    await this.synchronizeSelectedSlashCommands();
    if (!selected) return;
    this.roleplayWorkspaceDialogs.close("worlds");
  }

  async createRoleplayWorld(event: Event): Promise<void> {
    const created = await this.roleplayWorkspace.createWorld(event);
    await this.synchronizeSelectedSlashCommands();
    if (!created) return;
    this.roleplayWorkspaceDialogs.close("worlds");
  }

  async createRoleplayLibraryCharacter(event: Event): Promise<void> { await this.roleplayWorkspace.createCharacter(event); }

  async placeRoleplayCharacter(event: Event): Promise<void> {
    const placed = await this.roleplayWorkspace.placeCharacter(event);
    await this.synchronizeSelectedSlashCommands();
    if (!placed) return;
    this.roleplayWorkspaceDialogs.close("characters");
  }

  async loadMoreRoleplayWorlds(event: Event): Promise<void> { await this.roleplayWorkspace.loadMoreWorlds(event); }

  async loadMoreRoleplayCharacters(event: Event): Promise<void> { await this.roleplayWorkspace.loadMoreCharacters(event); }

  selectNewChannelMode(): void {
    this.creation.synchronize();
    this.dataSources.setCreationMode(this.creation.selectedMode());
  }

  async submitChannel(prompt: string, turn?: RoleplayTurnInput): Promise<ChatChannelTurnReceipt> {
    const mode = this.selectedCodingMode();
    return mode === "conversation" ? this.channel.submit(prompt, turn) : this.channel.submit(prompt, turn, mode);
  }

  async showPanel(event: Event): Promise<void> { await this.panels.show(event); }

  async activatePanel(name: OmniPanel, options: { pushHistory?: boolean } = {}): Promise<void> {
    await this.panels.activate(name, options);
  }

  composerKeydown(event: KeyboardEvent): void {
    if (event.defaultPrevented || event.isComposing) return;
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void this.submit(event);
    }
  }

  composerInput(): void {
    this.slashPalette.inputChanged();
  }

  slashCommandKeydown(event: KeyboardEvent): void {
    this.slashPalette.keydown(event);
  }

  chooseSlashCommand(event: Event): void {
    this.slashPalette.choose(event);
  }

  async submit(event: Event): Promise<void> {
    event.preventDefault();
    if (!this.hasInputTarget) {
      await this.activatePanel("chat", { pushHistory: true });
      if (!this.hasInputTarget) return;
    }
    const draft = this.inputTarget.value;
    if (this.busy) return;
    if (this.isTurnActive()) {
      this.setStatus("Wait for the current reply before sending another message.", "active");
      return;
    }
    if (this.channel.hasSelection() && !this.roleplay.isConfigured()) {
      this.setStatus("roleplay setup required", "error");
      return;
    }
    let prompt = draft;
    let roleplayTurn: RoleplayTurnInput | undefined;
    try {
      const submission = this.selectedRoleplaySubmission(draft);
      if (submission !== undefined) {
        prompt = submission.prompt;
        roleplayTurn = submission.turn;
      } else if (!prompt.trim()) {
        return;
      }
    } catch (error) {
      this.reportSubmitFailure(error);
      return;
    }
    this.slashPalette.dismiss();
    this.activityLabel = this.channel.hasSelection() ? "Sending…" : "Creating conversation…";
    this.setBusy(true);
    this.renderProgressActivity(this.activityLabel);

    try {
      const turn = await this.acceptSubmission(prompt, roleplayTurn);
      if (turn) {
        if (roleplayTurn === undefined) this.inputTarget.value = "";
        else this.clearAcceptedRoleplayDraft();
        this.trackAcceptedTurn(turn);
      }
    } catch (error) {
      this.reportSubmitFailure(error);
    }
  }

  private async acceptSubmission(
    prompt: string,
    roleplayTurn?: RoleplayTurnInput,
  ): Promise<ChatChannelTurnReceipt | null> {
    if (this.channel.hasSelection()) {
      return roleplayTurn === undefined
        ? this.submitChannel(prompt)
        : this.submitChannel(prompt, roleplayTurn);
    }
    const mode = this.selectedCodingMode();
    const result = mode === "conversation"
      ? await this.channel.createAndSubmit(prompt)
      : await this.channel.createAndSubmit(prompt, mode);
    if (result.kind === "roleplay_setup_required") {
      this.setBusy(false);
      this.setStatus("roleplay setup required", "error");
      void this.synchronizeSlashCommands(this.channel.selectedID());
      return null;
    }
    return result.turn;
  }

  private selectedCodingMode(): CodingMode {
    if (!this.hasCodingModeTarget) return "conversation";
    const mode = this.codingModeTarget.value;
    if (mode !== "conversation" && mode !== "agents") throw new Error("Choose a coding mode.");
    if (mode === "agents" && this.channel.hasSelection() && this.channel.selectedMode() === "roleplay") {
      throw new Error("Agents coding mode requires an assistant conversation.");
    }
    return mode;
  }

  private renderCodingAgentState(): void {
    if (!this.hasCodingModeTarget) return;
    const state = this.channel.agentTurnState();
    if (state) {
      if (this.codingPreferenceBeforeActive === undefined) this.codingPreferenceBeforeActive = this.selectedCodingMode();
      this.codingModeTarget.value = "agents";
      this.codingModeTarget.disabled = true;
      this.agentTurnWasActive = true;
      this.setTurnActive(state.status !== "waiting_input");
      if (state.status === "waiting_input") this.setStatus("Answer required", "ready");
      else this.renderProgressActivity("Working…");
    } else {
      this.codingModeTarget.disabled = false;
      if (this.codingPreferenceBeforeActive !== undefined) this.codingModeTarget.value = this.codingPreferenceBeforeActive;
      this.codingPreferenceBeforeActive = undefined;
      if (this.agentTurnWasActive) this.setTurnActive(false);
      this.agentTurnWasActive = false;
    }
    if (this.hasAgentQuestionsTarget) {
      this.agentQuestionsTarget.classList.toggle("hidden", state?.status !== "waiting_input");
      this.agentQuestionTextTarget.textContent = state?.question ?? "";
    }
  }

  private trackAcceptedTurn(turn: ChatChannelTurnReceipt): void {
    this.setTurnActive(true);
    this.setBusy(false);
    this.focusComposer();
    void this.synchronizeSlashCommands(this.channel.selectedID());
    void this.channel.reconcileTurn(turn)
      .catch((error) => this.reportTurnFailure(turn, error))
      .finally(() => {
        this.setTurnActive(false);
        this.renderCodingAgentState();
        if (this.hasInputTarget) this.focusComposer();
      });
  }

  private reportTurnFailure(turn: ChatChannelTurnReceipt, error: unknown): void {
    const message = error instanceof Error ? error.message : String(error);
    this.addEvent("channel_turn_failed", { channel_id: turn.channelID, job_id: turn.jobID, error: message });
    this.setStatus(message, "error");
    toastError(message);
  }

  private reportSubmitFailure(error: unknown): void {
    const message = error instanceof Error ? error.message : String(error);
    this.addEvent("request_failed", { error: message });
    this.setBusy(false);
    this.setStatus(message, "error");
    toastError(message);
  }

  async loadJobs(options: { quiet?: boolean; strict?: boolean } = {}): Promise<void> { await this.jobs.load(options); }

  async loadMoreJobs(event: Event): Promise<void> { await this.jobs.loadMore(event); }

  async selectJob(event: Event): Promise<void> { await this.jobs.select(event); }

  async interruptJob(event: Event): Promise<void> { await this.jobs.interrupt(event); }

  async replanJob(event: Event): Promise<void> { await this.jobs.replan(event); }

  async cancelJob(event: Event): Promise<void> { await this.jobs.cancel(event); }

  async loadMemoryCandidates(): Promise<void> { await this.memory.load(); }

  async loadMoreMemory(event: Event): Promise<void> { await this.memory.loadMore(event); }

  async openTimelineJob(event: Event): Promise<void> {
    await this.activatePanel("jobs", { pushHistory: true });
    await this.jobs.select(event);
  }

  async deleteMemory(event: Event): Promise<void> { await this.memory.deleteMemory(event); }

  async deleteMemoryCandidate(event: Event): Promise<void> { await this.memory.deleteCandidate(event); }

  async loadGlobalActivity(options: { quiet?: boolean; strict?: boolean } = {}): Promise<void> { await this.memory.loadGlobalActivity(options); }

  async promoteMemory(event: Event): Promise<void> { await this.memory.promote(event); }

  async rejectMemory(event: Event): Promise<void> { await this.memory.reject(event); }

  async addMemory(event: Event): Promise<void> { await this.memory.add(event); }

  async loadStatus(): Promise<void> { await this.system.loadStatus(); }

  async loadHostBridgeStatus(): Promise<void> { await this.system.loadHostBridgeStatus(); }

  async loadMetrics(options: { strict?: boolean } = {}): Promise<void> { await this.system.loadMetrics(options); }

  async newThread(): Promise<void> {
    if (this.busy) return;
    if (!this.panels.isCurrent("chat") || !this.hasMessagesTarget) {
      await this.activatePanel("chat", { pushHistory: true });
    }
    this.setBusy(true);
    try {
      await this.channel.beginNewConversation();
      await this.synchronizeSlashCommands("");
      this.focusComposer();
    } finally {
      this.setBusy(false);
    }
  }

  async loadRoleplayPage(event: Event): Promise<void> { await this.roleplay.loadPage(event); }

  useRoleplayCommand(event: Event): void {
    this.roleplay.useCommand(event);
    this.slashPalette.dismiss();
  }

  async createRoleplayScene(event: Event): Promise<void> { await this.roleplay.createScene(event); }

  async updateRoleplayScene(event: Event): Promise<void> { await this.roleplay.updateScene(event); }

  async saveRoleplaySceneDraftParticipant(event: Event): Promise<void> { await this.roleplay.saveSceneDraftParticipant(event); }

  async registerRoleplayMeter(event: Event): Promise<void> { await this.roleplay.registerMeter(event); }

  async setRoleplayMeter(event: Event): Promise<void> { await this.roleplay.setMeter(event); }

  async registerRoleplayInteraction(event: Event): Promise<void> { await this.roleplay.registerInteraction(event); }

  async registerRoleplayItem(event: Event): Promise<void> { await this.roleplay.registerItem(event); }

  private async synchronizeSelectedSlashCommands(): Promise<void> {
    if (!this.roleplay.isConfigured()) {
      await this.slashPalette.activate("");
      return;
    }
    await this.synchronizeSlashCommands(this.channel.selectedID());
  }

  private async synchronizeSlashCommands(channelID: string): Promise<void> {
    try {
      await this.slashPalette.activate(channelID);
    } catch (error) {
      this.setStatus("command hints unavailable", "error");
      this.addEvent("slash_commands_refresh_failed", {
        channel_id: channelID,
        error: error instanceof Error ? error.message : String(error),
      });
    }
  }
}
