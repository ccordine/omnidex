import { fetchServerComponent, type ServerComponent } from "./server_component_api";

export type SelectionComponent = ServerComponent & {
  selected_source_id?: string;
  selected_channel_id?: string;
  offset?: number;
  has_more?: boolean;
  source_offset?: number;
  source_has_more?: boolean;
  channel_offset?: number;
  channel_has_more?: boolean;
	message_offset?: number;
	message_has_more?: boolean;
};

export function fetchAdminComponent(tab: string, modelOffset = 0): Promise<ServerComponent> {
	const query = new URLSearchParams({ tab, model_offset: String(modelOffset) });
	return fetchServerComponent(`/v1/ui/admin?${query}`);
}

export function fetchAdminDataSourcesComponent(editingID = "", selectedID = "", offset = 0): Promise<SelectionComponent> {
  const query = new URLSearchParams({ editing_id: editingID, selected_id: selectedID, offset: String(offset) });
  return fetchServerComponent(`/v1/ui/admin/data-sources?${query}`);
}

export function fetchDataComponent(sourceID = "", channelID = "", sourceOffset = 0, channelOffset = 0, messageOffset = 0): Promise<SelectionComponent> {
  const query = new URLSearchParams({
    source_id: sourceID, channel_id: channelID,
    source_offset: String(sourceOffset), channel_offset: String(channelOffset), message_offset: String(messageOffset),
  });
  return fetchServerComponent(`/v1/ui/data?${query}`);
}

export function fetchProjectsComponent(offset = 0): Promise<ServerComponent & { count: number }> {
  return fetchServerComponent(`/v1/ui/projects?offset=${offset}`);
}

export type ProjectDetailComponent = ServerComponent & {
  project_id: number;
  project_name: string;
  project_location: string;
  tab: string;
};

export function fetchProjectDetailComponent(projectID: number, tab: string, signal?: AbortSignal, recipeOffset = 0): Promise<ProjectDetailComponent> {
	const query = new URLSearchParams({ tab, recipe_offset: String(recipeOffset) });
	return fetchServerComponent(`/v1/ui/projects/${projectID}?${query}`, { signal });
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
