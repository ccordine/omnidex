export type ChatRole = "user" | "assistant" | "system" | "error";

export interface ChatMessage {
  role: ChatRole;
  content: string;
  at: string;
}

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
