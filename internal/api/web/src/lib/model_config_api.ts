import { readJSON } from "./api";
import type { ModelDefaultsResponse } from "./model_config_types";

export async function fetchModelDefaults(
  projectID?: number | null,
  cardID?: string | null,
): Promise<ModelDefaultsResponse> {
  const params = new URLSearchParams();
  if (projectID && projectID > 0) params.set("project_id", String(projectID));
  if (cardID?.trim()) params.set("card_id", cardID.trim());
  const query = params.toString();
  const response = await fetch(`/v1/models/resolved${query ? `?${query}` : ""}`);
  return readJSON(response);
}
