import { readJSON } from "./api";
import type { LifecycleOperationID } from "./lifecycle_operation";
import { projectQuery } from "./project_api";
import type { ScrumAutoWorkConfig, ScrumBoard, ScrumBoardResponse, ScrumCard, ScrumCardModalResponse, ScrumChannelPage } from "./scrum_types";

export { elaborateScrumCardTicket, generateScrumCardTicket } from "./scrum_ticket_api";

function scrumBoardQuery(projectID?: number | null, options: { column?: string | null; cardOffset?: number } = {}): string {
  const query = new URLSearchParams();
  if (projectID != null) query.set("project_id", String(projectID));
  if (options.column?.trim()) query.set("column", options.column.trim());
  if (options.cardOffset != null) query.set("card_offset", String(options.cardOffset));
  const encoded = query.toString();
  return encoded ? `?${encoded}` : "";
}

export async function fetchScrumBoard(
  projectID?: number | null,
  options: { column?: string | null; cardOffset?: number } = {},
  signal?: AbortSignal,
): Promise<ScrumBoardResponse> {
  const response = await fetch(`/v1/scrum${scrumBoardQuery(projectID, options)}`, { signal });
  return readJSON<ScrumBoardResponse>(response);
}

export type ScrumCardLoadResponse = {
  card: ScrumCard;
};

export async function fetchScrumCard(cardID: string, projectID?: number | null): Promise<ScrumCard> {
  const payload = await fetchScrumCardPayload(cardID, projectID);
  return payload.card;
}

export async function fetchScrumCardPayload(cardID: string, projectID?: number | null): Promise<ScrumCardLoadResponse> {
  const response = await fetch(`/v1/scrum/cards/${encodeURIComponent(cardID)}${projectQuery(projectID)}`);
  const payload = await readJSON<{ card?: ScrumCard | null }>(response);
  if (!payload.card?.id) {
    throw new Error("Card load did not return a card");
  }
  return { card: payload.card };
}

export async function fetchScrumCardModal(
  cardID: string,
  projectID?: number | null,
  options: { tab?: string } = {},
): Promise<ScrumCardModalResponse> {
  const query = new URLSearchParams();
  if (projectID != null) query.set("project_id", String(projectID));
  if (options.tab?.trim()) query.set("tab", options.tab.trim());
  const suffix = query.toString() ? `?${query.toString()}` : "";
  const response = await fetch(`/v1/scrum/cards/${encodeURIComponent(cardID)}/modal${suffix}`);
  const payload = await readJSON<ScrumCardModalResponse>(response);
  if (!payload.card?.id) {
    throw new Error("Card modal did not return a card");
  }
  return payload;
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

export async function createScrumCard(
  title: string,
  description: string,
  column: string,
  projectID?: number | null,
): Promise<ScrumCard> {
  const body: Record<string, unknown> = { title, description, column };
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
  patch: ScrumCardEdit,
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

export type ScrumCardEdit = Partial<Pick<
  ScrumCard,
  | "title"
  | "description"
  | "ref_files"
  | "model_config"
  | "card_ticket"
  | "card_prompt"
  | "recipe_id"
  | "recipe"
  | "tags"
>>;

export type ScrumCardItemMutation =
  | { action: "add"; expected_updated_at: string; text: string }
  | { action: "toggle"; expected_updated_at: string; item_id: string; done: boolean }
  | { action: "remove"; expected_updated_at: string; item_id: string };

export async function mutateScrumCardItem(
  cardID: string,
  collection: "checklist" | "test-criteria",
  mutation: ScrumCardItemMutation,
  projectID?: number | null,
): Promise<ScrumCard> {
  const response = await fetch(cardURL(cardID, collection, projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(mutation),
  });
  const payload = await readJSON<{ card?: ScrumCard | null }>(response);
  if (!payload.card?.id) throw new Error("Scrum card item mutation did not return a card");
  return payload.card;
}

export async function chatScrumCard(
  cardID: string,
  message: string,
  operationID: LifecycleOperationID,
  projectID?: number | null,
): Promise<{ card: ScrumCard; reply: string; error?: string; agent?: string; action?: string }> {
  const response = await fetch(cardURL(cardID, "chat", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ operation_id: operationID, message }),
  });
  return readJSON(response);
}

export async function fetchScrumChannelPage(
  cardID: string,
  before: string,
  projectID?: number | null,
): Promise<ScrumChannelPage> {
  if (!before.trim()) throw new Error("An earlier channel page requires a cursor.");
  const query = new URLSearchParams({ before: before.trim(), limit: "50" });
  if (projectID != null) query.set("project_id", String(projectID));
  const response = await fetch(`/v1/scrum/cards/${encodeURIComponent(cardID)}/chat?${query.toString()}`);
  return readJSON<ScrumChannelPage>(response);
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
