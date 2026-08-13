import { readJSON } from "./api";
import { createUserChannel, fetchChannelTranscript, sendChannelMessage } from "./channel_api";
import { fetchChannelOptionsPage } from "./chat_component_api";
import { t } from "./i18n";
import type { StatusTone } from "./types";

const SELECTED_CHANNEL_KEY = "omni.chat.selected-channel.v1";

export interface ChatChannelHost {
  hasNetworkURL(): boolean;
  networkURL(): HTMLAnchorElement;
  hasTransport(): boolean;
  transport(): HTMLElement;
  hasChannelSelect(): boolean;
  channelSelect(): HTMLSelectElement;
  queueEnabled(): boolean;
  setQueueEnabled(enabled: boolean): void;
  setStatus(text: string, mode: StatusTone): void;
  addEvent(type: string, details?: Record<string, unknown>, full?: unknown): void;
  renderComponentBundle(bundle: string): Promise<void>;
  renderTranscriptBundle(bundle: string, preserveScroll: boolean): Promise<void>;
  workspaceRoot(): string | null;
  setActivityLabel(label: string): void;
  renderProgressActivity(label: string): void;
  setBusy(value: boolean): void;
  waitForJob(id: number): Promise<void>;
}

export class ChatChannelCoordinator {
  private selectedChannelID = "";

  constructor(private readonly host: ChatChannelHost) {}

  selectedID(): string {
    return this.selectedChannelID;
  }

  hasSelection(): boolean {
    return this.selectedChannelID.length > 0;
  }

  setNetworkURL(url: string): void {
    if (!this.host.hasNetworkURL()) return;
    const target = this.host.networkURL();
    const normalized = url.trim();
    if (!normalized) {
      target.textContent = t("network.notSet");
      return;
    }
    target.href = normalized;
    target.textContent = normalized;
  }

  async detectTransport(): Promise<void> {
    try {
      const health = await readJSON(await fetch("/healthz"));
      this.host.setQueueEnabled(Boolean(health.queue_enabled));
      this.updateTransportLabel();
      if (health.core_url) this.setNetworkURL(String(health.core_url));
      this.host.setStatus(t("status.ready"), "ready");
      this.host.addEvent("health", health);
    } catch (error) {
      this.host.setQueueEnabled(false);
      if (this.host.hasTransport()) this.host.transport().textContent = t("status.offline");
      this.host.setStatus(t("status.offline"), "error");
      this.host.addEvent("health_failed", { error: errorMessage(error) });
    }
  }

  updateTransportLabel(): void {
    if (!this.host.hasTransport()) return;
    if (this.hasSelection()) {
      const option = this.selectedOption(this.selectedChannelID);
      if (!option) throw new Error(`Selected channel ${JSON.stringify(this.selectedChannelID)} is absent from server markup.`);
      this.host.transport().textContent = `${t("transport.channel")} · ${option.textContent ?? option.value}`;
      return;
    }
    this.host.transport().textContent = this.host.queueEnabled()
      ? t("panel.chat.createChannel")
      : t("status.offline");
  }

  async loadChannels(offset = 0): Promise<void> {
    if (!this.host.hasChannelSelect()) return;
    try {
      const page = await fetchChannelOptionsPage(offset);
      await this.host.renderComponentBundle(page.html.bundle);
      if (offset > 0) return;
      const saved = this.selectedChannelID || localStorage.getItem(SELECTED_CHANNEL_KEY) || "";
      const selected = this.selectedOption(saved)?.value ?? page.default_channel_id ?? "";
      if (selected) {
        await this.loadTranscript(selected);
        this.selectedChannelID = selected;
        localStorage.setItem(SELECTED_CHANNEL_KEY, selected);
        this.host.channelSelect().value = selected;
      } else {
        this.selectedChannelID = "";
        localStorage.removeItem(SELECTED_CHANNEL_KEY);
        this.host.channelSelect().value = "";
      }
      this.host.channelSelect().disabled = !selected;
      this.updateTransportLabel();
    } catch (error) {
      this.host.channelSelect().disabled = true;
      this.host.setStatus(t("channel.statusUnavailable"), "error");
      this.host.addEvent("channels_load_failed", { error: errorMessage(error) });
    }
  }

  async select(event: Event): Promise<void> {
    const selectedID = (event.currentTarget as HTMLSelectElement).value;
    if (!selectedID) {
      this.host.channelSelect().value = this.selectedChannelID;
      throw new Error("A server-authoritative channel selection is required.");
    }
    if (!this.selectedOption(selectedID)) {
      throw new Error(`Selected channel ${JSON.stringify(selectedID)} is absent from server markup.`);
    }
    try {
      await this.loadTranscript(selectedID);
    } catch (error) {
      this.host.channelSelect().value = this.selectedChannelID;
      throw error;
    }
    this.selectedChannelID = selectedID;
    localStorage.setItem(SELECTED_CHANNEL_KEY, selectedID);
    this.updateTransportLabel();
  }

