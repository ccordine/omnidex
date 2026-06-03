import { readJSON } from "./api";
import { projectQuery } from "./project_api";
import type { ScrumAutoWorkConfig, ScrumBoard, ScrumBoardResponse, ScrumCard, ScrumCreateTicketConfig, ScrumHealthResponse } from "./scrum_types";

export type ScrumCardLlmJob = {
  id: number;
  status?: string;
  result?: string;
  error?: string;
};

export type ScrumCardLlmQueuedResponse = {
  queued?: boolean;
  job?: ScrumCardLlmJob;
  card?: ScrumCard;
  message?: string;
  html?: { bundle?: string };
  tags?: string[];
  notes?: string;
  ticket?: string;
};

function scrumBoardQuery(projectID?: number | null, options: { column?: string | null } = {}): string {
  const query = new URLSearchParams();
  if (projectID != null) query.set("project_id", String(projectID));
  if (options.column?.trim()) query.set("column", options.column.trim());
  const encoded = query.toString();
  return encoded ? `?${encoded}` : "";
}

export async function fetchScrumBoard(
  projectID?: number | null,
  options: { column?: string | null } = {},
  signal?: AbortSignal,
): Promise<ScrumBoardResponse> {
  const response = await fetch(`/v1/scrum${scrumBoardQuery(projectID, options)}`, { signal });
  return readJSON<ScrumBoardResponse>(response);
}

export type ScrumCardLoadResponse = {
  card: ScrumCard;
  html?: { bundle?: string };
};

export async function fetchScrumCard(cardID: string, projectID?: number | null): Promise<ScrumCard> {
  const payload = await fetchScrumCardPayload(cardID, projectID);
  return payload.card;
}

export async function fetchScrumCardPayload(cardID: string, projectID?: number | null): Promise<ScrumCardLoadResponse> {
  const response = await fetch(`/v1/scrum/cards/${encodeURIComponent(cardID)}${projectQuery(projectID)}`);
  const payload = await readJSON<{ card?: ScrumCard | null; html?: { bundle?: string } }>(response);
  if (!payload.card?.id) {
    throw new Error("Card load did not return a card");
  }
  return { card: payload.card, html: payload.html };
}

export type ScrumCardModalResponse = {
  card: ScrumCard;
  tab: string;
  html?: { bundle?: string };
};

export async function fetchScrumCardModal(
  cardID: string,
  projectID?: number | null,
  options: { tab?: string; partial?: boolean } = {},
): Promise<ScrumCardModalResponse> {
  const query = new URLSearchParams();
  if (projectID != null) query.set("project_id", String(projectID));
  if (options.tab?.trim()) query.set("tab", options.tab.trim());
  if (options.partial) query.set("partial", "true");
  const suffix = query.toString() ? `?${query.toString()}` : "";
  const response = await fetch(`/v1/scrum/cards/${encodeURIComponent(cardID)}/modal${suffix}`);
  const payload = await readJSON<ScrumCardModalResponse>(response);
  if (!payload.card?.id) {
    throw new Error("Card modal did not return a card");
  }
  return payload;
}

export async function fetchScrumHealth(
  projectID?: number | null,
  fingerprint = "",
  options: { column?: string | null } = {},
): Promise<ScrumHealthResponse> {
  const query = new URLSearchParams();
  if (projectID != null) query.set("project_id", String(projectID));
  if (fingerprint.trim()) query.set("fingerprint", fingerprint.trim());
  if (options.column?.trim()) query.set("column", options.column.trim());
  const suffix = query.toString() ? `?${query.toString()}` : "";
  const response = await fetch(`/v1/scrum/cards/health${suffix}`);
  return readJSON<ScrumHealthResponse>(response);
}

