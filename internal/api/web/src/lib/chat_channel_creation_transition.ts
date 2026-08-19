import { ChatChannelCreationFlow } from "./chat_channel_creation_flow";
import { t } from "./i18n";
import type { StatusTone, UserChannel } from "./types";

export interface ChatChannelCreationTransitionHost {
  selectedID(): string;
  selectedMode(id: string): "assistant" | "roleplay";
  restoreSelection(id: string): void;
  clearSelection(): void;
  commitSelection(id: string): void;
  reloadOptions(): Promise<void>;
  hasOption(id: string): boolean;
  loadTranscript(id: string): Promise<void>;
  synchronizeRoleplay(id: string, mode: "assistant" | "roleplay"): Promise<void>;
  setStatus(text: string, tone: StatusTone): void;
  addEvent(type: string, details: Record<string, unknown>): void;
}

export class ChatChannelCreationTransition {
  private pending: UserChannel | null = null;

  constructor(
    private readonly host: ChatChannelCreationTransitionHost,
    private readonly flow: ChatChannelCreationFlow,
  ) {}

  acceptSelection(id: string): void {
    if (this.pending?.id === id) this.pending = null;
  }

  async create(): Promise<boolean> {
    if (this.pending) {
      this.host.restoreSelection(this.host.selectedID());
      this.reportPending(this.pending.id, "channel_create_blocked_pending_reconciliation");
      return false;
    }
    const previousID = this.host.selectedID();
    const previousMode = previousID ? this.host.selectedMode(previousID) : "assistant";
    this.host.restoreSelection(previousID);
    this.host.setStatus(t("channel.statusCreating"), "active");
    let created: UserChannel | null = null;
    try {
      created = await this.flow.create();
      this.pending = created;
      await this.host.reloadOptions();
      if (!this.host.hasOption(created.id)) {
        throw new Error("The created channel is absent from the authoritative channel list.");
      }
      await this.host.loadTranscript(created.id);
      await this.host.synchronizeRoleplay(created.id, this.host.selectedMode(created.id));
      this.host.commitSelection(created.id);
      this.pending = null;
      this.host.setStatus(t("status.ready"), "ready");
      return true;
    } catch (error) {
      if (created) {
        this.host.clearSelection();
        await this.resetRoleplay();
        this.host.addEvent("channel_creation_reconciliation_failed", {
          channel_id: created.id,
          error: errorMessage(error),
        });
        this.reportPending(created.id, "channel_creation_pending_reload");
        return false;
      }
      this.host.restoreSelection(previousID);
      await this.rollbackRoleplay(previousID, previousMode);
      this.host.setStatus(errorMessage(error), "error");
      this.host.addEvent("channel_create_failed", { error: errorMessage(error) });
      return false;
    }
  }

  private reportPending(id: string, event: string): void {
    this.host.setStatus(pendingCreatedMessage(id), "error");
    this.host.addEvent(event, { channel_id: id });
  }

  private async resetRoleplay(): Promise<void> {
    try {
      await this.host.synchronizeRoleplay("", "assistant");
    } catch (error) {
      this.host.addEvent("roleplay_creation_reset_failed", { error: errorMessage(error) });
    }
  }

  private async rollbackRoleplay(id: string, mode: "assistant" | "roleplay"): Promise<void> {
    try {
      await this.host.synchronizeRoleplay(id, mode);
    } catch (error) {
      this.host.addEvent("roleplay_creation_rollback_failed", { error: errorMessage(error) });
    }
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function pendingCreatedMessage(id: string): string {
  return `Conversation ${id} was created but could not be reconciled. Reload and select it before creating another.`;
}
