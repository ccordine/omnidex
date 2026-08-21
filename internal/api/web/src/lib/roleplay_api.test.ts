import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  configureRoleplayResearch,
	createRoleplayScene,
  fetchRoleplayComponent,
  placeRoleplayLibraryCharacter,
  registerRoleplayItem,
	updateRoleplayScene,
  writeRoleplaySceneDraftParticipant,
	writeRoleplayGeneration,
  writeRoleplayPersona,
} from "./roleplay_api";

const channelID = "story-42";
const worldID = "rpw_0123456789abcdef0123456789abcdef";
const characterID = "rpc_0123456789abcdef0123456789abcdef";

function response(configured = true, status = 200): Response {
  return new Response(JSON.stringify({
    channel_id: channelID,
    world_id: worldID,
		configured,
		scene_draft_revision: 3,
    ...(configured ? { scene_revision: 7 } : {}),
    html: { bundle: '<template data-recyclr-target="roleplay-simulation">server</template>' },
  }), { status, headers: { "Content-Type": "application/json" } });
}

describe("roleplay simulation API", () => {
  beforeEach(() => vi.restoreAllMocks());

  it("loads only exact server pages and accepts the server component bundle", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => response());
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchRoleplayComponent(channelID, {
      characters: 4,
      personas: 8,
      turn_order: 0,
      meters: 4,
      inventory: 12,
		interactions: 0,
		item_templates: 16,
    })).resolves.toMatchObject({ channel_id: channelID, configured: true, scene_revision: 7 });

    const url = new URL(String(fetchMock.mock.calls[0]?.[0]), "https://omni.test");
    expect(url.pathname).toBe("/v1/ui/chat/roleplay");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      channel_id: channelID,
      characters_offset: "4",
      personas_offset: "8",
      turn_order_offset: "0",
      meters_offset: "4",
      inventory_offset: "12",
		interactions_offset: "0",
		item_templates_offset: "16",
	});
  });

	it("submits scene creation from one observed server draft and scene updates from one observed scene revision", async () => {
		const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) =>
			response(true, init?.method === "PUT" ? 200 : 201));
		vi.stubGlobal("fetch", fetchMock);

		await createRoleplayScene(channelID, {
			expected_draft_revision: 5,
			title: "Signal Room",
			description: "A bounded scene.",
			participant_ids: [characterID],
		});
		await updateRoleplayScene(channelID, 7, {
			expected_draft_revision: 6,
			title: "Signal Room II",
			description: "A revised bounded scene.",
			participant_ids: [characterID],
		});

		expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
			expected_draft_revision: 5,
			title: "Signal Room",
			description: "A bounded scene.",
			participant_ids: [characterID],
		});
		expect(JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit).body))).toEqual({
			expected_revision: 7,
			expected_draft_revision: 6,
			title: "Signal Room II",
			description: "A revised bounded scene.",
			participant_ids: [characterID],
		});
	});

	it("persists one observed scene-draft selection and character-page authority", async () => {
		const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => response(false, 200));
		vi.stubGlobal("fetch", fetchMock);

		await writeRoleplaySceneDraftParticipant(channelID, characterID, {
			expected_revision: 4, selected: true, characters_offset: 8,
		});

		expect(String(fetchMock.mock.calls[0]?.[0])).toContain(`/scene-draft/participants/${characterID}`);
		expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
			expected_revision: 4, selected: true, characters_offset: 8,
		});
	});

  it("places one canonical library character into the selected world", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => response(false, 201));
    vi.stubGlobal("fetch", fetchMock);

    const libraryID = "rpl_abcdefabcdefabcdefabcdefabcdefab";
    await placeRoleplayLibraryCharacter(channelID, libraryID);

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe(`/v1/channels/${channelID}/roleplay/library/${libraryID}`);
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(init.body).toBeUndefined();
  });

  it("writes a character sheet with its exact expected revision", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => response(true, 200));
    vi.stubGlobal("fetch", fetchMock);

    await writeRoleplayPersona(channelID, characterID, {
      expected_revision: 3,
      summary: "Archivist",
      voice: "Measured",
      traits: ["Patient"],
      goals: ["Understand"],
    });

    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("PUT");
    expect(JSON.parse(String(init.body))).toEqual({
      expected_revision: 3,
      summary: "Archivist",
      voice: "Measured",
      traits: ["Patient"],
      goals: ["Understand"],
    });
  });

  it("configures only the exact route character without browser page authority", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => response(true, 200));
    vi.stubGlobal("fetch", fetchMock);

    await configureRoleplayResearch(channelID, characterID, { enabled: true });

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe(
      `/v1/channels/${channelID}/roleplay/capabilities/${characterID}/web-research`,
    );
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("PUT");
    expect(JSON.parse(String(init.body))).toEqual({ enabled: true });
  });

	it("persists the exact installed response model for one character", async () => {
		const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => response(true, 200));
		vi.stubGlobal("fetch", fetchMock);

		await writeRoleplayGeneration(channelID, characterID, {
			expected_revision: 3,
			narrative_model: "dolphin3:latest",
		});

		expect(String(fetchMock.mock.calls[0]?.[0])).toBe(
			`/v1/channels/${channelID}/roleplay/generation/${characterID}`,
		);
		const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
		expect(init.method).toBe("PUT");
		expect(JSON.parse(String(init.body))).toEqual({
			expected_revision: 3,
			narrative_model: "dolphin3:latest",
		});
	});

  it("requires explicit finite/infinite item authority", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => response(true, 201));
    vi.stubGlobal("fetch", fetchMock);

    await registerRoleplayItem(channelID, {
      name: "Cipher Lens",
      description: "Decodes a bounded signal.",
      use_policy: "finite",
      initial_uses: 2,
      trigger: { meter_key: "signal", direction: "at_or_above", threshold: 8 },
      priority: 4,
      effects: [{ meter_key: "signal", delta: -2 }],
    });

    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(init.body))).toEqual({
      name: "Cipher Lens",
      description: "Decodes a bounded signal.",
      use_policy: "finite",
      initial_uses: 2,
      trigger: { meter_key: "signal", direction: "at_or_above", threshold: 8 },
      priority: 4,
      effects: [{ meter_key: "signal", delta: -2 }],
    });
  });

  it("rejects contradictory state before dispatch", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchRoleplayComponent(channelID, {
      characters: 1,
      personas: 0,
      turn_order: 0,
      meters: 0,
      inventory: 0,
		interactions: 0,
		item_templates: 0,
    })).rejects.toThrow("server page size");
    expect(() => registerRoleplayItem(channelID, {
      name: "Permanent seal",
      description: "No finite uses.",
      use_policy: "infinite",
      initial_uses: 2,
      trigger: null,
      priority: 0,
      effects: [{ meter_key: "signal", delta: 1 }],
    })).toThrow("infinite items require zero uses");
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
