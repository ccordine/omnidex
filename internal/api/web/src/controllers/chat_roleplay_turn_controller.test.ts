import { describe, expect, it, vi } from "vitest";
import { ChatRoleplayTurnController } from "./chat_roleplay_turn_controller";

function eventFor(element: Element): Event {
  return { currentTarget: element, preventDefault: vi.fn() } as unknown as Event;
}

function recoveryHarness() {
  const input = document.createElement("textarea");
  const persona = document.createElement("select");
  persona.innerHTML = `
    <option value="narrator" data-persona-kind="narrator">Narrator</option>
    <option value="rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" data-persona-kind="character">Gryph</option>
  `;
  return Object.assign(Object.create(ChatRoleplayTurnController.prototype), {
    busy: false,
    hasInputTarget: true,
    inputTarget: input,
    hasRoleplayPersonaTarget: true,
    roleplayPersonaTarget: persona,
    hasRoleplayDraftPartsTarget: false,
    hasRoleplayDraftPartPoolTarget: false,
    isTurnActive: vi.fn(() => false),
    setStatus: vi.fn(),
    focusComposer: vi.fn(),
  }) as ChatRoleplayTurnController & {
    setStatus: ReturnType<typeof vi.fn>;
    focusComposer: ReturnType<typeof vi.fn>;
  };
}

describe("ChatRoleplayTurnController failed-turn recovery", () => {
  it("restores exact prompt bytes and prior typed authority without requeueing", () => {
    const controller = recoveryHarness();
    const button = document.createElement("button");
    button.dataset.turnContent = `I take Mara's hand. "Stay," I say.`;
    button.dataset.roleplayPersonaKind = "character";
    button.dataset.roleplayCharacterId = "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
    button.dataset.roleplayContributionKind = "action_dialogue";

    controller.restoreFailedTurn({
      preventDefault: vi.fn(),
      currentTarget: button,
    } as unknown as Event);

    expect(controller.inputTarget.value).toBe(`I take Mara's hand. "Stay," I say.`);
    expect(controller.roleplayPersonaTarget.value).toBe("rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
    expect(controller.setStatus).toHaveBeenCalledWith(
      "Failed turn restored. Edit it or send again.",
      "active",
    );
    expect(controller.focusComposer).toHaveBeenCalledOnce();
  });

  it("fails loudly when the old persona is no longer eligible", () => {
    const controller = recoveryHarness();
    const button = document.createElement("button");
    button.dataset.turnContent = "Continue exactly.";
    button.dataset.roleplayPersonaKind = "character";
    button.dataset.roleplayCharacterId = "rpc_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";
    button.dataset.roleplayContributionKind = "action";

    expect(() => controller.restoreFailedTurn({
      preventDefault: vi.fn(),
      currentTarget: button,
    } as unknown as Event)).toThrow("no longer in the selected world");
  });
});

describe("ChatRoleplayTurnController composer setup routing", () => {
  it("opens the clicked character editor without changing responder authority", async () => {
    const open = vi.fn(async () => undefined);
    const updateResponders = vi.fn();
    const controller = Object.assign(Object.create(ChatRoleplayTurnController.prototype), {
      roleplayCharacterEditor: { open },
      roleplay: { updateResponders },
    }) as ChatRoleplayTurnController;
    const button = document.createElement("button");
    button.dataset.roleplayCharacterId = "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
    const event = { currentTarget: button, preventDefault: vi.fn() } as unknown as Event;

    await controller.openRoleplayCharacterEditor(event);

    expect(open).toHaveBeenCalledWith(event);
    expect(updateResponders).not.toHaveBeenCalled();
  });

  it("toggles only responder authority without opening the character editor", async () => {
    const open = vi.fn();
    const updateResponders = vi.fn(async () => undefined);
    const root = document.createElement("div");
    root.innerHTML = `<ul data-roleplay-cast-list>
      <li data-roleplay-cast-enabled="true" data-roleplay-cast-character="rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"></li>
      <li data-roleplay-cast-enabled="true" data-roleplay-cast-character="rpc_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"></li>
    </ul>`;
    const controller = Object.assign(Object.create(ChatRoleplayTurnController.prototype), {
      roleplayCharacterEditor: { open },
      roleplay: { updateResponders },
      setStatus: vi.fn(),
    }) as ChatRoleplayTurnController;
	Object.defineProperty(controller, "element", { value: root });
    const button = document.createElement("button");
    button.dataset.roleplayCharacterId = "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
    button.dataset.roleplayEnabled = "true";
    button.dataset.roleplaySceneRevision = "7";

    await controller.toggleRoleplayResponder(eventFor(button));

    expect(updateResponders).toHaveBeenCalledWith(
      ["rpc_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"],
      7,
    );
    expect(open).not.toHaveBeenCalled();
  });

  it("uses only the responder handle as drag authority and marks its cast row", () => {
    const controller = Object.assign(Object.create(ChatRoleplayTurnController.prototype), {
      draggedRoleplayResponderID: "",
    }) as ChatRoleplayTurnController;
    const row = document.createElement("li");
    row.dataset.roleplayCastEnabled = "true";
    row.dataset.roleplayCastCharacter = "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
    const handle = document.createElement("button");
    handle.draggable = true;
    row.append(handle);
    const dataTransfer = { effectAllowed: "none", setData: vi.fn() };

    controller.roleplayResponderDragStart({
      currentTarget: handle,
      dataTransfer,
      preventDefault: vi.fn(),
    } as unknown as DragEvent);

    expect(dataTransfer.setData).toHaveBeenCalledWith("text/plain", row.dataset.roleplayCastCharacter);
    expect(row.classList.contains("opacity-50")).toBe(true);
    controller.roleplayResponderDragEnd({ currentTarget: handle } as unknown as DragEvent);
    expect(row.classList.contains("opacity-50")).toBe(false);
  });

  it("opens the exact server-named scene or model section", () => {
    const openSetup = vi.fn();
    const controller = Object.assign(Object.create(ChatRoleplayTurnController.prototype), {
      channel: { hasSelection: vi.fn(() => true) },
      roleplayWorkspaceDialogs: { openSetup },
    }) as ChatRoleplayTurnController;
    const button = document.createElement("button");
    button.dataset.roleplaySetupSection = "cast";

    controller.openRoleplayWorldSetup({
      preventDefault: vi.fn(),
      currentTarget: button,
    } as unknown as Event);

    expect(openSetup).toHaveBeenCalledWith("cast");
  });

  it("rejects an unregistered composer setup destination", () => {
    const controller = Object.assign(Object.create(ChatRoleplayTurnController.prototype), {
      channel: { hasSelection: vi.fn(() => true) },
      roleplayWorkspaceDialogs: { openSetup: vi.fn() },
    }) as ChatRoleplayTurnController;
    const button = document.createElement("button");
    button.dataset.roleplaySetupSection = "models";

    expect(() => controller.openRoleplayWorldSetup({
      preventDefault: vi.fn(),
      currentTarget: button,
    } as unknown as Event)).toThrow("unsupported section");
  });
});
