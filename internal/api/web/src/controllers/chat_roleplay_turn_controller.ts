import { ChatRoleplayDraftController } from "./chat_roleplay_draft_controller";

export abstract class ChatRoleplayTurnController extends ChatRoleplayDraftController {
  private draggedRoleplayResponderID = "";

  async toggleRoleplayResponder(event: Event): Promise<void> {
    event.preventDefault();
    const button = event.currentTarget;
    if (!(button instanceof HTMLButtonElement)) throw new Error("Responder toggle is invalid.");
    const characterID = requireCharacterID(button.dataset.roleplayCharacterId);
    const enabled = button.dataset.roleplayEnabled;
    if (enabled !== "true" && enabled !== "false") throw new Error("Responder toggle state is invalid.");
    const responders = this.currentRoleplayResponderIDs();
    const next = enabled === "true"
      ? responders.filter((id) => id !== characterID)
      : [...responders, characterID];
    if (next.length < 1) {
      this.setStatus("Keep at least one responding character enabled.", "error");
      return;
    }
    await this.roleplay.updateResponders(next, requirePositiveRevision(button.dataset.roleplaySceneRevision));
  }

  roleplayResponderDragStart(event: DragEvent): void {
    const handle = event.currentTarget;
    const row = handle instanceof HTMLElement
      ? handle.closest<HTMLElement>("[data-roleplay-cast-character]")
      : null;
    if (!row || row.dataset.roleplayCastEnabled !== "true") {
      event.preventDefault();
      return;
    }
    this.draggedRoleplayResponderID = requireCharacterID(row.dataset.roleplayCastCharacter);
    if (!event.dataTransfer) throw new Error("Responder drag transport is unavailable.");
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", this.draggedRoleplayResponderID);
    row.classList.add("opacity-50");
  }

  roleplayResponderDragOver(event: DragEvent): void {
    const row = event.currentTarget;
    if (row instanceof HTMLElement && row.dataset.roleplayCastEnabled === "true" &&
        this.draggedRoleplayResponderID) {
      event.preventDefault();
      if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
    }
  }

  async roleplayResponderDrop(event: DragEvent): Promise<void> {
    event.preventDefault();
    const row = event.currentTarget;
    if (!(row instanceof HTMLElement) || row.dataset.roleplayCastEnabled !== "true") return;
    const source = requireCharacterID(this.draggedRoleplayResponderID);
    const target = requireCharacterID(row.dataset.roleplayCastCharacter);
    if (source === target) return;
    const responders = this.currentRoleplayResponderIDs().filter((id) => id !== source);
    const targetIndex = responders.indexOf(target);
    if (targetIndex < 0) throw new Error("Drop target is not an enabled responder.");
    responders.splice(targetIndex, 0, source);
    const container = row.closest<HTMLElement>("[data-roleplay-scene-revision]");
    await this.roleplay.updateResponders(
      responders,
      requirePositiveRevision(container?.dataset.roleplaySceneRevision),
    );
  }

  roleplayResponderDragEnd(event: DragEvent): void {
    const handle = event.currentTarget;
    const row = handle instanceof HTMLElement
      ? handle.closest<HTMLElement>("[data-roleplay-cast-character]")
      : null;
    if (row) row.classList.remove("opacity-50");
    this.draggedRoleplayResponderID = "";
  }

  private currentRoleplayResponderIDs(): string[] {
    const list = this.element.querySelector<HTMLElement>("[data-roleplay-cast-list]");
    if (!list) throw new Error("The responder list is unavailable.");
    const ids = [...list.querySelectorAll<HTMLElement>("[data-roleplay-cast-enabled='true']")]
      .map((row) => requireCharacterID(row.dataset.roleplayCastCharacter));
    if (ids.length < 1) throw new Error("The selected scene has no enabled responder.");
    return ids;
  }
}

function requireCharacterID(value: string | undefined): string {
  if (typeof value !== "string" || !/^rpc_[0-9a-f]{32}$/.test(value)) {
    throw new Error("Roleplay character identity is invalid.");
  }
  return value;
}

function requirePositiveRevision(value: string | undefined): number {
  if (typeof value !== "string" || !/^[1-9][0-9]*$/.test(value)) {
    throw new Error("Roleplay scene revision is invalid.");
  }
  const revision = Number(value);
  if (!Number.isSafeInteger(revision)) throw new Error("Roleplay scene revision is invalid.");
  return revision;
}
