import {
  roleplayTurnSubmission,
  restoredRoleplayTurnParts,
  type RoleplayTurnPart,
  type RoleplayTurnPartKind,
  type RoleplayTurnSubmission,
} from "../lib/roleplay_turn_input";
import type { RoleplayUserPersonaCreationResult } from "../lib/chat_roleplay_coordinator";
import { ChatRoleplayDialogController } from "./chat_roleplay_dialog_controller";

export abstract class ChatRoleplayDraftController extends ChatRoleplayDialogController {
  roleplayPersonaChanged(): void {
    if (this.hasRoleplayPersonaCreatorTarget) this.hideRoleplayPersonaCreatorView();
  }

  showRoleplayPersonaCreator(event: Event): void {
    event.preventDefault();
    if (!this.hasRoleplayPersonaCreatorTarget || !this.hasRoleplayNewPersonaTarget) {
      throw new Error("The inline roleplay identity creator is unavailable.");
    }
    this.roleplayPersonaCreatorTarget.classList.remove("hidden");
    this.roleplayPersonaCreatorTarget.classList.add("flex");
    this.roleplayNewPersonaTarget.focus();
  }

  hideRoleplayPersonaCreator(event: Event): void {
    event.preventDefault();
    this.hideRoleplayPersonaCreatorView();
  }

  roleplayPersonaCreatorKeydown(event: KeyboardEvent): void {
    if (event.key === "Escape") {
      event.preventDefault();
      this.hideRoleplayPersonaCreatorView();
    } else if (event.key === "Enter") {
      event.preventDefault();
      void this.createRoleplayPersona(event);
    }
  }

  async createRoleplayPersona(event: Event): Promise<void> {
    event.preventDefault();
    if (!this.hasRoleplayNewPersonaTarget) throw new Error("The new identity name field is unavailable.");
    const submittedChannelID = this.channel.selectedID();
    const submittedDraft = this.roleplayNewPersonaTarget.value;
    const name = submittedDraft.trim();
    if (!name) {
      this.setStatus("Enter a name for the identity.", "error");
      this.roleplayNewPersonaTarget.focus();
      return;
    }
    let result: RoleplayUserPersonaCreationResult;
    try {
      result = await this.roleplay.createUserPersona(name);
    } catch {
      // The roleplay coordinator has already surfaced and recorded the exact
      // mutation failure. Keep the editor and name intact for an explicit retry.
      if (this.channel.selectedID() === submittedChannelID) {
        this.roleplayNewPersonaTarget.focus();
      }
      return;
    }
    if (result.projection === "invalidated" ||
        this.channel.selectedID() !== result.channelID) return;
    const currentDraft = this.roleplayNewPersonaTarget.value;
    const ownsCurrentDraft = currentDraft === submittedDraft || currentDraft === "";
    if (ownsCurrentDraft) {
      this.roleplayNewPersonaTarget.value = "";
      this.hideRoleplayPersonaCreatorView();
    }
    if (result.projection !== "applied") return;
    this.selectRoleplayComposerAuthority(result.characterID);
    if (ownsCurrentDraft) this.focusComposer();
  }

  addRoleplayDraftPart(event: Event): void {
    event.preventDefault();
    const button = event.currentTarget;
    if (!(button instanceof HTMLButtonElement)) throw new Error("Roleplay part control is invalid.");
    const kind = requireRoleplayPartKind(button.dataset.roleplayPartKind);
    const part = this.availableRoleplayDraftPart();
    const text = this.hasInputTarget ? this.inputTarget.value.trim() : "";
    this.configureRoleplayDraftPart(part, kind, text);
    this.roleplayDraftPartsTarget.append(part);
    if (this.hasInputTarget) this.inputTarget.value = "";
    this.roleplayDraftPartTextarea(part).focus();
  }

  removeRoleplayDraftPart(event: Event): void {
    event.preventDefault();
    const part = this.roleplayDraftPartForEvent(event);
    this.resetRoleplayDraftPart(part);
    this.roleplayDraftPartPoolTarget.append(part);
    this.focusComposer();
  }

