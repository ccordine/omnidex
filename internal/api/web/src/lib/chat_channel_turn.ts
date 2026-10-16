import { sendChannelMessage } from "./channel_api";
import type { CodingMode } from "./channel_api";
import { t } from "./i18n";
import type { StatusTone } from "./types";
import type { RoleplayTurnInput } from "./roleplay_turn_input";
import { CodingAgentChannelTurn } from "./coding_agent_channel_turn";
import type { ChannelTranscriptPage } from "./types";
import type { AgentChannelState } from "./coding_agent_channel_contract";

export type ChatChannelTurnReceipt = {
  channelID: string;
  jobID: number;
  acceptedTranscript: Promise<void>;
  completion: Promise<void>;
};

export interface ChatChannelTurnHost {
  setStatus(text: string, mode: StatusTone): void;
  setActivityLabel(label: string): void;
  renderProgressActivity(label: string): void;
  addEvent(type: string, details?: Record<string, unknown>, full?: unknown): void;
  loadTranscript(channelID: string, requiredMessageID?: number): Promise<void>;
  waitForJob(jobID: number): Promise<void>;
  isSelected(channelID: string): boolean;
  refreshRoleplay(): Promise<void>;
}

export class ChatChannelTurnCoordinator {
  private readonly agents: CodingAgentChannelTurn;

  constructor(private readonly host: ChatChannelTurnHost) {
    this.agents = new CodingAgentChannelTurn((channelID, error) => {
      const message = error instanceof Error ? error.message : String(error);
      this.host.addEvent("agent_job_refresh_failed", { channel_id: channelID, error: message });
      if (this.host.isSelected(channelID)) this.host.setStatus(message, "error");
    });
  }

  onAgentTurnChange(handler: () => void): void { this.agents.onChange(handler); }
  agentTurnState(channelID: string): AgentChannelState | undefined { return this.agents.state(channelID); }
  recoverAgentTurn(channelID: string, turn: ChannelTranscriptPage["latest_user_turn"]): Promise<void> { return this.agents.recover(channelID, turn); }
  disconnectAgentTurns(): void { this.agents.disconnect(); }

  async accept(
    channelID: string,
    prompt: string,
    roleplayTurn?: RoleplayTurnInput,
    codingMode: CodingMode = "conversation",
  ): Promise<ChatChannelTurnReceipt> {
    const active = this.agents.state(channelID);
    if (active && active.status !== "waiting_input") throw new Error("Wait for the current response before sending another message.");
    this.host.setActivityLabel(t("channel.working"));
    this.host.setStatus(t("status.working"), "active");
    this.host.renderProgressActivity(t("channel.working"));
    if (active) {
      const jobID = await this.agents.continueConversation(channelID, prompt);
      return { channelID, jobID, acceptedTranscript: this.host.loadTranscript(channelID),
        completion: this.agents.watch(channelID, jobID) };
    }
    const payload = codingMode === "agents"
      ? await sendChannelMessage(channelID, prompt, roleplayTurn, codingMode)
      : roleplayTurn === undefined
        ? await sendChannelMessage(channelID, prompt)
        : await sendChannelMessage(channelID, prompt, roleplayTurn);
    const jobID = requirePositiveJobID(payload.job.id);
    this.host.addEvent("channel_message", {
      channel_id: channelID,
      message_id: payload.user_message.id,
      job_id: jobID,
    }, payload);
    return {
      channelID,
      jobID,
      acceptedTranscript: this.loadAcceptedTranscript(channelID, payload.user_message.id, jobID),
      completion: codingMode === "agents" ? this.agents.watch(channelID, jobID, undefined, payload.user_message.id) : this.host.waitForJob(jobID),
    };
  }

  async reconcile(receipt: ChatChannelTurnReceipt): Promise<void> {
    const settled = await Promise.allSettled([receipt.acceptedTranscript, receipt.completion]);
    const acceptedTranscriptFailure = settled[0].status === "rejected" ? settled[0].reason : undefined;
    const completionFailure = settled[1].status === "rejected" ? settled[1].reason : undefined;
    let refreshFailure: unknown;
    if (this.host.isSelected(receipt.channelID)) {
      try {
        await this.host.loadTranscript(receipt.channelID);
        await this.host.refreshRoleplay();
      } catch (error) {
        refreshFailure = error;
        this.host.addEvent("channel_terminal_refresh_failed", {
          channel_id: receipt.channelID,
          job_id: receipt.jobID,
          error: error instanceof Error ? error.message : String(error),
        });
      }
    }
    if (completionFailure !== undefined) throw completionFailure;
    if (acceptedTranscriptFailure !== undefined) throw acceptedTranscriptFailure;
    if (refreshFailure !== undefined) throw refreshFailure;
  }

  private async loadAcceptedTranscript(channelID: string, messageID: number, jobID: number): Promise<void> {
    try {
      await this.host.loadTranscript(channelID, messageID);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      const failure = `Message accepted as job #${jobID}, but its transcript refresh failed: ${message}`;
      this.host.setStatus(failure, "error");
      this.host.addEvent("channel_accepted_transcript_failed", {
        channel_id: channelID,
        message_id: messageID,
        job_id: jobID,
        error: message,
      });
      throw new Error(failure);
    }
  }
}

function requirePositiveJobID(value: number | string): number {
  const jobID = typeof value === "number" ? value : Number.NaN;
  if (!Number.isSafeInteger(jobID) || jobID < 1) {
    throw new Error("Channel turn did not return its authoritative job identity.");
  }
  return jobID;
}
