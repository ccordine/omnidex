import { HTTPResponseError } from "./api";
import {
  configureRoleplayResearch,
  writeRoleplayGeneration,
  writeRoleplayPersona,
  type RoleplayComponentResponse,
} from "./roleplay_api";
import { roleplayID } from "./roleplay_api_validation";
import {
  personaInput,
  requiredDataset,
  researchCapabilityInput,
  roleplayGenerationInput,
} from "./roleplay_form_input";
import {
  fetchRoleplayCharacterEditor,
} from "./roleplay_character_editor_api";
import {
  roleplayErrorMessage,
  withRoleplayControlFeedback,
  withRoleplayFormFeedback,
} from "./chat_roleplay_support";
import type { StatusTone } from "./types";

export interface RoleplayCharacterEditorHost {
  selectedChannelID(): string;
  renderComponentBundle(bundle: string): Promise<void>;
  refreshRoleplay(): Promise<void>;
  openDialog(): void;
  closeDialog(): void;
  closeDialogFromBackdrop(event: Event): void;
  dialogIsOpen(): boolean;
  setStatus(text: string, tone: StatusTone): void;
  addEvent(type: string, details?: Record<string, unknown>): void;
  reportError(error: unknown): void;
}

export class RoleplayCharacterEditorCoordinator {
  private activeChannelID = "";
  private activeCharacterID = "";
  private viewGeneration = 0;
  private mutationGate: Promise<void> = Promise.resolve();

  constructor(private readonly host: RoleplayCharacterEditorHost) {}

  async open(event: Event): Promise<void> {
    event.preventDefault();
    const button = event.currentTarget;
    if (!(button instanceof HTMLButtonElement)) {
      throw new Error("Character editing requires a server-rendered character button.");
    }
    const channelID = this.requireSelectedChannel();
    const characterID = roleplayID(button.dataset.roleplayCharacterId, "character", "rpc");
    const generation = ++this.viewGeneration;
    await withRoleplayControlFeedback(button, async () => {
      const component = await fetchRoleplayCharacterEditor(channelID, characterID);
      if (!this.isCurrentRequest(channelID, generation)) return;
      await this.host.renderComponentBundle(component.html.bundle);
      if (!this.isCurrentRequest(channelID, generation)) return;
      this.activeChannelID = channelID;
      this.activeCharacterID = characterID;
      this.host.openDialog();
      this.host.setStatus("character ready", "ready");
      this.host.addEvent("roleplay_character_editor_opened", {
        channel_id: channelID,
        character_id: characterID,
      });
    }, (error) => this.reportFailure("roleplay_character_editor_open_failed", error, "character editor unavailable"));
  }

  close(event: Event): void {
    event.preventDefault();
    this.host.closeDialog();
    this.clearActive();
  }

  closeFromBackdrop(event: Event): void {
    this.host.closeDialogFromBackdrop(event);
    if (!this.host.dialogIsOpen()) this.clearActive();
  }

  async refreshIfOpen(): Promise<void> {
    if (!this.host.dialogIsOpen() || !this.activeChannelID || !this.activeCharacterID) return;
    if (this.host.selectedChannelID() !== this.activeChannelID) {
      this.host.closeDialog();
      this.clearActive();
      return;
    }
    const channelID = this.activeChannelID;
    const characterID = this.activeCharacterID;
    const generation = ++this.viewGeneration;
    try {
      await this.rehydrateEditor(channelID, characterID, generation);
    } catch (error) {
      if (this.isActive(channelID, characterID)) {
        this.reportFailure("roleplay_character_editor_refresh_failed", error, "character refresh failed");
      }
    }
  }

  async savePersona(event: Event): Promise<void> {
    try {
      const form = this.requireForm(event);
      const characterID = this.requireFormCharacter(form);
      const input = personaInput(form);
      await this.enqueueMutation(form, characterID, "roleplay_character_persona_saved", (channelID) => (
        writeRoleplayPersona(channelID, characterID, input)
      ));
    } catch (error) {
      this.reportFailure("roleplay_character_editor_save_failed", error, "character update failed");
    }
  }

  async saveResearch(event: Event): Promise<void> {
    try {
      const form = this.requireForm(event);
      const characterID = this.requireFormCharacter(form);
      const input = researchCapabilityInput(form);
      await this.enqueueMutation(form, characterID, "roleplay_character_research_saved", (channelID) => (
        configureRoleplayResearch(channelID, characterID, input)
      ));
    } catch (error) {
      this.reportFailure("roleplay_character_editor_save_failed", error, "character update failed");
    }
  }

  async saveGeneration(event: Event): Promise<void> {
    try {
      const form = this.requireForm(event);
      const characterID = this.requireFormCharacter(form);
      const input = roleplayGenerationInput(form);
      await this.enqueueMutation(form, characterID, "roleplay_character_generation_saved", (channelID) => (
        writeRoleplayGeneration(channelID, characterID, input)
      ));
    } catch (error) {
      this.reportFailure("roleplay_character_editor_save_failed", error, "character update failed");
    }
  }

