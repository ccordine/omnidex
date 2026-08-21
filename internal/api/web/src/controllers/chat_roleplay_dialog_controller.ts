import { ChatRuntimeController } from "./chat_runtime_controller";
import type { RoleplaySetupSection } from "../lib/roleplay_workspace_dialogs";

export abstract class ChatRoleplayDialogController extends ChatRuntimeController {
  async openRoleplayCharacterEditor(event: Event): Promise<void> {
    await this.roleplayCharacterEditor.open(event);
  }

  closeRoleplayCharacterEditor(event: Event): void {
    this.roleplayCharacterEditor.close(event);
  }

  closeRoleplayCharacterEditorBackdrop(event: Event): void {
    this.roleplayCharacterEditor.closeFromBackdrop(event);
  }

  async saveRoleplayCharacterPersona(event: Event): Promise<void> {
    await this.roleplayCharacterEditor.savePersona(event);
  }

  async saveRoleplayCharacterResearch(event: Event): Promise<void> {
    await this.roleplayCharacterEditor.saveResearch(event);
  }

  async saveRoleplayCharacterGeneration(event: Event): Promise<void> {
    await this.roleplayCharacterEditor.saveGeneration(event);
  }

  openRoleplayWorldBrowser(event: Event): void {
    event.preventDefault();
    this.roleplayWorkspaceDialogs.open("worlds");
  }

  closeRoleplayWorldBrowser(event: Event): void {
    event.preventDefault();
    this.roleplayWorkspaceDialogs.close("worlds");
  }

  closeRoleplayWorldBrowserBackdrop(event: Event): void {
    this.roleplayWorkspaceDialogs.closeFromBackdrop("worlds", event);
  }

  openRoleplayCharacterLibrary(event: Event): void {
    event.preventDefault();
    this.roleplayWorkspaceDialogs.open("characters");
  }

  closeRoleplayCharacterLibrary(event: Event): void {
    event.preventDefault();
    this.roleplayWorkspaceDialogs.close("characters");
  }

  closeRoleplayCharacterLibraryBackdrop(event: Event): void {
    this.roleplayWorkspaceDialogs.closeFromBackdrop("characters", event);
  }

  openRoleplayWorldSetup(event: Event): void {
    event.preventDefault();
    if (!this.channel.hasSelection()) {
      this.roleplayWorkspaceDialogs.open("worlds");
      return;
    }
    const target = event.currentTarget;
    const requested = target instanceof HTMLElement
      ? target.dataset.roleplaySetupSection
      : undefined;
    let section: RoleplaySetupSection | undefined;
    if (requested !== undefined) {
      if (requested !== "scene" && requested !== "cast" &&
          requested !== "state" && requested !== "actions") {
        throw new Error("Roleplay setup control names an unsupported section.");
      }
      section = requested;
    }
    this.roleplayWorkspaceDialogs.openSetup(section);
  }

  closeRoleplayWorldSetup(event: Event): void {
    event.preventDefault();
    this.roleplayWorkspaceDialogs.close("setup");
  }

  closeRoleplayWorldSetupBackdrop(event: Event): void {
    this.roleplayWorkspaceDialogs.closeFromBackdrop("setup", event);
  }

  selectRoleplaySetupSection(event: Event): void {
    this.roleplayWorkspaceDialogs.selectSetupSection(event);
  }
}
