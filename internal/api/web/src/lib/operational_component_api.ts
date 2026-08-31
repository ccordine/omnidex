import { fetchServerComponent, type ServerComponent } from "./server_component_api";
import { exactInteger, exactRecord, exactString } from "./scrum_card_response";

export type SelectionComponent = ServerComponent & {
  selected_source_id?: string;
  selected_channel_id?: string;
  offset?: number;
  has_more?: boolean;
  source_offset?: number;
  source_has_more?: boolean;
  channel_offset?: number;
  channel_has_more?: boolean;
};

export type AdminComponentQuery = {
  modelOffset?: number;
  catalogQuery?: string;
  catalogPage?: number;
  downloadOffset?: number;
};

export function fetchAdminComponent(tab: string, options: AdminComponentQuery = {}): Promise<ServerComponent> {
  const query = new URLSearchParams({ tab });
  if (tab === "ai") {
    query.set("model_offset", String(options.modelOffset ?? 0));
    query.set("catalog_query", options.catalogQuery ?? "");
    query.set("catalog_page", String(options.catalogPage ?? 1));
    query.set("download_offset", String(options.downloadOffset ?? 0));
  }
  return fetchServerComponent(`/v1/ui/admin?${query}`);
}

export function fetchAdminDataSourcesComponent(editingID = "", selectedID = "", offset = 0): Promise<SelectionComponent> {
  const query = new URLSearchParams({ editing_id: editingID, selected_id: selectedID, offset: String(offset) });
  return fetchServerComponent(`/v1/ui/admin/data-sources?${query}`);
}

export function fetchDataComponent(sourceID = "", channelID = "", sourceOffset = 0, channelOffset = 0): Promise<SelectionComponent> {
  const query = new URLSearchParams({
    source_id: sourceID, channel_id: channelID,
    source_offset: String(sourceOffset), channel_offset: String(channelOffset),
  });
  return fetchServerComponent(`/v1/ui/data?${query}`);
}

export async function fetchProjectsComponent(offset = 0): Promise<ServerComponent & { count: number }> {
  if (!Number.isSafeInteger(offset) || offset < 0) throw new Error("Project component offset must be a non-negative safe integer.");
  const payload = await fetchServerComponent<ServerComponent & { count: number }>(`/v1/ui/projects?offset=${offset}`);
  const record = exactRecord(payload, "Projects component response", ["html", "count"]);
  return { html: record.html as ServerComponent["html"], count: exactInteger(record.count, "Projects component response.count", 20) };
}

export type ProjectDetailComponent = ServerComponent & {
  project_id: number;
  project_name: string;
  project_location: string;
  tab: string;
};

export async function fetchProjectDetailComponent(projectID: number, tab: string, signal?: AbortSignal): Promise<ProjectDetailComponent> {
  if (!Number.isSafeInteger(projectID) || projectID <= 0) throw new Error("Project detail component requires one positive safe integer project id.");
  if (!["scrum", "terminal", "screen", "settings", "map", "git"].includes(tab)) throw new Error("Project detail component tab is not registered.");
  const query = new URLSearchParams({ tab });
  const payload = await fetchServerComponent<ProjectDetailComponent>(`/v1/ui/projects/${projectID}?${query}`, { signal });
  const record = exactRecord(payload, "Project detail component response", [
    "html", "project_id", "project_name", "project_location", "tab",
  ]);
  if (exactInteger(record.project_id, "Project detail component response.project_id") !== projectID || record.tab !== tab) {
    throw new Error("Project detail component response does not match its requested route.");
  }
  return {
    html: record.html as ServerComponent["html"], project_id: projectID,
    project_name: exactString(record.project_name, "Project detail component response.project_name", { maxBytes: 256, nonblank: true, canonical: true }),
    project_location: exactString(record.project_location, "Project detail component response.project_location", { maxBytes: 4096, nonblank: true, canonical: true }),
    tab,
  };
}

export function fetchProjectModalComponent(kind: "create" | "browse", query = new URLSearchParams()): Promise<ServerComponent> {
  query.set("kind", kind);
  return fetchServerComponent(`/v1/ui/projects/modal?${query}`);
}

export function fetchScreenMonitorsComponent(projectID: number, offset = 0): Promise<ServerComponent & { monitor_id?: string; offset: number; has_previous: boolean; has_more: boolean }> {
  const query = new URLSearchParams({ project_id: String(projectID), offset: String(offset) });
  return fetchServerComponent(`/v1/ui/screen/monitors?${query}`);
}

export function fetchScrumCreateCardComponent(projectID: number, column: string): Promise<ServerComponent> {
  const query = new URLSearchParams({ project_id: String(projectID), column });
  return fetchServerComponent(`/v1/ui/scrum/create-card?${query}`);
}
