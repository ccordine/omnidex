import { afterEach, describe, expect, it, vi } from "vitest";
import { newLifecycleOperationID } from "./lifecycle_operation";
import {
  chatScrumCard,
  fetchScrumCardFilePage,
  fetchScrumCardModal,
  fetchScrumChannelPage,
  pauseScrumCard,
  playScrumCard,
} from "./scrum_api";
import { SCRUM_COLUMNS } from "./scrum_types";

function card(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: "card-7",
    title: "Authoritative card",
    description: "Exact description",
    column: "assigned",
    checklist: [],
    ref_files: [],
    chat: [],
    tags: [],
    test_criteria: [],
    channel_before_cursor: "",
    channel_has_more: false,
    created_at: "2026-08-13T12:00:00Z",
    updated_at: "2026-08-13T12:00:00.123456Z",
    ...overrides,
  };
}

function modal(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    card: card(),
    board: {
      id: "project_14",
      name: "Board",
      project_directory: "/workspace",
      columns: [...SCRUM_COLUMNS],
      cards: [],
      updated_at: "2026-08-13T12:00:00Z",
    },
    tab: "card",
    project_id: 14,
    files: [],
    dirs: [],
    file_path: "",
    file_parent: "",
    file_has_parent: false,
    file_offset: 0,
    file_has_previous: false,
    file_previous_offset: 0,
    file_has_more: false,
    file_next_offset: 0,
    play_queue: {
      running_card_id: "",
      queued_count: 0,
      queued_card_ids: [],
      queued_has_more: false,
    },
    pilot_pending: false,
    channel_before_cursor: "",
    channel_has_more: false,
    ...overrides,
  };
}

