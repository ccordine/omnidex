export type AIControlCommitState = "committed" | "committed_degraded";

export type AIControlStatePayload = {
  paused: boolean;
  canceled_jobs: number;
  resumed: boolean;
  counts: { pending: number; running: number; waiting_input: number };
  updated_at: string;
};

export type AIControlMutationPayload = Omit<AIControlStatePayload, "counts"> & {
  commit_state: AIControlCommitState;
  counts?: AIControlStatePayload["counts"];
  realtime_published: boolean;
  operation_error?: string;
  realtime_error?: string;
};

function exactAIControlRecord(value: unknown, allowed: readonly string[], required: readonly string[]): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("AI control response must be one typed object.");
  const record = value as Record<string, unknown>;
  const unknown = Object.keys(record).find((field) => !allowed.includes(field));
  if (unknown) throw new Error(`AI control response contains unknown field ${JSON.stringify(unknown)}.`);
  const missing = required.find((field) => !Object.prototype.hasOwnProperty.call(record, field));
  if (missing) throw new Error(`AI control response is missing ${JSON.stringify(missing)}.`);
  return record;
}

function validateCounts(value: unknown): AIControlStatePayload["counts"] {
  const record = exactAIControlRecord(value, ["pending", "running", "waiting_input"], ["pending", "running", "waiting_input"]);
  for (const field of ["pending", "running", "waiting_input"] as const) {
    if (!Number.isSafeInteger(record[field]) || (record[field] as number) < 0) throw new Error(`AI control count ${field} must be a non-negative integer.`);
  }
  return record as AIControlStatePayload["counts"];
}

function validateStateFields(record: Record<string, unknown>): Omit<AIControlStatePayload, "counts"> {
  if (typeof record.paused !== "boolean" || typeof record.resumed !== "boolean") throw new Error("AI control paused/resumed authority must be boolean.");
  if (!Number.isSafeInteger(record.canceled_jobs) || (record.canceled_jobs as number) < 0) throw new Error("AI control canceled_jobs must be a non-negative integer.");
  if (typeof record.updated_at !== "string" ||
      !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.[0-9]{1,9})?Z$/.test(record.updated_at) ||
      /\.[0-9]*0Z$/.test(record.updated_at) || !Number.isFinite(Date.parse(record.updated_at)) ||
      new Date(record.updated_at).toISOString().slice(0, 19) !== record.updated_at.slice(0, 19)) {
    throw new Error("AI control updated_at must be a canonical timestamp.");
  }
  return {
    paused: record.paused,
    canceled_jobs: record.canceled_jobs as number,
    resumed: record.resumed,
    updated_at: record.updated_at,
  };
}

export function validateAIControlStatePayload(value: unknown): AIControlStatePayload {
  const fields = ["paused", "canceled_jobs", "resumed", "counts", "updated_at"];
  const record = exactAIControlRecord(value, fields, fields);
  return { ...validateStateFields(record), counts: validateCounts(record.counts) };
}

export function validateAIControlMutationPayload(value: unknown): AIControlMutationPayload {
  const allowed = ["commit_state", "paused", "canceled_jobs", "resumed", "counts", "updated_at", "realtime_published", "operation_error", "realtime_error"];
  const required = ["commit_state", "paused", "canceled_jobs", "resumed", "updated_at", "realtime_published"];
  const record = exactAIControlRecord(value, allowed, required);
  if (typeof record.realtime_published !== "boolean") throw new Error("AI control realtime_published must be boolean.");
  const result: AIControlMutationPayload = {
    ...validateStateFields(record),
    commit_state: record.commit_state as AIControlCommitState,
    realtime_published: record.realtime_published,
  };
  if ("counts" in record) result.counts = validateCounts(record.counts);
  if ("operation_error" in record) result.operation_error = validateFailureText(record.operation_error, "operation_error");
  if ("realtime_error" in record) result.realtime_error = validateFailureText(record.realtime_error, "realtime_error");
  aiControlDegradedMessage(result);
  if (result.commit_state === "committed" && (!result.counts || !result.realtime_published)) {
    throw new Error("AI control committed result requires counts and successful realtime publication.");
  }
  if (result.realtime_error !== undefined && result.realtime_published) {
    throw new Error("AI control realtime failure contradicts successful publication authority.");
  }
  return result;
}

function validateFailureText(value: unknown, label: string): string {
  if (typeof value !== "string") throw new Error(`AI control ${label} must be one bounded nonblank string.`);
  const bytes = new TextEncoder().encode(value);
  if (!value.trim() || value.includes("\0") || new TextDecoder().decode(bytes) !== value || bytes.byteLength > 16 * 1024) {
    throw new Error(`AI control ${label} must be one bounded nonblank string.`);
  }
  return value;
}

export function aiControlDegradedMessage(payload: {
  commit_state: AIControlCommitState;
  operation_error?: string;
  realtime_error?: string;
}): string {
  if (payload.commit_state === undefined) {
    throw new Error("AI control payload.commit_state is required.");
  }
  if (payload.commit_state !== "committed" && payload.commit_state !== "committed_degraded") {
    throw new Error("AI control payload.commit_state must be an exact registered state.");
  }
  const details = [payload.operation_error, payload.realtime_error]
    .filter((value): value is string => value !== undefined)
    .map((value, index) => validateFailureText(value, index === 0 ? "operation_error" : "realtime_error"));
  if (payload.commit_state === "committed_degraded") {
    if (details.length === 0) throw new Error("AI control committed_degraded result requires one explicit post-commit failure.");
    return details.join(" ");
  }
  if (details.length > 0) throw new Error("AI control committed result must not contain post-commit failure feedback.");
  return "";
}
