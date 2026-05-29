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
  jira_ticket?: string;
  jira_prompt?: string;
  recipe_id?: string;
  recipe?: Record<string, unknown>;
  tags?: string[];
  test_criteria?: ScrumTestCriterion[];
  planning_chat?: ScrumChatMessage[];
  coach_config?: ScrumCoachConfig;
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
