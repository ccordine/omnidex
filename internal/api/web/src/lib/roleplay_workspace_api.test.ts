import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createRoleplayLibraryCharacter,
  fetchRoleplayLibraryPage,
  fetchRoleplayWorldsPage,
} from "./roleplay_workspace_api";

function page(status = 200, nextOffset?: number): Response {
  return new Response(JSON.stringify({
    has_more: nextOffset !== undefined,
    ...(nextOffset === undefined ? {} : { next_offset: nextOffset }),
    html: { bundle: '<template data-recyclr-target="roleplay-world-list">server</template>' },
  }), { status, headers: { "Content-Type": "application/json" } });
}

describe("roleplay workspace API", () => {
  beforeEach(() => vi.restoreAllMocks());

  it("loads bounded server-rendered world and character pages", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => page(200, 40));
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchRoleplayWorldsPage(20)).resolves.toMatchObject({ has_more: true, next_offset: 40 });
    await expect(fetchRoleplayLibraryPage("story-world", 20)).resolves.toMatchObject({ has_more: true, next_offset: 40 });

    expect(String(fetchMock.mock.calls[0]?.[0])).toBe("/v1/ui/roleplay/worlds?limit=20&offset=20");
    expect(String(fetchMock.mock.calls[1]?.[0])).toBe("/v1/ui/roleplay/library?limit=20&offset=20&channel_id=story-world");
  });

  it("creates one exact library character and rejects inexact names before transport", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => page(201));
    vi.stubGlobal("fetch", fetchMock);

    await createRoleplayLibraryCharacter("Mira", "story-world");
    expect(fetchMock).toHaveBeenCalledWith("/v1/ui/roleplay/library?channel_id=story-world", expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ name: "Mira" }),
    }));
    await expect(createRoleplayLibraryCharacter(" Mira ", "story-world")).rejects.toThrow("exact nonblank");
    expect(fetchMock).toHaveBeenCalledOnce();
  });
});
