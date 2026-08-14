import {
  SCRUM_COLUMNS,
  type ScrumCard,
  type ScrumCardFilePage,
  type ScrumCardModalResponse,
  type ScrumChannelPage,
} from "./scrum_types";
import {
  exactInteger,
  exactRecord,
  exactString,
  exactTimestamp,
  validateScrumCardProjection,
  validateScrumMessages,
} from "./scrum_card_response";
import { scrumChannelCursorOrdinal, validateScrumChannelCursor } from "./scrum_channel_cursor";
import { validateCanonicalRevision } from "./revision_authority";

const MODAL_TABS = ["card", "files", "tests", "channel"] as const;
const FILE_PAGE_FIELDS = [
  "files", "dirs", "file_path", "file_parent", "file_has_parent", "file_offset",
  "file_has_previous", "file_previous_offset", "file_has_more", "file_next_offset",
] as const;
const FILE_PAGE_SIZE = 50;

export { validateScrumChannelCursor } from "./scrum_channel_cursor";

function validatePath(value: unknown, label: string): string {
  const path = exactString(value, label, { maxBytes: 4096, canonical: true });
  if (path === "") return path;
  const segments = path.split("/");
  if (path.startsWith("/") || path.endsWith("/") || path.includes("\\") ||
      /^[A-Za-z]:\//.test(path) || segments.some((segment) => !segment || segment === "." || segment === "..")) {
    throw new Error(`${label} must be one canonical relative path or the explicit empty root.`);
  }
  return path;
}

function validatePathList(value: unknown, label: string): string[] {
  if (!Array.isArray(value)) throw new Error(`${label} must be an array.`);
  const paths = value.map((path, index) => validatePath(path, `${label}[${index}]`));
  if (paths.some((path) => path === "") || new Set(paths).size !== paths.length) {
    throw new Error(`${label} contains an empty or duplicate path.`);
  }
  return paths;
}

function validateFilePageRecord(
  value: unknown,
  expectedPath: string,
  expectedOffset: number,
  label: string,
): ScrumCardFilePage {
  const record = exactRecord(value, label, FILE_PAGE_FIELDS);
  const files = validatePathList(record.files, `${label}.files`);
  const dirs = validatePathList(record.dirs, `${label}.dirs`);
  if (files.length + dirs.length > FILE_PAGE_SIZE) throw new Error(`${label} exceeds the server page bound.`);
  const filePath = validatePath(record.file_path, `${label}.file_path`);
  const parent = validatePath(record.file_parent, `${label}.file_parent`);
  const offset = exactInteger(record.file_offset, `${label}.file_offset`, 1_000_000);
  const previousOffset = exactInteger(record.file_previous_offset, `${label}.file_previous_offset`, 1_000_000);
  const nextOffset = exactInteger(record.file_next_offset, `${label}.file_next_offset`, 1_000_000);
  for (const field of ["file_has_parent", "file_has_previous", "file_has_more"] as const) {
    if (typeof record[field] !== "boolean") throw new Error(`${label}.${field} must be boolean.`);
  }
  if (filePath !== expectedPath || offset !== expectedOffset) throw new Error(`${label} does not match the requested page cursor.`);
  if (record.file_has_parent !== (filePath !== "") || (record.file_has_parent ? parent === filePath : parent !== "")) {
    throw new Error(`${label} contains contradictory parent authority.`);
  }
  if (record.file_has_previous !== (offset > 0) || (record.file_has_previous ? previousOffset >= offset : previousOffset !== 0)) {
    throw new Error(`${label} contains contradictory previous-page authority.`);
  }
  if (record.file_has_more !== (nextOffset > offset) || (!record.file_has_more && nextOffset !== offset)) {
    throw new Error(`${label} contains contradictory next-page authority.`);
  }
  return {
    files, dirs, file_path: filePath, file_parent: parent,
    file_has_parent: record.file_has_parent, file_offset: offset,
    file_has_previous: record.file_has_previous, file_previous_offset: previousOffset,
    file_has_more: record.file_has_more, file_next_offset: nextOffset,
  };
}