  private async enqueueMutation(
    form: HTMLFormElement,
    characterID: string,
    eventName: string,
    operation: (channelID: string) => Promise<RoleplayComponentResponse>,
  ): Promise<void> {
    const task = this.mutationGate.then(() => this.executeMutation(form, characterID, eventName, operation));
    this.mutationGate = task.catch(() => undefined);
    await task;
  }

  private async executeMutation(
    form: HTMLFormElement,
    characterID: string,
    eventName: string,
    operation: (channelID: string) => Promise<RoleplayComponentResponse>,
  ): Promise<void> {
    let channelID: string;
    try {
      channelID = this.requireActiveCharacter(characterID);
    } catch (error) {
      this.reportFailure("roleplay_character_editor_save_failed", error, "character update failed");
      return;
    }
    await withRoleplayFormFeedback(form, async () => {
      let accepted = false;
      try {
        const component = await operation(channelID);
        accepted = true;
        if (component.channel_id !== channelID) {
          throw new Error("Character mutation changed its requested channel authority.");
        }
        await this.host.refreshRoleplay();
        if (this.isActive(channelID, characterID) && this.host.dialogIsOpen()) {
          const generation = ++this.viewGeneration;
          await this.rehydrateEditor(channelID, characterID, generation);
        }
        this.host.setStatus("character saved", "ready");
        this.host.addEvent(eventName, { channel_id: channelID, character_id: characterID });
      } catch (error) {
        if (!accepted && error instanceof HTTPResponseError && error.status === 409) {
          await this.rehydrateConflict(channelID, characterID, error);
          return;
        }
        this.reportFailure(
          "roleplay_character_editor_save_failed",
          error,
          accepted ? "character saved; refresh failed" : "character update failed",
        );
      }
    });
  }

  private async rehydrateConflict(channelID: string, characterID: string, conflict: unknown): Promise<void> {
    this.reportFailure("roleplay_character_editor_conflict", conflict, "character changed elsewhere");
    try {
      await this.host.refreshRoleplay();
      if (this.isActive(channelID, characterID) && this.host.dialogIsOpen()) {
        const generation = ++this.viewGeneration;
        await this.rehydrateEditor(channelID, characterID, generation);
      }
      this.host.setStatus("character state refreshed", "active");
    } catch (error) {
      this.reportFailure("roleplay_character_editor_conflict_refresh_failed", error, "character refresh failed");
    }
  }

  private async rehydrateEditor(channelID: string, characterID: string, generation: number): Promise<void> {
    const component = await fetchRoleplayCharacterEditor(channelID, characterID);
    if (!this.isActiveRequest(channelID, characterID, generation)) return;
    await this.host.renderComponentBundle(component.html.bundle);
  }

  private requireForm(event: Event): HTMLFormElement {
    event.preventDefault();
    const form = event.currentTarget;
    if (!(form instanceof HTMLFormElement)) {
      throw new Error("Character editing requires a server-rendered form.");
    }
    return form;
  }

  private requireFormCharacter(form: HTMLFormElement): string {
    const characterID = roleplayID(requiredDataset(form, "characterId"), "character", "rpc");
    this.requireActiveCharacter(characterID);
    return characterID;
  }

  private requireActiveCharacter(characterID: string): string {
    const channelID = this.requireSelectedChannel();
    if (!this.host.dialogIsOpen() || channelID !== this.activeChannelID || characterID !== this.activeCharacterID) {
      throw new Error("Character editor form does not match the open character authority.");
    }
    return channelID;
  }

  private requireSelectedChannel(): string {
    const channelID = this.host.selectedChannelID();
    if (!/^[a-z0-9][a-z0-9_.:-]{0,95}$/.test(channelID)) {
      throw new Error("Character editing requires one selected roleplay channel.");
    }
    return channelID;
  }

  private isCurrentRequest(channelID: string, generation: number): boolean {
    return this.host.selectedChannelID() === channelID && this.viewGeneration === generation;
  }

  private isActiveRequest(channelID: string, characterID: string, generation: number): boolean {
    return this.isCurrentRequest(channelID, generation) && this.isActive(channelID, characterID);
  }

  private isActive(channelID: string, characterID: string): boolean {
    return this.activeChannelID === channelID && this.activeCharacterID === characterID;
  }

  private clearActive(): void {
    this.viewGeneration += 1;
    this.activeChannelID = "";
    this.activeCharacterID = "";
  }

  private reportFailure(eventName: string, error: unknown, status: string): void {
    this.host.setStatus(status, "error");
    this.host.addEvent(eventName, { error: roleplayErrorMessage(error) });
    this.host.reportError(error);
  }
}
