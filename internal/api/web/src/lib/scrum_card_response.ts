import { SCRUM_CHANNEL_RESPONSE_MAX_BYTES } from "./api";
import { SCRUM_COLUMNS, type ScrumCard, type ScrumChatMessage } from "./scrum_types";
import { validateScrumChannelCursor } from "./scrum_channel_cursor";
import { exactInteger, exactMicrosecondTimestamp, exactRecord, exactString, exactTimestamp } from "./response_validation";

export { exactInteger, exactRecord, exactString, exactTimestamp } from "./response_validation";

const encoder = new TextEncoder();
const CHANNEL_PAGE_MAX_MESSAGES = 50;
const CHANNEL_MESSAGE_MAX_BYTES = 4 * 1024 * 1024;
const CHANNEL_PAGE_MAX_BYTES = 4 * 1024 * 1024;
// Mirrors the frozen queue presentation bounds; browser responses fail closed
// before becoming React or Stimulus authority.
const CARD_ARRAY_MAX_ITEMS = 256;
const CARD_ITEM_ID_MAX_BYTES = 256;
const CARD_ITEM_TEXT_MAX_BYTES = 16 * 1024;
const CARD_PRESENTATION_AGGREGATE_MAX_BYTES = 1024 * 1024;
const CARD_REF_MAX_BYTES = 4 * 1024;
const CARD_REFS_AGGREGATE_MAX_BYTES = 256 * 1024;
const CARD_TAG_MAX_ITEMS = 64;
const CARD_TAG_MAX_BYTES = 128;
const CARD_TAGS_AGGREGATE_MAX_BYTES = 16 * 1024;
const CARD_TICKET_MAX_BYTES = 64 * 1024;
const CARD_PROMPT_MAX_BYTES = 16 * 1024;
const CARD_TITLE_MAX_BYTES = 1024;
const CARD_DESCRIPTION_MAX_BYTES = 32 * 1024;
const FLOW_TRANSITION_COUNTER_MAX = 2_147_483_647;
const FLOW_WIDE_COUNTER_MAX = Number.MAX_SAFE_INTEGER;
const FLOW_SIGNAL_MAX_ITEMS = 64;
const FLOW_SIGNAL_MAX_BYTES = 1024;
const FLOW_SIGNALS_AGGREGATE_MAX_BYTES = 64 * 1024;

const CARD_FIELDS = [
  "id", "title", "description", "column", "checklist", "ref_files", "chat",
  "card_ticket", "card_prompt", "tags", "test_criteria", "flow_metrics", "summary",
  "checklist_done", "checklist_total", "ref_file_count", "chat_count",
  "channel_before_cursor", "channel_has_more", "test_criteria_done", "test_criteria_total",
  "has_card_ticket", "job_id", "play_state", "queue_order", "board_order", "created_at", "updated_at",
] as const;

const REQUIRED_CARD_FIELDS = [
  "id", "title", "description", "column", "checklist", "ref_files", "chat", "tags",
  "test_criteria", "channel_before_cursor", "channel_has_more", "created_at", "updated_at",
] as const;

function exactStringArray(value: unknown, label: string, maximum: number, itemBytes: number, aggregateBytes: number): string[] {
  if (!Array.isArray(value) || value.length > maximum) throw new Error(`${label} exceeds its item bound.`);
  const result = value.map((item, index) => exactString(item, `${label}[${index}]`, {
    maxBytes: itemBytes, nonblank: true, canonical: true,
  }));
  if (new Set(result).size !== result.length) throw new Error(`${label} must not contain duplicates.`);
  if (encoder.encode(JSON.stringify(result)).byteLength > aggregateBytes) throw new Error(`${label} exceeds its aggregate bound.`);
  return result;
}

function validateRefFiles(value: unknown): string[] {
  const refs = exactStringArray(
    value, "Scrum card response.ref_files", CARD_ARRAY_MAX_ITEMS,
    CARD_REF_MAX_BYTES, CARD_REFS_AGGREGATE_MAX_BYTES,
  );
  if (refs.some((path) => {
    const segments = path.split("/");
    return path.startsWith("/") || path.endsWith("/") || path.includes("\\") ||
      /^[A-Za-z]:\//.test(path) || segments.some((segment) => !segment || segment === "." || segment === "..");
  })) {
    throw new Error("Scrum card response.ref_files contains a noncanonical relative path.");
  }
  return refs;
}

