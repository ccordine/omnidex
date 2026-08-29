import { readJSON, SCRUM_CHANNEL_RESPONSE_MAX_BYTES } from "./api";
import type { LifecycleOperationID } from "./lifecycle_operation";
import { projectQuery } from "./project_api";
import { validateScrumBoardResponse } from "./scrum_board_response";
import { validateScrumCardProjection } from "./scrum_card_response";
import {
  validateScrumCardEnvelope,
  validateScrumChannelActionResponse,
  validateScrumChannelPageResponse,
  validateScrumDeleteResponse,
  validateScrumPauseResponse,
  validateScrumPlayResponse,
  validateScrumChannelCursor,
} from "./scrum_response_authority";
import { validateCanonicalRevision } from "./revision_authority";
import { SCRUM_COLUMNS, type ScrumBoardResponse, type ScrumCard, type ScrumChannelPage, type ScrumColumn } from "./scrum_types";

export { applyScrumCardElaboration, assembleScrumCardTicket } from "./scrum_ticket_api";
export { patchScrumAutoWork, type ScrumAutoWorkMutationResult } from "./scrum_auto_work_api";
export { fetchScrumCardFilePage, fetchScrumCardModal } from "./scrum_modal_api";

function scrumBoardQuery(projectID: number, options: { column: ScrumColumn; cardOffset: number }): string {
  const query = new URLSearchParams(projectQuery(projectID).slice(1));
  if (!SCRUM_COLUMNS.includes(options.column)) {
    throw new Error("Scrum board column must be one exact registered column.");
  }
  query.set("column", options.column);
  if (!Number.isSafeInteger(options.cardOffset) || options.cardOffset < 0 || options.cardOffset > 1_000_000) {
    throw new Error("Scrum board card offset must be an integer between 0 and 1000000.");
  }
  query.set("card_offset", String(options.cardOffset));
  const encoded = query.toString();
  return encoded ? `?${encoded}` : "";
}

export async function fetchScrumBoard(
  projectID: number,
  options: { column: ScrumColumn; cardOffset: number },
  signal?: AbortSignal,
): Promise<ScrumBoardResponse> {
  const response = await fetch(`/v1/scrum${scrumBoardQuery(projectID, options)}`, { signal });
  return validateScrumBoardResponse(
    await readJSON<unknown>(response),
    projectID,
    options.column,
    options.cardOffset,
  );
}

export async function createScrumCard(
  title: string,
  description: string,
  column: string,
  projectID: number,
): Promise<ScrumCard> {
  const body: Record<string, unknown> = { title, description, column };
  const response = await fetch(`/v1/scrum/cards${projectQuery(projectID)}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return validateScrumCardEnvelope(await readJSON<unknown>(response));
}

function cardURL(cardID: string, suffix: string, projectID: number): string {
  return `/v1/scrum/cards/${encodeURIComponent(cardID)}/${suffix}${projectQuery(projectID)}`;
}

export type ScrumCardMutationResult = {
  commit_state: "committed" | "committed_degraded";
  card: ScrumCard;
  operation_error?: string;
};

function validateScrumCardMutationResult(payload: unknown, expectedCardID: string): ScrumCardMutationResult {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new Error("Scrum card mutation response must be one typed object.");
  }
  const record = payload as Record<string, unknown>;
  const fields = Object.keys(record);
  if (fields.some((field) => !["commit_state", "card", "operation_error"].includes(field))) {
    throw new Error("Scrum card mutation response contains an unknown field.");
  }
  if (record.commit_state !== "committed" && record.commit_state !== "committed_degraded") {
    throw new Error("Scrum card mutation response lacks one exact registered commit state.");
  }
  if (!record.card || typeof record.card !== "object" || Array.isArray(record.card)) {
    throw new Error("Scrum card mutation response lacks authoritative card state.");
  }
  const card = validateScrumCardProjection(record.card, expectedCardID);
  if (record.commit_state === "committed_degraded") {
    if (typeof record.operation_error !== "string" || !record.operation_error.trim()) {
      throw new Error("Degraded Scrum card mutation response lacks its post-commit failure.");
    }
  } else if (Object.prototype.hasOwnProperty.call(record, "operation_error")) {
    throw new Error("Complete Scrum card mutation response must not contain a post-commit failure.");
  }
  return {
    commit_state: record.commit_state,
    card,
    ...(record.operation_error === undefined ? {} : { operation_error: record.operation_error as string }),
  };
}

export async function moveScrumCard(
  cardID: string,
  column: string,
  expectedUpdatedAt: string,
  projectID: number,
  options: { before_card_id?: string | null } = {},
): Promise<ScrumCardMutationResult> {
  const body: Record<string, string> = { column, expected_updated_at: expectedUpdatedAt };
  if (options.before_card_id) {
    body.before_card_id = options.before_card_id;
  }
  const response = await fetch(cardURL(cardID, "move", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return validateScrumCardMutationResult(await readJSON<unknown>(response), cardID);
}

export async function playScrumCard(
  cardID: string,
  expectedUpdatedAt: string,
  projectID: number,
  options: { pivot: boolean },
): Promise<ScrumCard> {
  const body: Record<string, unknown> = {
    expected_updated_at: expectedUpdatedAt,
    pivot: options.pivot,
  };
  const response = await fetch(cardURL(cardID, "play", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return validateScrumPlayResponse(await readJSON<unknown>(response), cardID, projectID);
}

export async function pauseScrumCard(cardID: string, expectedUpdatedAt: string, projectID: number): Promise<ScrumCard> {
  const response = await fetch(cardURL(cardID, "pause", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ expected_updated_at: expectedUpdatedAt }),
  });
  return validateScrumPauseResponse(await readJSON<unknown>(response), cardID, projectID);
}

export async function doneScrumCard(cardID: string, expectedUpdatedAt: string, projectID: number): Promise<ScrumCardMutationResult> {
  const response = await fetch(cardURL(cardID, "done", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ expected_updated_at: expectedUpdatedAt }),
  });
  return validateScrumCardMutationResult(await readJSON<unknown>(response), cardID);
}

export async function deleteScrumCard(cardID: string, expectedUpdatedAt: string, projectID: number): Promise<void> {
  const revision = validateCanonicalRevision(expectedUpdatedAt, "Scrum card deletion expected_updated_at");
  const response = await fetch(`/v1/scrum/cards/${encodeURIComponent(cardID)}${projectQuery(projectID)}`, {
    method: "DELETE",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify({ expected_updated_at: revision }),
  });
  validateScrumDeleteResponse(await readJSON<unknown>(response), cardID, projectID, revision);
}

export async function patchScrumCard(
  cardID: string,
  expectedUpdatedAt: string,
  patch: ScrumCardEdit,
  projectID: number,
): Promise<ScrumCard> {
  const response = await fetch(`/v1/scrum/cards/${encodeURIComponent(cardID)}${projectQuery(projectID)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ expected_updated_at: expectedUpdatedAt, ...patch }),
  });
  return validateScrumCardEnvelope(await readJSON<unknown>(response), cardID);
}

