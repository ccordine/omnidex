import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  configureRoleplayResearch,
  createRoleplayCharacter,
	fetchRoleplayComponent,
	updateRoleplayScene,
  type RoleplayComponentResponse,
} from "./roleplay_api";
import { HTTPResponseError } from "./api";
import { ChatRoleplayCoordinator, type ChatRoleplayHost } from "./chat_roleplay_coordinator";

vi.mock("./roleplay_api", () => ({
	emptyRoleplayPage: { characters: 0, personas: 0, turn_order: 0, meters: 0, inventory: 0, interactions: 0, item_templates: 0 },
  fetchRoleplayComponent: vi.fn(),
  createRoleplayCharacter: vi.fn(),
  createRoleplayScene: vi.fn(),
  configureRoleplayResearch: vi.fn(),
  registerRoleplayInteraction: vi.fn(),
  registerRoleplayItem: vi.fn(),
  registerRoleplayMeter: vi.fn(),
	setRoleplayMeter: vi.fn(),
	updateRoleplayScene: vi.fn(),
	writeRoleplaySceneDraftParticipant: vi.fn(),
  writeRoleplayPersona: vi.fn(),
}));

const channelID = "story-42";

function component(configured: boolean, marker = "server-state"): RoleplayComponentResponse {
  return {
    channel_id: channelID,
    world_id: "rpw_0123456789abcdef0123456789abcdef",
		configured,
		scene_draft_revision: 2,
    ...(configured ? { scene_revision: 4 } : {}),
    html: { bundle: `<template data-recyclr-target="roleplay-simulation">${marker}</template>` },
  };
}

function createHost() {
  const panel = document.createElement("aside");
  panel.classList.add("hidden");
  const loading = document.createElement("div");
  loading.classList.add("hidden");
  let composerText = "";
  const host: ChatRoleplayHost = {
    hasPanel: () => true,
    panel: () => panel,
    hasLoading: () => true,
    loading: () => loading,
    renderComponentBundle: vi.fn(async () => undefined),
    setComposerAvailable: vi.fn(),
    setComposerText: (value) => { composerText = value; },
    focusComposer: vi.fn(),
    setStatus: vi.fn(),
    addEvent: vi.fn(),
    reportError: vi.fn(),
    refreshSlashCommands: vi.fn(async () => undefined),
  };
  return { host, panel, loading, composerText: () => composerText };
}

