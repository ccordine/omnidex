import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchScrumBoard } from "./scrum_api";
import { SCRUM_COLUMNS } from "./scrum_types";

function summaryCard(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: "card-7",
    title: "Bounded board card",
    description: "Summary projection",
    column: "assigned",
    checklist: [],
    ref_files: [],
    chat: [],
    tags: ["authority"],
    test_criteria: [],
    flow_metrics: {},
    summary: true,
    checklist_done: 0,
    checklist_total: 0,
    ref_file_count: 0,
    chat_count: 0,
    test_criteria_done: 0,
    test_criteria_total: 0,
    has_card_ticket: false,
    channel_before_cursor: "",
    channel_has_more: false,
    board_order: 1,
    created_at: "2026-08-13T12:00:00Z",
    updated_at: "2026-08-13T12:00:00.123456Z",
    ...overrides,
  };
}

function boardResponse(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  const card = summaryCard();
  return {
    board: {
      id: "project_14",
      name: "Authority board",
      project_directory: "/workspace",
      columns: ["assigned"],
      cards: [card],
      updated_at: "2026-08-13T12:00:00Z",
    },
    cards_by_col: { assigned: [card] },
    html: {
      board: "<section></section>",
      columns: "<nav></nav>",
      focus: "",
      flow_summary: "",
      pagination: "",
      bundle: '<template data-recyclr-target="scrum-board"></template>',
    },
    project_id: 14,
    all_columns: [...SCRUM_COLUMNS],
    visible_column: "assigned",
    column_counts: Object.fromEntries(SCRUM_COLUMNS.map((column) => [column, column === "assigned" ? 1 : 0])),
    card_offset: 0,
    card_has_more: false,
    auto_work: { enabled: true, source_columns: ["assigned"] },
    auto_work_complete: false,
    play_queue: { running_card_id: "", queued_count: 0, queued_card_ids: [], queued_has_more: false },
    flow_summary: {
      total_cards: 1,
      likely_incomplete: 0,
      uncertain: 1,
      likely_complete: 0,
      assigned_returns_total: 0,
      long_conversations: 0,
    },
    ...overrides,
  };
}

function response(payload: unknown): Response {
  return new Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } });
}

describe("closed Scrum board browser response authority", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("accepts only the requested project, column, and page", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response(boardResponse())));

    await expect(fetchScrumBoard(14, { column: "assigned", cardOffset: 0 })).resolves.toMatchObject({
      project_id: 14,
      visible_column: "assigned",
      card_offset: 0,
    });
  });

  it.each([
    ["unknown field", { ...boardResponse(), model_action: "forbidden" }],
    ["missing project", (() => { const payload = boardResponse(); delete payload.project_id; return payload; })()],
    ["wrong project", { ...boardResponse(), project_id: 15 }],
    ["wrong viewport", { ...boardResponse(), visible_column: "ready" }],
    ["wrong page", { ...boardResponse(), card_offset: 20 }],
    ["wrong board ID", { ...boardResponse(), board: { ...(boardResponse().board as object), id: "project_15" } }],
    ["wrong board column", { ...boardResponse(), board: { ...(boardResponse().board as object), columns: ["ready"] } }],
    ["embedded history", (() => {
      const card = summaryCard({ chat: [{ role: "assistant", content: "unbounded", created_at: "2026-08-13T12:00:00Z" }] });
      return { ...boardResponse(), board: { ...(boardResponse().board as object), cards: [card] }, cards_by_col: { assigned: [card] } };
    })()],
    ["duplicate cards", (() => {
      const card = summaryCard();
      return { ...boardResponse(), board: { ...(boardResponse().board as object), cards: [card, card] }, cards_by_col: { assigned: [card, card] } };
    })()],
    ["over-limit cards", (() => {
      const cards = Array.from({ length: 21 }, (_, index) => summaryCard({ id: `card-${index}`, board_order: index }));
      return { ...boardResponse(), board: { ...(boardResponse().board as object), cards }, cards_by_col: { assigned: cards }, column_counts: { ...(boardResponse().column_counts as object), assigned: 21 }, card_has_more: false };
    })()],
    ["mismatched column projection", { ...boardResponse(), cards_by_col: { assigned: [] } }],
    ["contradictory count", { ...boardResponse(), column_counts: { ...(boardResponse().column_counts as object), assigned: 2 }, card_has_more: false }],
    ["invalid queue", { ...boardResponse(), play_queue: { running_card_id: "", queued_count: 0, queued_card_ids: ["card-7"], queued_has_more: false } }],
    ["embedded cursor", (() => {
      const card = summaryCard({ channel_before_cursor: "scrumchat_v1_1", channel_has_more: true });
      return { ...boardResponse(), board: { ...(boardResponse().board as object), cards: [card] }, cards_by_col: { assigned: [card] } };
    })()],
  ])("rejects %s", async (_name, payload) => {
    vi.stubGlobal("fetch", vi.fn(async () => response(payload)));
    await expect(fetchScrumBoard(14, { column: "assigned", cardOffset: 0 })).rejects.toThrow();
  });
});
