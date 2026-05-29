import { readJSON, jsonRequest } from "./api";

export type NetworkSettings = {
  core_url: string;
  source: "database" | "environment" | "default" | string;
  host: string;
  port: number;
  listen_addr: string;
  request_url?: string;
  default_url: string;
};

export async function fetchNetworkSettings(): Promise<NetworkSettings> {
  const response = await fetch("/v1/settings/network");
  return readJSON(response);
}

export async function saveNetworkSettings(values: { host: string; port: number; url?: string }): Promise<NetworkSettings> {
  const response = await fetch("/v1/settings/network", jsonRequest(values));
  return readJSON(response);
}

export type MindStats = {
  memory_chunks: number;
  memory_candidates: number;
  candidate_pending: number;
  jobs: number;
  telemetry_events: number;
};

export type OllamaModelInfo = {
  name: string;
  size: number;
  modified_at: string;
  configured: boolean;
};

export async function fetchMindStats(): Promise<MindStats> {
  const response = await fetch("/v1/admin/mind/stats");
  const payload = await readJSON<{ stats: MindStats }>(response);
  return payload.stats;
}

export async function fetchOllamaModels(): Promise<{
  endpoint: string;
  models: OllamaModelInfo[];
  configured_models: string[];
}> {
  const response = await fetch("/v1/ollama/models");
  return readJSON(response);
}

export async function pullOllamaModel(model: string): Promise<void> {
  const response = await fetch("/v1/ollama/models", jsonRequest({ model }));
  await readJSON(response);
}

export async function deleteOllamaModel(name: string): Promise<void> {
  const response = await fetch(`/v1/ollama/models/${encodeURIComponent(name)}`, { method: "DELETE" });
  await readJSON(response);
}

export async function fetchModelSettings(): Promise<{
  env_file: string;
  fields: Array<{ key: string; label: string; description: string; env_keys: string[]; value: string }>;
}> {
  const response = await fetch("/v1/settings/models");
  return readJSON(response);
}

export async function saveModelSettings(values: Record<string, string>): Promise<void> {
  const response = await fetch("/v1/settings/models", jsonRequest({ values }));
  await readJSON(response);
}

export type APISecretField = {
  key: string;
  label: string;
  description: string;
  env_keys: string[];
  configured: boolean;
  source: "database" | "environment" | "none";
  hint: string;
};

export async function fetchAPISecrets(): Promise<{ storage: string; fields: APISecretField[] }> {
  const response = await fetch("/v1/settings/secrets");
  return readJSON(response);
}

export async function saveAPISecrets(values: Record<string, string>, clearKeys: string[] = []): Promise<void> {
  const response = await fetch("/v1/settings/secrets", jsonRequest({ values, clear_keys: clearKeys }));
  await readJSON(response);
}

export async function ingestDocuments(
  files: FileList | File[],
  options: { stage?: string; kind?: string; tags?: string },
): Promise<{ stage: string; results: Array<Record<string, unknown>>; message: string }> {
  const form = new FormData();
  for (const file of Array.from(files)) {
    form.append("files", file, file.name);
  }
  if (options.stage) form.append("stage", options.stage);
  if (options.kind) form.append("kind", options.kind);
  if (options.tags) form.append("tags", options.tags);
  const response = await fetch("/v1/ingest/documents", { method: "POST", body: form });
  return readJSON(response);
}

export async function deleteMemory(id: number): Promise<void> {
  const response = await fetch(`/v1/memory/${id}`, { method: "DELETE" });
  await readJSON(response);
}

export async function deleteMemoryCandidate(id: number): Promise<void> {
  const response = await fetch(`/v1/memory-candidates/${id}`, { method: "DELETE" });
  await readJSON(response);
}

export async function fetchMemories(params: { limit?: number; kind?: string; tags?: string } = {}): Promise<unknown[]> {
  const query = new URLSearchParams();
  if (params.limit) query.set("limit", String(params.limit));
  if (params.kind) query.set("kind", params.kind);
  if (params.tags) query.set("tags", params.tags);
  const suffix = query.toString() ? `?${query}` : "";
  const response = await fetch(`/v1/memory${suffix}`);
  const payload = await readJSON<{ memories: unknown[] }>(response);
  return payload.memories ?? [];
}

export async function fetchMemoryCandidates(params: { limit?: number; status?: string } = {}): Promise<unknown[]> {
  const query = new URLSearchParams();
  if (params.limit) query.set("limit", String(params.limit));
  if (params.status) query.set("status", params.status);
  const suffix = query.toString() ? `?${query}` : "";
  const response = await fetch(`/v1/memory-candidates${suffix}`);
  const payload = await readJSON<{ memory_candidates: unknown[] }>(response);
  return payload.memory_candidates ?? [];
}