  moveRoleplayDraftPartUp(event: Event): void {
    event.preventDefault();
    const part = this.roleplayDraftPartForEvent(event);
    const previous = part.previousElementSibling;
    if (previous) previous.before(part);
  }

  moveRoleplayDraftPartDown(event: Event): void {
    event.preventDefault();
    const part = this.roleplayDraftPartForEvent(event);
    const next = part.nextElementSibling;
    if (next) next.after(part);
  }

  roleplayDraftPartInput(event: Event): void {
    const target = event.currentTarget;
    if (!(target instanceof HTMLTextAreaElement)) throw new Error("Roleplay part editor is invalid.");
    target.style.height = "auto";
    target.style.height = `${Math.min(target.scrollHeight, 160)}px`;
  }

  restoreFailedTurn(event: Event): void {
    event.preventDefault();
    if (this.busy || this.isTurnActive()) {
      this.setStatus("Wait for the current turn before restoring a failed one.", "active");
      return;
    }
    const button = event.currentTarget;
    if (!(button instanceof HTMLButtonElement)) {
      throw new Error("Failed turn recovery requires the server-rendered retry control.");
    }
    const prompt = button.dataset.turnContent;
    if (typeof prompt !== "string" || !prompt.trim()) {
      throw new Error("Failed turn recovery has no exact prompt bytes.");
    }
    const personaKind = button.dataset.roleplayPersonaKind;
    if (personaKind === undefined) {
      this.clearRoleplayDraftParts();
      this.setComposerText(prompt);
      this.setStatus("Failed turn restored. Edit it or send again.", "active");
      this.focusComposer();
      return;
    }
    if (personaKind === "legacy_untyped") {
      const message = "This historical failed turn has no recorded actor or modality, so it cannot be restored safely.";
      this.setStatus(message, "error");
      throw new Error(message);
    }
    const personaValue = personaKind === "narrator" ? "narrator" : button.dataset.roleplayCharacterId;
    if ((personaKind !== "narrator" && personaKind !== "character") ||
        typeof personaValue !== "string" ||
        (personaKind === "character" && !/^rpc_[0-9a-f]{32}$/.test(personaValue))) {
      throw new Error("Failed roleplay turn recovery has contradictory persona authority.");
    }
    if (!this.roleplayComposerAuthorityExists(personaValue)) {
      const message = "The failed turn's acting character is no longer eligible in this scene.";
      this.setStatus(message, "error");
      throw new Error(message);
    }
    const contributionKind = button.dataset.roleplayContributionKind;
    const encodedParts = button.dataset.roleplayTurnParts;
    if (typeof contributionKind !== "string" || typeof encodedParts !== "string") {
      throw new Error("Failed roleplay turn recovery is missing exact contribution authority.");
    }
    const parts = restoredRoleplayTurnParts(prompt, personaKind, contributionKind, encodedParts);
    this.selectRoleplayComposerAuthority(personaValue);
    this.clearRoleplayDraftParts();
    if (parts === null) {
      this.setComposerText(prompt);
    } else {
      for (const item of parts) {
        const part = this.availableRoleplayDraftPart();
        this.configureRoleplayDraftPart(part, item.kind, item.text);
        this.roleplayDraftPartsTarget.append(part);
      }
      this.setComposerText("");
    }
    this.setStatus("Failed turn restored. Edit it or send again.", "active");
    this.focusComposer();
  }

  protected selectedRoleplaySubmission(draft: string): RoleplayTurnSubmission | undefined {
    if (!this.channel.hasSelection()) return undefined;
    if (!this.hasRoleplayPersonaTarget) return undefined;
    if (this.channel.selectedMode() !== "roleplay") {
      throw new Error("Roleplay workspace selection does not identify a roleplay channel.");
    }
    return roleplayTurnSubmission(draft, this.roleplayPersonaTarget, this.queuedRoleplayParts());
  }

  protected clearAcceptedRoleplayDraft(): void {
    this.clearRoleplayDraftParts();
    if (this.hasInputTarget) this.inputTarget.value = "";
  }