describe("ChatRoleplayCoordinator", () => {
  beforeEach(() => vi.resetAllMocks());

  it("blocks the canonical composer until the server reports a configured scene", async () => {
    vi.mocked(fetchRoleplayComponent)
      .mockResolvedValueOnce(component(false, "setup"))
      .mockResolvedValueOnce(component(true, "configured"));
    const fixture = createHost();
    const coordinator = new ChatRoleplayCoordinator(fixture.host);

    await coordinator.activate(channelID, "roleplay");
    expect(coordinator.isConfigured()).toBe(false);
    expect(fixture.panel.classList.contains("hidden")).toBe(false);
    expect(fixture.host.setComposerAvailable).toHaveBeenLastCalledWith(false);
    expect(fixture.host.renderComponentBundle).toHaveBeenLastCalledWith(expect.stringContaining("setup"));

    await coordinator.refresh();
    expect(coordinator.isConfigured()).toBe(true);
    expect(fixture.host.setComposerAvailable).toHaveBeenLastCalledWith(true);
    expect(fixture.host.renderComponentBundle).toHaveBeenLastCalledWith(expect.stringContaining("configured"));
  });

  it("disables configuration controls while awaiting server reconciliation", async () => {
    vi.mocked(fetchRoleplayComponent).mockResolvedValueOnce(component(false));
    let resolveMutation!: (value: RoleplayComponentResponse) => void;
    vi.mocked(createRoleplayCharacter).mockReturnValueOnce(new Promise((resolve) => { resolveMutation = resolve; }));
    const fixture = createHost();
    const coordinator = new ChatRoleplayCoordinator(fixture.host);
    await coordinator.activate(channelID, "roleplay");
    const form = document.createElement("form");
    form.innerHTML = '<input name="name" value="Signal Keeper"><button type="submit">Create</button>';
    const input = form.elements.namedItem("name") as HTMLInputElement;
    const pending = coordinator.createCharacter({
      currentTarget: form,
      preventDefault: vi.fn(),
    } as unknown as Event);

    expect(input.disabled).toBe(true);
    expect(form.getAttribute("aria-busy")).toBe("true");
    expect(fixture.loading.classList.contains("hidden")).toBe(false);

    resolveMutation(component(false, "reconciled"));
    await pending;
    expect(input.disabled).toBe(false);
    expect(form.getAttribute("aria-busy")).toBe("false");
    expect(fixture.host.reportError).not.toHaveBeenCalled();
    expect(fixture.host.renderComponentBundle).toHaveBeenLastCalledWith(expect.stringContaining("reconciled"));
  });

  it("reconciles research access for one server-rendered visible character", async () => {
    vi.mocked(fetchRoleplayComponent).mockResolvedValueOnce(component(true));
    vi.mocked(configureRoleplayResearch).mockResolvedValueOnce(component(true, "research-reconciled"));
    const fixture = createHost();
    const coordinator = new ChatRoleplayCoordinator(fixture.host);
    await coordinator.activate(channelID, "roleplay");
    const form = document.createElement("form");
    form.dataset.characterId = "rpc_0123456789abcdef0123456789abcdef";
    form.dataset.charactersOffset = "8";
    form.innerHTML = '<input name="enabled" type="checkbox" checked><button type="submit">Save</button>';

    await coordinator.configureResearch({
      currentTarget: form,
      preventDefault: vi.fn(),
    } as unknown as Event);

    expect(configureRoleplayResearch).toHaveBeenCalledWith(
      channelID,
      "rpc_0123456789abcdef0123456789abcdef",
      { enabled: true, characters_offset: 8 },
    );
    expect(fixture.host.renderComponentBundle).toHaveBeenLastCalledWith(expect.stringContaining("research-reconciled"));
    expect(fixture.host.refreshSlashCommands).toHaveBeenCalledOnce();
  });

  it("places only exact server-rendered command syntax in the canonical composer", () => {
    const fixture = createHost();
    const coordinator = new ChatRoleplayCoordinator(fixture.host);
    const button = document.createElement("button");
    button.dataset.roleplayCommand = '/calibrate "signal array"';

    coordinator.useCommand({ currentTarget: button } as unknown as Event);

    expect(fixture.composerText()).toBe('/calibrate "signal array"');
    expect(fixture.host.focusComposer).toHaveBeenCalledOnce();
  });

  it("refetches every visible list from exact server page cursors", async () => {
    vi.mocked(fetchRoleplayComponent)
      .mockResolvedValueOnce(component(true))
      .mockResolvedValueOnce(component(true));
    const fixture = createHost();
    const coordinator = new ChatRoleplayCoordinator(fixture.host);
    await coordinator.activate(channelID, "roleplay");
    const button = document.createElement("button");
    button.dataset.roleplayPageSection = "inventory";
    button.dataset.charactersOffset = "4";
    button.dataset.personasOffset = "8";
    button.dataset.turnOrderOffset = "0";
    button.dataset.metersOffset = "4";
    button.dataset.inventoryOffset = "12";
		button.dataset.interactionsOffset = "0";
		button.dataset.itemTemplatesOffset = "16";

    await coordinator.loadPage({ currentTarget: button } as unknown as Event);

    expect(fetchRoleplayComponent).toHaveBeenLastCalledWith(channelID, {
      characters: 4,
      personas: 8,
      turn_order: 0,
      meters: 4,
      inventory: 12,
		interactions: 0,
		item_templates: 16,
	});
  });

	it("drops a reversed older response within the same channel generation", async () => {
		vi.mocked(fetchRoleplayComponent).mockResolvedValueOnce(component(true, "initial"));
		let resolveOlder!: (value: RoleplayComponentResponse) => void;
		let resolveNewer!: (value: RoleplayComponentResponse) => void;
		vi.mocked(fetchRoleplayComponent)
			.mockReturnValueOnce(new Promise((resolve) => { resolveOlder = resolve; }))
			.mockReturnValueOnce(new Promise((resolve) => { resolveNewer = resolve; }));
		const fixture = createHost();
		const coordinator = new ChatRoleplayCoordinator(fixture.host);
		await coordinator.activate(channelID, "roleplay");

		const older = coordinator.refresh();
		const newer = coordinator.refresh();
		resolveNewer(component(true, "newer"));
		await newer;
		resolveOlder(component(true, "older"));
		await older;

		const rendered = vi.mocked(fixture.host.renderComponentBundle).mock.calls.map(([bundle]) => bundle);
		expect(rendered.some((bundle) => bundle.includes("newer"))).toBe(true);
		expect(rendered.some((bundle) => bundle.includes("older"))).toBe(false);
	});

	it("serializes component rendering so an in-flight stale render cannot finish after newer state", async () => {
		vi.mocked(fetchRoleplayComponent)
			.mockResolvedValueOnce(component(true, "initial"))
			.mockResolvedValueOnce(component(true, "older"))
			.mockResolvedValueOnce(component(true, "newer"));
		const fixture = createHost();
		let releaseOlder!: () => void;
		let markOlderStarted!: () => void;
		const olderStarted = new Promise<void>((resolve) => { markOlderStarted = resolve; });
		vi.mocked(fixture.host.renderComponentBundle).mockImplementation(async (bundle) => {
			if (bundle.includes("older")) {
				markOlderStarted();
				await new Promise<void>((resolve) => { releaseOlder = resolve; });
			}
			fixture.panel.textContent = bundle;
		});
		const coordinator = new ChatRoleplayCoordinator(fixture.host);
		await coordinator.activate(channelID, "roleplay");

		const older = coordinator.refresh();
		await olderStarted;
		const newer = coordinator.refresh();
		await Promise.resolve();
		await Promise.resolve();
		releaseOlder();
		await Promise.all([older, newer]);

		expect(fixture.panel.textContent).toContain("newer");
	});

	it("rehydrates server state after a stale revision conflict without applying a stale response", async () => {
		vi.mocked(fetchRoleplayComponent)
			.mockResolvedValueOnce(component(true, "initial"))
			.mockResolvedValueOnce(component(true, "rehydrated"));
		vi.mocked(updateRoleplayScene).mockRejectedValueOnce(new HTTPResponseError(409, "stale scene revision"));
		const fixture = createHost();
		const coordinator = new ChatRoleplayCoordinator(fixture.host);
		await coordinator.activate(channelID, "roleplay");
		const form = document.createElement("form");
		form.dataset.sceneRevision = "4";
		form.innerHTML = `
			<input name="expected_draft_revision" value="2">
			<input name="title" value="Signal Room">
			<textarea name="description">Revised scene.</textarea>
			<input type="hidden" name="participant_id" value="rpc_0123456789abcdef0123456789abcdef">
		`;

		await coordinator.updateScene({ currentTarget: form, preventDefault: vi.fn() } as unknown as Event);

		expect(fetchRoleplayComponent).toHaveBeenCalledTimes(2);
		expect(fixture.host.renderComponentBundle).toHaveBeenLastCalledWith(expect.stringContaining("rehydrated"));
		expect(fixture.host.addEvent).toHaveBeenCalledWith("roleplay_conflict_rehydrated", { channel_id: channelID });
	});

  it("hides the simulation panel and restores the composer for assistant channels", async () => {
    const fixture = createHost();
    fixture.panel.classList.remove("hidden");
    const coordinator = new ChatRoleplayCoordinator(fixture.host);

    await coordinator.activate("assistant-42", "assistant");

    expect(fixture.panel.classList.contains("hidden")).toBe(true);
    expect(fixture.host.setComposerAvailable).toHaveBeenLastCalledWith(true);
    expect(fetchRoleplayComponent).not.toHaveBeenCalled();
  });
});
