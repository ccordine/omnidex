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

export type ProjectPlanningSuggestion = {
  level?: string;
  text: string;
};

export type ProjectPlanningChatState = {
  chat: ScrumChatMessage[];
  config: ProjectPlanningChatConfig;
  web_search_enabled?: boolean;
  resolved_models?: {
    resolved?: Record<string, string>;
  };
};

export type ProjectPlanningChatResponse = ProjectPlanningChatState & {
  reply?: string;
  suggestions?: ProjectPlanningSuggestion[];
  card_drafts?: ProjectPlanningCardDraft[];
  memory_stored?: number;
  research_used?: boolean;
  mode?: string;
  model?: string;
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

export async function fetchOllamaModels(): Promise<{ models: Array<{ name: string }> }> {
  const response = await fetch("/v1/ollama/models");
  return readJSON(response);
}
