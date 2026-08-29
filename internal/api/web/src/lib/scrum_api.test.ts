import { afterEach, describe, expect, it, vi } from "vitest";
import { newLifecycleOperationID } from "./lifecycle_operation";
import {
  applyScrumCardElaboration,
  assembleScrumCardTicket,
  chatScrumCard,
  createScrumCard,
  fetchScrumBoard,
  fetchScrumCardFilePage,
  fetchScrumCardModal,
  fetchScrumChannelPage,
  patchScrumAutoWork,
  pauseScrumCard,
  playScrumCard,
} from "./scrum_api";
import { SCRUM_COLUMNS } from "./scrum_types";

function authoritativeCard(overrides: Record<string, unknown> = {}): Record<string, unknown> {
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

function authoritativeFilePage(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    files: [], dirs: [], file_path: "", file_parent: "", file_has_parent: false,
    file_offset: 0, file_has_previous: false, file_previous_offset: 0,
    file_has_more: false, file_next_offset: 0, ...overrides,
  };
}

function authoritativeModal(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    card: authoritativeCard(),
    board: {
      id: "project_14", name: "Board", project_directory: "/workspace",
      columns: [...SCRUM_COLUMNS], cards: [], updated_at: "2026-08-13T12:00:00Z",
    },
    tab: "files", project_id: 14,
    ...authoritativeFilePage(),
    play_queue: { running_card_id: "", queued_count: 0, queued_card_ids: [], queued_has_more: false },
    pilot_pending: false, channel_before_cursor: "", channel_has_more: false,
    ...overrides,
  };
}

describe("chatScrumCard", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("sends one explicit lifecycle identity with the authoritative message", async () => {
    const operationID = newLifecycleOperationID();
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) =>
      new Response(JSON.stringify({
        operation_id: operationID,
        project_id: 14,
        card: authoritativeCard({
          column: "in_progress", job_id: "41", play_state: "running",
          chat: [{ role: "user", content: "Continue once.", created_at: "2026-08-13T12:00:00Z", operation_id: operationID }],
        }),
        action: "started",
      }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await chatScrumCard("card-7", "Continue once.", operationID, 14);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const call = fetchMock.mock.calls[0];
    expect(call).toBeDefined();
    if (!call) {
      throw new Error("Expected chatScrumCard to issue one fetch call.");
    }
    const [url, init] = call;
    expect(init).toBeDefined();
    if (!init) {
      throw new Error("Expected chatScrumCard to provide request options.");
    }
    expect(url).toBe("/v1/scrum/cards/card-7/chat?project_id=14");
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({
      operation_id: operationID,
      message: "Continue once.",
    });
  });
});