  protected selectRoleplayComposerAuthority(personaValue: string): void {
    if (!this.hasRoleplayPersonaTarget) throw new Error("The acting-as control is unavailable.");
    if (!this.roleplayComposerAuthorityExists(personaValue)) {
      throw new Error("The requested acting-as authority is unavailable.");
    }
    this.roleplayPersonaTarget.value = personaValue;
  }

  private roleplayComposerAuthorityExists(personaValue: string): boolean {
    if (!this.hasRoleplayPersonaTarget) throw new Error("The acting-as control is unavailable.");
    return [...this.roleplayPersonaTarget.options].some((option) => option.value === personaValue);
  }

  private queuedRoleplayParts(): RoleplayTurnPart[] {
    if (!this.hasRoleplayDraftPartsTarget) return [];
    return [...this.roleplayDraftPartsTarget.children].map((element, index) => {
      if (!(element instanceof HTMLElement)) throw new Error(`Roleplay part ${index + 1} is invalid.`);
      return {
        kind: requireRoleplayPartKind(element.dataset.roleplayPartKind),
        text: this.roleplayDraftPartTextarea(element).value,
      };
    });
  }

  private availableRoleplayDraftPart(): HTMLElement {
    if (!this.hasRoleplayDraftPartsTarget || !this.hasRoleplayDraftPartPoolTarget) {
      throw new Error("The roleplay turn builder is unavailable.");
    }
    const part = this.roleplayDraftPartPoolTarget.firstElementChild;
    if (!(part instanceof HTMLElement)) throw new Error("This roleplay turn already contains 16 parts.");
    return part;
  }

  private configureRoleplayDraftPart(part: HTMLElement, kind: RoleplayTurnPartKind, text: string): void {
    part.dataset.roleplayPartKind = kind;
    const label = part.querySelector<HTMLElement>("[data-roleplay-part-label]");
    if (!label) throw new Error("Roleplay part label is missing.");
    label.textContent = kind.charAt(0).toUpperCase() + kind.slice(1);
    const textarea = this.roleplayDraftPartTextarea(part);
    textarea.value = text;
    textarea.setAttribute("aria-label", label.textContent);
  }

  private resetRoleplayDraftPart(part: HTMLElement): void {
    this.configureRoleplayDraftPart(part, "message", "");
    this.roleplayDraftPartTextarea(part).style.height = "";
  }

  private clearRoleplayDraftParts(): void {
    if (!this.hasRoleplayDraftPartsTarget || !this.hasRoleplayDraftPartPoolTarget) return;
    for (const child of [...this.roleplayDraftPartsTarget.children]) {
      if (!(child instanceof HTMLElement)) continue;
      this.resetRoleplayDraftPart(child);
      this.roleplayDraftPartPoolTarget.append(child);
    }
  }

  private roleplayDraftPartForEvent(event: Event): HTMLElement {
    const control = event.currentTarget;
    if (!(control instanceof HTMLElement)) throw new Error("Roleplay part control is invalid.");
    const part = control.closest<HTMLElement>("[data-roleplay-part-kind]");
    if (!part || part.parentElement !== this.roleplayDraftPartsTarget) {
      throw new Error("Roleplay part is not in the ordered turn.");
    }
    return part;
  }

  private roleplayDraftPartTextarea(part: HTMLElement): HTMLTextAreaElement {
    const textarea = part.querySelector<HTMLTextAreaElement>("[data-roleplay-part-text]");
    if (!textarea) throw new Error("Roleplay part editor is missing.");
    return textarea;
  }

  private hideRoleplayPersonaCreatorView(): void {
    if (!this.hasRoleplayPersonaCreatorTarget) return;
    this.roleplayPersonaCreatorTarget.classList.add("hidden");
    this.roleplayPersonaCreatorTarget.classList.remove("flex");
  }
}

function requireRoleplayPartKind(value: string | undefined): RoleplayTurnPartKind {
  if (value !== "message" && value !== "action" && value !== "event") {
    throw new Error("Roleplay part type is invalid.");
  }
  return value;
}
