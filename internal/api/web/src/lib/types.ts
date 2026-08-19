export type StatusTone = "error" | "active" | "ready";

export interface TimelineEvent {
  id: string;
  type: string;
  details: Record<string, unknown>;
  full?: unknown;
  at: string;
}

export interface JobSummary {
  id: number | string;
  instruction?: string;
  status?: string;
  pipeline?: string;
  updated_at?: string;
  created_at?: string;
  result?: string;
  error?: string;
}

export interface JobStep {
  id: number | string;
  status?: string;
  action?: string;
  output?: string;
  error?: string;
}

export interface JobContext {
  id?: number | string;
  step_id?: number | string;
  key?: string;
  value?: string;
}

export interface JobDetails {
  job?: JobSummary;
  steps?: JobStep[];
  contexts?: JobContext[];
}

export interface MemoryRecord {
  id: number | string;
  kind?: string;
  source?: string;
  content?: string;
  tags?: string[];
  created_at?: string;
  status?: string;
}

export interface MemoryCandidate {
  id: number | string;
  status?: string;
  candidate_kind?: string;
  content?: string;
}

export interface UserChannel {
  id: string;
  scope: "user";
  name: string;
  tags: string[];
  project_id: number;
  workspace_root: string;
  data_source_id?: string;
  mode: "assistant" | "roleplay";
  roleplay_viewpoint_character_id?: string;
  created_at: string;
  updated_at: string;
}

export interface ChannelMessage {
  id: number;
  channel_id: string;
  role: "user" | "assistant";
  content: string;
  created_at: string;
}

export interface ChannelTranscriptPage {
  channel_id: string;
  next_before_id?: number;
  has_more: boolean;
  html: { bundle: string };
}

export type ChannelJobStatus = "pending" | "running" | "waiting_input" | "completed" | "failed" | "canceled";

export interface ChannelTurnJob {
  id: number;
  instruction: string;
  pipeline: "chat";
  status: ChannelJobStatus;
}

export interface ChannelTurnAccepted {
  channel: UserChannel;
  user_message: ChannelMessage;
  job: ChannelTurnJob;
}
