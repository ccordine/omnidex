import { readJSON } from "./api";
import { fetchChannelTranscript, sendChannelMessage } from "./channel_api";
import {
  ChatChannelCreationFlow,
  type ChannelIdentityFactory,
  type ChatChannelCreationFlowHost,
} from "./chat_channel_creation_flow";
import { ChatChannelCreationTransition } from "./chat_channel_creation_transition";
import { ChatChannelTransitionGate } from "./chat_channel_transition_gate";
import { fetchChannelOptionsPage } from "./chat_component_api";
import { t } from "./i18n";
import type { StatusTone } from "./types";

const MAX_AUTOMATIC_CHANNEL_PAGES = 100;
const CANONICAL_CHANNEL_ID = /^[a-z0-9][a-z0-9_.:-]{0,95}$/;
export const NEW_CONVERSATION_OPTION_VALUE = "__omnidex_new_conversation__";
export interface ChatChannelHost extends ChatChannelCreationFlowHost {
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
  setActivityLabel(label: string): void;
  renderProgressActivity(label: string): void;
  setBusy(value: boolean): void;
  waitForJob(id: number): Promise<void>;
  synchronizeRoleplay(channelID: string, mode: "assistant" | "roleplay"): Promise<void>;
  roleplayConfigured(): boolean;
  refreshRoleplay(): Promise<void>;
}

export type NeutralChannelSubmitResult = "submitted" | "creation_failed" | "roleplay_setup_required";

export class ChatChannelCoordinator {
  private selectedChannelID = "";
  private readonly transitions: ChatChannelTransitionGate;
  private readonly creation: ChatChannelCreationTransition;

  constructor(
    private readonly host: ChatChannelHost,
    identityFactory?: ChannelIdentityFactory,
  ) {
    this.transitions = new ChatChannelTransitionGate(host);
    this.creation = new ChatChannelCreationTransition({
      selectedID: () => this.selectedChannelID,
      selectedMode: (id) => this.selectedChannelMode(id),
      restoreSelection: (id) => this.restoreSelection(id),
      clearSelection: () => { this.selectedChannelID = ""; this.restoreSelection(""); },
      commitSelection: (id) => { this.selectedChannelID = id; this.host.channelSelect().value = id; this.updateTransportLabel(); },
      reloadOptions: () => this.loadAllChannelOptions(),
      hasOption: (id) => this.selectedOption(id) !== null,
      loadTranscript: (id) => this.loadTranscript(id),
      synchronizeRoleplay: (id, mode) => this.host.synchronizeRoleplay(id, mode),
      setStatus: (text, tone) => this.host.setStatus(text, tone),
      addEvent: (type, details) => this.host.addEvent(type, details),
    }, new ChatChannelCreationFlow(host, identityFactory));
  }
  selectedID(): string { return this.selectedChannelID; }

  hasSelection(): boolean { return this.selectedChannelID.length > 0; }

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

  async loadChannels(): Promise<void> {
    if (!this.host.hasChannelSelect()) return;
    try {
      await this.loadAllChannelOptions();
      this.selectedChannelID = "";
      this.host.channelSelect().value = "";
      this.host.channelSelect().disabled = false;
      await this.host.synchronizeRoleplay("", "assistant");
      this.updateTransportLabel();
    } catch (error) {
      this.host.channelSelect().disabled = true;
      this.host.setStatus(t("channel.statusUnavailable"), "error");
      this.host.addEvent("channels_load_failed", { error: errorMessage(error) });
    }
  }

  async select(event: Event): Promise<void> {
    const target = event.currentTarget;
    if (!(target instanceof HTMLSelectElement)) throw new Error("Channel selection requires the canonical select control.");
    const selectedID = target.value;
    await this.transitions.run(async () => {
      if (selectedID === NEW_CONVERSATION_OPTION_VALUE) {
        await this.creation.create();
        return;
      }
      await this.selectLocked(selectedID);
    });
  }

