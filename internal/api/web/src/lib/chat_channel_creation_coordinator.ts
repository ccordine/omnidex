import type { ChannelCreationContext } from "./channel_api";

export interface ChatChannelCreationHost {
  hasMode(): boolean;
  mode(): HTMLSelectElement;
  hasRoleplayFields(): boolean;
  roleplayFields(): HTMLElement;
  hasWorldName(): boolean;
  worldName(): HTMLInputElement;
  hasViewpointName(): boolean;
  viewpointName(): HTMLInputElement;
}

export class ChatChannelCreationCoordinator {
  constructor(private readonly host: ChatChannelCreationHost) {}

  synchronize(): void {
    if (!this.host.hasMode()) return;
    const roleplay = this.selectedMode() === "roleplay";
    if (roleplay && (!this.host.hasRoleplayFields() || !this.host.hasWorldName() || !this.host.hasViewpointName())) {
      throw new Error("The server-rendered roleplay creation fields are incomplete.");
    }
    if (this.host.hasRoleplayFields()) this.host.roleplayFields().classList.toggle("hidden", !roleplay);
    if (this.host.hasWorldName()) {
      this.host.worldName().disabled = !roleplay;
      this.host.worldName().required = roleplay;
    }
    if (this.host.hasViewpointName()) {
      this.host.viewpointName().disabled = !roleplay;
      this.host.viewpointName().required = roleplay;
    }
  }

  parameters(): ChannelCreationContext {
    const mode = this.selectedMode();
    if (mode === "assistant") return { mode };
    if (!this.host.hasWorldName() || !this.host.hasViewpointName()) {
      throw new Error("The server-rendered roleplay creation fields are incomplete.");
    }
    return {
      mode,
      roleplay_world_name: exactName(this.host.worldName().value, "Roleplay world name"),
      roleplay_viewpoint_name: exactName(this.host.viewpointName().value, "Roleplay viewpoint name"),
    };
  }

  selectedMode(): "assistant" | "roleplay" {
    if (!this.host.hasMode()) throw new Error("The server-rendered channel mode selector is unavailable.");
    const mode = this.host.mode().value;
    if (mode !== "assistant" && mode !== "roleplay") {
      throw new Error("The selected channel mode is not server-authorized.");
    }
    return mode;
  }
}

function exactName(value: string, source: string): string {
  if (!value.trim() || value !== value.trim() || value.includes("\0") || new TextEncoder().encode(value).byteLength > 256) {
    throw new Error(`${source} must be exact nonblank text of at most 256 UTF-8 bytes.`);
  }
  return value;
}
