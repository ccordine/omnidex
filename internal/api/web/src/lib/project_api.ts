import { readJSON } from "./api";
import type {
  BrowseResponse,
  ProjectMapSummary,
  ProjectRecord,
  RecipeCatalogItem,
  WorkspaceResponse,
} from "./project_types";
import type { ResolvedModelConfig } from "./model_config_types";

export async function fetchProjects(): Promise<{ projects: ProjectRecord[]; active_project_id: number }> {
  const response = await fetch("/v1/projects");
  return readJSON(response);
}

export async function fetchProject(id: number): Promise<{ project: ProjectRecord; modelConfig?: ResolvedModelConfig }> {
  const response = await fetch(`/v1/projects/${id}`);
  const payload = await readJSON<{ project: ProjectRecord; model_config?: ResolvedModelConfig }>(response);
  return { project: payload.project, modelConfig: payload.model_config };
}

export async function createProject(input: {
  name: string;
  location: string;
  description?: string;
  recipe_id?: string;
  activate?: boolean;
}): Promise<{ project: ProjectRecord; active_project_id: number }> {
  const response = await fetch("/v1/projects", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  return readJSON(response);
}

export async function updateProject(
  id: number,
  patch: Partial<{
    name: string;
    location: string;
    description: string;
    recipe_id: string;
    recipe: Record<string, unknown>;
    project_state: string;
    model_config: Record<string, string>;
    agent_config: Record<string, string>;
  }>,
): Promise<ProjectRecord> {
  const response = await fetch(`/v1/projects/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
  });
  const payload = await readJSON<{ project: ProjectRecord }>(response);
  return payload.project;
}

export async function deleteProject(id: number): Promise<void> {
  const response = await fetch(`/v1/projects/${id}`, { method: "DELETE" });
  await readJSON(response);
}

export async function activateProject(id: number): Promise<WorkspaceResponse> {
  const response = await fetch(`/v1/projects/${id}/activate`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
  return readJSON(response);
}

export async function surveyProject(id: number): Promise<ProjectRecord> {
  const response = await fetch(`/v1/projects/${id}/survey`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
  const payload = await readJSON<{ project: ProjectRecord }>(response);
  return payload.project;
}

export async function fetchProjectMap(id: number): Promise<ProjectMapSummary> {
  const response = await fetch(`/v1/projects/${id}/map`);
  return readJSON(response);
}

export async function scanProjectMap(id: number): Promise<ProjectMapSummary> {
  const response = await fetch(`/v1/projects/${id}/map/scan`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
  return readJSON(response);
}

export async function fetchWorkspace(): Promise<WorkspaceResponse> {
  const response = await fetch("/v1/workspace");
  return readJSON(response);
}

export async function fetchRecipes(): Promise<{ recipes: RecipeCatalogItem[]; root: string }> {
  const response = await fetch("/v1/recipes");
  return readJSON(response);
}

export async function browseDirectory(path = ""): Promise<BrowseResponse> {
  const query = path ? `?path=${encodeURIComponent(path)}` : "";
  const response = await fetch(`/v1/browse${query}`);
  return readJSON(response);
}

export async function pickHostDirectory(startPath = ""): Promise<{ path?: string; canceled?: boolean }> {
  const response = await fetch("/v1/host/pick-directory", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ start_path: startPath }),
  });
  return readJSON(response);
}

export async function fetchHostBridgeStatus(): Promise<Record<string, unknown>> {
  const response = await fetch("/v1/host/status");
  return readJSON(response);
}

export function projectQuery(projectID?: number | null): string {
  if (projectID && projectID > 0) return `?project_id=${projectID}`;
  return "";
}
