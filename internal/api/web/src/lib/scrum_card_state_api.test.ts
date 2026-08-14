import { afterEach, describe, expect, it, vi } from "vitest";
import { deleteScrumCard, doneScrumCard, moveScrumCard, patchScrumCard } from "./scrum_api";

function authoritativeCard(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: "card-7", title: "Authoritative card", description: "Exact description", column: "assigned",
    checklist: [], ref_files: [], chat: [], tags: [], test_criteria: [],
    channel_before_cursor: "", channel_has_more: false,
    created_at: "2026-08-13T12:00:00Z", updated_at: "2026-08-13T12:00:01.123456Z",
    ...overrides,
  };
}

describe("revision-bound Scrum card state actions", () => {
  afterEach(() => vi.unstubAllGlobals());

  it.each([
    {
      name: "move",
      invoke: () => moveScrumCard("card-7", "review", "2026-08-13T12:00:00.123456Z", 14, {
        before_card_id: "card-9",
      }),
      path: "/v1/scrum/cards/card-7/move?project_id=14",
			method: "POST",
      body: {
        column: "review",
        before_card_id: "card-9",
        expected_updated_at: "2026-08-13T12:00:00.123456Z",
      },
    },
    {
      name: "done",
      invoke: () => doneScrumCard("card-7", "2026-08-13T12:00:00.123456Z", 14),
      path: "/v1/scrum/cards/card-7/done?project_id=14",
			method: "POST",
      body: { expected_updated_at: "2026-08-13T12:00:00.123456Z" },
    },
		{
			name: "edit",
			invoke: () => patchScrumCard("card-7", "2026-08-13T12:00:00.123456Z", { title: "Exact title" }, 14),
			path: "/v1/scrum/cards/card-7?project_id=14",
			method: "PATCH",
			body: { expected_updated_at: "2026-08-13T12:00:00.123456Z", title: "Exact title" },
		},
		{
			name: "delete",
			invoke: () => deleteScrumCard("card-7", "2026-08-13T12:00:00.123456Z", 14),
			path: "/v1/scrum/cards/card-7?project_id=14",
			method: "DELETE",
			body: { expected_updated_at: "2026-08-13T12:00:00.123456Z" },
		},
  ])("sends the exact observed server revision for $name", async ({ invoke, path, method, body }) => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      let payload: unknown;
      if (init?.method === "DELETE") payload = {
        commit_state: "committed",
        project_id: 14,
        card_id: "card-7",
        expected_updated_at: "2026-08-13T12:00:00.123456Z",
        deleted: true,
      };
      else if (init?.method === "PATCH") payload = { card: authoritativeCard() };
      else payload = { commit_state: "committed", card: authoritativeCard({ column: url.includes("/done?") ? "done" : "review" }) };
      return new Response(JSON.stringify(payload), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);

    await invoke();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const call = fetchMock.mock.calls[0];
    if (!call) throw new Error("Expected one card state request.");
    const [url, init] = call;
    expect(url).toBe(path);
    expect(init?.method).toBe(method);
    expect(JSON.parse(String(init?.body))).toEqual(body);
  });

  it.each([
    ["wrong project", { commit_state: "committed", project_id: 15, card_id: "card-7", expected_updated_at: "2026-08-13T12:00:00.123456Z", deleted: true }],
    ["wrong card", { commit_state: "committed", project_id: 14, card_id: "card-8", expected_updated_at: "2026-08-13T12:00:00.123456Z", deleted: true }],
    ["wrong revision", { commit_state: "committed", project_id: 14, card_id: "card-7", expected_updated_at: "2026-08-13T12:00:01.123456Z", deleted: true }],
    ["unknown field", { commit_state: "committed", project_id: 14, card_id: "card-7", expected_updated_at: "2026-08-13T12:00:00.123456Z", deleted: true, fallback: true }],
  ])("rejects a deletion receipt with %s", async (_name, payload) => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(payload), { status: 200 })));
    await expect(deleteScrumCard("card-7", "2026-08-13T12:00:00.123456Z", 14)).rejects.toThrow();
  });

  it("rejects a noncanonical deletion revision before transport", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    await expect(deleteScrumCard("card-7", "2026-08-13T12:00:00.000Z", 14)).rejects.toThrow(/canonical/);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("returns authoritative card state with an explicit committed-degraded failure", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      commit_state: "committed_degraded",
      card: authoritativeCard({ updated_at: "2026-08-13T12:00:02.123456Z" }),
      operation_error: "Move committed, but play-queue reconciliation failed.",
    }), { status: 207 })));

    await expect(doneScrumCard("card-7", "2026-08-13T12:00:00.123456Z", 14)).resolves.toMatchObject({
      commit_state: "committed_degraded",
      card: { id: "card-7", updated_at: "2026-08-13T12:00:02.123456Z" },
      operation_error: "Move committed, but play-queue reconciliation failed.",
    });
  });

  it.each([
    { commit_state: "success", card: { id: "card-7", updated_at: "now" } },
    { commit_state: "committed_degraded", card: { id: "card-7", updated_at: "now" } },
    { commit_state: "committed", card: { id: "card-7", updated_at: "now" }, operation_error: "stale" },
    { commit_state: "committed", card: { id: "card-7", updated_at: "now" }, extra: true },
  ])("rejects malformed mutation response %#", async (payload) => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(payload), { status: 200 })));
    await expect(doneScrumCard("card-7", "2026-08-13T12:00:00.123456Z", 14)).rejects.toThrow();
  });
});