function validateTags(value: unknown): string[] {
  const tags = exactStringArray(
    value, "Scrum card response.tags", CARD_TAG_MAX_ITEMS,
    CARD_TAG_MAX_BYTES, CARD_TAGS_AGGREGATE_MAX_BYTES,
  );
  if (tags.some((tag) => tag.trim().toLowerCase().replace(/\s+/g, "-") !== tag)) {
    throw new Error("Scrum card response.tags contains a noncanonical server tag.");
  }
  return tags;
}

function exactChecklist(value: unknown, label: string): Array<{ id: string; text: string; done: boolean }> {
  if (!Array.isArray(value) || value.length > CARD_ARRAY_MAX_ITEMS) throw new Error(`${label} exceeds its item bound.`);
  if (encoder.encode(JSON.stringify(value)).byteLength > CARD_PRESENTATION_AGGREGATE_MAX_BYTES) throw new Error(`${label} exceeds its aggregate bound.`);
  const items = value.map((item, index) => {
    const record = exactRecord(item, `${label}[${index}]`, ["id", "text", "done"]);
    if (typeof record.done !== "boolean") throw new Error(`${label}[${index}].done must be boolean.`);
    return {
      id: exactString(record.id, `${label}[${index}].id`, { maxBytes: CARD_ITEM_ID_MAX_BYTES, nonblank: true, canonical: true }),
      text: exactString(record.text, `${label}[${index}].text`, { maxBytes: CARD_ITEM_TEXT_MAX_BYTES, nonblank: true }),
      done: record.done,
    };
  });
  if (new Set(items.map((item) => item.id)).size !== items.length) throw new Error(`${label} contains duplicate item IDs.`);
  return items;
}

function validateFlowMetrics(value: unknown): Record<string, unknown> {
  const fields = [
    "assigned_returns", "review_bounces", "regression_count", "play_runs", "channel_messages",
    "conversation_chars", "incomplete_score", "completion_status", "signals", "last_play_outcome",
    "review_gate", "column", "updated_at",
  ] as const;
  const empty = exactRecord(value, "Scrum card response.flow_metrics", fields, []);
  if (Object.keys(empty).length === 0) return {};
  const required = fields.slice(0, 9);
  const record = exactRecord(value, "Scrum card response.flow_metrics", fields, required);
  for (const field of ["assigned_returns", "review_bounces", "regression_count", "play_runs", "incomplete_score"] as const) {
    exactInteger(record[field], `Scrum card response.flow_metrics.${field}`, FLOW_TRANSITION_COUNTER_MAX);
  }
  for (const field of ["channel_messages", "conversation_chars"] as const) {
    exactInteger(record[field], `Scrum card response.flow_metrics.${field}`, FLOW_WIDE_COUNTER_MAX);
  }
  if (!["likely_complete", "likely_incomplete", "uncertain"].includes(String(record.completion_status))) {
    throw new Error("Scrum card response.flow_metrics.completion_status is not registered.");
  }
  if (!Array.isArray(record.signals) || record.signals.length > FLOW_SIGNAL_MAX_ITEMS) {
    throw new Error("Scrum card response.flow_metrics.signals exceeds its item bound.");
  }
  const signals = record.signals.map((signal, index) => exactString(signal, `Scrum card response.flow_metrics.signals[${index}]`, {
    maxBytes: FLOW_SIGNAL_MAX_BYTES, nonblank: true, canonical: true,
  }));
  if (new Set(signals).size !== signals.length) throw new Error("Scrum card response.flow_metrics.signals contains duplicates.");
  if (encoder.encode(JSON.stringify(signals)).byteLength > FLOW_SIGNALS_AGGREGATE_MAX_BYTES) {
    throw new Error("Scrum card response.flow_metrics.signals exceeds its aggregate bound.");
  }
  if (encoder.encode(JSON.stringify(record)).byteLength > FLOW_SIGNALS_AGGREGATE_MAX_BYTES) {
    throw new Error("Scrum card response.flow_metrics exceeds its aggregate bound.");
  }
  if ("last_play_outcome" in record && !["success", "failed"].includes(String(record.last_play_outcome))) {
    throw new Error("Scrum card response.flow_metrics.last_play_outcome is not registered.");
  }
  if ("review_gate" in record && !["passed", "failed", "pending", "running"].includes(String(record.review_gate))) {
    throw new Error("Scrum card response.flow_metrics.review_gate is not registered.");
  }
  if ("column" in record && !SCRUM_COLUMNS.includes(record.column as (typeof SCRUM_COLUMNS)[number])) {
    throw new Error("Scrum card response.flow_metrics.column is not registered.");
  }
  if ("updated_at" in record) exactMicrosecondTimestamp(record.updated_at, "Scrum card response.flow_metrics.updated_at");
  return { ...record, signals };
}