export function validateScrumPlayQueue(value: unknown): ScrumCardModalResponse["play_queue"] {
  const record = exactRecord(value, "Scrum modal play_queue", [
    "running_card_id", "queued_count", "queued_card_ids", "queued_has_more",
  ]);
  const running = exactString(record.running_card_id, "Scrum modal play_queue.running_card_id", { maxBytes: 256, canonical: true });
  const count = exactInteger(record.queued_count, "Scrum modal play_queue.queued_count");
  if (!Array.isArray(record.queued_card_ids) || record.queued_card_ids.length > 20) {
    throw new Error("Scrum modal play_queue.queued_card_ids exceeds its bound.");
  }
  const ids = record.queued_card_ids.map((id, index) => exactString(id, `Scrum modal play_queue.queued_card_ids[${index}]`, {
    maxBytes: 256, nonblank: true, canonical: true,
  }));
  if (new Set(ids).size !== ids.length || count < ids.length || typeof record.queued_has_more !== "boolean" || record.queued_has_more !== (count > ids.length)) {
    throw new Error("Scrum modal play_queue contains contradictory queue authority.");
  }
  return { running_card_id: running, queued_count: count, queued_card_ids: ids, queued_has_more: record.queued_has_more };
}

export function validateScrumCardModalResponse(
  value: unknown,
  expectedCardID: string,
  expectedProjectID: number,
  expectedTab: (typeof MODAL_TABS)[number],
  expectedPath: string,
  expectedOffset: number,
): ScrumCardModalResponse {
  const fields = ["card", "board", "tab", "project_id", ...FILE_PAGE_FIELDS, "play_queue", "pilot_pending", "channel_before_cursor", "channel_has_more"];
  const record = exactRecord(value, "Scrum card modal response", fields);
  const card = validateScrumCardProjection(record.card, expectedCardID);
  const board = exactRecord(record.board, "Scrum card modal board", ["id", "name", "project_directory", "columns", "cards", "updated_at"]);
  const tab = exactString(record.tab, "Scrum card modal response.tab", { maxBytes: 16, nonblank: true, canonical: true });
  if (!MODAL_TABS.includes(tab as (typeof MODAL_TABS)[number]) || tab !== expectedTab) throw new Error("Scrum card modal response.tab does not match the request.");
  if (record.project_id !== expectedProjectID) throw new Error("Scrum card modal response.project_id does not match the request.");
  if (!Array.isArray(board.columns) || board.columns.length !== SCRUM_COLUMNS.length ||
      board.columns.some((column, index) => column !== SCRUM_COLUMNS[index])) {
    throw new Error("Scrum card modal board.columns must be the exact server inventory.");
  }
  if (!Array.isArray(board.cards) || board.cards.length !== 0) throw new Error("Scrum card modal board must not embed card inventory.");
  const boardID = exactString(board.id, "Scrum card modal board.id", { maxBytes: 256, nonblank: true, canonical: true });
  exactString(board.name, "Scrum card modal board.name", { nonblank: true });
  exactString(board.project_directory, "Scrum card modal board.project_directory");
  exactTimestamp(board.updated_at, "Scrum card modal board.updated_at");
  const modalFilePage = Object.fromEntries(FILE_PAGE_FIELDS.map((field) => [field, record[field]]));
  const filePage = validateFilePageRecord(modalFilePage, expectedTab === "files" ? expectedPath : "", expectedTab === "files" ? expectedOffset : 0, "Scrum card modal file page");
  if (expectedTab !== "files" && (filePage.files.length !== 0 || filePage.dirs.length !== 0)) {
    throw new Error("Non-file Scrum modal response must not contain file inventory.");
  }
  if (typeof record.pilot_pending !== "boolean" || typeof record.channel_has_more !== "boolean") {
    throw new Error("Scrum card modal pending/channel authority must be boolean.");
  }
  const cursor = validateScrumChannelCursor(record.channel_before_cursor, "Scrum card modal channel_before_cursor", true);
  if (record.channel_has_more !== (cursor !== "")) throw new Error("Scrum card modal channel cursor is contradictory.");
  if (card.channel_before_cursor !== cursor || card.channel_has_more !== record.channel_has_more) {
    throw new Error("Scrum card modal channel cursor contradicts its authoritative card.");
  }
  return {
    card,
    board: {
      id: boardID,
      name: board.name as string,
      project_directory: board.project_directory as string,
      columns: [...SCRUM_COLUMNS],
      cards: [],
      updated_at: board.updated_at as string,
    },
    tab: tab as (typeof MODAL_TABS)[number], project_id: expectedProjectID,
    ...filePage, play_queue: validateScrumPlayQueue(record.play_queue),
    pilot_pending: record.pilot_pending, channel_before_cursor: cursor,
    channel_has_more: record.channel_has_more,
  };
}

