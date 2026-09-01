import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createRoleplayUserPersona,
  fetchRoleplayComponent,
  updateRoleplayResponders,
  type RoleplayComponentResponse,
  type RoleplayUserPersonaCreationReceipt,
} from "./roleplay_api";
import { ChatRoleplayCoordinator, type ChatRoleplayHost } from "./chat_roleplay_coordinator";

vi.mock("./roleplay_api", () => ({
  emptyRoleplayPage: { characters: 0, personas: 0, turn_order: 0, meters: 0, inventory: 0, interactions: 0, item_templates: 0 },
  createRoleplayUserPersona: vi.fn(),
  fetchRoleplayComponent: vi.fn(),
  createRoleplayScene: vi.fn(),
  registerRoleplayInteraction: vi.fn(),
  registerRoleplayItem: vi.fn(),
  registerRoleplayMeter: vi.fn(),
  setRoleplayMeter: vi.fn(),
  updateRoleplayResponders: vi.fn(),
  updateRoleplayScene: vi.fn(),
  writeRoleplaySceneDraftParticipant: vi.fn(),
}));

const firstChannelID = "story-42";
const secondChannelID = "story-43";
const personaID = "rpc_abcdef0123456789abcdef0123456789";
const responderID = "rpc_0123456789abcdef0123456789abcdef";

