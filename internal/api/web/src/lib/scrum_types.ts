export const SCRUM_COLUMNS = [
  "backlog",
  "ready",
  "assigned",
  "in_progress",
  "review",
  "blocked",
  "done",
] as const;

export type ScrumColumn = (typeof SCRUM_COLUMNS)[number];

export type ScrumChecklistItem = {
  id: string;
  text: string;
  done: boolean;
};

export type ScrumChatMessage = {
  role: string;
  content: string;
  created_at: string;
};

export type ScrumTestCriterion = {
  id: string;
  text: string;
  done: boolean;
};

export type ScrumCoachConfig = {
  enabled?: boolean;
  auto_scan?: boolean;
  model?: string;
};

export type ScrumCoachSuggestion = {
  level: "info" | "warn" | "tip" | string;
  text: string;
};

export type ScrumCoachResponse = {
  card: ScrumCard;
  reply: string;
  suggestions?: ScrumCoachSuggestion[];
  card_prompt?: string;
  memory_stored?: number;
  mode?: string;
  model?: string;
  enabled?: boolean;
};

export type ScrumFlowMetrics = {
  assigned_returns?: number;
  review_bounces?: number;
  regression_count?: number;
  play_runs?: number;
  channel_messages?: number;
  planning_messages?: number;
  conversation_chars?: number;
  incomplete_score?: number;
  completion_status?: "likely_complete" | "likely_incomplete" | "uncertain" | string;
  signals?: string[];
  last_play_outcome?: string;
  review_gate?: "" | "passed" | "failed" | "pending" | "running" | string;
  column?: string;
};

export type ScrumFlowSummary = {
  total_cards: number;
  likely_incomplete: number;
  uncertain: number;
  likely_complete: number;
  assigned_returns_total: number;
  long_conversations: number;
};

export type ScrumCard = {
  id: string;
  title: string;
  description: string;
  column: ScrumColumn | string;
  checklist: ScrumChecklistItem[];
  ref_files: string[];
  chat: ScrumChatMessage[];
  model_config?: Record<string, string>;
  agent_config?: Record<string, string>;
  job_id?: string;
  tags_job_id?: string;
  ticket_job_id?: string;
  console_log?: string;
  play_state?: "" | "queued" | "running" | "paused" | "reviewing";
  queue_order?: number;
  board_order?: number;
  card_ticket?: string;
  card_prompt?: string;
  recipe_id?: string;
  recipe?: Record<string, unknown>;
  tags?: string[];
  test_criteria?: ScrumTestCriterion[];
  planning_chat?: ScrumChatMessage[];
  coach_config?: ScrumCoachConfig;
  flow_metrics?: ScrumFlowMetrics;
  created_at: string;
  updated_at: string;
};

export type ScrumBoard = {
  id: string;
  name: string;
  project_directory: string;
  columns: string[];
  cards: ScrumCard[];
  updated_at: string;
};

export type ScrumAutoReviewConfig = {
  enabled?: boolean;
  bounce_column?: string;
};

export type ScrumAutoWorkConfig = {
  enabled?: boolean;
  source_columns?: string[];
};

export type ScrumBoardResponse = {
  board: ScrumBoard;
  cards_by_col: Record<string, ScrumCard[]>;
  project_id?: number;
  auto_play_through?: boolean;
  auto_work?: ScrumAutoWorkConfig;
  auto_review?: ScrumAutoReviewConfig;
  play_queue?: {
    running_card_id?: string;
    queued_count: number;
    queued_card_ids: string[];
  };
  flow_summary?: ScrumFlowSummary;
};

export const COLUMN_LABELS: Record<string, string> = {
  backlog: "Backlog",
  ready: "Ready",
  assigned: "Assigned",
  in_progress: "In Progress",
  review: "Review",
  blocked: "Blocked",
  done: "Done",
};

export const PLAYABLE_COLUMNS = new Set(["ready", "assigned", "in_progress"]);

