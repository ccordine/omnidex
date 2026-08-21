import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchRoleplayCharacterEditor } from "./roleplay_character_editor_api";

const channelID = "story-world";
const characterID = "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

function response(overrides: Record<string, unknown> = {}): Response {
  return new Response(JSON.stringify({
    channel_id: channelID,
    world_id: "rpw_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    character_id: characterID,
    html: {
      bundle: '<template data-recyclr-target="roleplay-character-editor">Mara</template>',
    },
    ...overrides,
  }), { status: 200, headers: { "Content-Type": "application/json" } });
}

describe("roleplay character editor API", () => {
  beforeEach(() => vi.restoreAllMocks());

  it("loads one exact channel character through the focused server component route", async () => {
    const fetchMock = vi.fn(async () => response());
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchRoleplayCharacterEditor(channelID, characterID)).resolves.toMatchObject({
      channel_id: channelID,
      character_id: characterID,
      world_id: "rpw_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      `/v1/ui/roleplay/character?channel_id=${channelID}&character_id=${characterID}`,
    );
  });

  it("rejects a mismatched or missing response character authority", async () => {
    const fetchMock = vi.fn(async () => response({
      character_id: "rpc_cccccccccccccccccccccccccccccccc",
    }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchRoleplayCharacterEditor(channelID, characterID)).rejects.toThrow(
      "changed its requested authority",
    );
    fetchMock.mockResolvedValueOnce(response({ character_id: undefined }));
    await expect(fetchRoleplayCharacterEditor(channelID, characterID)).rejects.toThrow(
      "response character identity is invalid",
    );
  });

  it("rejects invalid identities before issuing transport", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchRoleplayCharacterEditor(channelID, "Mara")).rejects.toThrow(
      "character identity is invalid",
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