  async loadTranscript(channelID: string, requiredMessageID?: number, beforeID?: number): Promise<void> {
    if (!/^[a-z0-9][a-z0-9_.:-]{0,95}$/.test(channelID)) {
      throw new Error("A canonical channel id is required to load a transcript.");
    }
    try {
      const page = await fetchChannelTranscript(channelID, { beforeID, requiredMessageID });
      await this.host.renderTranscriptBundle(page.html.bundle, beforeID !== undefined);
    } catch (error) {
      this.host.setStatus(t("channel.statusUnavailableOne"), "error");
      this.host.addEvent("channel_transcript_failed", { channel_id: channelID, error: errorMessage(error) });
      throw error;
    }
  }

  async loadOlder(event: Event): Promise<void> {
    const button = event.currentTarget as HTMLButtonElement;
    const beforeID = Number(button.dataset.beforeId ?? "");
    if (!Number.isSafeInteger(beforeID) || beforeID < 1) {
      throw new Error("The server-rendered transcript cursor is invalid.");
    }
    if (!this.selectedChannelID) throw new Error("A channel must be selected before loading transcript history.");
    button.disabled = true;
    button.setAttribute("aria-busy", "true");
    const label = button.textContent;
    button.textContent = "Loading older messages…";
    try {
      await this.loadTranscript(this.selectedChannelID, undefined, beforeID);
    } catch (error) {
      button.disabled = false;
      button.setAttribute("aria-busy", "false");
      button.textContent = label;
      throw error;
    }
  }

  async loadMoreChannels(event: Event): Promise<void> {
    const button = requirePaginationButton(event, "channels");
    await this.withPaginationFeedback(button, () => this.loadChannels(Number(button.dataset.nextOffset)));
  }

  async create(event: Event): Promise<void> {
    event.preventDefault();
    const workspaceRoot = this.host.workspaceRoot();
    if (!workspaceRoot) {
      throw new Error("Open a project before creating a workspace-bound channel.");
    }
    const id = window.prompt(t("channel.idPrompt"), `chat-${Date.now()}`);
    if (id === null) return;
    const name = window.prompt(t("channel.namePrompt"), id);
    if (name === null) return;
    this.host.setStatus(t("channel.statusCreating"), "active");
    try {
      const channel = await createUserChannel({
        id, name, tags: ["user-channel"], workspace_root: workspaceRoot,
      });
      this.selectedChannelID = channel.id;
      await this.loadChannels();
      if (this.selectedChannelID !== channel.id) {
        throw new Error("The created channel is absent from the authoritative first page.");
      }
      this.host.setStatus(t("status.ready"), "ready");
    } catch (error) {
      this.host.setStatus(t("status.failed"), "error");
      this.host.addEvent("channel_create_failed", { error: errorMessage(error) });
    }
  }

  async submit(prompt: string): Promise<void> {
    if (!this.selectedChannelID) throw new Error("A channel must be selected before sending a channel message.");
    this.host.setActivityLabel(t("channel.working"));
    this.host.setStatus(t("status.working"), "active");
    this.host.renderProgressActivity(t("channel.working"));
    const payload = await sendChannelMessage(this.selectedChannelID, prompt);
    const jobID = requirePositiveJobID(payload.job.id);
    this.host.addEvent("channel_message", {
      channel_id: this.selectedChannelID,
      message_id: payload.user_message.id,
      job_id: jobID,
    }, payload);
    await this.loadTranscript(this.selectedChannelID, payload.user_message.id);
    await this.host.waitForJob(jobID);
    await this.loadTranscript(this.selectedChannelID);
    this.host.setStatus(t("status.ready"), "ready");
    this.host.setBusy(false);
  }

  private selectedOption(id: string): HTMLOptionElement | null {
    if (!id || !this.host.hasChannelSelect()) return null;
    return [...this.host.channelSelect().options].find((option) => option.value === id) ?? null;
  }

  private async withPaginationFeedback(button: HTMLButtonElement, operation: () => Promise<void>): Promise<void> {
    const label = button.textContent;
    button.disabled = true;
    button.setAttribute("aria-busy", "true");
    button.textContent = "Loading channels…";
    try {
      await operation();
    } catch (error) {
      button.disabled = false;
      button.setAttribute("aria-busy", "false");
      button.textContent = label;
      throw error;
    }
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function requirePositiveJobID(value: number | string): number {
  const jobID = typeof value === "number" ? value : Number.NaN;
  if (!Number.isSafeInteger(jobID) || jobID < 1) {
    throw new Error("Channel turn did not return its authoritative job identity.");
  }
  return jobID;
}

function requirePaginationButton(event: Event, section: string): HTMLButtonElement {
  const button = event.currentTarget as HTMLButtonElement;
  const offset = Number(button.dataset.nextOffset ?? "");
  if (button.dataset.pageSection !== section || !Number.isSafeInteger(offset) || offset < 1) {
    throw new Error(`The server-rendered ${section} page cursor is invalid.`);
  }
  return button;
}