/** Play controls unlock when the card is in Assigned (or already active in the queue). */
export const ASSIGNED_COLUMN = "assigned" as const;

export function isPlayControlUnlocked(card: ScrumCard): boolean {
  if (card.column === ASSIGNED_COLUMN) return true;
  return card.play_state === "running" || card.play_state === "queued" || card.play_state === "paused";
}

export function nextColumn(current: string): string | null {
  const index = SCRUM_COLUMNS.indexOf(current as ScrumColumn);
  if (index < 0 || index >= SCRUM_COLUMNS.length - 1) return null;
  return SCRUM_COLUMNS[index + 1];
}

export function prevColumn(current: string): string | null {
  const index = SCRUM_COLUMNS.indexOf(current as ScrumColumn);
  if (index <= 0) return null;
  return SCRUM_COLUMNS[index - 1];
}

/** Top play focus: running in-progress, else first in-progress, else first assigned. */
export function groupCardsByColumn(board: ScrumBoard): Record<string, ScrumCard[]> {
  const columns = board.columns?.length ? board.columns : [...SCRUM_COLUMNS];
  const out: Record<string, ScrumCard[]> = {};
  for (const col of columns) out[col] = [];
  for (const card of board.cards) {
    const col = columns.includes(card.column) ? card.column : "backlog";
    out[col].push(card);
  }
  for (const col of columns) {
    out[col].sort((a, b) => (a.board_order ?? 0) - (b.board_order ?? 0));
  }
  return out;
}

/** Columns auto-play may pull from; project config defaults to Assigned only. */
export const AUTO_PLAY_WORK_COLUMNS = ["backlog", "ready", "assigned", "in_progress", "blocked"] as const;
export const DEFAULT_AUTO_WORK_COLUMNS = ["assigned"] as const;

export function autoPlayThroughComplete(cardsByCol: Record<string, ScrumCard[]>, autoReviewEnabled = false): boolean {
  const cards = Object.values(cardsByCol).flat();
  if (!cards.length) return false;
  return cards.every((card) => {
    if (card.column === "done") return true;
    if (card.column === "review") {
      return !autoReviewEnabled || card.play_state !== "reviewing";
    }
    return false;
  });
}

export function pickScrumAutoPlayFocusCard(
  board: ScrumBoard,
  cardsByCol: Record<string, ScrumCard[]>,
  playQueue?: ScrumBoardResponse["play_queue"],
  sourceColumns: readonly string[] = DEFAULT_AUTO_WORK_COLUMNS,
): ScrumCard | null {
  const running = pickScrumFocusCard(board, cardsByCol, playQueue);
  if (running?.play_state === "running" || running?.play_state === "queued") {
    return running;
  }
  const columns = sourceColumns.length ? sourceColumns : DEFAULT_AUTO_WORK_COLUMNS;
  for (const column of columns) {
    const cards = [...(cardsByCol[column] ?? [])].sort((a, b) => (a.board_order ?? 0) - (b.board_order ?? 0));
    const next = cards.find((card) => card.play_state !== "running" && card.play_state !== "queued");
    if (next) return next;
  }
  return running;
}

export function pickScrumFocusCard(
  board: ScrumBoard,
  cardsByCol: Record<string, ScrumCard[]>,
  playQueue?: ScrumBoardResponse["play_queue"],
): ScrumCard | null {
  const inProgress = cardsByCol.in_progress ?? [];
  const assigned = cardsByCol.assigned ?? [];

  if (playQueue?.running_card_id) {
    const running = board.cards.find((card) => card.id === playQueue.running_card_id);
    if (running) return running;
  }

  const runningInColumn = inProgress.find((card) => card.play_state === "running");
  if (runningInColumn) return runningInColumn;

  if (inProgress.length > 0) return inProgress[0];

  if (assigned.length > 0) return assigned[0];

  return null;
}
