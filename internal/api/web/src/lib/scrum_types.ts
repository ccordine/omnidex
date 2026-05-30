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
  jira_prompt?: string;
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
  jira_ticket?: string;
  jira_prompt?: string;
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

export type ScrumBoardResponse = {
  board: ScrumBoard;
  cards_by_col: Record<string, ScrumCard[]>;
  project_id?: number;
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
