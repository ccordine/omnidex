import { readJSON } from "./api";
import { fetchChannelTranscript } from "./channel_api";
import {
  ChatChannelCreationFlow,
  type ChannelIdentityFactory,
  type ChatChannelCreationFlowHost,
} from "./chat_channel_creation_flow";
import { ChatChannelCreationTransition } from "./chat_channel_creation_transition";
import { ChatChannelTransitionGate } from "./chat_channel_transition_gate";
import { fetchNeutralChatTranscript } from "./chat_component_api";
import { ChatChannelOptions } from "./chat_channel_options";
import {
  ChatChannelTurnCoordinator,
  type ChatChannelTurnReceipt,
} from "./chat_channel_turn";
import { t } from "./i18n";
import type { StatusTone } from "./types";
import type { RoleplayTurnInput } from "./roleplay_turn_input";

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
  waitForJob(id: number): Promise<void>;
  synchronizeRoleplay(channelID: string, mode: "assistant" | "roleplay"): Promise<void>;
  roleplayConfigured(): boolean;
  refreshRoleplay(): Promise<void>;
}

export type NeutralChannelSubmitResult =
  | { kind: "submitted"; turn: ChatChannelTurnReceipt }
  | { kind: "creation_failed" }
  | { kind: "roleplay_setup_required" };

export class ChatChannelCoordinator {
  private selectedChannelID = "";
  private loadedMode: "assistant" | "roleplay" | undefined;
  private readonly transitions: ChatChannelTransitionGate;
  private readonly creation: ChatChannelCreationTransition;
  private readonly options: ChatChannelOptions;
  private readonly turns: ChatChannelTurnCoordinator;

  constructor(
    private readonly host: ChatChannelHost,
    identityFactory?: ChannelIdentityFactory,
  ) {
    this.transitions = new ChatChannelTransitionGate(host);
    this.options = new ChatChannelOptions({
      channelSelect: () => this.host.channelSelect(),
      renderComponentBundle: (bundle) => this.host.renderComponentBundle(bundle),
    });
    this.creation = new ChatChannelCreationTransition({
      selectedID: () => this.selectedChannelID,
      selectedMode: (id) => this.selectedChannelMode(id),
      restoreSelection: (id) => this.restoreSelection(id),
      clearSelection: () => { this.selectedChannelID = ""; this.restoreSelection(""); },
      commitSelection: (id) => { this.selectedChannelID = id; this.host.channelSelect().value = id; this.updateTransportLabel(); },
      reloadOptions: () => this.options.loadAll(this.loadedMode),
      hasOption: (id) => this.selectedOption(id) !== null,
      loadTranscript: (id) => this.loadTranscript(id),
      synchronizeRoleplay: (id, mode) => this.host.synchronizeRoleplay(id, mode),
      setStatus: (text, tone) => this.host.setStatus(text, tone),
      addEvent: (type, details) => this.host.addEvent(type, details),
    }, new ChatChannelCreationFlow(host, identityFactory));
    this.turns = new ChatChannelTurnCoordinator({
      setStatus: (text, mode) => this.host.setStatus(text, mode),
      setActivityLabel: (label) => this.host.setActivityLabel(label),
      renderProgressActivity: (label) => this.host.renderProgressActivity(label),
      addEvent: (type, details, full) => this.host.addEvent(type, details, full),
      loadTranscript: (channelID, requiredMessageID) => this.loadTranscript(channelID, requiredMessageID),
      waitForJob: (jobID) => this.host.waitForJob(jobID),
      isSelected: (channelID) => this.selectedChannelID === channelID,
      refreshRoleplay: () => this.host.refreshRoleplay(),
    });
  }
  selectedID(): string { return this.selectedChannelID; }

  selectedMode(): "assistant" | "roleplay" {
    if (!this.selectedChannelID) throw new Error("A selected channel is required to resolve its mode.");
    return this.selectedChannelMode(this.selectedChannelID);
  }

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