function response(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("closed Scrum browser response authority", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("accepts the exact modal and explicit root file-page projections", async () => {
    const filePage = {
      files: ["README.md"],
      dirs: ["internal"],
      file_path: "",
      file_parent: "",
      file_has_parent: false,
      file_offset: 0,
      file_has_previous: false,
      file_previous_offset: 0,
      file_has_more: false,
      file_next_offset: 0,
    };
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) =>
      response(String(input).includes("/files?") ? filePage : modal()),
    ));

    await expect(fetchScrumCardModal("card-7", 14, { tab: "card" })).resolves.toMatchObject({
      project_id: 14,
      tab: "card",
    });
    await expect(fetchScrumCardFilePage("card-7", 14, "", 0)).resolves.toEqual(filePage);
  });

  it.each([
    ["unknown modal field", { ...modal(), generated_content: "forbidden" }],
    ["missing modal file authority", (() => { const value = modal(); delete value.file_path; return value; })()],
    ["wrong project", { ...modal(), project_id: 15 }],
    ["client fallback columns", { ...modal(), board: { ...(modal().board as object), columns: [] } }],
    ["embedded board inventory", { ...modal(), board: { ...(modal().board as object), cards: [card()] } }],
  ])("rejects %s", async (_name, payload) => {
    vi.stubGlobal("fetch", vi.fn(async () => response(payload)));
    await expect(fetchScrumCardModal("card-7", 14, { tab: "card" })).rejects.toThrow();
  });

  it.each([
    ["unknown file-page field", {
      files: [], dirs: [], file_path: "", file_parent: "", file_has_parent: false,
      file_offset: 0, file_has_previous: false, file_previous_offset: 0,
      file_has_more: false, file_next_offset: 0, fallback: true,
    }],
    ["wrong file-page cursor", {
      files: [], dirs: [], file_path: "", file_parent: "", file_has_parent: false,
      file_offset: 50, file_has_previous: true, file_previous_offset: 0,
      file_has_more: false, file_next_offset: 50,
    }],
  ])("rejects %s", async (_name, payload) => {
    vi.stubGlobal("fetch", vi.fn(async () => response(payload)));
    await expect(fetchScrumCardFilePage("card-7", 14, "", 0)).rejects.toThrow();
  });

  it("accepts only the exact play and pause action projections", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes("/play?")) {
        return response({
          project_id: 14,
          card_id: "card-7",
          card: card({ job_id: "41", column: "in_progress", play_state: "running" }),
          action: "started",
          job_id: "41",
          queue_order: 0,
        });
      }
      return response({
        project_id: 14,
        card_id: "card-7",
        action: "paused",
        job_id: "",
        queue_order: 0,
        card: card({ column: "assigned", play_state: "paused" }),
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(playScrumCard("card-7", "2026-08-13T12:00:00.123456Z", 14, { pivot: false })).resolves.toMatchObject({ id: "card-7" });
    await expect(pauseScrumCard("card-7", "2026-08-13T12:00:00.123456Z", 14)).resolves.toMatchObject({ id: "card-7" });
  });

  it("binds replay identity while accepting current card truth after the original run changes state", async () => {
    const operationID = newLifecycleOperationID();
    vi.stubGlobal("fetch", vi.fn(async () => response({
      operation_id: operationID,
      project_id: 14,
      card: card({ column: "done", play_state: "paused", chat: [] }),
      action: "replanned",
    })));

    await expect(chatScrumCard("card-7", "Exact revision", operationID, 14)).resolves.toMatchObject({
      action: "replanned",
      card: { column: "done", play_state: "paused" },
    });
  });

  it.each([
    ["play missing action", { project_id: 14, card_id: "card-7", card: card(), job_id: "", queue_order: 0 }, "play"],
    ["play mismatched card", { project_id: 14, card_id: "card-7", card: card({ id: "card-other" }), action: "queued", job_id: "", queue_order: 1 }, "play"],
    ["pause extra field", { project_id: 14, card_id: "card-7", action: "paused", job_id: "", queue_order: 0, card: card(), extra: true }, "pause"],
    ["play wrong project", { project_id: 15, card_id: "card-7", action: "started", job_id: "41", queue_order: 0, card: card({ job_id: "41", column: "in_progress", play_state: "running" }) }, "play"],
  ])("rejects %s", async (_name, payload, action) => {
    vi.stubGlobal("fetch", vi.fn(async () => response(payload)));
    const call = action === "play"
      ? playScrumCard("card-7", "2026-08-13T12:00:00.123456Z", 14, { pivot: false })
      : pauseScrumCard("card-7", "2026-08-13T12:00:00.123456Z", 14);
    await expect(call).rejects.toThrow();
  });

  it("accepts one typed channel action and one bounded page", async () => {
    const operationID = newLifecycleOperationID();
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes("?project_id=14&before=")) {
        return response({
          project_id: 14, card_id: "card-7", requested_before: "scrumchat_v1_1", limit: 50,
          messages: [], before_cursor: "", has_more: false, busy: false,
        });
      }
      return response({
        operation_id: operationID,
        project_id: 14,
        card: card({
          column: "in_progress", job_id: "41", play_state: "running",
          chat: [{ role: "user", content: "Exact revision", created_at: "2026-08-13T12:00:00Z", operation_id: operationID }],
        }),
        action: "started",
      });
    }));

    await expect(chatScrumCard("card-7", "Exact revision", operationID, 14)).resolves.toMatchObject({ action: "started" });
    await expect(fetchScrumChannelPage("card-7", "scrumchat_v1_1", 14)).resolves.toEqual({
      project_id: 14, card_id: "card-7", requested_before: "scrumchat_v1_1", limit: 50,
      messages: [], before_cursor: "", has_more: false, busy: false,
    });
  });

  it("accepts the JS-safe channel cursor maximum and rejects one ordinal beyond it", async () => {
    const maximum = `scrumchat_v1_${BigInt(Number.MAX_SAFE_INTEGER).toString(36)}`;
    const overMaximum = `scrumchat_v1_${(BigInt(Number.MAX_SAFE_INTEGER) + 1n).toString(36)}`;
    vi.stubGlobal("fetch", vi.fn(async () => response({
      project_id: 14, card_id: "card-7", requested_before: maximum, limit: 50,
      messages: [], before_cursor: "", has_more: false, busy: false,
    })));
    await expect(fetchScrumChannelPage("card-7", maximum, 14)).resolves.toMatchObject({ requested_before: maximum });
    await expect(fetchScrumChannelPage("card-7", overMaximum, 14)).rejects.toThrow(/canonical cursor/);
  });

  it("rejects an out-of-range cursor embedded in an authoritative card", async () => {
    const overMaximum = `scrumchat_v1_${(BigInt(Number.MAX_SAFE_INTEGER) + 1n).toString(36)}`;
    vi.stubGlobal("fetch", vi.fn(async () => response({
      project_id: 14, card_id: "card-7", action: "queued", job_id: "", queue_order: 1,
      card: card({ channel_before_cursor: overMaximum, channel_has_more: true, play_state: "queued", queue_order: 1 }),
    })));
    await expect(playScrumCard("card-7", "2026-08-13T12:00:00.123456Z", 14, { pivot: false })).rejects.toThrow(/canonical cursor/);
  });

  const rejectedOperationID = newLifecycleOperationID();
  it.each([
    ["wrong operation identity", { operation_id: newLifecycleOperationID(), project_id: 14, card: card(), action: "started" }, "post"],
    ["unknown channel action", { operation_id: rejectedOperationID, project_id: 14, card: card(), action: "agent" }, "post"],
    ["retired steered action", { operation_id: rejectedOperationID, project_id: 14, card: card(), action: "steered" }, "post"],
    ["retired revised action", { operation_id: rejectedOperationID, project_id: 14, card: card(), action: "revised" }, "post"],
    ["channel response extra field", { operation_id: rejectedOperationID, project_id: 14, card: card(), action: "started", model: "forbidden" }, "post"],
    ["wrong channel project", { operation_id: rejectedOperationID, project_id: 15, card: card(), action: "started" }, "post"],
    ["unbounded page", {
      project_id: 14, card_id: "card-7", requested_before: "scrumchat_v1_1", limit: 50,
      messages: Array.from({ length: 51 }, (_, index) => ({
        id: `message_${index}`, role: "assistant", content: "x", created_at: "2026-08-13T12:00:00Z",
      })),
      before_cursor: "", has_more: false, busy: false,
    }, "page"],
    ["page cursor contradiction", {
      project_id: 14, card_id: "card-7", requested_before: "scrumchat_v1_1", limit: 50,
      messages: [], before_cursor: "", has_more: true, busy: false,
    }, "page"],
    ["page route mismatch", {
      project_id: 14, card_id: "card-other", requested_before: "scrumchat_v1_1", limit: 50,
      messages: [], before_cursor: "", has_more: false, busy: false,
    }, "page"],
  ])("rejects %s", async (_name, payload, kind) => {
    vi.stubGlobal("fetch", vi.fn(async () => response(payload)));
    const call = kind === "post"
      ? chatScrumCard("card-7", "Exact revision", rejectedOperationID, 14)
      : fetchScrumChannelPage("card-7", "scrumchat_v1_1", 14);
    await expect(call).rejects.toThrow();
  });
});