export function validateScrumCardFilePageResponse(value: unknown, path: string, offset: number): ScrumCardFilePage {
  return validateFilePageRecord(value, path, offset, "Scrum card file-page response");
}

export function validateScrumPlayResponse(value: unknown, cardID: string, projectID: number): ScrumCard {
  const record = exactRecord(value, "Scrum play response", ["project_id", "card_id", "action", "job_id", "queue_order", "card"]);
  if (exactInteger(record.project_id, "Scrum play response.project_id") !== projectID ||
      exactString(record.card_id, "Scrum play response.card_id", { maxBytes: 256, nonblank: true, canonical: true }) !== cardID) {
    throw new Error("Scrum play response does not match the requested project/card route.");
  }
  const card = validateScrumCardProjection(record.card, cardID);
  const jobID = exactString(record.job_id, "Scrum play response.job_id", { maxBytes: 64, canonical: true });
  const queueOrder = exactInteger(record.queue_order, "Scrum play response.queue_order");
  if (jobID !== (card.job_id ?? "") || queueOrder !== (card.queue_order ?? 0)) {
    throw new Error("Scrum play response contradicts its authoritative card.");
  }
  if (record.action === "started" || record.action === "already_running") {
    if (!/^[1-9][0-9]*$/.test(jobID) || card.column !== "in_progress" || card.play_state !== "running" || queueOrder !== 0) {
      throw new Error("Scrum play response has contradictory running job authority.");
    }
  } else if (record.action === "queued" || record.action === "already_queued") {
    if (jobID !== "" || card.column !== "assigned" || card.play_state !== "queued" || queueOrder <= 0) {
      throw new Error("Scrum play response has contradictory queue authority.");
    }
  } else {
    throw new Error("Scrum play response.action is not registered.");
  }
  return card;
}

export function validateScrumPauseResponse(value: unknown, cardID: string, projectID: number): ScrumCard {
  const record = exactRecord(value, "Scrum pause response", ["project_id", "card_id", "action", "job_id", "queue_order", "card"]);
  if (exactInteger(record.project_id, "Scrum pause response.project_id") !== projectID ||
      exactString(record.card_id, "Scrum pause response.card_id", { maxBytes: 256, nonblank: true, canonical: true }) !== cardID ||
      record.action !== "paused" || record.job_id !== "" || record.queue_order !== 0) {
    throw new Error("Scrum pause response does not attest the requested project/card pause.");
  }
  const card = validateScrumCardProjection(record.card, cardID);
  if (card.play_state !== "paused" || card.column !== "assigned" || (card.job_id ?? "") !== "" || (card.queue_order ?? 0) !== 0) {
    throw new Error("Scrum pause response contradicts its authoritative card.");
  }
  return card;
}

