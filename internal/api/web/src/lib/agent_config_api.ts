import { readJSON, jsonRequest } from "./api";
import type { ResolvedAgentConfig } from "./agent_config_types";

export async function fetchAgentDefaults(
  projectID?: number | null,
  cardID?: string | null,
): Promise<{ env_defaults: Record<string, string>; fields: ResolvedAgentConfig["fields"]; resolved?: ResolvedAgentConfig }> {
  const params = new URLSearchParams();
  if (projectID && projectID > 0) params.set("project_id", String(projectID));
  if (cardID?.trim()) params.set("card_id", cardID.trim());
  const query = params.toString();
  const response = await fetch(`/v1/agents/resolved${query ? `?${query}` : ""}`);
  return readJSON(response);
}

export async function fetchGlobalAgentSettings(): Promise<{
  env_file: string;
  fields: ResolvedAgentConfig["fields"];
  resolved: Record<string, string>;
}> {
  const response = await fetch("/v1/settings/agents");
  return readJSON(response);
}

export async function saveGlobalAgentSettings(values: Record<string, string>): Promise<void> {
  const response = await fetch("/v1/settings/agents", jsonRequest({ values }));
  await readJSON(response);
}
