import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  configureRoleplayResearch,
  writeRoleplayGeneration,
  writeRoleplayPersona,
  type RoleplayComponentResponse,
} from "./roleplay_api";
import { fetchRoleplayCharacterEditor } from "./roleplay_character_editor_api";
import {
  RoleplayCharacterEditorCoordinator,
  type RoleplayCharacterEditorHost,
} from "./roleplay_character_editor_coordinator";

vi.mock("./roleplay_api", () => ({
  configureRoleplayResearch: vi.fn(),
  writeRoleplayGeneration: vi.fn(),
  writeRoleplayPersona: vi.fn(),
}));
vi.mock("./roleplay_character_editor_api", () => ({ fetchRoleplayCharacterEditor: vi.fn() }));

const channelID = "story-world";
const characterID = "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

function component(marker: string): RoleplayComponentResponse {
  return {
    channel_id: channelID,
    world_id: "rpw_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    configured: true,
    scene_revision: 7,
    scene_draft_revision: 4,
    html: { bundle: `<template>${marker}</template>` },
  };
}

function editor(marker: string) {
  return { ...component(marker), character_id: characterID };
}

function fixture() {
  let selected = channelID;
  let open = false;
  const host: RoleplayCharacterEditorHost = {
    selectedChannelID: () => selected,
    renderComponentBundle: vi.fn(async () => undefined),
    refreshRoleplay: vi.fn(async () => undefined),
    openDialog: vi.fn(() => { open = true; }),
    closeDialog: vi.fn(() => { open = false; }),
    closeDialogFromBackdrop: vi.fn((event) => {
      if (event.target === event.currentTarget) open = false;
    }),
    dialogIsOpen: () => open,
    setStatus: vi.fn(),
    addEvent: vi.fn(),
    reportError: vi.fn(),
  };
  return { host, selected: (value: string) => { selected = value; } };
}

function characterButton(id = characterID): HTMLButtonElement {
  const button = document.createElement("button");
  button.dataset.roleplayCharacterId = id;
  return button;
}

function eventFor(element: Element): Event {
  return { currentTarget: element, preventDefault: vi.fn() } as unknown as Event;
}

describe("RoleplayCharacterEditorCoordinator", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.mocked(fetchRoleplayCharacterEditor).mockResolvedValue(editor("editor"));
  });

  it("loads and renders the exact clicked character before opening its modal", async () => {
    const value = fixture();
    const order: string[] = [];
    vi.mocked(value.host.renderComponentBundle).mockImplementation(async () => { order.push("render"); });
    vi.mocked(value.host.openDialog).mockImplementation(() => { order.push("open"); });
    const coordinator = new RoleplayCharacterEditorCoordinator(value.host);

    await coordinator.open(eventFor(characterButton()));

    expect(fetchRoleplayCharacterEditor).toHaveBeenCalledWith(channelID, characterID);
    expect(value.host.renderComponentBundle).toHaveBeenCalledWith(expect.stringContaining("editor"));
    expect(order).toEqual(["render", "open"]);
    expect(value.host.addEvent).toHaveBeenCalledWith("roleplay_character_editor_opened", {
      channel_id: channelID,
      character_id: characterID,
    });
  });

  it("saves persona bytes and rehydrates the same character after world reconciliation", async () => {
    const value = fixture();
    const coordinator = new RoleplayCharacterEditorCoordinator(value.host);
    await coordinator.open(eventFor(characterButton()));
    vi.mocked(writeRoleplayPersona).mockResolvedValue(component("accepted-persona"));
    vi.mocked(fetchRoleplayCharacterEditor).mockResolvedValueOnce(editor("rehydrated-persona"));
    const form = document.createElement("form");
    form.dataset.characterId = characterID;
    form.innerHTML = `
      <input name="expected_revision" value="3">
      <textarea name="summary">Archive keeper</textarea>
      <textarea name="voice">Dry and exact</textarea>
      <textarea name="traits">watchful\npatient</textarea>
      <textarea name="goals">protect the archive</textarea>
      <button type="submit">Save</button>
    `;

    await coordinator.savePersona(eventFor(form));

    expect(writeRoleplayPersona).toHaveBeenCalledWith(channelID, characterID, {
      expected_revision: 3,
      summary: "Archive keeper",
      voice: "Dry and exact",
      traits: ["watchful", "patient"],
      goals: ["protect the archive"],
    });
    expect(value.host.refreshRoleplay).toHaveBeenCalledOnce();
    expect(fetchRoleplayCharacterEditor).toHaveBeenLastCalledWith(channelID, characterID);
    expect(value.host.renderComponentBundle).toHaveBeenLastCalledWith(expect.stringContaining("rehydrated-persona"));
  });

  it("saves exact research and installed-model selections through their existing APIs", async () => {
    const value = fixture();
    const coordinator = new RoleplayCharacterEditorCoordinator(value.host);
    await coordinator.open(eventFor(characterButton()));
    vi.mocked(configureRoleplayResearch).mockResolvedValue(component("research"));
    vi.mocked(writeRoleplayGeneration).mockResolvedValue(component("generation"));

    const research = document.createElement("form");
    research.dataset.characterId = characterID;
    research.innerHTML = '<input name="enabled" type="checkbox" checked>';
    await coordinator.saveResearch(eventFor(research));

    const generation = document.createElement("form");
    generation.dataset.characterId = characterID;
    generation.innerHTML = `
      <input name="expected_revision" value="5">
      <select name="narrative_model"><option value="qwen3.5:9b" selected>qwen3.5:9b</option></select>
    `;
    await coordinator.saveGeneration(eventFor(generation));

    expect(configureRoleplayResearch).toHaveBeenCalledWith(channelID, characterID, {
      enabled: true,
    });
    expect(writeRoleplayGeneration).toHaveBeenCalledWith(channelID, characterID, {
      expected_revision: 5,
      narrative_model: "qwen3.5:9b",
    });
    expect(value.host.refreshRoleplay).toHaveBeenCalledTimes(2);
  });

  it("discards an editor response after the request is closed or the selected world changes", async () => {
    const value = fixture();
    let resolve!: (value: ReturnType<typeof editor>) => void;
    vi.mocked(fetchRoleplayCharacterEditor).mockReturnValueOnce(new Promise((done) => { resolve = done; }));
    const coordinator = new RoleplayCharacterEditorCoordinator(value.host);
    const opening = coordinator.open(eventFor(characterButton()));

    coordinator.close(eventFor(document.createElement("button")));
    value.selected("another-world");
    resolve(editor("stale"));
    await opening;

    expect(value.host.renderComponentBundle).not.toHaveBeenCalled();
    expect(value.host.openDialog).not.toHaveBeenCalled();
  });

  it("rejects a form belonging to a different character without issuing a mutation", async () => {
    const value = fixture();
    const coordinator = new RoleplayCharacterEditorCoordinator(value.host);
    await coordinator.open(eventFor(characterButton()));
    const form = document.createElement("form");
    form.dataset.characterId = "rpc_cccccccccccccccccccccccccccccccc";
    form.innerHTML = `
      <input name="expected_revision" value="3">
      <textarea name="summary">Wrong authority</textarea>
      <textarea name="voice"></textarea><textarea name="traits"></textarea><textarea name="goals"></textarea>
    `;

    await coordinator.savePersona(eventFor(form));
    expect(writeRoleplayPersona).not.toHaveBeenCalled();
    expect(value.host.setStatus).toHaveBeenCalledWith("character update failed", "error");
    expect(value.host.reportError).toHaveBeenCalledWith(expect.objectContaining({
      message: expect.stringContaining("does not match"),
    }));
  });
});
