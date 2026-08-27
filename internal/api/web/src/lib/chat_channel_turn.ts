import { sendChannelMessage } from "./channel_api";
import { t } from "./i18n";
import type { StatusTone } from "./types";
import type { RoleplayTurnInput } from "./roleplay_turn_input";

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
  constructor(private readonly host: ChatChannelTurnHost) {}

  async accept(
    channelID: string,
    prompt: string,
    roleplayTurn?: RoleplayTurnInput,
  ): Promise<ChatChannelTurnReceipt> {
    this.host.setActivityLabel(t("channel.working"));
    this.host.setStatus(t("status.working"), "active");
    this.host.renderProgressActivity(t("channel.working"));
    const payload = roleplayTurn === undefined
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
      completion: this.host.waitForJob(jobID),
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
