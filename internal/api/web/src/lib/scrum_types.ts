export const SCRUM_COLUMNS = [
  "backlog",
  "ready",
  "assigned",
  "in_progress",
  "review",
  "blocked",
  "error",
  "done",
] as const;

export type ScrumColumn = (typeof SCRUM_COLUMNS)[number];

export type ScrumChecklistItem = {
  id: string;
  text: string;
  done: boolean;
};

export type ScrumChatMessage = {
  id?: string;
  role: string;
  content: string;
  created_at: string;
  status?: "sent" | "working" | "done" | "error" | string;
  operation_id?: string;
};

export type ScrumTestCriterion = {
  id: string;
  text: string;
  done: boolean;
};

export type ScrumFlowMetrics = {
  assigned_returns?: number;
  review_bounces?: number;
  regression_count?: number;
  play_runs?: number;
  channel_messages?: number;
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
  console_log?: string;
  play_state?: "" | "queued" | "running" | "paused";
  queue_order?: number;
  board_order?: number;
  card_ticket?: string;
  card_prompt?: string;
  recipe_id?: string;
  recipe?: Record<string, unknown>;
  tags?: string[];
  test_criteria?: ScrumTestCriterion[];
  flow_metrics?: ScrumFlowMetrics;
  summary?: boolean;
  checklist_done?: number;
  checklist_total?: number;
  ref_file_count?: number;
  chat_count?: number;
  test_criteria_done?: number;
  test_criteria_total?: number;
  has_card_ticket?: boolean;
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

export type ScrumAutoWorkConfig = {
  enabled?: boolean;
  source_columns?: string[];
};

export type ScrumConfigField = {
  key: string;
  label: string;
  description?: string;
  options?: string[];
  value?: string;
};

export type ScrumBoardHTML = {
  board?: string;
  columns?: string;
  focus?: string;
  flow_summary?: string;
  pagination?: string;
  bundle?: string;
};

export type ScrumBoardResponse = {
  board: ScrumBoard;
  cards_by_col: Record<string, ScrumCard[]>;
  html?: ScrumBoardHTML;
  project_id?: number;
  all_columns?: string[];
  visible_column?: string;
  column_counts?: Record<string, number>;
  card_offset?: number;
  card_has_more?: boolean;
  auto_work?: ScrumAutoWorkConfig;
  play_queue?: {
    running_card_id?: string;
    queued_count: number;
    queued_card_ids: string[];
  };
  flow_summary?: ScrumFlowSummary;
};

export type ScrumCardModalResponse = {
  card: ScrumCard;
  board: ScrumBoard;
  tab: string;
  project_id?: number;
  files?: string[];
  dirs?: string[];
  play_queue?: ScrumBoardResponse["play_queue"];
  model_fields?: ScrumConfigField[];
  model_source?: string;
  model_overrides?: Record<string, string>;
  agent_fields?: ScrumConfigField[];
  agent_source?: string;
  agent_system?: string;
  agent_overrides?: Record<string, string>;
  recipes?: Array<{ id: string; description?: string; [key: string]: unknown }>;
	recipe_offset?: number;
	recipe_has_more?: boolean;
  project_recipe_id?: string;
  project_recipe?: Record<string, unknown>;
  pilot_pending?: boolean;
  channel_before_cursor?: string;
  channel_has_more?: boolean;
};

export type ScrumChannelPage = {
  messages: ScrumChatMessage[];
  before_cursor: string;
  has_more: boolean;
  busy: boolean;
};

export const COLUMN_LABELS: Record<string, string> = {
  backlog: "Backlog",
  ready: "Ready",
  assigned: "Assigned",
  in_progress: "In Progress",
  review: "Review",
  blocked: "Blocked",
  error: "Error",
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

/** Columns auto-play may pull from; project config defaults to Assigned only. */
export const AUTO_PLAY_WORK_COLUMNS = ["backlog", "ready", "assigned", "in_progress", "blocked"] as const;
export const DEFAULT_AUTO_WORK_COLUMNS = ["assigned"] as const;

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