function component(
  channelID: string,
  marker: string,
): RoleplayComponentResponse {
  return {
    channel_id: channelID,
    world_id: "rpw_0123456789abcdef0123456789abcdef",
    configured: true,
    scene_revision: 4,
    scene_draft_revision: 2,
    html: { bundle: `<template data-recyclr-target="roleplay-simulation">${marker}</template>` },
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((accept) => { resolve = accept; });
  return { promise, resolve };
}

function createHost() {
  const panel = document.createElement("aside");
  const loading = document.createElement("div");
  const host: ChatRoleplayHost = {
    hasPanel: () => true,
    panel: () => panel,
    hasLoading: () => true,
    loading: () => loading,
    renderComponentBundle: vi.fn(async () => undefined),
    setComposerAvailable: vi.fn(),
    setComposerText: vi.fn(),
    focusComposer: vi.fn(),
    setStatus: vi.fn(),
    addEvent: vi.fn(),
    reportError: vi.fn(),
    refreshSlashCommands: vi.fn(async () => undefined),
  };
  return { host };
}

describe("ChatRoleplayCoordinator mutation serialization", () => {
  beforeEach(() => vi.resetAllMocks());

  it.each([
    { name: "creation response first", resolveLaterFirst: false },
    { name: "later response first", resolveLaterFirst: true },
  ])("keeps the later mutation authoritative with $name", async ({ resolveLaterFirst }) => {
    vi.mocked(fetchRoleplayComponent)
      .mockResolvedValueOnce(component(firstChannelID, "initial"))
      .mockResolvedValueOnce(component(firstChannelID, "committed-persona"));
    const creationResponse = deferred<RoleplayUserPersonaCreationReceipt>();
    const responderResponse = deferred<RoleplayComponentResponse>();
    vi.mocked(createRoleplayUserPersona).mockReturnValueOnce(creationResponse.promise);
    vi.mocked(updateRoleplayResponders).mockReturnValueOnce(responderResponse.promise);
    const fixture = createHost();
    const coordinator = new ChatRoleplayCoordinator(fixture.host);
    await coordinator.activate(firstChannelID, "roleplay");

    const creation = coordinator.createUserPersona("Orchid Cartographer");
    const laterMutation = coordinator.updateResponders([responderID], 4);
    await vi.waitFor(() => expect(createRoleplayUserPersona).toHaveBeenCalledOnce());
    expect(updateRoleplayResponders).not.toHaveBeenCalled();

    if (resolveLaterFirst) {
      responderResponse.resolve(component(firstChannelID, "later-responders"));
    }
    creationResponse.resolve({ channel_id: firstChannelID, character_id: personaID });
    await vi.waitFor(() => expect(updateRoleplayResponders).toHaveBeenCalledOnce());
    if (!resolveLaterFirst) {
      responderResponse.resolve(component(firstChannelID, "later-responders"));
    }

    await expect(creation).resolves.toEqual({
      channelID: firstChannelID,
      characterID: personaID,
      projection: "applied",
    });
    await laterMutation;
    expect(createRoleplayUserPersona).toHaveBeenCalledTimes(1);
    expect(updateRoleplayResponders).toHaveBeenCalledTimes(1);
    expect(fetchRoleplayComponent).toHaveBeenLastCalledWith(
      firstChannelID, expect.anything(), personaID,
    );
    const rendered = vi.mocked(fixture.host.renderComponentBundle).mock.calls
      .map(([bundle]) => bundle);
    expect(rendered.slice(-2)).toEqual([
      expect.stringContaining("committed-persona"),
      expect.stringContaining("later-responders"),
    ]);
    expect(fixture.host.addEvent).toHaveBeenCalledWith("roleplay_user_persona_created", {
      channel_id: firstChannelID,
      character_id: personaID,
    });
    expect(fixture.host.reportError).not.toHaveBeenCalled();
  });

  it("invalidates queued and in-flight mutations after a real channel change", async () => {
    vi.mocked(fetchRoleplayComponent)
      .mockResolvedValueOnce(component(firstChannelID, "first-channel"))
      .mockResolvedValueOnce(component(secondChannelID, "second-channel"));
    const creationResponse = deferred<RoleplayUserPersonaCreationReceipt>();
    vi.mocked(createRoleplayUserPersona).mockReturnValueOnce(creationResponse.promise);
    const fixture = createHost();
    const coordinator = new ChatRoleplayCoordinator(fixture.host);
    await coordinator.activate(firstChannelID, "roleplay");

    const creation = coordinator.createUserPersona("Orchid Cartographer");
    const queuedMutation = coordinator.updateResponders([responderID], 4);
    await vi.waitFor(() => expect(createRoleplayUserPersona).toHaveBeenCalledOnce());
    expect(updateRoleplayResponders).not.toHaveBeenCalled();
    await coordinator.activate(secondChannelID, "roleplay");
    creationResponse.resolve({ channel_id: firstChannelID, character_id: personaID });

    await expect(creation).resolves.toEqual({
      channelID: firstChannelID,
      characterID: personaID,
      projection: "invalidated",
    });
    await queuedMutation;
    expect(createRoleplayUserPersona).toHaveBeenCalledTimes(1);
    expect(updateRoleplayResponders).not.toHaveBeenCalled();
    expect(fixture.host.renderComponentBundle).toHaveBeenLastCalledWith(
      expect.stringContaining("second-channel"),
    );
    expect(fixture.host.addEvent).toHaveBeenCalledWith(
      "roleplay_user_persona_projection_invalidated",
      { channel_id: firstChannelID, character_id: personaID, committed: true },
    );
  });

  it("keeps an exact committed persona through an intervening same-channel refresh", async () => {
    vi.mocked(fetchRoleplayComponent)
      .mockResolvedValueOnce(component(firstChannelID, "initial"))
      .mockResolvedValueOnce(component(firstChannelID, "realtime-refresh"))
      .mockResolvedValueOnce(component(firstChannelID, "committed-persona"));
    const creationResponse = deferred<RoleplayUserPersonaCreationReceipt>();
    vi.mocked(createRoleplayUserPersona).mockReturnValueOnce(creationResponse.promise);
    const fixture = createHost();
    const coordinator = new ChatRoleplayCoordinator(fixture.host);
    await coordinator.activate(firstChannelID, "roleplay");

    const creation = coordinator.createUserPersona("Orchid Cartographer");
    await coordinator.refresh();
    creationResponse.resolve({ channel_id: firstChannelID, character_id: personaID });

    await expect(creation).resolves.toEqual({
      channelID: firstChannelID,
      characterID: personaID,
      projection: "applied",
    });
    expect(createRoleplayUserPersona).toHaveBeenCalledOnce();
    expect(fetchRoleplayComponent).toHaveBeenLastCalledWith(
      firstChannelID, expect.anything(), personaID,
    );
    expect(fixture.host.renderComponentBundle).toHaveBeenLastCalledWith(
      expect.stringContaining("committed-persona"),
    );
    expect(fixture.host.addEvent).not.toHaveBeenCalledWith(
      "roleplay_mutation_failed", expect.anything(),
    );
  });

  it("acknowledges the committed receipt when its component refresh fails", async () => {
    const projectionError = new Error("component projection unavailable");
    vi.mocked(fetchRoleplayComponent)
      .mockResolvedValueOnce(component(firstChannelID, "initial"))
      .mockRejectedValueOnce(projectionError);
    vi.mocked(createRoleplayUserPersona).mockResolvedValueOnce({
      channel_id: firstChannelID,
      character_id: personaID,
    });
    const fixture = createHost();
    const coordinator = new ChatRoleplayCoordinator(fixture.host);
    await coordinator.activate(firstChannelID, "roleplay");

    await expect(coordinator.createUserPersona("Orchid Cartographer")).resolves.toEqual({
      channelID: firstChannelID,
      characterID: personaID,
      projection: "failed",
    });

    expect(createRoleplayUserPersona).toHaveBeenCalledTimes(1);
    expect(fixture.host.addEvent).toHaveBeenCalledWith("roleplay_user_persona_created", {
      channel_id: firstChannelID,
      character_id: personaID,
    });
    expect(fixture.host.addEvent).toHaveBeenCalledWith(
      "roleplay_user_persona_projection_failed",
      {
        channel_id: firstChannelID,
        character_id: personaID,
        committed: true,
        error: projectionError.message,
      },
    );
    expect(fixture.host.addEvent).not.toHaveBeenCalledWith(
      "roleplay_mutation_failed", expect.anything(),
    );
    expect(fixture.host.setStatus).toHaveBeenLastCalledWith(
      "identity added; controls refresh failed", "error",
    );
    expect(fixture.host.reportError).toHaveBeenCalledWith(projectionError);
  });

  it("does not expose a retry signal when receipt observation throws", async () => {
    const observationError = new Error("activity recorder unavailable");
    vi.mocked(fetchRoleplayComponent)
      .mockResolvedValueOnce(component(firstChannelID, "initial"))
      .mockResolvedValueOnce(component(firstChannelID, "committed-persona"));
    vi.mocked(createRoleplayUserPersona).mockResolvedValueOnce({
      channel_id: firstChannelID,
      character_id: personaID,
    });
    const fixture = createHost();
    vi.mocked(fixture.host.addEvent).mockImplementation((type) => {
      if (type === "roleplay_user_persona_created") throw observationError;
    });
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const coordinator = new ChatRoleplayCoordinator(fixture.host);
    await coordinator.activate(firstChannelID, "roleplay");

    await expect(coordinator.createUserPersona("Orchid Cartographer")).resolves.toEqual({
      channelID: firstChannelID,
      characterID: personaID,
      projection: "applied",
    });

    expect(createRoleplayUserPersona).toHaveBeenCalledOnce();
    expect(fetchRoleplayComponent).toHaveBeenLastCalledWith(
      firstChannelID, expect.anything(), personaID,
    );
    expect(fixture.host.addEvent).not.toHaveBeenCalledWith(
      "roleplay_mutation_failed", expect.anything(),
    );
    expect(consoleError).toHaveBeenCalledWith(
      "Committed roleplay identity observation failed", observationError,
    );
  });
});
