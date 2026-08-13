import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  fetchChannelOptionsPage,
  fetchChatMemoryPage,
  fetchChatTimelinePage,
  requireServerComponentBundle,
} from "./chat_component_api";

function response(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("chat server component API", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("accepts an exact paginated server bundle", async () => {
    const fetchMock = vi.fn(async () => response({
      default_channel_id: "chat-42",
      has_more: true,
      next_offset: 20,
      html: { bundle: "server-channel-options" },
    }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchChannelOptionsPage()).resolves.toEqual({
      default_channel_id: "chat-42",
      has_more: true,
      next_offset: 20,
      html: { bundle: "server-channel-options" },
    });
    expect(fetchMock).toHaveBeenCalledWith("/v1/ui/chat/channels?limit=20&offset=0");
  });

  it("rejects contradictory pagination and missing component markup", async () => {
    vi.stubGlobal("fetch", vi.fn()
      .mockResolvedValueOnce(response({ has_more: true, html: { bundle: "server" } }))
      .mockResolvedValueOnce(response({ has_more: false, next_offset: 20, html: { bundle: "server" } }))
      .mockResolvedValueOnce(response({ has_more: false, html: {} })));

    await expect(fetchChatTimelinePage()).rejects.toThrow("pagination fields are contradictory");
    await expect(fetchChatTimelinePage()).rejects.toThrow("pagination fields are contradictory");
    await expect(fetchChatTimelinePage()).rejects.toThrow("server-rendered bundle");
  });

  it("requires exactly the requested memory sections", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response({
      memory: { has_more: false },
      html: { bundle: "server-memory" },
    })));

    await expect(fetchChatMemoryPage("memory", "reference")).resolves.toEqual({
      memory: { has_more: false },
      html: { bundle: "server-memory" },
    });
    expect(fetch).toHaveBeenCalledWith(
      "/v1/ui/chat/memory?limit=20&offset=0&section=memory&kind=reference",
    );
  });

  it("extracts only a nonblank server component bundle", () => {
    expect(requireServerComponentBundle({ html: { bundle: "exact-server-bundle" } }, "Job"))
      .toBe("exact-server-bundle");
    expect(() => requireServerComponentBundle({ html: {} }, "Job")).toThrow("server-rendered bundle");
  });
});
