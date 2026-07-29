import { readJSON } from "./api";
import { createUserChannel, fetchChannelMessages, fetchUserChannels, isUserChannel, sendChannelMessage } from "./channel_api";
import { t, tf } from "./i18n";
import type { ChatMessage, UserChannel } from "./types";

const SELECTED_CHANNEL_KEY = "omni.chat.selected-channel.v1";

export interface ChatChannelHost {
  hasNetworkURL(): boolean;
  networkURL(): HTMLElement;
  hasTransport(): boolean;
  transport(): HTMLElement;
  hasChannelSelect(): boolean;
  channelSelect(): HTMLSelectElement;
  queueEnabled(): boolean;
  setQueueEnabled(enabled: boolean): void;
  setStatus(text: string, mode: string): void;
  addEvent(type: string, details?: Record<string, unknown>, full?: unknown): void;
  addMessage(role: "assistant" | "error" | "system", content: string): void;
  replaceMessages(messages: ChatMessage[]): void;
  restorePipelineTranscript(): void;
  setActivityLabel(label: string): void;
  renderProgressActivity(label: string): void;
  setBusy(value: boolean): void;
}

export class ChatChannelCoordinator {
  private channels: UserChannel[] = [];
  private selectedChannelID = "";

  constructor(private readonly host: ChatChannelHost) {}

  selectedID(): string {
    return this.selectedChannelID;
  }

  isChannelMode(): boolean {
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
    const anchor = document.createElement("a");
    anchor.href = normalized;
    anchor.className = "text-cyan-200 hover:text-cyan-100";
    anchor.textContent = normalized;
    target.replaceChildren(anchor);
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
    if (this.isChannelMode()) {
      const channel = this.channels.find((item) => item.id === this.selectedChannelID);
      const label = channel?.name?.trim() || this.selectedChannelID;
      this.host.transport().textContent = `${t("transport.channel")} · ${label}`;
      return;
    }
    this.host.transport().textContent = this.host.queueEnabled() ? t("transport.queue") : t("transport.direct");
  }

  async loadChannels(): Promise<void> {
    if (!this.host.hasChannelSelect()) return;
    try {
      this.channels = (await fetchUserChannels()).filter(isUserChannel);
      this.renderOptions();
      const saved = localStorage.getItem(SELECTED_CHANNEL_KEY) || "";
      if (saved && this.channels.some((channel) => channel.id === saved)) {
        this.selectedChannelID = saved;
        this.host.channelSelect().value = saved;
        await this.loadTranscript(saved);
      } else {
        this.selectedChannelID = "";
        this.host.channelSelect().value = "";
      }
      this.updateTransportLabel();
    } catch (error) {
      this.host.channelSelect().disabled = true;
      this.host.channelSelect().replaceChildren(new Option(t("channel.unavailable"), ""));
      this.host.setStatus(t("channel.statusUnavailable"), "error");
      this.host.addEvent("channels_load_failed", { error: errorMessage(error) });
    }
  }

  renderOptions(): void {
    if (!this.host.hasChannelSelect()) return;
    const select = this.host.channelSelect();
    const options = [new Option(t("panel.chat.agentPipeline"), "")];
    for (const channel of this.channels) {
      const label = channel.name?.trim() || channel.id;
      const meta = channel.persona && channel.persona !== "assistant" ? ` (${channel.persona})` : "";
      const option = new Option(label + meta, channel.id, false, channel.id === this.selectedChannelID);
      options.push(option);
    }
    select.replaceChildren(...options);
    select.disabled = false;
  }

  async select(event: Event): Promise<void> {
    this.selectedChannelID = (event.currentTarget as HTMLSelectElement).value.trim();
    if (this.selectedChannelID) {
      localStorage.setItem(SELECTED_CHANNEL_KEY, this.selectedChannelID);
      await this.loadTranscript(this.selectedChannelID);
    } else {
      localStorage.removeItem(SELECTED_CHANNEL_KEY);
      this.host.restorePipelineTranscript();
    }
    this.updateTransportLabel();
  }

  async loadTranscript(channelID: string): Promise<void> {
    if (!channelID.trim()) throw new Error("Channel id is required to load a transcript.");
    const channel = this.channels.find((item) => item.id === channelID);
    try {
      const rows = await fetchChannelMessages(channelID);
      const messages: ChatMessage[] = rows.map((row) => ({
        role: normalizeMessageRole(row.role),
        content: row.content,
        at: requireChannelMessageTimestamp(row.created_at),
      }));
      if (messages.length === 0) {
        messages.push({
          role: "system",
          content: tf("channel.initial", { name: channel?.name || channelID }),
          at: new Date().toISOString(),
        });
      }
      this.host.replaceMessages(messages);
    } catch (error) {
      this.host.replaceMessages([{ role: "error", content: errorMessage(error), at: new Date().toISOString() }]);
      this.host.setStatus(t("channel.statusUnavailableOne"), "error");
      this.host.addEvent("channel_transcript_failed", { channel_id: channelID, error: errorMessage(error) });
    }
  }

  async create(event: Event): Promise<void> {
    event.preventDefault();
    const id = window.prompt(t("channel.idPrompt"), `chat-${Date.now()}`)?.trim();
    if (!id) return;
    const name = window.prompt(t("channel.namePrompt"), id)?.trim() || id;
    this.host.setStatus(t("channel.statusCreating"), "active");
    try {
      const channel = await createUserChannel({ id, name, tags: ["user-channel"] });
      if (!this.channels.some((item) => item.id === channel.id)) this.channels.unshift(channel);
      this.selectedChannelID = channel.id;
      localStorage.setItem(SELECTED_CHANNEL_KEY, channel.id);
      this.renderOptions();
      if (this.host.hasChannelSelect()) this.host.channelSelect().value = channel.id;
      await this.loadTranscript(channel.id);
      this.updateTransportLabel();
      this.host.setStatus(t("status.ready"), "ready");
    } catch (error) {
      this.host.setStatus(t("status.failed"), "error");
      this.host.addMessage("error", errorMessage(error));
    }
  }

  async submit(prompt: string): Promise<void> {
    if (!this.selectedChannelID) throw new Error("A channel must be selected before sending a channel message.");
    this.host.setActivityLabel(t("channel.thinking"));
    this.host.setStatus(t("status.thinking"), "active");
    this.host.renderProgressActivity(t("channel.thinking"));
    const payload = await sendChannelMessage(this.selectedChannelID, prompt);
    this.host.addEvent("channel_message", {
      channel_id: this.selectedChannelID,
      model: payload.model,
      latency_ms: payload.latency_ms,
    }, payload);
    this.host.addMessage("assistant", payload.output || t("channel.emptyResponse"));
    this.host.setStatus(t("status.ready"), "ready");
    this.host.setBusy(false);
  }
}

function normalizeMessageRole(role: string): ChatMessage["role"] {
  return role === "assistant" || role === "user" || role === "system" || role === "error" ? role : "assistant";
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function requireChannelMessageTimestamp(value: unknown): string {
  if (typeof value !== "string" || !value.trim()) {
    throw new Error("Channel message did not include created_at.");
  }
  return value;
}
