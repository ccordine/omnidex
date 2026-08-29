import type { ScrumCardModalResponse } from "../../lib/scrum_types";
import { SCRUM_COLUMNS } from "../../lib/scrum_types";

type ModalContextOverrides = Partial<Omit<ScrumCardModalResponse, "card" | "board">> & {
  card?: Partial<ScrumCardModalResponse["card"]>;
  board?: Partial<ScrumCardModalResponse["board"]>;
};

export const modalContext: ScrumCardModalResponse = {
  card: {
    id: "card_1", title: "React modal card", description: "Rendered by React", column: "assigned",
    checklist: [], ref_files: [], chat: [], tags: ["react"], test_criteria: [],
    channel_before_cursor: "", channel_has_more: false,
    created_at: "2026-06-13T12:00:00Z", updated_at: "2026-06-13T12:00:00Z",
  },
  board: {
    id: "board_1", name: "Board", project_directory: "", columns: [...SCRUM_COLUMNS], cards: [],
    updated_at: "2026-06-13T12:00:00Z",
  },
  tab: "card", project_id: 7,
  files: [], dirs: [], file_path: "", file_parent: "", file_has_parent: false,
  file_offset: 0, file_has_previous: false, file_previous_offset: 0,
  file_has_more: false, file_next_offset: 0,
  play_queue: { running_card_id: "", queued_count: 0, queued_card_ids: [], queued_has_more: false },
  pilot_pending: false, channel_before_cursor: "", channel_has_more: false,
};

export function makeModalContext(overrides: ModalContextOverrides = {}): ScrumCardModalResponse {
  const { card, board, ...rest } = overrides;
  const channelBeforeCursor = rest.channel_before_cursor ?? card?.channel_before_cursor ?? modalContext.channel_before_cursor;
  const channelHasMore = rest.channel_has_more ?? card?.channel_has_more ?? modalContext.channel_has_more;
  return {
    ...modalContext,
    ...rest,
    channel_before_cursor: channelBeforeCursor,
    channel_has_more: channelHasMore,
    card: {
      ...modalContext.card,
      ...card,
      channel_before_cursor: channelBeforeCursor,
      channel_has_more: channelHasMore,
    },
    board: { ...modalContext.board, ...board },
  };
}

export function jsonResponse(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), { status, headers: { "Content-Type": "application/json" } });
}

export function realtimeCardUpdate(
  card: ScrumCardModalResponse["card"],
  overrides: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    id: 41,
    stateKey: `scrum-card:7:${card.id}`,
    occurredAt: "2026-06-13T12:01:00Z",
    eventName: "scrum-card-updated",
    reason: "job_progress",
    projectID: 7,
    cardID: card.id,
    card,
    ...overrides,
  };
}

export function prepareCardModalDOM(): void {
  document.body.innerHTML = `<div data-controller="card-modal-spa" data-card-modal-spa-card-id-value="card_1" data-card-modal-spa-project-id-value="7" data-card-modal-spa-initial-tab-value="card"></div>`;
}