export function validateScrumMessages(value: unknown, label: string, limit = CHANNEL_PAGE_MAX_MESSAGES): ScrumChatMessage[] {
  if (!Array.isArray(value) || value.length > limit) {
    throw new Error(`${label} must contain at most ${limit} messages.`);
  }
  let contentBytes = 0;
  const messages = value.map((item, index) => {
    const record = exactRecord(
      item,
      `${label}[${index}]`,
      ["id", "role", "content", "created_at", "status", "operation_id"],
      ["role", "content", "created_at"],
    );
    const role = exactString(record.role, `${label}[${index}].role`, { maxBytes: 32, nonblank: true, canonical: true });
    if (!["user", "assistant", "system", "error", "tool", "thinking", "status"].includes(role)) {
      throw new Error(`${label}[${index}].role is not registered.`);
    }
    const content = exactString(record.content, `${label}[${index}].content`, {
      maxBytes: CHANNEL_MESSAGE_MAX_BYTES, nonblank: true,
    });
    contentBytes += encoder.encode(content).byteLength;
    const result: ScrumChatMessage = {
      role,
      content,
      created_at: exactTimestamp(record.created_at, `${label}[${index}].created_at`),
    };
    if ("id" in record) {
      result.id = exactString(record.id, `${label}[${index}].id`, { maxBytes: 256, nonblank: true, canonical: true });
      if (!/^[A-Za-z0-9][A-Za-z0-9_.:-]*$/.test(result.id)) throw new Error(`${label}[${index}].id is not canonical.`);
    }
    if ("status" in record) {
      result.status = exactString(record.status, `${label}[${index}].status`, { maxBytes: 32, canonical: true });
      if (!["", "running", "completed", "failed"].includes(result.status)) {
        throw new Error(`${label}[${index}].status is not registered.`);
      }
    }
    if ("operation_id" in record) {
      result.operation_id = exactString(record.operation_id, `${label}[${index}].operation_id`, { maxBytes: 128, nonblank: true, canonical: true });
      if (!/^lifecycle_operation_[0-9a-f]{64}$/.test(result.operation_id)) {
        throw new Error(`${label}[${index}].operation_id is not canonical.`);
      }
    }
    return result;
  });
  if (contentBytes > CHANNEL_PAGE_MAX_BYTES) throw new Error(`${label} exceeds its byte bound.`);
  const ids = messages.flatMap((message) => message.id ? [message.id] : []);
  if (new Set(ids).size !== ids.length) throw new Error(`${label} contains duplicate message IDs.`);
  return messages;
}