export type ScrumCardEdit = Partial<Pick<
  ScrumCard,
  | "title"
  | "description"
  | "ref_files"
  | "card_ticket"
  | "card_prompt"
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
  projectID: number,
): Promise<ScrumCard> {
  const response = await fetch(cardURL(cardID, collection, projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(mutation),
  });
  return validateScrumCardEnvelope(await readJSON<unknown>(response), cardID);
}

export async function chatScrumCard(
  cardID: string,
  message: string,
  operationID: LifecycleOperationID,
  projectID: number,
): Promise<{ card: ScrumCard; action: "started" | "replanned" | "feedback" }> {
  if (!cardID || cardID !== cardID.trim() || cardID.includes("\0")) {
    throw new Error("Scrum channel action requires one exact canonical card ID.");
  }
  const response = await fetch(cardURL(cardID, "chat", projectID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ operation_id: operationID, message }),
  });
  return validateScrumChannelActionResponse(
    await readJSON<unknown>(response, SCRUM_CHANNEL_RESPONSE_MAX_BYTES),
    cardID,
    projectID,
    operationID,
    message,
  );
}

export async function fetchScrumChannelPage(
  cardID: string,
  before: string,
  projectID: number,
): Promise<ScrumChannelPage> {
  if (!cardID || cardID !== cardID.trim() || cardID.includes("\0")) {
    throw new Error("Scrum channel page requires one exact canonical card ID.");
  }
  before = validateScrumChannelCursor(before, "An earlier channel page", false);
  const query = new URLSearchParams(projectQuery(projectID).slice(1));
  query.set("before", before);
  query.set("limit", "50");
  const response = await fetch(`/v1/scrum/cards/${encodeURIComponent(cardID)}/chat?${query.toString()}`);
  return validateScrumChannelPageResponse(
    await readJSON<unknown>(response, SCRUM_CHANNEL_RESPONSE_MAX_BYTES),
    cardID,
    projectID,
    before,
    50,
  );
}

export async function fetchScrumTags(
  query: string,
  projectID: number,
  limit = 40,
): Promise<string[]> {
  if (typeof projectID !== "number" || !Number.isSafeInteger(projectID) || projectID <= 0) {
    throw new Error("Scrum tag catalog requires one positive integer project ID.");
  }
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100) {
    throw new Error("Scrum tag catalog limit must be an integer between 1 and 100.");
  }
  if (query.includes("\0")) {
    throw new Error("Scrum tag catalog search must not contain NUL.");
  }
  const encodedQuery = new TextEncoder().encode(query);
  if (new TextDecoder().decode(encodedQuery) !== query) {
    throw new Error("Scrum tag catalog search must be valid Unicode.");
  }
  if (encodedQuery.byteLength > 256) {
    throw new Error("Scrum tag catalog search exceeds the 256-byte bound.");
  }
  const params = new URLSearchParams({ project_id: String(projectID) });
  if (query !== "") params.set("q", query);
  params.set("limit", String(limit));
  const response = await fetch(`/v1/scrum/tags?${params.toString()}`);
  const payload = await readJSON<unknown>(response);
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new Error("Scrum tag catalog response must be one typed object.");
  }
  const fields = Object.keys(payload);
  if (fields.length !== 1 || fields[0] !== "tags") {
    throw new Error("Scrum tag catalog response must contain only tags.");
  }
  const tags = (payload as { tags?: unknown }).tags;
  if (!Array.isArray(tags) || tags.length > limit || tags.some((tag) => typeof tag !== "string" || !tag || tag.includes("\0"))) {
    throw new Error("Scrum tag catalog response contains invalid or unbounded tags.");
  }
  if (new Set(tags).size !== tags.length || tags.some((tag, index) => index > 0 && tags[index - 1] >= tag)) {
    throw new Error("Scrum tag catalog response must be uniquely sorted.");
  }
  return tags;
}
