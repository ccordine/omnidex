import { readJSON } from "./api";
import { projectQuery } from "./project_api";
import type { ScrumBoard, ScrumBoardResponse, ScrumCard } from "./scrum_types";

export async function fetchScrumBoard(projectID?: number | null): Promise<ScrumBoardResponse> {
  const response = await fetch(`/v1/scrum${projectQuery(projectID)}`);
  return readJSON<ScrumBoardResponse>(response);
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

export async function createScrumCard(
  title: string,
  description: string,
  column: string,
  projectID?: number | null,
): Promise<ScrumCard> {
  const response = await fetch(`/v1/scrum/cards${projectQuery(projectID)}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ title, description, column }),
  });
  const payload = await readJSON<{ card: ScrumCard }>(response);
  return payload.card;
}

function cardURL(cardID: string, suffix: string, projectID?: number | null): string {
  return `/v1/scrum/cards/${encodeURIComponent(cardID)}/${suffix}${projectQuery(projectID)}`;
}

export async function moveScrumCard(cardID: string, column: string, projectID?: number | null): Promise<ScrumCard> {
  const response = await fetch(cardURL(cardID, "move", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ column }),
  });
  const payload = await readJSON<{ card: ScrumCard }>(response);
  return payload.card;
}

export async function playScrumCard(
  cardID: string,
  projectID?: number | null,
  options: { pivot?: boolean } = {},
): Promise<ScrumCard & { message?: string }> {
  const response = await fetch(cardURL(cardID, "play", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ pivot: Boolean(options.pivot) }),
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
  const payload = await readJSON<{ card: ScrumCard }>(response);
  return payload.card;
}

export async function chatScrumCard(
  cardID: string,
  message: string,
  projectID?: number | null,
): Promise<{ card: ScrumCard; reply: string }> {
  const response = await fetch(cardURL(cardID, "chat", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ message }),
  });
  return readJSON(response);
}

export async function fetchScrumFiles(projectID?: number | null): Promise<{ files: string[]; root: string }> {
  const response = await fetch(`/v1/scrum/files${projectQuery(projectID)}`);
  return readJSON(response);
}
