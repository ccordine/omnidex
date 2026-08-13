import type { OmniPanel } from "../lib/panel_routing";
import { ChatRuntimeController } from "./chat_runtime_controller";

export default class ChatController extends ChatRuntimeController {
  async selectChannel(event: Event): Promise<void> {
    await this.channel.select(event);
  }

  async createChannel(event: Event): Promise<void> {
    await this.channel.create(event);
  }

  async loadOlderChannelMessages(event: Event): Promise<void> {
    await this.channel.loadOlder(event);
  }

  async loadMoreChannels(event: Event): Promise<void> {
    await this.channel.loadMoreChannels(event);
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
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault();
      void this.submit(event);
    }
  }

  async submit(event: Event): Promise<void> {
    event.preventDefault();
    if (!this.hasInputTarget) {
      await this.activatePanel("chat", { pushHistory: true });
      if (!this.hasInputTarget) return;
    }
    const prompt = this.inputTarget.value;
    if (!prompt.trim() || this.busy) return;

    this.activityLabel = "Sending…";
    this.setBusy(true);
    this.renderProgressActivity(this.activityLabel);

    try {
      await this.submitChannel(prompt);
      if (this.inputTarget.value === prompt) this.inputTarget.value = "";
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      this.addEvent("request_failed", { error: message });
      this.setBusy(false);
      this.setStatus("failed", "error");
    }
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
}
