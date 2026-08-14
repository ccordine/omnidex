import {
  exactInteger,
  exactRecord,
  exactString,
  exactTimestamp,
  validateScrumCardProjection,
} from "./scrum_card_response";
import { validateScrumPlayQueue } from "./scrum_response_authority";
import {
  SCRUM_COLUMNS,
  type ScrumAutoWorkConfig,
  type ScrumBoardHTML,
  type ScrumBoardResponse,
  type ScrumColumn,
  type ScrumFlowSummary,
} from "./scrum_types";

const BOARD_RESPONSE_MAX_BYTES = 6 * 1024 * 1024;
const BOARD_PAGE_SIZE = 20;
const MAX_CARD_OFFSET = 1_000_000;
const AUTO_WORK_COLUMNS = ["backlog", "ready", "assigned", "in_progress", "blocked"] as const;

function exactColumn(value: unknown, label: string): ScrumColumn {
  const column = exactString(value, label, { maxBytes: 32, nonblank: true, canonical: true });
  if (!SCRUM_COLUMNS.includes(column as ScrumColumn)) throw new Error(`${label} is not registered.`);
  return column as ScrumColumn;
}

function validateColumns(value: unknown, label: string): string[] {
  if (!Array.isArray(value) || value.length !== SCRUM_COLUMNS.length ||
      value.some((column, index) => column !== SCRUM_COLUMNS[index])) {
    throw new Error(`${label} must be the exact server column inventory.`);
  }
  return [...SCRUM_COLUMNS];
}

function validateColumnCounts(value: unknown): Record<string, number> {
  const record = exactRecord(value, "Scrum board column_counts", SCRUM_COLUMNS, SCRUM_COLUMNS);
  return Object.fromEntries(SCRUM_COLUMNS.map((column) => [
    column,
    exactInteger(record[column], `Scrum board column_counts.${column}`),
  ]));
}

export function validateScrumAutoWorkConfig(value: unknown, label = "Scrum board auto_work"): ScrumAutoWorkConfig {
  const record = exactRecord(value, label, ["enabled", "source_columns"]);
  if (typeof record.enabled !== "boolean" || !Array.isArray(record.source_columns) || record.source_columns.length < 1) {
    throw new Error(`${label} is invalid.`);
  }
  const columns = record.source_columns.map((column, index) => {
    const value = exactString(column, `${label}.source_columns[${index}]`, { maxBytes: 32, nonblank: true, canonical: true });
    if (!AUTO_WORK_COLUMNS.includes(value as (typeof AUTO_WORK_COLUMNS)[number])) {
      throw new Error(`${label} contains an unsupported source column.`);
    }
    return value;
  });
  if (new Set(columns).size !== columns.length) throw new Error(`${label} contains duplicate source columns.`);
  return { enabled: record.enabled, source_columns: columns };
}

function validateFlowSummary(value: unknown): ScrumFlowSummary {
  const fields = ["total_cards", "likely_incomplete", "uncertain", "likely_complete", "assigned_returns_total", "long_conversations"] as const;
  const record = exactRecord(value, "Scrum board flow_summary", fields, fields);
  const result = Object.fromEntries(fields.map((field) => [
    field,
    exactInteger(record[field], `Scrum board flow_summary.${field}`),
  ])) as ScrumFlowSummary;
  if (result.total_cards !== result.likely_incomplete + result.uncertain + result.likely_complete ||
      result.long_conversations > result.total_cards) {
    throw new Error("Scrum board flow_summary contains contradictory totals.");
  }
  return result;
}

function validateHTML(value: unknown): ScrumBoardHTML {
  const fields = ["board", "columns", "focus", "flow_summary", "pagination", "bundle"] as const;
  const record = exactRecord(value, "Scrum board html", fields, fields);
  const result = Object.fromEntries(fields.map((field) => [
    field,
    exactString(record[field], `Scrum board html.${field}`, { maxBytes: 2 * 1024 * 1024 }),
  ])) as ScrumBoardHTML;
  if (!result.bundle.trim()) throw new Error("Scrum board html.bundle must not be blank.");
  return result;
}

