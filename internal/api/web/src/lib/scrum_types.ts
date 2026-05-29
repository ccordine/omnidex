export const SCRUM_COLUMNS = [
  "backlog",
  "ready",
  "assigned",
  "in_progress",
  "review",
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
  done: "Done",
};

export const PLAYABLE_COLUMNS = new Set(["ready", "assigned", "in_progress"]);

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
