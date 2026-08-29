import { afterEach, describe, expect, it, vi } from "vitest";
import { SCRUM_CHANNEL_RESPONSE_MAX_BYTES } from "./api";
import { newLifecycleOperationID } from "./lifecycle_operation";
import { chatScrumCard, fetchScrumChannelPage } from "./scrum_api";

const CHANNEL_RAW_CONTENT_BYTES = 4 * 1024 * 1024;
const timestamp = "2026-08-13T12:00:00Z";

function card(chat: unknown[]): Record<string, unknown> {
  return {
    id: "card-7", title: "Card", description: "", column: "in_progress",
    checklist: [], ref_files: [], chat, tags: [], test_criteria: [],
    job_id: "41", play_state: "running", channel_before_cursor: "",
    channel_has_more: false, created_at: timestamp, updated_at: timestamp,
  };
}

describe("Scrum channel encoded response authority", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("accepts a maximally escaped byte-bounded page and replay without dropping messages", async () => {
    const operationID = newLifecycleOperationID();
    const requestedMessage = "Exact revision";
    const escapedContent = "\u0001".repeat(CHANNEL_RAW_CONTENT_BYTES - requestedMessage.length);
    const messages = [
      { id: "message-1", role: "assistant", content: escapedContent, created_at: timestamp },
      { id: "message-2", role: "user", content: requestedMessage, created_at: timestamp, operation_id: operationID },
    ];
    vi.stubGlobal("fetch", vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      const payload = init?.method === "POST"
        ? { operation_id: operationID, project_id: 14, card: card(messages), action: "replanned" }
        : {
            project_id: 14, card_id: "card-7", requested_before: "scrumchat_v1_2", limit: 50,
            messages, before_cursor: "", has_more: false, busy: true,
          };
      const encoded = JSON.stringify(payload);
      expect(new TextEncoder().encode(encoded).byteLength).toBeGreaterThan(8 * 1024 * 1024);
      expect(new TextEncoder().encode(encoded).byteLength).toBeLessThanOrEqual(SCRUM_CHANNEL_RESPONSE_MAX_BYTES);
      return new Response(encoded, { status: 200 });
    }));

    await expect(chatScrumCard("card-7", requestedMessage, operationID, 14)).resolves.toMatchObject({ action: "replanned" });
    await expect(fetchScrumChannelPage("card-7", "scrumchat_v1_2", 14)).resolves.toMatchObject({
      messages: [{ content: escapedContent }, { content: requestedMessage }],
    });
  });

  it("rejects one encoded byte beyond the explicit channel response bound", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("{}", {
      status: 200,
      headers: { "Content-Length": String(SCRUM_CHANNEL_RESPONSE_MAX_BYTES + 1) },
    })));

    await expect(fetchScrumChannelPage("card-7", "scrumchat_v1_2", 14)).rejects.toThrow(
      new RegExp(`${SCRUM_CHANNEL_RESPONSE_MAX_BYTES}-byte transport bound`),
    );
  });
});
