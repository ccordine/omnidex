import { readJSON } from "./api";
import type { ProjectRecord } from "./project_types";
import { exactInteger, exactRecord, exactString, exactTimestamp } from "./scrum_card_response";
import { validateCanonicalRevision } from "./revision_authority";
import {
  validateProjectAutoWorkResponse,
  type ProjectAutoWorkResponse,
} from "./project_auto_work_response";

export type { ProjectAutoWorkResponse } from "./project_auto_work_response";

export type ProjectMutationResponse = {
  commit_state: "committed" | "committed_degraded";
  project: ProjectRecord;
  operation_error?: string;
};

function validateProjectRecord(value: unknown, expectedID?: number): ProjectRecord {
  const record = exactRecord(value, "Project mutation project", [
    "id", "name", "location", "description", "project_state", "last_seen_at", "created_at", "updated_at",
    "job_count", "card_count",
  ], ["id", "name", "location", "description", "project_state", "last_seen_at", "created_at", "updated_at"]);
  const id = exactInteger(record.id, "Project mutation project.id");
  if (id <= 0 || (expectedID !== undefined && id !== expectedID)) {
    throw new Error("Project mutation response does not match the requested project.");
  }
  const project: ProjectRecord = {
    id,
    name: exactString(record.name, "Project mutation project.name", { maxBytes: 256, nonblank: true, canonical: true }),
    location: exactString(record.location, "Project mutation project.location", { maxBytes: 4096, nonblank: true, canonical: true }),
    description: exactString(record.description, "Project mutation project.description", { maxBytes: 16 * 1024 }),
    project_state: exactString(record.project_state, "Project mutation project.project_state", { maxBytes: 64 * 1024 }),
    last_seen_at: exactTimestamp(record.last_seen_at, "Project mutation project.last_seen_at"),
    created_at: exactTimestamp(record.created_at, "Project mutation project.created_at"),
    updated_at: validateProjectRevision(record.updated_at, "Project mutation project.updated_at"),
  };
  for (const field of ["job_count", "card_count"] as const) {
    if (field in record) project[field] = exactInteger(record[field], `Project mutation project.${field}`);
  }
  return project;
}

export function validateProjectRevision(value: unknown, label: string): string {
  return validateCanonicalRevision(value, label);
}

function validateProjectMutationResponse(
  payload: unknown,
  expectedID: number | undefined,
  expectedUpdatedAt: string | undefined,
  status: number,
  committedStatus: number,
): ProjectMutationResponse {
  const record = exactRecord(payload, "Project mutation response", ["commit_state", "project", "operation_error"], ["commit_state", "project"]);
  if (record.commit_state !== "committed" && record.commit_state !== "committed_degraded") {
    throw new Error("Project mutation response lacks one exact registered commit state.");
  }
  if ((record.commit_state === "committed" && status !== committedStatus) ||
      (record.commit_state === "committed_degraded" && status !== 207)) {
    throw new Error("Project mutation response commit state contradicts its HTTP status.");
  }
  const project = validateProjectRecord(record.project, expectedID);
  if (expectedUpdatedAt !== undefined && project.updated_at === expectedUpdatedAt) {
    throw new Error("Project mutation response did not provide the new authoritative revision.");
  }
  if (record.commit_state === "committed_degraded") {
    return {
      commit_state: "committed_degraded",
      project,
      operation_error: exactString(record.operation_error, "Project mutation response.operation_error", { maxBytes: 16 * 1024, nonblank: true }),
    };
  }
  if (Object.prototype.hasOwnProperty.call(record, "operation_error")) {
    throw new Error("Complete project mutation response must not contain a post-commit failure.");
  }
  return { commit_state: "committed", project };
}