describe("Scrum browser authority", () => {
  afterEach(() => vi.unstubAllGlobals());

  it.each([
    ["board", () => fetchScrumBoard(0, { column: "assigned", cardOffset: 0 })],
    ["modal", () => fetchScrumCardModal("card-7", 0)],
    ["file page", () => fetchScrumCardFilePage("card-7", Number.NaN, "", 0)],
    ["chat", () => chatScrumCard("card-7", "Continue.", newLifecycleOperationID(), Number.NaN)],
    ["ticket", () => assembleScrumCardTicket("card-7", "2026-08-13T12:00:00Z", -1)],
    ["play", () => playScrumCard("card-7", "2026-08-13T12:00:00Z", 1.5, { pivot: false })],
    ["pause", () => pauseScrumCard("card-7", "2026-08-13T12:00:00Z", Number.MAX_SAFE_INTEGER + 1)],
  ])("rejects invalid project identity before %s transport", async (_name, invoke) => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    await expect(invoke()).rejects.toThrow(/positive safe-integer project ID/);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("sends an explicit root path and canonical offset for bounded file views", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => new Response(JSON.stringify(
      String(input).includes("/files?") ? authoritativeFilePage() : authoritativeModal(),
    ), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await fetchScrumCardModal("card-7", 14, { tab: "files", filePath: "", fileOffset: 0 });
    await fetchScrumCardFilePage("card-7", 14, "", 0);

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "/v1/scrum/cards/card-7/modal?project_id=14&tab=files&file_path=&file_offset=0",
    );
    expect(fetchMock.mock.calls[1]?.[0]).toBe(
      "/v1/scrum/cards/card-7/files?project_id=14&file_path=&file_offset=0",
    );
  });

  it.each([
    ["case-folded tab", () => fetchScrumCardModal("card-7", 14, { tab: "FILES" as never })],
    ["whitespace tab", () => fetchScrumCardModal("card-7", 14, { tab: " files" as never })],
    ["explicit empty tab", () => fetchScrumCardModal("card-7", 14, { tab: "" as never })],
    ["missing explicit file state", () => fetchScrumCardModal("card-7", 14, { tab: "files" } as never)],
    ["file state on card tab", () => fetchScrumCardModal("card-7", 14, { tab: "card", filePath: "" } as never)],
    ["absolute path", () => fetchScrumCardFilePage("card-7", 14, "/workspace", 0)],
    ["dot path", () => fetchScrumCardFilePage("card-7", 14, "pkg/../other", 0)],
    ["NUL path", () => fetchScrumCardFilePage("card-7", 14, "pkg\0file", 0)],
    ["invalid Unicode path", () => fetchScrumCardFilePage("card-7", 14, "\ud800", 0)],
    ["fractional offset", () => fetchScrumCardFilePage("card-7", 14, "", 0.5)],
    ["oversized offset", () => fetchScrumCardFilePage("card-7", 14, "", 1_000_001)],
  ])("rejects inexact modal/file state before %s transport", async (_name, invoke) => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    await expect(invoke()).rejects.toThrow();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects inexact channel cursor bytes before transport", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    await expect(fetchScrumChannelPage("card-7", " scrumchat_v1_a", 14)).rejects.toThrow(/exact canonical cursor/);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("sends observed revisions for play and pause", async () => {
    const fetchMock = vi.fn(async (url: RequestInfo | URL, _init?: RequestInit) => {
      if (String(url).includes("/play?")) {
        return new Response(JSON.stringify({
          project_id: 14,
          card_id: "card-7",
          card: authoritativeCard({ column: "in_progress", job_id: "41", play_state: "running" }),
          action: "started",
          job_id: "41",
          queue_order: 0,
        }), { status: 200 });
      }
      return new Response(JSON.stringify({
        project_id: 14,
        card_id: "card-7",
        action: "paused",
        job_id: "",
        queue_order: 0,
        card: authoritativeCard({ play_state: "paused" }),
      }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);
    await playScrumCard("card-7", "2026-08-13T12:00:00.123456Z", 14, { pivot: true });
    await pauseScrumCard("card-7", "2026-08-13T12:01:00.123456Z", 14);
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      expected_updated_at: "2026-08-13T12:00:00.123456Z",
      pivot: true,
    });
    expect(JSON.parse(String(fetchMock.mock.calls[1]?.[1]?.body))).toEqual({
      expected_updated_at: "2026-08-13T12:01:00.123456Z",
    });
  });

  it("preserves exact nonblank create-card description bytes", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(JSON.stringify({
      card: authoritativeCard(),
    }), { status: 201 }));
    vi.stubGlobal("fetch", fetchMock);

    await createScrumCard("Exact title", "  preserve description\nwith tab:\t ", "ready", 14);

    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      title: "Exact title",
      description: "  preserve description\nwith tab:\t ",
      column: "ready",
    });
  });

  it("returns authoritative auto-work settings with explicit committed degradation", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      commit_state: "committed_degraded",
      auto_work: { enabled: true, source_columns: ["assigned"] },
      operation_error: "Settings committed, but scheduling failed.",
    }), { status: 207 })));

    await expect(patchScrumAutoWork({ enabled: true, source_columns: ["assigned"] }, 14)).resolves.toEqual({
      commit_state: "committed_degraded",
      auto_work: { enabled: true, source_columns: ["assigned"] },
      operation_error: "Settings committed, but scheduling failed.",
    });
  });

  it.each([
    { commit_state: "success", auto_work: { enabled: true, source_columns: ["assigned"] } },
    { commit_state: "committed_degraded", auto_work: { enabled: true, source_columns: ["assigned"] } },
    { commit_state: "committed", auto_work: { enabled: true, source_columns: ["review"] } },
    { commit_state: "committed", auto_work: { enabled: true, source_columns: ["assigned"] }, extra: true },
  ])("rejects malformed auto-work mutation response %#", async (payload) => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(payload), { status: 200 })));
    await expect(patchScrumAutoWork({ enabled: true, source_columns: ["assigned"] }, 14)).rejects.toThrow();
  });
});

describe("Scrum card ticket actions", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("sends only the code-owned assemble action and observed card revision", async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) =>
      new Response(JSON.stringify({ card: authoritativeCard({ updated_at: "2026-08-13T12:01:00Z" }) }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await assembleScrumCardTicket("card-7", "2026-08-13T12:00:00Z", 14);

    const call = fetchMock.mock.calls[0];
    if (!call) throw new Error("Expected ticket generation to issue one request.");
    const [url, init] = call;
    expect(url).toBe("/v1/scrum/cards/card-7/card-ticket?project_id=14");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      action: "assemble",
      expected_updated_at: "2026-08-13T12:00:00Z",
    });
  });

  it("sends explicit user-authored elaboration without legacy prompt fields", async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) =>
      new Response(JSON.stringify({ card: authoritativeCard({ updated_at: "2026-08-13T12:01:00Z" }) }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);

		await applyScrumCardElaboration("card-7", "2026-08-13T12:00:00Z", "  Preserve exact state.\nKeep tab:\t ", 14);

    const call = fetchMock.mock.calls[0];
    if (!call) throw new Error("Expected ticket elaboration to issue one request.");
    const [, init] = call;
    const body = JSON.parse(String(init?.body));
    expect(body).toEqual({
      action: "apply_elaboration",
      expected_updated_at: "2026-08-13T12:00:00Z",
		elaboration: "  Preserve exact state.\nKeep tab:\t ",
    });
    expect(body).not.toHaveProperty("prompt");
    expect(body).not.toHaveProperty("iterate");
    expect(body).not.toHaveProperty("ticket");
  });

  it.each([
    ["inexact card id", " card-7", "2026-08-13T12:00:00Z"],
    ["inexact revision", "card-7", "2026-08-13T12:00:00.000Z"],
  ])("rejects %s before ticket transport", async (_name, cardID, revision) => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    await expect(assembleScrumCardTicket(cardID, revision, 14)).rejects.toThrow(/canonical/);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
