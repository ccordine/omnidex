import { beforeEach, describe, expect, it, vi } from "vitest";
import { placeRoleplayLibraryCharacter } from "./roleplay_api";
import {
  createRoleplayLibraryCharacter,
  fetchRoleplayLibraryPage,
  fetchRoleplayWorldsPage,
} from "./roleplay_workspace_api";
import { RoleplayWorkspaceCoordinator, type RoleplayWorkspaceHost } from "./roleplay_workspace_coordinator";

vi.mock("./roleplay_api", () => ({ placeRoleplayLibraryCharacter: vi.fn() }));
vi.mock("./roleplay_workspace_api", () => ({
  createRoleplayLibraryCharacter: vi.fn(),
  fetchRoleplayLibraryPage: vi.fn(),
  fetchRoleplayWorldsPage: vi.fn(),
}));

const channelID = "story-world";
const page = (marker: string) => ({ has_more: false, html: { bundle: `<template>${marker}</template>` } });

function fixture() {
  const loading = document.createElement("div");
  loading.classList.add("hidden");
  let selected = "";
  const host: RoleplayWorkspaceHost = {
    hasLoading: () => true,
    loading: () => loading,
    renderComponentBundle: vi.fn(async () => undefined),
    selectedChannelID: () => selected,
    firstChannelID: () => channelID,
    selectChannelID: vi.fn(async (id) => { selected = id; }),
    createWorld: vi.fn(async () => undefined),
    refreshRoleplay: vi.fn(async () => undefined),
    setStatus: vi.fn(),
    addEvent: vi.fn(),
    reportError: vi.fn(),
  };
  return { host, loading, selected: () => selected };
}

describe("RoleplayWorkspaceCoordinator", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.mocked(fetchRoleplayWorldsPage).mockResolvedValue(page("worlds"));
    vi.mocked(fetchRoleplayLibraryPage).mockResolvedValue(page("characters"));
  });

  it("loads both server lists and selects the first persisted world", async () => {
    const value = fixture();
    const coordinator = new RoleplayWorkspaceCoordinator(value.host);
    await coordinator.activate();

    expect(value.host.renderComponentBundle).toHaveBeenCalledWith(expect.stringContaining("worlds"));
    expect(value.host.renderComponentBundle).toHaveBeenCalledWith(expect.stringContaining("characters"));
    expect(value.host.selectChannelID).toHaveBeenCalledWith(channelID);
		expect(fetchRoleplayLibraryPage).toHaveBeenCalledWith(channelID, 0);
    expect(value.selected()).toBe(channelID);
    expect(value.loading.classList.contains("hidden")).toBe(true);
  });

  it("places a stable library identity and reconciles the server components", async () => {
    const value = fixture();
    await value.host.selectChannelID(channelID);
    vi.mocked(placeRoleplayLibraryCharacter).mockResolvedValue({
      channel_id: channelID,
      world_id: "rpw_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      configured: false,
      scene_draft_revision: 0,
      html: { bundle: "<template>placed</template>" },
    });
    const coordinator = new RoleplayWorkspaceCoordinator(value.host);
    const button = document.createElement("button");
    button.dataset.libraryCharacterId = "rpl_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";

    await coordinator.placeCharacter({ currentTarget: button } as unknown as Event);

    expect(placeRoleplayLibraryCharacter).toHaveBeenCalledWith(channelID, button.dataset.libraryCharacterId);
    expect(value.host.renderComponentBundle).toHaveBeenCalledWith(expect.stringContaining("placed"));
    expect(value.host.refreshRoleplay).toHaveBeenCalledOnce();
    expect(value.host.addEvent).toHaveBeenCalledWith("roleplay_character_placed", expect.objectContaining({ channel_id: channelID }));
  });

  it("creates a library character from a form and clears it only after acceptance", async () => {
    const value = fixture();
    vi.mocked(createRoleplayLibraryCharacter).mockResolvedValue(page("created"));
    const coordinator = new RoleplayWorkspaceCoordinator(value.host);
    const form = document.createElement("form");
    form.innerHTML = '<input name="name" value="Mira"><button type="submit">Create</button>';

    await coordinator.createCharacter({ currentTarget: form, preventDefault: vi.fn() } as unknown as Event);

    expect(createRoleplayLibraryCharacter).toHaveBeenCalledWith("Mira", "");
    expect((form.elements.namedItem("name") as HTMLInputElement).value).toBe("");
  });

  it("keeps rejected character input and exposes the failure through system activity", async () => {
    const value = fixture();
    vi.mocked(createRoleplayLibraryCharacter).mockRejectedValue(new Error("character name already exists"));
    const coordinator = new RoleplayWorkspaceCoordinator(value.host);
    const form = document.createElement("form");
    form.innerHTML = '<input name="name" value="Mira"><button type="submit">Create</button>';

    await coordinator.createCharacter({ currentTarget: form, preventDefault: vi.fn() } as unknown as Event);

    expect((form.elements.namedItem("name") as HTMLInputElement).value).toBe("Mira");
    expect(value.host.setStatus).toHaveBeenCalledWith("roleplay update failed", "error");
    expect(value.host.addEvent).toHaveBeenCalledWith("roleplay_workspace_failed", {
      error: "character name already exists",
    });
    expect(value.host.reportError).toHaveBeenCalledWith(expect.any(Error));
  });
});