export async function createProject(input: {
  name: string;
  location: string;
  description?: string;
}): Promise<ProjectMutationResponse> {
  const response = await fetch("/v1/projects", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  return validateProjectMutationResponse(await readJSON<unknown>(response, 128 * 1024), undefined, undefined, response.status, 201);
}

export async function updateProject(
  id: number,
  expectedUpdatedAt: string,
  patch: Partial<{
    name: string;
    location: string;
    description: string;
    model_config: Record<string, string>;
  }>,
): Promise<ProjectMutationResponse> {
  const revision = validateProjectRevision(expectedUpdatedAt, "Project patch expected_updated_at");
  projectQuery(id);
  const response = await fetch(`/v1/projects/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ expected_updated_at: revision, ...patch }),
  });
  return validateProjectMutationResponse(await readJSON<unknown>(response, 128 * 1024), id, revision, response.status, 200);
}

export async function deleteProject(id: number, expectedUpdatedAt: string): Promise<void> {
  projectQuery(id);
  const revision = validateProjectRevision(expectedUpdatedAt, "Project delete expected_updated_at");
  const response = await fetch(`/v1/projects/${id}`, {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ expected_updated_at: revision }),
  });
  const record = exactRecord(await readJSON<unknown>(response, 16 * 1024), "Project deletion response", [
    "commit_state", "project_id", "expected_updated_at", "deleted",
  ]);
  if (record.commit_state !== "committed" || exactInteger(record.project_id, "Project deletion response.project_id") !== id ||
      validateProjectRevision(record.expected_updated_at, "Project deletion response.expected_updated_at") !== revision || record.deleted !== true) {
    throw new Error("Project deletion response does not attest the requested revision-bound deletion.");
  }
}

export async function startProjectAutoWork(id: number): Promise<ProjectAutoWorkResponse> {
  projectQuery(id);
  const response = await fetch(`/v1/projects/${id}/play`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
  return validateProjectAutoWorkResponse(await readJSON<unknown>(response), id, response.status);
}

export async function pauseProjectAutoWork(id: number): Promise<ProjectAutoWorkResponse> {
  projectQuery(id);
  const response = await fetch(`/v1/projects/${id}/pause`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
  return validateProjectAutoWorkResponse(await readJSON<unknown>(response), id, response.status);
}

export function projectAutoWorkFailure(payload: ProjectAutoWorkResponse): string {
  return payload.commit_state === "committed_degraded"
    ? payload.operation_error
    : "";
}

export async function surveyProject(id: number): Promise<ProjectMutationResponse> {
  projectQuery(id);
  const response = await fetch(`/v1/projects/${id}/survey`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
  return validateProjectMutationResponse(await readJSON<unknown>(response, 128 * 1024), id, undefined, response.status, 200);
}

export function projectMutationFailure(payload: ProjectMutationResponse): string {
  return payload.commit_state === "committed_degraded"
    ? payload.operation_error as string
    : "";
}

export async function scanProjectMap(id: number): Promise<void> {
  projectQuery(id);
  const response = await fetch(`/v1/projects/${id}/map/scan`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
  const record = exactRecord(await readJSON<unknown>(response, 16 * 1024), "Project map scan response", [
    "project_id", "generated_at", "source", "file_count", "module_count", "scan_truncated",
  ]);
  if (exactInteger(record.project_id, "Project map scan response.project_id") !== id) {
    throw new Error("Project map scan response does not match the requested project.");
  }
  exactTimestamp(record.generated_at, "Project map scan response.generated_at");
  if (record.source !== "host-bridge" && record.source !== "core-local") {
    throw new Error("Project map scan response.source is not registered.");
  }
  exactInteger(record.file_count, "Project map scan response.file_count");
  exactInteger(record.module_count, "Project map scan response.module_count");
  if (typeof record.scan_truncated !== "boolean") throw new Error("Project map scan response.scan_truncated must be boolean.");
}

export async function createBrowseDirectory(parent: string, name: string): Promise<{ path: string; source: "host-bridge" | "core-local" }> {
  const response = await fetch("/v1/browse/mkdir", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ parent, name }),
  });
  const record = exactRecord(await readJSON<unknown>(response, 16 * 1024), "Directory creation response", ["path", "source"]);
  const path = exactString(record.path, "Directory creation response.path", { maxBytes: 4096, nonblank: true });
  if (record.source !== "host-bridge" && record.source !== "core-local") {
    throw new Error("Directory creation response.source is not registered.");
  }
  return { path, source: record.source };
}

export function projectQuery(projectID: number): string {
  if (!Number.isSafeInteger(projectID) || projectID <= 0) {
    throw new Error("Scrum transport requires one positive safe-integer project ID.");
  }
  return `?project_id=${projectID}`;
}
