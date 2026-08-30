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
  job_id?: string;
  play_state?: "" | "queued" | "running" | "paused";
  queue_order?: number;
  board_order?: number;
  card_ticket?: string;
  card_prompt?: string;
  tags: string[];
  test_criteria: ScrumTestCriterion[];
  flow_metrics?: ScrumFlowMetrics;
  summary?: boolean;
  checklist_done?: number;
  checklist_total?: number;
  ref_file_count?: number;
  chat_count?: number;
  channel_before_cursor: string;
  channel_has_more: boolean;
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
  enabled: boolean;
  source_columns: string[];
};

export type ScrumBoardHTML = {
  board: string;
  columns: string;
  focus: string;
  flow_summary: string;
  pagination: string;
  bundle: string;
};

export type ScrumBoardResponse = {
  board: ScrumBoard;
  cards_by_col: Record<string, ScrumCard[]>;
  html: ScrumBoardHTML;
  project_id: number;
  all_columns: string[];
  visible_column: ScrumColumn;
  column_counts: Record<string, number>;
  card_offset: number;
  card_has_more: boolean;
  auto_work: ScrumAutoWorkConfig;
  auto_work_complete: boolean;
  play_queue: {
    running_card_id: string;
    queued_count: number;
    queued_card_ids: string[];
    queued_has_more: boolean;
  };
  flow_summary: ScrumFlowSummary;
};

export type ScrumCardModalResponse = {
  card: ScrumCard;
  board: ScrumBoard;
  tab: "card" | "files" | "tests" | "channel";
  project_id: number;
  files: string[];
  dirs: string[];
  file_path: string;
  file_parent: string;
  file_has_parent: boolean;
  file_offset: number;
  file_has_previous: boolean;
  file_previous_offset: number;
  file_has_more: boolean;
  file_next_offset: number;
  play_queue: {
    running_card_id: string;
    queued_count: number;
    queued_card_ids: string[];
    queued_has_more: boolean;
  };
  pilot_pending: boolean;
  channel_before_cursor: string;
  channel_has_more: boolean;
};

export type ScrumCardFilePage = {
  files: string[];
  dirs: string[];
  file_path: string;
  file_parent: string;
  file_has_parent: boolean;
  file_offset: number;
  file_has_previous: boolean;
  file_previous_offset: number;
  file_has_more: boolean;
  file_next_offset: number;
};

export type ScrumChannelPage = {
	project_id: number;
	card_id: string;
	requested_before: string;
	limit: number;
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
export const SCRUM_CARD_REALTIME_REASON = {
  jobProgress: "job_progress",
} as const;

export type ScrumCardRealtimeReason =
  (typeof SCRUM_CARD_REALTIME_REASON)[keyof typeof SCRUM_CARD_REALTIME_REASON];