export function validateScrumBoardResponse(
  value: unknown,
  expectedProjectID: number,
  expectedColumn: ScrumColumn,
  expectedOffset: number,
): ScrumBoardResponse {
  if (new TextEncoder().encode(JSON.stringify(value)).byteLength > BOARD_RESPONSE_MAX_BYTES) {
    throw new Error("Scrum board response exceeds its byte bound.");
  }
  const fields = [
    "board", "cards_by_col", "html", "project_id", "all_columns", "visible_column",
    "column_counts", "card_offset", "card_has_more", "auto_work", "auto_work_complete",
    "play_queue", "flow_summary",
  ] as const;
  const record = exactRecord(value, "Scrum board response", fields, fields);
  if (record.project_id !== expectedProjectID) throw new Error("Scrum board response project_id does not match the request.");
  const visibleColumn = exactColumn(record.visible_column, "Scrum board visible_column");
  if (visibleColumn !== expectedColumn) throw new Error("Scrum board response visible_column does not match the request.");
  const cardOffset = exactInteger(record.card_offset, "Scrum board card_offset", MAX_CARD_OFFSET);
  if (cardOffset !== expectedOffset) throw new Error("Scrum board response card_offset does not match the request.");
  if (typeof record.card_has_more !== "boolean" || typeof record.auto_work_complete !== "boolean") {
    throw new Error("Scrum board page/auto-work completion authority must be boolean.");
  }
  const board = exactRecord(record.board, "Scrum board", ["id", "name", "project_directory", "columns", "cards", "updated_at"]);
  if (board.id !== `project_${expectedProjectID}`) throw new Error("Scrum board ID does not match the requested project.");
  exactString(board.name, "Scrum board.name", { nonblank: true });
  exactString(board.project_directory, "Scrum board.project_directory");
  exactTimestamp(board.updated_at, "Scrum board.updated_at");
  if (!Array.isArray(board.columns) || board.columns.length !== 1 || board.columns[0] !== visibleColumn) {
    throw new Error("Scrum board columns do not match the requested viewport.");
  }
  if (!Array.isArray(board.cards) || board.cards.length > BOARD_PAGE_SIZE) {
    throw new Error(`Scrum board page must contain at most ${BOARD_PAGE_SIZE} cards.`);
  }
  const cards = board.cards.map((card) => validateScrumCardProjection(card, undefined, { summary: "required" }));
  if (cards.some((card) => card.column !== visibleColumn) || new Set(cards.map((card) => card.id)).size !== cards.length) {
    throw new Error("Scrum board page contains a wrong-column or duplicate card.");
  }
  for (let index = 1; index < cards.length; index += 1) {
    if ((cards[index - 1].board_order as number) > (cards[index].board_order as number)) {
      throw new Error("Scrum board page cards are not in authoritative order.");
    }
  }
  const cardsByColumn = exactRecord(record.cards_by_col, "Scrum board cards_by_col", [visibleColumn], [visibleColumn]);
  if (JSON.stringify(cardsByColumn[visibleColumn]) !== JSON.stringify(board.cards)) {
    throw new Error("Scrum board cards_by_col contradicts the board page.");
  }
  const counts = validateColumnCounts(record.column_counts);
  const visibleCount = counts[visibleColumn];
  if (cardOffset > visibleCount || record.card_has_more !== (cardOffset + cards.length < visibleCount)) {
    throw new Error("Scrum board pagination contradicts its column count.");
  }
  const allColumns = validateColumns(record.all_columns, "Scrum board all_columns");
  return {
    board: {
      id: board.id as string,
      name: board.name as string,
      project_directory: board.project_directory as string,
      columns: [visibleColumn], cards,
      updated_at: board.updated_at as string,
    },
    cards_by_col: { [visibleColumn]: cards },
    html: validateHTML(record.html),
    project_id: expectedProjectID,
    all_columns: allColumns,
    visible_column: visibleColumn,
    column_counts: counts,
    card_offset: cardOffset,
    card_has_more: record.card_has_more,
    auto_work: validateScrumAutoWorkConfig(record.auto_work),
    auto_work_complete: record.auto_work_complete,
    play_queue: validateScrumPlayQueue(record.play_queue),
    flow_summary: validateFlowSummary(record.flow_summary),
  };
}