export function validateScrumCardProjection(
  value: unknown,
  expectedCardID?: string,
  options: { summary?: "required" | "forbidden" } = { summary: "forbidden" },
): ScrumCard {
  const encoded = encoder.encode(JSON.stringify(value));
  if (encoded.byteLength > SCRUM_CHANNEL_RESPONSE_MAX_BYTES) throw new Error("Scrum card response exceeds its byte bound.");
  const record = exactRecord(value, "Scrum card response", CARD_FIELDS, REQUIRED_CARD_FIELDS);
  const id = exactString(record.id, "Scrum card response.id", { maxBytes: 256, nonblank: true, canonical: true });
  if (expectedCardID !== undefined && id !== expectedCardID) throw new Error("Scrum card response does not match the requested card.");
  const column = exactString(record.column, "Scrum card response.column", { maxBytes: 32, nonblank: true, canonical: true });
  if (!SCRUM_COLUMNS.includes(column as (typeof SCRUM_COLUMNS)[number])) throw new Error("Scrum card response.column is not registered.");
  if (options.summary === "required" && record.summary !== true) throw new Error("Scrum board response requires summary cards.");
  if (options.summary !== "required" && "summary" in record && record.summary !== false) throw new Error("Scrum action response must not contain a summary card.");
  const card: ScrumCard = {
    ...(record as ScrumCard),
    id,
    title: exactString(record.title, "Scrum card response.title", { maxBytes: CARD_TITLE_MAX_BYTES, nonblank: true }),
    description: exactString(record.description, "Scrum card response.description", { maxBytes: CARD_DESCRIPTION_MAX_BYTES }),
    column,
    checklist: exactChecklist(record.checklist, "Scrum card response.checklist"),
    ref_files: validateRefFiles(record.ref_files),
    chat: validateScrumMessages(record.chat, "Scrum card response.chat", CHANNEL_PAGE_MAX_MESSAGES),
    tags: validateTags(record.tags),
    test_criteria: exactChecklist(record.test_criteria, "Scrum card response.test_criteria"),
    created_at: exactTimestamp(record.created_at, "Scrum card response.created_at"),
    updated_at: exactTimestamp(record.updated_at, "Scrum card response.updated_at"),
  };
  if (options.summary === "required") {
    for (const field of [
      "checklist_done", "checklist_total", "ref_file_count", "chat_count",
      "test_criteria_done", "test_criteria_total", "board_order",
    ] as const) {
      if (!Object.prototype.hasOwnProperty.call(record, field)) {
        throw new Error(`Scrum board summary card is missing required field ${JSON.stringify(field)}.`);
      }
    }
    if (typeof record.has_card_ticket !== "boolean") {
      throw new Error("Scrum board summary card has_card_ticket must be boolean.");
    }
  }
  if (options.summary === "required" && (card.checklist.length || card.ref_files.length || card.chat.length || card.test_criteria.length ||
      "card_ticket" in record || "card_prompt" in record || record.channel_before_cursor !== "" || record.channel_has_more !== false)) {
    throw new Error("Scrum board summary card must not embed operational histories or content.");
  }
  for (const field of ["checklist_done", "checklist_total", "ref_file_count", "chat_count", "test_criteria_done", "test_criteria_total", "queue_order", "board_order"] as const) {
    if (field in record) (card as Record<string, unknown>)[field] = exactInteger(record[field], `Scrum card response.${field}`);
  }
  if (options.summary === "required" &&
      ((card.checklist_done as number) > (card.checklist_total as number) ||
       (card.test_criteria_done as number) > (card.test_criteria_total as number))) {
    throw new Error("Scrum board summary card completed counts exceed their totals.");
  }
  if (typeof record.channel_has_more !== "boolean") throw new Error("Scrum card response.channel_has_more must be boolean.");
  if ("has_card_ticket" in record && typeof record.has_card_ticket !== "boolean") throw new Error("Scrum card response.has_card_ticket must be boolean.");
  if ("job_id" in record) exactString(record.job_id, "Scrum card response.job_id", { maxBytes: 64, nonblank: true, canonical: true });
  if ("play_state" in record && !["queued", "running", "paused"].includes(String(record.play_state))) throw new Error("Scrum card response.play_state is not registered.");
  if ("card_ticket" in record) exactString(record.card_ticket, "Scrum card response.card_ticket", { maxBytes: CARD_TICKET_MAX_BYTES });
  if ("card_prompt" in record) exactString(record.card_prompt, "Scrum card response.card_prompt", { maxBytes: CARD_PROMPT_MAX_BYTES });
  const channelCursor = validateScrumChannelCursor(record.channel_before_cursor, "Scrum card response.channel_before_cursor", true);
  if (record.channel_has_more !== (channelCursor !== "")) {
    throw new Error("Scrum card response channel cursor authority is contradictory.");
  }
  if ("flow_metrics" in record) card.flow_metrics = validateFlowMetrics(record.flow_metrics);
  return card;
}
