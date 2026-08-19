import type { OmniPanel } from "../lib/panel_routing";
import { ChatRuntimeController } from "./chat_runtime_controller";

export default class ChatController extends ChatRuntimeController {
  async selectChannel(event: Event): Promise<void> {
    const wasBusy = this.busy;
    this.setBusy(true);
    try {
      await this.channel.select(event);
      await this.synchronizeSlashCommands(this.channel.selectedID());
    } finally {
      this.setBusy(wasBusy);
    }
  }

  async loadOlderChannelMessages(event: Event): Promise<void> {
    await this.channel.loadOlder(event);
  }

  selectNewChannelMode(): void {
    this.creation.synchronize();
		this.dataSources.setCreationMode(this.creation.selectedMode());
  }

  async submitChannel(prompt: string): Promise<void> {
    await this.channel.submit(prompt);
  }

  async showPanel(event: Event): Promise<void> {
    await this.panels.show(event);
  }

  async activatePanel(name: OmniPanel, options: { pushHistory?: boolean } = {}): Promise<void> {
    await this.panels.activate(name, options);
  }

  composerKeydown(event: KeyboardEvent): void {
    if (event.defaultPrevented) return;
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
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
    const prompt = this.inputTarget.value;
    if (!prompt.trim() || this.busy) return;
	this.slashPalette.dismiss();
	if (!this.channel.hasSelection()) {
		this.activityLabel = "Creating conversation…";
		this.setBusy(true);
		this.renderProgressActivity(this.activityLabel);
		try {
			const result = await this.channel.createAndSubmit(prompt);
			if (result === "creation_failed") {
				this.setBusy(false);
				return;
			}
			if (result === "roleplay_setup_required") {
				await this.synchronizeSlashCommands(this.channel.selectedID());
				this.setBusy(false);
				this.setStatus("roleplay setup required", "error");
				return;
			}
			if (this.inputTarget.value === prompt) this.inputTarget.value = "";
			await this.synchronizeSlashCommands(this.channel.selectedID());
			return;
		} catch (error) {
			this.reportSubmitFailure(error);
			return;
		}
	}
	if (!this.roleplay.isConfigured()) {
		this.setBusy(false);
		this.setStatus("roleplay setup required", "error");
		return;
	}

    this.activityLabel = "Sending…";
    this.setBusy(true);
    this.renderProgressActivity(this.activityLabel);

    try {
      await this.submitChannel(prompt);
      if (this.inputTarget.value === prompt) this.inputTarget.value = "";
      await this.synchronizeSlashCommands(this.channel.selectedID());
    } catch (error) {
      this.reportSubmitFailure(error);
    }
  }

  private reportSubmitFailure(error: unknown): void {
    const message = error instanceof Error ? error.message : String(error);
    this.addEvent("request_failed", { error: message });
    this.setBusy(false);
    this.setStatus("failed", "error");
  }

  async loadJobs(options: { quiet?: boolean; strict?: boolean } = {}): Promise<void> {
    await this.jobs.load(options);
  }

  async loadMoreJobs(event: Event): Promise<void> {
    await this.jobs.loadMore(event);
  }

  async selectJob(event: Event): Promise<void> {
    await this.jobs.select(event);
  }

  async interruptJob(event: Event): Promise<void> {
    await this.jobs.interrupt(event);
  }

  async replanJob(event: Event): Promise<void> {
    await this.jobs.replan(event);
  }

  async cancelJob(event: Event): Promise<void> {
    await this.jobs.cancel(event);
  }

  async loadMemoryCandidates(): Promise<void> {
    await this.memory.load();
  }

  async loadMoreMemory(event: Event): Promise<void> {
    await this.memory.loadMore(event);
  }

  async openTimelineJob(event: Event): Promise<void> {
    await this.activatePanel("jobs", { pushHistory: true });
    await this.jobs.select(event);
  }

  async deleteMemory(event: Event): Promise<void> {
    await this.memory.deleteMemory(event);
  }

  async deleteMemoryCandidate(event: Event): Promise<void> {
    await this.memory.deleteCandidate(event);
  }

  async loadGlobalActivity(options: { quiet?: boolean; strict?: boolean } = {}): Promise<void> {
    await this.memory.loadGlobalActivity(options);
  }

  async promoteMemory(event: Event): Promise<void> {
    await this.memory.promote(event);
  }

  async rejectMemory(event: Event): Promise<void> {
    await this.memory.reject(event);
  }

  async addMemory(event: Event): Promise<void> {
    await this.memory.add(event);
  }

  async loadStatus(): Promise<void> {
    await this.system.loadStatus();
  }

  async loadHostBridgeStatus(): Promise<void> {
    await this.system.loadHostBridgeStatus();
  }

  async loadResearchStatus(): Promise<void> {
    await this.system.loadResearchStatus();
  }

  async loadMetrics(options: { strict?: boolean } = {}): Promise<void> {
    await this.system.loadMetrics(options);
  }

  async migrateFresh(): Promise<void> {
    await this.system.migrateFresh();
  }

  async newThread(): Promise<void> {
    if (!this.panels.isCurrent("chat") || !this.hasMessagesTarget) {
      await this.activatePanel("chat", { pushHistory: true });
    }
    if (!this.channel.hasSelection()) {
      throw new Error("A server-authoritative channel must be selected before refreshing chat.");
    }
    await this.channel.loadTranscript(this.channel.selectedID());
  }

  clearTranscript(): void {
    throw new Error("A server-authoritative channel transcript cannot be cleared from the browser.");
  }

  async loadRoleplayPage(event: Event): Promise<void> {
	await this.roleplay.loadPage(event);
  }

  useRoleplayCommand(event: Event): void {
	this.roleplay.useCommand(event);
	this.slashPalette.dismiss();
  }

  async createRoleplayCharacter(event: Event): Promise<void> {
	await this.roleplay.createCharacter(event);
  }

  async saveRoleplayPersona(event: Event): Promise<void> {
	await this.roleplay.savePersona(event);
  }

  async createRoleplayScene(event: Event): Promise<void> {
	await this.roleplay.createScene(event);
  }

  async updateRoleplayScene(event: Event): Promise<void> {
	await this.roleplay.updateScene(event);
  }

  async saveRoleplaySceneDraftParticipant(event: Event): Promise<void> {
	await this.roleplay.saveSceneDraftParticipant(event);
  }

  async registerRoleplayMeter(event: Event): Promise<void> {
	await this.roleplay.registerMeter(event);
  }

  async setRoleplayMeter(event: Event): Promise<void> {
	await this.roleplay.setMeter(event);
  }

  async configureRoleplayResearch(event: Event): Promise<void> {
	await this.roleplay.configureResearch(event);
  }

  async registerRoleplayInteraction(event: Event): Promise<void> {
	await this.roleplay.registerInteraction(event);
  }

  async registerRoleplayItem(event: Event): Promise<void> {
	await this.roleplay.registerItem(event);
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
