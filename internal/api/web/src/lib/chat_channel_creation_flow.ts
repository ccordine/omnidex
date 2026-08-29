import { createUserChannel, type ChannelCreationContext } from "./channel_api";
import type { UserChannel } from "./types";

export interface MechanicalChannelIdentity {
  id: string;
  name: string;
}

export type ChannelIdentityFactory = () => MechanicalChannelIdentity;

export interface ChatChannelCreationFlowHost {
  workspaceRoot(): string | null;
  newChannelDataSourceID(): string | undefined;
  newChannelCreationContext(): ChannelCreationContext;
}

export class ChatChannelCreationFlow {
  constructor(
    private readonly host: ChatChannelCreationFlowHost,
    private readonly identityFactory: ChannelIdentityFactory = mechanicalChannelIdentity,
  ) {}

  async create(): Promise<UserChannel> {
    const identity = this.identityFactory();
    const workspaceRoot = this.host.workspaceRoot();
    const dataSourceID = this.host.newChannelDataSourceID();
    const creation = this.host.newChannelCreationContext();
    return createUserChannel({
      id: identity.id,
      name: identity.name,
      tags: ["user-channel"],
      ...(workspaceRoot === null ? {} : { workspace_root: workspaceRoot }),
      ...(dataSourceID === undefined ? {} : { data_source_id: dataSourceID }),
      ...creation,
    });
  }
}

export function mechanicalChannelIdentity(): MechanicalChannelIdentity {
  if (typeof globalThis.crypto?.randomUUID !== "function") {
    throw new Error("Secure random channel identity generation is unavailable.");
  }
  return {
    id: `chat-${globalThis.crypto.randomUUID()}`,
    name: `Conversation ${new Date().toISOString()}`,
  };
}
