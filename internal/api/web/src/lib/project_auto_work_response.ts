import { validateScrumAutoWorkConfig } from "./scrum_board_response";
import {
  exactInteger,
  exactRecord,
  exactString,
  exactTimestamp,
  validateScrumCardProjection,
} from "./scrum_card_response";
import { validateScrumPlayQueue } from "./scrum_response_authority";
import { SCRUM_COLUMNS, type ScrumBoard, type ScrumBoardResponse, type ScrumCard } from "./scrum_types";

const PROJECT_AUTO_WORK_RESPONSE_MAX_BYTES = 6 * 1024 * 1024;

type ProjectAutoWorkAuthority = {
  project_id: number;
  auto_work: { enabled: boolean; source_columns: string[] };
  active_card_id: string;
  active_card_updated_at: string;
  job_id: number;
  paused_cards: number;
  message: string;
  board?: ScrumBoard;
  card?: ScrumCard;
  play_queue?: ScrumBoardResponse["play_queue"];
};

export type ProjectAutoWorkResponse = ProjectAutoWorkAuthority & (
  | { commit_state: "committed"; operation_error?: never }
  | { commit_state: "committed_degraded"; operation_error: string }
);

function validateBoard(value: unknown, projectID: number): ScrumBoard {
  const record = exactRecord(value, "Project auto-work board", [
    "id", "name", "project_directory", "columns", "cards", "updated_at",
  ]);
  if (record.id !== `project_${projectID}`) throw new Error("Project auto-work board ID does not match the request.");
  const name = exactString(record.name, "Project auto-work board.name", { nonblank: true });
  const directory = exactString(record.project_directory, "Project auto-work board.project_directory");
  const updatedAt = exactTimestamp(record.updated_at, "Project auto-work board.updated_at");
  if (!Array.isArray(record.columns) || record.columns.length !== SCRUM_COLUMNS.length ||
      record.columns.some((column, index) => column !== SCRUM_COLUMNS[index])) {
    throw new Error("Project auto-work board.columns is not the exact server inventory.");
  }
  if (!Array.isArray(record.cards) || record.cards.length !== 0) {
    throw new Error("Project auto-work board must not embed card inventory.");
  }
  return {
    id: record.id as string,
    name,
    project_directory: directory,
    columns: [...SCRUM_COLUMNS],
    cards: [],
    updated_at: updatedAt,
  };
}

export function validateProjectAutoWorkResponse(
  value: unknown,
  expectedProjectID: number,
  status: number,
): ProjectAutoWorkResponse {
  if (new TextEncoder().encode(JSON.stringify(value)).byteLength > PROJECT_AUTO_WORK_RESPONSE_MAX_BYTES) {
    throw new Error("Project auto-work response exceeds its byte bound.");
  }
  const core = [
    "commit_state", "project_id", "auto_work", "active_card_id", "active_card_updated_at",
    "job_id", "paused_cards", "message",
  ] as const;
  const record = exactRecord(value, "Project auto-work response", [
    ...core, "board", "card", "play_queue", "operation_error",
  ], core);
  if (record.commit_state !== "committed" && record.commit_state !== "committed_degraded") {
    throw new Error("Project auto-work response lacks one exact registered commit state.");
  }
  if ((record.commit_state === "committed" && status !== 200) ||
      (record.commit_state === "committed_degraded" && status !== 207)) {
    throw new Error("Project auto-work response commit state contradicts its HTTP status.");
  }
  const projectID = exactInteger(record.project_id, "Project auto-work response.project_id");
  if (projectID !== expectedProjectID) throw new Error("Project auto-work response.project_id does not match the request.");
  const activeCardID = exactString(record.active_card_id, "Project auto-work response.active_card_id", {
    maxBytes: 256, canonical: true,
  });
  const activeRevision = exactString(record.active_card_updated_at, "Project auto-work response.active_card_updated_at", {
    maxBytes: 64, canonical: true,
  });
  if (activeRevision !== "") exactTimestamp(activeRevision, "Project auto-work response.active_card_updated_at");
  const jobID = exactInteger(record.job_id, "Project auto-work response.job_id");
  const pausedCards = exactInteger(record.paused_cards, "Project auto-work response.paused_cards");
  if ((activeCardID === "") !== (activeRevision === "" && jobID === 0)) {
    throw new Error("Project auto-work response has contradictory active-card authority.");
  }
  const authority: ProjectAutoWorkAuthority = {
    project_id: projectID,
    auto_work: validateScrumAutoWorkConfig(record.auto_work, "Project auto-work response.auto_work"),
    active_card_id: activeCardID,
    active_card_updated_at: activeRevision,
    job_id: jobID,
    paused_cards: pausedCards,
    message: exactString(record.message, "Project auto-work response.message", { maxBytes: 512, nonblank: true }),
  };
  if ("board" in record) authority.board = validateBoard(record.board, projectID);
  if ("card" in record) authority.card = validateScrumCardProjection(record.card, activeCardID || undefined);
  if ("play_queue" in record) authority.play_queue = validateScrumPlayQueue(record.play_queue);
  if (record.commit_state === "committed") {
    if ("operation_error" in record || !authority.board || !authority.play_queue ||
        (activeCardID !== "" && (!authority.card || authority.card.updated_at !== activeRevision)) ||
        (activeCardID === "" && authority.card !== undefined)) {
      throw new Error("Committed project auto-work response lacks its exact authoritative projections.");
    }
    return { commit_state: "committed", ...authority };
  }
  const operationError = exactString(record.operation_error, "Project auto-work response.operation_error", {
    maxBytes: 16 * 1024, nonblank: true,
  });
  return { commit_state: "committed_degraded", ...authority, operation_error: operationError };
}
