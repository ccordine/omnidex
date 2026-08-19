import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchSlashCommandComponent } from "./chat_slash_palette_api";

describe("fetchSlashCommandComponent", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("requests the one channel-scoped server component endpoint", async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      channel_id: "story-42",
      command_count: 0,
      html: { bundle: '<template data-recyclr-target="slash-command-options">empty</template>' },
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    const component = await fetchSlashCommandComponent("story-42");

    expect(component.command_count).toBe(0);
    expect(fetchMock).toHaveBeenCalledWith("/v1/ui/chat/slash-commands?channel_id=story-42");
  });

  it("rejects identity changes, extra fields, and counts above the exact server bound", async () => {
    for (const payload of [
      { channel_id: "other", command_count: 0, html: { bundle: "<template>x</template>" } },
      { channel_id: "story-42", command_count: 0, html: { bundle: "<template>x</template>" }, extra: true },
      { channel_id: "story-42", command_count: 98, html: { bundle: "<template>x</template>" } },
    ]) {
      vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(payload), { status: 200 })));
      await expect(fetchSlashCommandComponent("story-42")).rejects.toThrow();
    }
  });
});