export function validateScrumChannelActionResponse(
  value: unknown,
  cardID: string,
  projectID: number,
  operationID: string,
  message: string,
): { card: ScrumCard; action: "started" | "replanned" | "feedback" } {
  const record = exactRecord(value, "Scrum channel action response", ["operation_id", "project_id", "card", "action"]);
  if (exactString(record.operation_id, "Scrum channel action response.operation_id", { maxBytes: 84, canonical: true }) !== operationID) {
    throw new Error("Scrum channel action response.operation_id does not match the request.");
  }
  if (exactInteger(record.project_id, "Scrum channel action response.project_id") !== projectID) {
    throw new Error("Scrum channel action response.project_id does not match the request.");
  }
  if (!["started", "replanned", "feedback"].includes(String(record.action))) {
    throw new Error("Scrum channel action response.action is not registered.");
  }
  const card = validateScrumCardProjection(record.card, cardID);
  return { card, action: record.action as "started" | "replanned" | "feedback" };
}

export function validateScrumChannelPageResponse(
  value: unknown,
  cardID: string,
  projectID: number,
  requestedBefore: string,
  requestedLimit: number,
): ScrumChannelPage {
  const record = exactRecord(value, "Scrum channel page response", [
    "project_id", "card_id", "requested_before", "limit", "messages", "before_cursor", "has_more", "busy",
  ]);
  const responseProjectID = exactInteger(record.project_id, "Scrum channel page response.project_id");
  const responseCardID = exactString(record.card_id, "Scrum channel page response.card_id", {
    maxBytes: 256, nonblank: true, canonical: true,
  });
  const responseRequestedBefore = validateScrumChannelCursor(
    record.requested_before,
    "Scrum channel page response.requested_before",
    false,
  );
  const responseLimit = exactInteger(record.limit, "Scrum channel page response.limit", 50);
  if (responseProjectID !== projectID || responseCardID !== cardID ||
      responseRequestedBefore !== requestedBefore || responseLimit !== requestedLimit) {
    throw new Error("Scrum channel page response does not match the requested route and page cursor.");
  }
  const messages = validateScrumMessages(record.messages, "Scrum channel page response.messages", 50);
  const cursor = validateScrumChannelCursor(record.before_cursor, "Scrum channel page response.before_cursor", true);
  if (typeof record.has_more !== "boolean" || typeof record.busy !== "boolean") throw new Error("Scrum channel page state must be boolean.");
  if (record.has_more !== (cursor !== "")) throw new Error("Scrum channel page cursor is contradictory.");
  if (cursor !== "" && scrumChannelCursorOrdinal(cursor) >= scrumChannelCursorOrdinal(requestedBefore)) {
    throw new Error("Scrum channel page did not advance to an earlier authoritative cursor.");
  }
  return {
    project_id: responseProjectID,
    card_id: responseCardID,
    requested_before: responseRequestedBefore,
    limit: responseLimit,
    messages,
    before_cursor: cursor,
    has_more: record.has_more,
    busy: record.busy,
  };
}

export function validateScrumCardEnvelope(value: unknown, cardID?: string): ScrumCard {
  const record = exactRecord(value, "Scrum card envelope", ["card"]);
  return validateScrumCardProjection(record.card, cardID);
}

export function validateScrumDeleteResponse(
  value: unknown,
  cardID: string,
  projectID: number,
  expectedUpdatedAt: string,
): void {
  const revision = validateCanonicalRevision(expectedUpdatedAt, "Scrum card deletion expected_updated_at");
  const record = exactRecord(value, "Scrum card deletion response", [
    "commit_state", "project_id", "card_id", "expected_updated_at", "deleted",
  ]);
  if (record.commit_state !== "committed" ||
      exactInteger(record.project_id, "Scrum card deletion response.project_id") !== projectID ||
      exactString(record.card_id, "Scrum card deletion response.card_id", {
        maxBytes: 256, nonblank: true, canonical: true,
      }) !== cardID ||
      validateCanonicalRevision(record.expected_updated_at, "Scrum card deletion response.expected_updated_at") !== revision ||
      record.deleted !== true) {
    throw new Error("Scrum card deletion response does not attest the requested revision-bound deletion.");
  }
}