  async loadChannels(mode?: "assistant" | "roleplay"): Promise<void> {
    if (!this.host.hasChannelSelect()) return;
    const previousID = this.selectedChannelID;
    const scopeChanged = this.loadedMode !== mode;
    try {
      await this.options.loadAll(mode);
      this.loadedMode = mode;
      this.host.channelSelect().disabled = false;
      if (previousID && !scopeChanged) {
        if (!this.options.isCanonicalID(previousID) || !this.selectedOption(previousID)) {
          throw new Error(`Selected channel ${JSON.stringify(previousID)} is absent from refreshed server markup.`);
        }
        this.host.channelSelect().value = previousID;
        await this.loadTranscript(previousID);
        await this.host.synchronizeRoleplay(previousID, this.selectedChannelMode(previousID));
      } else {
		this.selectedChannelID = "";
        this.host.channelSelect().value = "";
        await this.host.synchronizeRoleplay("", "assistant");
      }
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
    await this.transitions.run(() => this.selectLocked(selectedID));
  }

  async selectID(id: string): Promise<void> {
    await this.transitions.run(() => this.selectLocked(id));
  }

  firstAvailableID(): string {
    if (!this.host.hasChannelSelect()) return "";
    return [...this.host.channelSelect().options].find((option) => option.value !== "")?.value ?? "";
  }

  async beginNewConversation(): Promise<void> {
    await this.transitions.run(() => this.beginNewConversationLocked());
  }

  private async beginNewConversationLocked(): Promise<void> {
    const previousID = this.selectedChannelID;
    const previousMode = previousID ? this.selectedChannelMode(previousID) : "assistant";
    this.restoreSelection(previousID);
    this.host.setStatus("Starting a new conversation…", "active");
    try {
      const page = await fetchNeutralChatTranscript();
      await this.host.synchronizeRoleplay("", "assistant");
      await this.host.renderTranscriptBundle(page.html.bundle, false);
    } catch (error) {
      this.restoreSelection(previousID);
      try {
        await this.host.synchronizeRoleplay(previousID, previousMode);
      } catch (rollbackError) {
        this.host.addEvent("channel_neutral_rollback_failed", { error: errorMessage(rollbackError) });
      }
      this.host.setStatus(errorMessage(error), "error");
      this.host.addEvent("channel_neutral_failed", { error: errorMessage(error) });
      throw error;
    }
    this.selectedChannelID = "";
    this.restoreSelection("");
    this.updateTransportLabel();
    this.host.setStatus(t("status.ready"), "ready");
    this.host.addEvent("channel_neutral", {});
  }

  private async selectLocked(selectedID: string): Promise<void> {
    if (!selectedID) {
      this.restoreSelection(this.selectedChannelID);
      throw new Error("A server-authoritative channel selection is required.");
    }
    if (!this.options.isCanonicalID(selectedID) || !this.selectedOption(selectedID)) {
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
      if (!await this.creation.create()) return { kind: "creation_failed" };
      if (!this.host.roleplayConfigured()) return { kind: "roleplay_setup_required" };
      return { kind: "submitted", turn: await this.acceptTurnLocked(prompt) };
    });
  }

  async createConversation(): Promise<boolean> {
    return this.transitions.run(() => this.creation.create());
  }

  async submit(prompt: string, roleplayTurn?: RoleplayTurnInput): Promise<ChatChannelTurnReceipt> {
		return this.transitions.run(() => this.acceptTurnLocked(prompt, roleplayTurn));
  }

  async reconcileTurn(receipt: ChatChannelTurnReceipt): Promise<void> {
    await this.turns.reconcile(receipt);
  }

  private async acceptTurnLocked(
    prompt: string,
    roleplayTurn?: RoleplayTurnInput,
  ): Promise<ChatChannelTurnReceipt> {
    const channelID = this.selectedChannelID;
    if (!channelID) throw new Error("A channel must be selected before sending a channel message.");
    return roleplayTurn === undefined
      ? this.turns.accept(channelID, prompt)
      : this.turns.accept(channelID, prompt, roleplayTurn);
  }

  private selectedOption(id: string): HTMLOptionElement | null {
    if (!id || !this.host.hasChannelSelect()) return null;
    return this.options.option(id);
  }

  private restoreSelection(id: string): void {
    if (!this.host.hasChannelSelect()) return;
    this.host.channelSelect().value = id && this.selectedOption(id) ? id : "";
  }

  private selectedChannelMode(id: string): "assistant" | "roleplay" {
    return this.options.mode(id);
  }

}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
