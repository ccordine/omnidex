import { describe, expect, it } from "vitest";
import { ScrumBoardTracker } from "./scrum_board_tracker";
import type { ScrumBoardResponse, ScrumCard } from "./scrum_types";

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
    created_at: "2026-07-26T12:00:00Z",
    updated_at: "2026-07-26T12:00:00Z",
    ...overrides,
  };
}

function payload(item: ScrumCard): ScrumBoardResponse {
  return {
    board: { id: "board_1", name: "Board", project_directory: "", columns: ["assigned", "done"], cards: [item], updated_at: item.updated_at },
    cards_by_col: { [item.column]: [item] },
    all_columns: ["assigned", "done"],
    column_counts: { [item.column]: 1 },
    html: { bundle: `<template data-recyclr-target="scrum-board"></template>` },
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

  it("summarizes agent moves and suppresses a server-confirmed manual move", () => {
    const tracker = new ScrumBoardTracker();
    tracker.prepare(payload(card())).commit();

    const agentMove = tracker.prepare(payload(card({ column: "done", updated_at: "2026-07-26T12:01:00Z" })));
    expect(agentMove.notices.map((notice) => notice.message)).toEqual([`Moved "Realtime work" to Done`]);
    agentMove.commit();

    tracker.markManualMove("card_1");
    const manualMove = tracker.prepare(payload(card({ column: "assigned", updated_at: "2026-07-26T12:02:00Z" })));
    expect(manualMove.notices).toEqual([]);
  });

  it("reports completed LLM work once and rejects stale transition commits", () => {
    const tracker = new ScrumBoardTracker();
    tracker.rememberLLMPending([card({ tags_job_id: "job_1" })]);
    const completion = tracker.prepare(payload(card({ updated_at: "2026-07-26T12:01:00Z" })));
    expect(completion.notices).toEqual([
      expect.objectContaining({ message: `Tags updated for Realtime work`, llmActivity: true }),
    ]);

    const competing = tracker.prepare(payload(card({ updated_at: "2026-07-26T12:02:00Z" })));
    completion.commit();
    expect(() => competing.commit()).toThrow("stale Scrum board transition");
  });
});
