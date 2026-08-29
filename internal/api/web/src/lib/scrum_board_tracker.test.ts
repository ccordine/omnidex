import { describe, expect, it } from "vitest";
import { ScrumBoardTracker } from "./scrum_board_tracker";
import { SCRUM_COLUMNS, type ScrumBoardResponse, type ScrumCard, type ScrumColumn } from "./scrum_types";

function card(overrides: Partial<ScrumCard> = {}): ScrumCard {
  return {
    id: "card_1",
    title: "Realtime work",
    description: "",
    column: "assigned",
    checklist: [],
    ref_files: [],
    chat: [],
    tags: [],
    test_criteria: [],
    channel_before_cursor: "",
    channel_has_more: false,
    created_at: "2026-07-26T12:00:00Z",
    updated_at: "2026-07-26T12:00:00Z",
    ...overrides,
  };
}

function payload(item: ScrumCard): ScrumBoardResponse {
  return {
    board: { id: "project_1", name: "Board", project_directory: "", columns: [item.column], cards: [item], updated_at: item.updated_at },
    cards_by_col: { [item.column]: [item] },
    project_id: 1,
    all_columns: [...SCRUM_COLUMNS],
    visible_column: item.column as ScrumColumn,
    column_counts: Object.fromEntries(SCRUM_COLUMNS.map((column) => [column, column === item.column ? 1 : 0])),
    card_offset: 0,
    card_has_more: false,
    auto_work: { enabled: false, source_columns: ["assigned"] },
    auto_work_complete: false,
    play_queue: { running_card_id: "", queued_count: 0, queued_card_ids: [], queued_has_more: false },
    flow_summary: {
      total_cards: 1, likely_incomplete: 0, uncertain: 1, likely_complete: 0,
      assigned_returns_total: 0, long_conversations: 0,
    },
    html: {
      board: "", columns: "", focus: "", flow_summary: "", pagination: "",
      bundle: `<template data-recyclr-target="scrum-board"></template>`,
    },
  };
}

describe("ScrumBoardTracker", () => {
  it("deduplicates only committed server state", () => {
    const tracker = new ScrumBoardTracker();
    const first = tracker.prepare(payload(card()));
    expect(first.duplicate).toBe(false);
    expect(tracker.prepare(payload(card())).duplicate).toBe(false);
    first.commit();
    expect(tracker.prepare(payload(card())).duplicate).toBe(true);
  });

  it("summarizes server moves and suppresses a server-confirmed manual move", () => {
    const tracker = new ScrumBoardTracker();
    tracker.prepare(payload(card())).commit();

    const serverMove = tracker.prepare(payload(card({ column: "done", updated_at: "2026-07-26T12:01:00Z" })));
    expect(serverMove.notices.map((notice) => notice.message)).toEqual([`Moved "Realtime work" to Done`]);
    serverMove.commit();

    tracker.markManualMove("card_1");
    const manualMove = tracker.prepare(payload(card({ column: "assigned", updated_at: "2026-07-26T12:02:00Z" })));
    expect(manualMove.notices).toEqual([]);
  });

  it("rejects stale transition commits", () => {
    const tracker = new ScrumBoardTracker();
    const completion = tracker.prepare(payload(card({ updated_at: "2026-07-26T12:01:00Z" })));

    const competing = tracker.prepare(payload(card({ updated_at: "2026-07-26T12:02:00Z" })));
    completion.commit();
    expect(() => competing.commit()).toThrow("stale Scrum board transition");
  });
});
