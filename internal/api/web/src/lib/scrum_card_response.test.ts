import { describe, expect, it } from "vitest";
import { validateScrumCardProjection } from "./scrum_card_response";

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

function flow(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    assigned_returns: 0,
    review_bounces: 0,
    regression_count: 0,
    play_runs: 1,
    channel_messages: 1,
    conversation_chars: 7,
    incomplete_score: 0,
    completion_status: "uncertain",
    signals: [],
    column: "assigned",
    updated_at: "2026-08-13T12:00:00.123456Z",
    ...overrides,
  };
}

describe("closed Scrum card response authority", () => {
  it("accepts the exact bounded card and registered flow projection", () => {
    expect(validateScrumCardProjection(card({
      checklist: [{ id: "item_1", text: "Exact item", done: false }],
      ref_files: ["internal/api/server.go"],
      tags: ["server-authority"],
      flow_metrics: flow(),
    }), "card-7")).toMatchObject({ id: "card-7", tags: ["server-authority"] });
  });

  it.each([
    ["duplicate checklist IDs", { checklist: [
      { id: "item_1", text: "One", done: false },
      { id: "item_1", text: "Two", done: true },
    ] }],
    ["too many references", { ref_files: Array.from({ length: 257 }, (_, index) => `path-${index}`) }],
    ["noncanonical reference", { ref_files: ["../outside"] }],
    ["duplicate tags", { tags: ["server-authority", "server-authority"] }],
    ["noncanonical tag", { tags: ["Server Authority"] }],
    ["unknown flow field", { flow_metrics: flow({ inferred_action: "move" }) }],
    ["negative flow counter", { flow_metrics: flow({ play_runs: -1 }) }],
    ["oversized flow transition counter", { flow_metrics: flow({ play_runs: 2_147_483_648 }) }],
    ["unsafe flow wide counter", { flow_metrics: flow({ conversation_chars: Number.MAX_SAFE_INTEGER + 1 }) }],
    ["unregistered flow status", { flow_metrics: flow({ completion_status: "complete" }) }],
    ["unregistered flow outcome", { flow_metrics: flow({ last_play_outcome: "agent_done" }) }],
    ["explicit empty flow review gate", { flow_metrics: flow({ review_gate: "" }) }],
    ["explicit empty flow outcome", { flow_metrics: flow({ last_play_outcome: "" }) }],
    ["explicit empty flow column", { flow_metrics: flow({ column: "" }) }],
    ["explicit empty flow timestamp", { flow_metrics: flow({ updated_at: "" }) }],
    ["seven-digit flow timestamp", { flow_metrics: flow({ updated_at: "2026-08-13T12:00:00.1234567Z" }) }],
    ["trailing-zero flow timestamp", { flow_metrics: flow({ updated_at: "2026-08-13T12:00:00.123450Z" }) }],
    ["duplicate flow signal", { flow_metrics: flow({ signals: ["one", "one"] }) }],
    ["noncanonical flow signal", { flow_metrics: flow({ signals: [" padded "] }) }],
    ["Go whitespace blank flow signal", { flow_metrics: flow({ signals: ["\u0085"] }) }],
    ["too many flow signals", { flow_metrics: flow({ signals: Array.from({ length: 65 }, (_, index) => `signal-${index}`) }) }],
    ["duplicate channel message ID", { chat: [
      { id: "message_1", role: "assistant", content: "One", created_at: "2026-08-13T12:00:00Z" },
      { id: "message_1", role: "assistant", content: "Two", created_at: "2026-08-13T12:00:01Z" },
    ] }],
    ["unregistered channel role", { chat: [
      { id: "message_1", role: "agent", content: "One", created_at: "2026-08-13T12:00:00Z" },
    ] }],
    ["unregistered channel status", { chat: [
      { id: "message_1", role: "assistant", content: "One", created_at: "2026-08-13T12:00:00Z", status: "done" },
    ] }],
  ])("rejects %s", (_name, overrides) => {
    expect(() => validateScrumCardProjection(card(overrides), "card-7")).toThrow();
  });

  it("does not treat U+FEFF as Go Unicode whitespace", () => {
    expect(validateScrumCardProjection(card({ flow_metrics: flow({ signals: ["\ufeff"] }) }), "card-7"))
      .toMatchObject({ flow_metrics: { signals: ["\ufeff"] } });
  });
});