export async function updateScrumBoard(
  name: string,
  projectDirectory: string,
  projectID?: number | null,
): Promise<ScrumBoard> {
  const response = await fetch(`/v1/scrum${projectQuery(projectID)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, project_directory: projectDirectory }),
  });
  const payload = await readJSON<{ board: ScrumBoard }>(response);
  return payload.board;
}

export async function patchScrumAutoPlay(
  enabled: boolean,
  projectID?: number | null,
  autoWork?: ScrumAutoWorkConfig,
): Promise<ScrumBoardResponse> {
  const body: Record<string, unknown> = { auto_play_through: enabled };
  if (autoWork) body.auto_work = { ...autoWork, enabled };
  const response = await fetch(`/v1/scrum${projectQuery(projectID)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return readJSON<ScrumBoardResponse>(response);
}

export async function patchScrumAutoWork(
  config: ScrumAutoWorkConfig,
  projectID?: number | null,
): Promise<ScrumBoardResponse> {
  const response = await fetch(`/v1/scrum${projectQuery(projectID)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ auto_work: config }),
  });
  return readJSON<ScrumBoardResponse>(response);
}

export async function patchScrumAutoReview(
  config: import("./scrum_types").ScrumAutoReviewConfig,
  projectID?: number | null,
): Promise<ScrumBoardResponse> {
  const response = await fetch(`/v1/scrum${projectQuery(projectID)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ auto_review: config }),
  });
  return readJSON<ScrumBoardResponse>(response);
}

