import { readJSON } from "./api";
import type { ScrumChatMessage } from "./scrum_types";

export type ProjectPlanningChatConfig = {
  model?: string;
  reasoning_mode?: "instant" | "thinking";
};

export type ProjectPlanningCardDraft = {
  title: string;
  description?: string;
  column?: string;
  checklist?: string[];
};

export type ProjectPlanningStoredDraft = ProjectPlanningCardDraft & {
  id: string;
  status: "pending" | "added" | "dismissed";
  source?: string;
  batch_id?: string;
  created_at?: string;
  added_at?: string;
  card_id?: string;
};

export type ProjectPlanningSuggestion = {
  level?: string;
  text: string;
};

export type ProjectPlanningChatState = {
  chat: ScrumChatMessage[];
  config: ProjectPlanningChatConfig;
  draft_queue?: ProjectPlanningStoredDraft[];
  pending_count?: number;
  web_search_enabled?: boolean;
  resolved_models?: {
    resolved?: Record<string, string>;
  };
};

export type ProjectPlanningChatResponse = ProjectPlanningChatState & {
  reply?: string;
  suggestions?: ProjectPlanningSuggestion[];
  card_drafts?: ProjectPlanningCardDraft[];
  batch_id?: string;
  memory_stored?: number;
  research_used?: boolean;
  mode?: string;
  model?: string;
};

export type ProjectPlanningDraftActionResponse = {
  draft_queue: ProjectPlanningStoredDraft[];
  pending_count: number;
  created_cards?: Array<{ id: string; title: string; column: string }>;
  created_count: number;
};

export async function fetchProjectPlanningChat(projectID: number): Promise<ProjectPlanningChatState> {
  const response = await fetch(`/v1/projects/${projectID}/planning-chat`);
  return readJSON<ProjectPlanningChatState>(response);
}

export async function updateProjectPlanningChatConfig(
  projectID: number,
  config: ProjectPlanningChatConfig,
): Promise<{ config: ProjectPlanningChatConfig }> {
  const response = await fetch(`/v1/projects/${projectID}/planning-chat`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ config }),
  });
  return readJSON(response);
}

export async function sendProjectPlanningChat(
  projectID: number,
  input: {
    message?: string;
    mode?: string;
    config?: ProjectPlanningChatConfig;
  },
): Promise<ProjectPlanningChatResponse> {
  const response = await fetch(`/v1/projects/${projectID}/planning-chat`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  return readJSON<ProjectPlanningChatResponse>(response);
}

export async function mutateProjectPlanningDrafts(
  projectID: number,
  input: {
    action: "add" | "add_all" | "dismiss" | "dismiss_all" | "clear";
    draft_id?: string;
    draft_ids?: string[];
    status?: "added" | "dismissed";
  },
): Promise<ProjectPlanningDraftActionResponse> {
  const response = await fetch(`/v1/projects/${projectID}/planning-chat/drafts`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  return readJSON<ProjectPlanningDraftActionResponse>(response);
}

export async function fetchOllamaModels(): Promise<{ models: Array<{ name: string }> }> {
  const response = await fetch("/v1/ollama/models");
  return readJSON(response);
}