  private async selectLocked(selectedID: string): Promise<void> {
    if (!selectedID) {
      this.restoreSelection(this.selectedChannelID);
      throw new Error("A server-authoritative channel selection is required.");
    }
    if (!CANONICAL_CHANNEL_ID.test(selectedID) || !this.selectedOption(selectedID)) {
      this.restoreSelection(this.selectedChannelID);
      throw new Error(`Selected channel ${JSON.stringify(selectedID)} is absent from server markup.`);
    }
    const previousID = this.selectedChannelID;
    const previousMode = previousID ? this.selectedChannelMode(previousID) : "assistant";
    try {
      await this.loadTranscript(selectedID);
      await this.host.synchronizeRoleplay(selectedID, this.selectedChannelMode(selectedID));
    } catch (error) {
      this.restoreSelection(this.selectedChannelID);
      try {
        await this.host.synchronizeRoleplay(previousID, previousMode);
      } catch (rollbackError) {
        this.host.addEvent("roleplay_selection_rollback_failed", { error: errorMessage(rollbackError) });
      }
      throw error;
    }
    this.selectedChannelID = selectedID;
    this.creation.acceptSelection(selectedID);
    this.host.channelSelect().value = selectedID;
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

  async createAndSubmit(prompt: string): Promise<NeutralChannelSubmitResult> {
    return this.transitions.run(async () => {
      if (this.selectedChannelID) throw new Error("Neutral channel creation requires neutral channel authority.");
      if (!await this.creation.create()) return "creation_failed";
      if (!this.host.roleplayConfigured()) return "roleplay_setup_required";
      await this.submitLocked(prompt);
      return "submitted";
    });
  }

  async submit(prompt: string): Promise<void> {
    await this.transitions.run(() => this.submitLocked(prompt));
  }

  private async submitLocked(prompt: string): Promise<void> {
    const channelID = this.selectedChannelID;
    if (!channelID) throw new Error("A channel must be selected before sending a channel message.");
    this.host.setActivityLabel(t("channel.working"));
    this.host.setStatus(t("status.working"), "active");
    this.host.renderProgressActivity(t("channel.working"));
    const payload = await sendChannelMessage(channelID, prompt);
    const jobID = requirePositiveJobID(payload.job.id);
    this.host.addEvent("channel_message", {
      channel_id: channelID,
      message_id: payload.user_message.id,
      job_id: jobID,
    }, payload);
    await this.loadTranscript(channelID, payload.user_message.id);
    await this.host.waitForJob(jobID);
    await this.loadTranscript(channelID);
    await this.host.refreshRoleplay();
    this.host.setStatus(t("status.ready"), "ready");
    this.host.setBusy(false);
  }

  private selectedOption(id: string): HTMLOptionElement | null {
    if (!id || !this.host.hasChannelSelect()) return null;
    return [...this.host.channelSelect().options].find((option) => option.value === id) ?? null;
  }

  private restoreSelection(id: string): void {
    if (!this.host.hasChannelSelect()) return;
    this.host.channelSelect().value = id && this.selectedOption(id) ? id : "";
  }

  private selectedChannelMode(id: string): "assistant" | "roleplay" {
    const option = this.selectedOption(id);
    if (!option || (option.dataset.channelMode !== "assistant" && option.dataset.channelMode !== "roleplay")) {
      throw new Error(`Selected channel ${JSON.stringify(id)} has no exact server-owned mode.`);
    }
    return option.dataset.channelMode;
  }

  private async loadAllChannelOptions(): Promise<void> {
    let offset = 0;
    const bundles: string[] = [];
    for (let pageCount = 0; pageCount < MAX_AUTOMATIC_CHANNEL_PAGES; pageCount += 1) {
      const page = await fetchChannelOptionsPage(offset);
      bundles.push(page.html.bundle);
      if (page.next_offset === undefined) {
        for (const bundle of bundles) await this.host.renderComponentBundle(bundle);
        this.validateChannelOptions();
        return;
      }
      if (page.next_offset <= offset) {
        throw new Error("The server channel page cursor did not advance.");
      }
      offset = page.next_offset;
    }
    throw new Error(`Channel pagination exceeded ${MAX_AUTOMATIC_CHANNEL_PAGES} server pages.`);
  }

  private validateChannelOptions(): void {
    const options = [...this.host.channelSelect().options];
    const neutral = options.filter((option) => option.value === "");
    const create = options.filter((option) => option.value === NEW_CONVERSATION_OPTION_VALUE);
    if (neutral.length !== 1 || !neutral[0].disabled || create.length !== 1 || CANONICAL_CHANNEL_ID.test(NEW_CONVERSATION_OPTION_VALUE)) {
      throw new Error("Channel options lack the exact neutral and new-conversation controls.");
    }
    const firstActionable = options.find((option) => !option.disabled);
    if (firstActionable !== create[0]) {
      throw new Error("New conversation must be the first actionable channel option.");
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