export async function patchScrumAutomation(
  config: {
    auto_work?: ScrumAutoWorkConfig;
    auto_review?: import("./scrum_types").ScrumAutoReviewConfig;
    create_ticket?: ScrumCreateTicketConfig;
  },
  projectID?: number | null,
): Promise<ScrumBoardResponse> {
  const response = await fetch(`/v1/scrum${projectQuery(projectID)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(config),
  });
  return readJSON<ScrumBoardResponse>(response);
}

export async function createScrumCard(
  title: string,
  description: string,
  column: string,
  projectID?: number | null,
  options: { createTicket?: boolean; createTicketConfig?: ScrumCreateTicketConfig } = {},
): Promise<ScrumCard> {
  const body: Record<string, unknown> = { title, description, column };
  if (options.createTicketConfig) {
    body.create_ticket_config = options.createTicketConfig;
    if (options.createTicket) {
      body.column = options.createTicketConfig.column || column;
    }
  }
  if (options.createTicket) {
    body.create_ticket = true;
  }
  const response = await fetch(`/v1/scrum/cards${projectQuery(projectID)}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const payload = await readJSON<{ card: ScrumCard }>(response);
  return payload.card;
}

function cardURL(cardID: string, suffix: string, projectID?: number | null): string {
  return `/v1/scrum/cards/${encodeURIComponent(cardID)}/${suffix}${projectQuery(projectID)}`;
}

export async function moveScrumCard(
  cardID: string,
  column: string,
  projectID?: number | null,
  options: { before_card_id?: string | null } = {},
): Promise<ScrumCard> {
  const body: Record<string, string> = { column };
  if (options.before_card_id) {
    body.before_card_id = options.before_card_id;
  }
  const response = await fetch(cardURL(cardID, "move", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const payload = await readJSON<{ card: ScrumCard }>(response);
  return payload.card;
}

export async function playScrumCard(
  cardID: string,
  projectID?: number | null,
  options: { pivot?: boolean; agentConfig?: Record<string, string> } = {},
): Promise<ScrumCard & { message?: string }> {
  const body: Record<string, unknown> = { pivot: Boolean(options.pivot) };
  if (options.agentConfig && Object.keys(options.agentConfig).length > 0) {
    body.agent_config = options.agentConfig;
  }
  const response = await fetch(cardURL(cardID, "play", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const payload = await readJSON<{ card: ScrumCard; message?: string }>(response);
  return { ...payload.card, message: payload.message };
}

export async function pauseScrumCard(cardID: string, projectID?: number | null): Promise<ScrumCard> {
  const response = await fetch(cardURL(cardID, "pause", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
  const payload = await readJSON<{ card: ScrumCard }>(response);
  return payload.card;
}

export async function syncScrumBoard(projectID?: number | null): Promise<ScrumBoardResponse> {
  const response = await fetch(`/v1/scrum/cards/sync${projectQuery(projectID)}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
  return readJSON(response);
}

export async function doneScrumCard(cardID: string, projectID?: number | null): Promise<ScrumCard> {
  const response = await fetch(cardURL(cardID, "done", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
  const payload = await readJSON<{ card: ScrumCard }>(response);
  return payload.card;
}

export async function syncScrumCard(cardID: string, projectID?: number | null): Promise<ScrumCard> {
  const response = await fetch(cardURL(cardID, "sync", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
  const payload = await readJSON<{ card: ScrumCard }>(response);
  return payload.card;
}

export async function deleteScrumCard(cardID: string, projectID?: number | null): Promise<void> {
  const response = await fetch(`/v1/scrum/cards/${encodeURIComponent(cardID)}${projectQuery(projectID)}`, {
    method: "DELETE",
  });
  await readJSON(response);
}

export async function patchScrumCard(
  cardID: string,
  patch: Partial<ScrumCard>,
  projectID?: number | null,
): Promise<ScrumCard> {
  const response = await fetch(`/v1/scrum/cards/${encodeURIComponent(cardID)}${projectQuery(projectID)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
  });
  const payload = await readJSON<{ card?: ScrumCard | null }>(response);
  if (!payload.card?.id) {
    throw new Error("Card update did not return a card");
  }
  return payload.card;
}

export async function chatScrumCard(
  cardID: string,
  message: string,
  projectID?: number | null,
): Promise<{ card: ScrumCard; reply: string; error?: string; agent?: string; action?: string }> {
  const response = await fetch(cardURL(cardID, "chat", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ message }),
  });
  return readJSON(response);
}

export async function cardTicketScrumCard(
  cardID: string,
  payload: {
    prompt?: string;
    card_prompt?: string;
    ticket?: string;
    iterate?: boolean;
    iterate_notes?: string;
  },
  projectID?: number | null,
): Promise<ScrumCardLlmQueuedResponse> {
  const response = await fetch(cardURL(cardID, "card-ticket", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  return readJSON(response);
}

export async function coachScrumCard(
  cardID: string,
  payload: { message?: string; mode?: string; snapshot?: Record<string, string> },
  projectID?: number | null,
): Promise<import("./scrum_types").ScrumCoachResponse> {
  const response = await fetch(cardURL(cardID, "coach", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  return readJSON(response);
}

export async function updateScrumCoachConfig(
  cardID: string,
  config: import("./scrum_types").ScrumCoachConfig,
  projectID?: number | null,
): Promise<{ card: ScrumCard; coach_config: import("./scrum_types").ScrumCoachConfig }> {
  const response = await fetch(cardURL(cardID, "coach-config", projectID), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(config),
  });
  return readJSON(response);
}

export async function fetchScrumTags(
  query = "",
  projectID?: number | null,
  limit = 40,
): Promise<string[]> {
  const params = new URLSearchParams();
  if (query.trim()) params.set("q", query.trim());
  if (limit > 0) params.set("limit", String(limit));
  const base = `/v1/scrum/tags${projectQuery(projectID)}`;
  const extra = params.toString();
  const url = extra ? `${base}${base.includes("?") ? "&" : "?"}${extra}` : base;
  const response = await fetch(url);
  const payload = await readJSON<{ tags: string[] }>(response);
  return payload.tags ?? [];
}

export async function suggestScrumTags(
  cardID: string,
  projectID?: number | null,
): Promise<ScrumCardLlmQueuedResponse> {
  const response = await fetch(cardURL(cardID, "tags-suggest", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
  return readJSON(response);
}

export async function fetchScrumFiles(projectID?: number | null): Promise<{ files: string[]; dirs?: string[]; root: string }> {
  const response = await fetch(`/v1/scrum/files${projectQuery(projectID)}`);
  return readJSON(response);
}

export async function uploadScrumCardFiles(
  cardID: string,
  files: FileList | File[],
  projectID?: number | null,
): Promise<{ card: ScrumCard; uploaded: string[] }> {
  const body = new FormData();
  Array.from(files).forEach((file) => body.append("files", file));
  const response = await fetch(cardURL(cardID, "files", projectID), {
    method: "POST",
    body,
  });
  return readJSON(response);
}
