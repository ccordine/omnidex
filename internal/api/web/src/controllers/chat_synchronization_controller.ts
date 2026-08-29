import type { ChatChannelCoordinator } from "../lib/chat_channel_coordinator";
import type { ChatChannelCreationCoordinator } from "../lib/chat_channel_creation_coordinator";
import type { ChatDataSourceCoordinator } from "../lib/chat_data_source_coordinator";
import type { ChatExecutionCoordinator } from "../lib/chat_execution_coordinator";
import type { ChatJobsCoordinator } from "../lib/chat_jobs_coordinator";
import type { ChatMemoryCoordinator } from "../lib/chat_memory_coordinator";
import type { ChatPanelCoordinator } from "../lib/chat_panel_coordinator";
import type { ChatRoleplayCoordinator } from "../lib/chat_roleplay_coordinator";
import type { RoleplayCharacterEditorCoordinator } from "../lib/roleplay_character_editor_coordinator";
import type { RoleplayWorkspaceCoordinator } from "../lib/roleplay_workspace_coordinator";
import type { RoleplayWorkspaceDialogs } from "../lib/roleplay_workspace_dialogs";
import type { ChatSlashPaletteCoordinator } from "../lib/chat_slash_palette_coordinator";
import type { ChatSystemCoordinator } from "../lib/chat_system_coordinator";
import type { OmniPanel } from "../lib/panel_routing";
import { ChatViewController } from "./chat_view_controller";

export abstract class ChatSynchronizationController extends ChatViewController {
  protected channel!: ChatChannelCoordinator;
  protected creation!: ChatChannelCreationCoordinator;
  protected dataSources!: ChatDataSourceCoordinator;
  protected jobs!: ChatJobsCoordinator;
  protected memory!: ChatMemoryCoordinator;
  protected system!: ChatSystemCoordinator;
  protected execution!: ChatExecutionCoordinator;
  protected panels!: ChatPanelCoordinator;
  protected roleplay!: ChatRoleplayCoordinator;
  protected roleplayCharacterEditor!: RoleplayCharacterEditorCoordinator;
  protected roleplayWorkspace!: RoleplayWorkspaceCoordinator;
  protected roleplayWorkspaceDialogs!: RoleplayWorkspaceDialogs;
  protected slashPalette!: ChatSlashPaletteCoordinator;

  protected async loadPanelData(panel: OmniPanel): Promise<void> {
    if (panel === "chat") {
      this.creation.synchronize();
      this.dataSources.setCreationMode(this.creation.selectedMode());
      await Promise.all([this.dataSources.load(), this.channel.loadChannels("assistant")]);
      this.refreshSystemActivity();
      if (this.hasInputTarget) this.focusComposer();
      return;
    }
    if (panel === "roleplay") {
      this.creation.synchronize();
      this.dataSources.setCreationMode("roleplay");
      await this.channel.loadChannels("roleplay");
      await this.roleplayWorkspace.activate();
      this.refreshSystemActivity();
      if (this.hasInputTarget && this.channel.hasSelection()) this.focusComposer();
      return;
    }
    if (panel === "jobs") await this.jobs.load({ strict: true });
    if (panel === "memory") await this.memory.load();
    if (panel === "metrics") await this.system.loadMetrics({ strict: true });
  }

  protected async synchronizeRealtimeState(): Promise<void> {
    const tasks: Promise<unknown>[] = [this.memory.loadGlobalActivity({ quiet: true, strict: true })];
    if (this.execution.currentJobID() !== null) tasks.push(this.execution.refreshCurrent());
    if (this.channel.hasSelection()) tasks.push(this.channel.loadTranscript(this.channel.selectedID()));
    if (this.channel.hasSelection()) tasks.push(this.roleplay.refresh());
    if (this.channel.hasSelection()) tasks.push(this.roleplayCharacterEditor.refreshIfOpen());
    if (this.panels.isCurrent("roleplay")) tasks.push(this.roleplayWorkspace.refresh());
    if (this.channel.hasSelection()) tasks.push(this.slashPalette.refresh());
    if (this.panels.isCurrent("jobs")) tasks.push(this.jobs.load({ quiet: true, strict: true }));
    if (this.panels.isCurrent("metrics")) tasks.push(this.system.loadMetrics({ strict: true }));
    await Promise.all(tasks);
  }
}
