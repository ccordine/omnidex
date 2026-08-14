import { readJSON } from "./api";
import { projectQuery } from "./project_api";
import type { ScrumAutoWorkConfig } from "./scrum_types";

export type ScrumAutoWorkMutationResult = {
  commit_state: "committed" | "committed_degraded";
  auto_work: { enabled: boolean; source_columns: string[] };
  operation_error?: string;
};

const SCRUM_AUTO_WORK_COLUMNS = ["backlog", "ready", "assigned", "in_progress", "blocked"] as const;

function validateScrumAutoWorkMutationResult(payload: unknown): ScrumAutoWorkMutationResult {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new Error("Scrum auto-work mutation response must be one typed object.");
  }
  const record = payload as Record<string, unknown>;
  if (Object.keys(record).some((field) => !["commit_state", "auto_work", "operation_error"].includes(field))) {
    throw new Error("Scrum auto-work mutation response contains an unknown field.");
  }
  if (record.commit_state !== "committed" && record.commit_state !== "committed_degraded") {
    throw new Error("Scrum auto-work mutation response lacks one exact registered commit state.");
  }
  if (!record.auto_work || typeof record.auto_work !== "object" || Array.isArray(record.auto_work)) {
    throw new Error("Scrum auto-work mutation response lacks authoritative settings.");
  }
  const config = record.auto_work as Record<string, unknown>;
  if (Object.keys(config).length !== 2 || !("enabled" in config) || !("source_columns" in config) ||
      typeof config.enabled !== "boolean" || !Array.isArray(config.source_columns) || config.source_columns.length < 1 ||
      config.source_columns.some((column) => typeof column !== "string" ||
        !SCRUM_AUTO_WORK_COLUMNS.includes(column as (typeof SCRUM_AUTO_WORK_COLUMNS)[number])) ||
      new Set(config.source_columns).size !== config.source_columns.length) {
    throw new Error("Scrum auto-work mutation response contains invalid authoritative settings.");
  }
  if (record.commit_state === "committed_degraded") {
    if (typeof record.operation_error !== "string" || !record.operation_error.trim()) {
      throw new Error("Degraded Scrum auto-work mutation response lacks its post-commit failure.");
    }
  } else if (Object.prototype.hasOwnProperty.call(record, "operation_error")) {
    throw new Error("Complete Scrum auto-work mutation response must not contain a post-commit failure.");
  }
  return record as ScrumAutoWorkMutationResult;
}

export async function patchScrumAutoWork(
  config: ScrumAutoWorkConfig,
  projectID: number,
): Promise<ScrumAutoWorkMutationResult> {
  const response = await fetch(`/v1/scrum${projectQuery(projectID)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ auto_work: config }),
  });
  return validateScrumAutoWorkMutationResult(await readJSON<unknown>(response));
}
