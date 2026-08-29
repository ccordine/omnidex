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

function addRoleplayPartBuilder(controller: ChatRoleplayTurnController, capacity: number): void {
  const selected = document.createElement("ol");
  const pool = document.createElement("div");
  for (let index = 0; index < capacity; index += 1) {
    const part = document.createElement("li");
    part.dataset.roleplayPartKind = "message";
    const label = document.createElement("span");
    label.dataset.roleplayPartLabel = "";
    const text = document.createElement("textarea");
    text.dataset.roleplayPartText = "";
    part.append(label, text);
    pool.append(part);
  }
  Object.assign(controller, {
    hasRoleplayDraftPartsTarget: true,
    roleplayDraftPartsTarget: selected,
    hasRoleplayDraftPartPoolTarget: true,
    roleplayDraftPartPoolTarget: pool,
  });
}

describe("ChatRoleplayTurnController failed-turn recovery", () => {
  it("restores assistant text exactly even when it resembles a composed roleplay turn", () => {
    const controller = recoveryHarness();
    const button = document.createElement("button");
    button.dataset.turnContent = "[Action]\nThis is an assistant heading, not typed roleplay authority.";

    controller.restoreFailedTurn({
      preventDefault: vi.fn(),
      currentTarget: button,
    } as unknown as Event);

    expect(controller.inputTarget.value).toBe(
      "[Action]\nThis is an assistant heading, not typed roleplay authority.",
    );
    expect(controller.roleplayPersonaTarget.value).toBe("narrator");
    expect(controller.setStatus).toHaveBeenCalledWith(
      "Failed turn restored. Edit it or send again.",
      "active",
    );
  });

  it("restores exact persisted parts and prior typed authority without reinterpreting prompt text", () => {
    const controller = recoveryHarness();
    addRoleplayPartBuilder(controller, 2);
    const button = document.createElement("button");
    button.dataset.turnContent = `[Action]\nI take Mara's hand.\n\nThe lantern flickers.\n\n[Message]\n"Stay," I say.`;
    button.dataset.roleplayPersonaKind = "character";
    button.dataset.roleplayCharacterId = "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
    button.dataset.roleplayContributionKind = "action_dialogue";
    button.dataset.roleplayTurnParts = JSON.stringify([
      { kind: "action", text: "I take Mara's hand.\n\nThe lantern flickers." },
      { kind: "message", text: '"Stay," I say.' },
    ]);

    controller.restoreFailedTurn({
      preventDefault: vi.fn(),
      currentTarget: button,
    } as unknown as Event);

    expect(controller.inputTarget.value).toBe("");
    expect([...controller.roleplayDraftPartsTarget.querySelectorAll("textarea")].map((part) => part.value)).toEqual([
      "I take Mara's hand.\n\nThe lantern flickers.",
      '"Stay," I say.',
    ]);
    expect(controller.roleplayPersonaTarget.value).toBe("rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
    expect(controller.setStatus).toHaveBeenCalledWith(
      "Failed turn restored. Edit it or send again.",
      "active",
    );
    expect(controller.focusComposer).toHaveBeenCalledOnce();
  });

  it("rejects a historical typed turn whose exact ordered parts were never persisted", () => {
    const controller = recoveryHarness();
    controller.inputTarget.value = "Keep this newer draft.";
    const button = document.createElement("button");
    button.dataset.turnContent = "I cross the bridge.";
    button.dataset.roleplayPersonaKind = "character";
    button.dataset.roleplayCharacterId = "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
    button.dataset.roleplayContributionKind = "action";
    button.dataset.roleplayTurnParts = "[]";

    expect(() => controller.restoreFailedTurn(eventFor(button))).toThrow(
      "This historical failed turn has no exact ordered parts, so its original modality cannot be restored safely.",
    );
    expect(controller.inputTarget.value).toBe("Keep this newer draft.");
    expect(controller.focusComposer).not.toHaveBeenCalled();
  });

  it("rejects a legacy roleplay turn whose actor and modality were never recorded", () => {
    const controller = recoveryHarness();
    controller.inputTarget.value = "Keep this newer draft.";
    const button = document.createElement("button");
    button.dataset.turnContent = "Unattributed historical prose.";
    button.dataset.roleplayPersonaKind = "legacy_untyped";
    button.dataset.roleplayContributionKind = "legacy_untyped";
    button.dataset.roleplayTurnParts = "[]";

    expect(() => controller.restoreFailedTurn(eventFor(button))).toThrow(
      "This historical failed turn has no recorded actor or modality, so it cannot be restored safely.",
    );
    expect(controller.inputTarget.value).toBe("Keep this newer draft.");
    expect(controller.setStatus).toHaveBeenCalledWith(
      "This historical failed turn has no recorded actor or modality, so it cannot be restored safely.",
      "error",
    );
    expect(controller.focusComposer).not.toHaveBeenCalled();
  });

  it("rejects a restore when the exact old persona is no longer eligible", () => {
    const controller = recoveryHarness();
    controller.roleplayPersonaTarget.value = "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
    controller.inputTarget.value = "Keep this newer draft.";
    const button = document.createElement("button");
    button.dataset.turnContent = "Continue exactly.";
    button.dataset.roleplayPersonaKind = "character";
    button.dataset.roleplayCharacterId = "rpc_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";
    button.dataset.roleplayContributionKind = "action";
    button.dataset.roleplayTurnParts = JSON.stringify([{ kind: "action", text: "Continue exactly." }]);

    expect(() => controller.restoreFailedTurn({
        preventDefault: vi.fn(),
        currentTarget: button,
      } as unknown as Event),
    ).toThrow("The failed turn's acting character is no longer eligible in this scene.");

    expect(controller.roleplayPersonaTarget.value).toBe("rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
    expect(controller.inputTarget.value).toBe("Keep this newer draft.");
    expect(controller.setStatus).toHaveBeenCalledWith(
      "The failed turn's acting character is no longer eligible in this scene.",
      "error",
    );
    expect(controller.focusComposer).not.toHaveBeenCalled();
  });
});

describe("ChatRoleplayTurnController composer setup routing", () => {
	it("acknowledges a committed identity when its server controls did not refresh", async () => {
		const characterID = "rpc_abcdef0123456789abcdef0123456789";
		const name = document.createElement("input");
		name.value = "Orchid Cartographer";
		const creator = document.createElement("span");
		creator.classList.add("flex");
		const persona = document.createElement("select");
		persona.innerHTML = '<option value="narrator">Narrator</option>';
		const controller = Object.assign(Object.create(ChatRoleplayTurnController.prototype), {
			hasRoleplayNewPersonaTarget: true,
			roleplayNewPersonaTarget: name,
			hasRoleplayPersonaCreatorTarget: true,
			roleplayPersonaCreatorTarget: creator,
			hasRoleplayPersonaTarget: true,
			roleplayPersonaTarget: persona,
			roleplay: { createUserPersona: vi.fn(async () => ({
				channelID: "story-42",
				characterID,
				projection: "failed" as const,
			})) },
			channel: { selectedID: () => "story-42" },
			setStatus: vi.fn(),
			focusComposer: vi.fn(),
		}) as ChatRoleplayTurnController;

		await expect(controller.createRoleplayPersona({
			preventDefault: vi.fn(),
		} as unknown as Event)).resolves.toBeUndefined();

		expect(name.value).toBe("");
		expect(creator.classList.contains("hidden")).toBe(true);
		expect(persona.value).toBe("narrator");
		expect(controller.setStatus).not.toHaveBeenCalled();
		expect(controller.focusComposer).not.toHaveBeenCalled();
	});

	it("selects the exact committed identity only after its authoritative projection applied", async () => {
		const characterID = "rpc_abcdef0123456789abcdef0123456789";
		const name = document.createElement("input");
		name.value = "Orchid Cartographer";
		const creator = document.createElement("span");
		creator.classList.add("flex");
		const persona = document.createElement("select");
		persona.innerHTML = `<option value="narrator">Narrator</option><option value="${characterID}">Orchid Cartographer</option>`;
		const controller = Object.assign(Object.create(ChatRoleplayTurnController.prototype), {
			hasRoleplayNewPersonaTarget: true,
			roleplayNewPersonaTarget: name,
			hasRoleplayPersonaCreatorTarget: true,
			roleplayPersonaCreatorTarget: creator,
			hasRoleplayPersonaTarget: true,
			roleplayPersonaTarget: persona,
			roleplay: { createUserPersona: vi.fn(async () => ({
				channelID: "story-42",
				characterID,
				projection: "applied" as const,
			})) },
			channel: { selectedID: () => "story-42" },
			focusComposer: vi.fn(),
		}) as ChatRoleplayTurnController;

		await controller.createRoleplayPersona({ preventDefault: vi.fn() } as unknown as Event);

		expect(name.value).toBe("");
		expect(creator.classList.contains("hidden")).toBe(true);
		expect(persona.value).toBe(characterID);
		expect(controller.focusComposer).toHaveBeenCalledOnce();
	});

	it("preserves a new-channel draft when an old-channel receipt settles later", async () => {
		const characterID = "rpc_abcdef0123456789abcdef0123456789";
		const oldName = document.createElement("input");
		oldName.value = "Orchid Cartographer";
		const oldCreator = document.createElement("span");
		oldCreator.classList.add("flex");
		const oldPersona = document.createElement("select");
		oldPersona.innerHTML = '<option value="narrator">Narrator</option>';
		let resolveCreation!: (result: {
			channelID: string;
			characterID: string;
			projection: "invalidated";
		}) => void;
		const creation = new Promise<{
			channelID: string;
			characterID: string;
			projection: "invalidated";
		}>((resolve) => { resolveCreation = resolve; });
		let selectedChannelID = "story-42";
		const controller = Object.assign(Object.create(ChatRoleplayTurnController.prototype), {
			hasRoleplayNewPersonaTarget: true,
			roleplayNewPersonaTarget: oldName,
			hasRoleplayPersonaCreatorTarget: true,
			roleplayPersonaCreatorTarget: oldCreator,
			hasRoleplayPersonaTarget: true,
			roleplayPersonaTarget: oldPersona,
			roleplay: { createUserPersona: vi.fn(() => creation) },
			channel: { selectedID: () => selectedChannelID },
			setStatus: vi.fn(),
			focusComposer: vi.fn(),
		}) as ChatRoleplayTurnController;

		const pending = controller.createRoleplayPersona({ preventDefault: vi.fn() } as unknown as Event);
		selectedChannelID = "story-43";
		const newName = document.createElement("input");
		newName.value = "New World Cartographer";
		const newCreator = document.createElement("span");
		newCreator.classList.add("flex");
		const newPersona = document.createElement("select");
		newPersona.innerHTML = '<option value="narrator">Narrator</option>';
		Object.assign(controller, {
			roleplayNewPersonaTarget: newName,
			roleplayPersonaCreatorTarget: newCreator,
			roleplayPersonaTarget: newPersona,
		});
		resolveCreation({ channelID: "story-42", characterID, projection: "invalidated" });
		await pending;

		expect(newName.value).toBe("New World Cartographer");
		expect(newCreator.classList.contains("hidden")).toBe(false);
		expect(newPersona.value).toBe("narrator");
		expect(controller.setStatus).not.toHaveBeenCalled();
		expect(controller.focusComposer).not.toHaveBeenCalled();
	});

	it.each(["failed", "applied"] as const)(
		"preserves a newer same-channel draft after a committed %s projection",
		async (projection) => {
			const characterID = "rpc_abcdef0123456789abcdef0123456789";
			const name = document.createElement("input");
			name.value = "Orchid Cartographer";
			const creator = document.createElement("span");
			creator.classList.add("flex");
			const persona = document.createElement("select");
			persona.innerHTML = `<option value="narrator">Narrator</option><option value="${characterID}">Orchid Cartographer</option>`;
			let resolveCreation!: (result: {
				channelID: string;
				characterID: string;
				projection: typeof projection;
			}) => void;
			const creation = new Promise<{
				channelID: string;
				characterID: string;
				projection: typeof projection;
			}>((resolve) => { resolveCreation = resolve; });
			const controller = Object.assign(Object.create(ChatRoleplayTurnController.prototype), {
				hasRoleplayNewPersonaTarget: true,
				roleplayNewPersonaTarget: name,
				hasRoleplayPersonaCreatorTarget: true,
				roleplayPersonaCreatorTarget: creator,
				hasRoleplayPersonaTarget: true,
				roleplayPersonaTarget: persona,
				roleplay: { createUserPersona: vi.fn(() => creation) },
				channel: { selectedID: () => "story-42" },
				focusComposer: vi.fn(),
			}) as ChatRoleplayTurnController;

			const pending = controller.createRoleplayPersona({ preventDefault: vi.fn() } as unknown as Event);
			name.value = "Second Cartographer";
			resolveCreation({ channelID: "story-42", characterID, projection });
			await pending;

			expect(name.value).toBe("Second Cartographer");
			expect(creator.classList.contains("hidden")).toBe(false);
			expect(persona.value).toBe(projection === "applied" ? characterID : "narrator");
			expect(controller.focusComposer).not.toHaveBeenCalled();
		},
	);

	it("does not refocus a new-channel draft after an old-channel precommit failure", async () => {
		const oldName = document.createElement("input");
		oldName.value = "Orchid Cartographer";
		let rejectCreation!: (error: Error) => void;
		const creation = new Promise<never>((_resolve, reject) => { rejectCreation = reject; });
		let selectedChannelID = "story-42";
		const controller = Object.assign(Object.create(ChatRoleplayTurnController.prototype), {
			hasRoleplayNewPersonaTarget: true,
			roleplayNewPersonaTarget: oldName,
			roleplay: { createUserPersona: vi.fn(() => creation) },
			channel: { selectedID: () => selectedChannelID },
		}) as ChatRoleplayTurnController;

		const pending = controller.createRoleplayPersona({ preventDefault: vi.fn() } as unknown as Event);
		selectedChannelID = "story-43";
		const newName = document.createElement("input");
		newName.value = "New World Cartographer";
		const focus = vi.spyOn(newName, "focus");
		Object.assign(controller, { roleplayNewPersonaTarget: newName });
		rejectCreation(new Error("request was not committed"));
		await pending;

		expect(newName.value).toBe("New World Cartographer");
		expect(focus).not.toHaveBeenCalled();
	});

  it("keeps a failed inline identity draft without an unhandled rejection", async () => {
    const name = document.createElement("input");
    name.value = "Orchid Cartographer";
    const creator = document.createElement("span");
    creator.classList.add("flex");
    document.body.append(creator, name);
    const createUserPersona = vi.fn(async () => {
      throw new Error("scene participant bound reached");
    });
    const controller = Object.assign(Object.create(ChatRoleplayTurnController.prototype), {
      hasRoleplayNewPersonaTarget: true,
      roleplayNewPersonaTarget: name,
      hasRoleplayPersonaCreatorTarget: true,
      roleplayPersonaCreatorTarget: creator,
      roleplay: { createUserPersona },
		channel: { selectedID: () => "story-42" },
      focusComposer: vi.fn(),
    }) as ChatRoleplayTurnController;

    await expect(controller.createRoleplayPersona({
      preventDefault: vi.fn(),
    } as unknown as Event)).resolves.toBeUndefined();

    expect(createUserPersona).toHaveBeenCalledWith("Orchid Cartographer");
    expect(name.value).toBe("Orchid Cartographer");
    expect(creator.classList.contains("hidden")).toBe(false);
    expect(document.activeElement).toBe(name);
    document.body.replaceChildren();
  });

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
