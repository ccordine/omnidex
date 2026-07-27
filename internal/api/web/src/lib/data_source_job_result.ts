import type { DataSourceQueryResult } from "./admin_api";

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function assertOptionalString(value: unknown, field: string): void {
  if (value !== undefined && typeof value !== "string") {
    throw new Error(`Data source job query field ${JSON.stringify(field)} must be a string.`);
  }
}

export function parseDataSourceJobResult(raw: string): DataSourceQueryResult {
  const text = raw.trim();
  if (!text) throw new Error("Data source job completed without a result payload.");

  let payload: unknown;
  try {
    payload = JSON.parse(text);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(`Data source job result is not valid JSON: ${message}`);
  }
  if (!isRecord(payload) || !isRecord(payload.query)) {
    throw new Error("Data source job result is missing its required query object.");
  }

  const query = payload.query;
  if (!Array.isArray(query.columns) || !query.columns.every((column) => typeof column === "string")) {
    throw new Error("Data source job query requires a string columns array.");
  }
  if (!Array.isArray(query.rows) || !query.rows.every(isRecord)) {
    throw new Error("Data source job query requires an object rows array.");
  }
  if (!Number.isSafeInteger(query.count) || Number(query.count) < 0) {
    throw new Error("Data source job query requires a non-negative integer count.");
  }
  for (const field of ["question", "sql", "answer"] as const) assertOptionalString(query[field], field);
  if (query.hard_facts !== undefined && (!Array.isArray(query.hard_facts) || !query.hard_facts.every((fact) => typeof fact === "string"))) {
    throw new Error("Data source job query hard_facts must be a string array.");
  }
  if (query.query_steps !== undefined && (!Array.isArray(query.query_steps) || !query.query_steps.every(isRecord))) {
    throw new Error("Data source job query query_steps must be an object array.");
  }
  if (query.text_insights !== undefined && (!Array.isArray(query.text_insights) || !query.text_insights.every(isRecord))) {
    throw new Error("Data source job query text_insights must be an object array.");
  }

  return query as unknown as DataSourceQueryResult;
}
