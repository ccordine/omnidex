import { readJSON, jsonPut, jsonRequest } from "./api";

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
  const response = await fetch("/v1/settings/network", jsonPut(values));
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
  const response = await fetch("/v1/settings/secrets", jsonPut({ values, clear_keys: clearKeys }));
  await readJSON(response);
}

export type DataSource = {
  id: string;
  name: string;
  driver: string;
  domain: string;
  context_prompt: string;
  privacy_mode: string;
  host: string;
  port: number;
  database_name: string;
  username: string;
  ssl_mode: string;
  use_dsn: boolean;
  read_only: boolean;
  password_set: boolean;
  password_hint: string;
  last_test_status?: string;
  last_test_message?: string;
  last_test_at?: string;
  catalog_updated_at?: string;
  created_at: string;
  updated_at: string;
};

export type DataSourceSchemaTable = {
  schema: string;
  name: string;
  columns: Array<{ name: string; data_type: string; nullable: boolean }>;
};

export type DataSourceQueryResult = {
  sql?: string;
  answer?: string;
  question?: string;
  columns: string[];
  rows: Array<Record<string, unknown>>;
  count: number;
  hard_facts?: string[];
  query_steps?: Array<{ step: number; purpose?: string; sql: string; row_count: number; hard_facts?: string[] }>;
  text_insights?: Array<{ field: string; table?: string; samples: number; summary: string; themes?: string[]; sentiment?: Record<string, number> }>;
};

export type DataSourceUpsertPayload = {
  name: string;
  driver?: string;
  domain?: string;
  context_prompt?: string;
  privacy_mode?: string;
  host?: string;
  port?: number;
  database_name?: string;
  username?: string;
  password?: string;
  ssl_mode?: string;
  use_dsn?: boolean;
  dsn?: string;
  read_only?: boolean;
};

export type DataSourceCatalogTable = {
  schema: string;
  name: string;
  full_name: string;
  purpose?: string;
  row_estimate?: number;
  columns: Array<{ name: string; data_type: string; nullable: boolean; sensitive?: boolean; hint?: string }>;
};

export type DataSourceCatalog = {
  source_id: string;
  source_name: string;
  driver: string;
  domain: string;
  fingerprint: string;
  summary?: string;
  updated_at?: string;
  tables: DataSourceCatalogTable[];
};

export async function fetchDataSources(): Promise<DataSource[]> {
  const response = await fetch("/v1/admin/data-sources");
  const payload = await readJSON<{ sources: DataSource[] }>(response);
  return payload.sources ?? [];
}

export async function createDataSource(input: DataSourceUpsertPayload): Promise<DataSource> {
  const response = await fetch("/v1/admin/data-sources", jsonRequest({ ...input, driver: input.driver || "postgres", read_only: true }));
  const payload = await readJSON<{ source: DataSource }>(response);
  return payload.source;
}

export async function updateDataSource(id: string, input: DataSourceUpsertPayload): Promise<DataSource> {
  const response = await fetch(`/v1/admin/data-sources/${encodeURIComponent(id)}`, jsonPut({ ...input, driver: input.driver || "postgres", read_only: true }));
  const payload = await readJSON<{ source: DataSource }>(response);
  return payload.source;
}

export async function deleteDataSource(id: string): Promise<void> {
  const response = await fetch(`/v1/admin/data-sources/${encodeURIComponent(id)}`, { method: "DELETE" });
  await readJSON(response);
}

export async function testDataSource(id: string): Promise<{ source: DataSource; status: string; message: string }> {
  const response = await fetch(`/v1/admin/data-sources/${encodeURIComponent(id)}/test`, jsonRequest({}));
  return readJSON(response);
}

export async function fetchDataSourceSchema(id: string): Promise<DataSourceSchemaTable[]> {
  const response = await fetch(`/v1/admin/data-sources/${encodeURIComponent(id)}/schema`);
  const payload = await readJSON<{ schema: DataSourceSchemaTable[] }>(response);
  return payload.schema ?? [];
}

export async function runDataSourceQuery(id: string, sql: string): Promise<DataSourceQueryResult> {
  const response = await fetch(`/v1/admin/data-sources/${encodeURIComponent(id)}/query`, jsonRequest({ sql }));
  return readJSON(response);
}

export async function askDataSource(id: string, question: string): Promise<{ job: JobRecord; question: string; message: string }> {
  const response = await fetch(`/v1/admin/data-sources/${encodeURIComponent(id)}/ask`, jsonRequest({ question }));
  return readJSON(response);
}

export async function fetchDataSourceCatalog(id: string): Promise<{ catalog: DataSourceCatalog; ready: boolean }> {
  const response = await fetch(`/v1/admin/data-sources/${encodeURIComponent(id)}/catalog`);
  const payload = await readJSON<{ catalog: DataSourceCatalog; ready: boolean }>(response);
  return { catalog: payload.catalog ?? { source_id: id, source_name: "", driver: "postgres", domain: "generic", fingerprint: "", tables: [] }, ready: Boolean(payload.ready) };
}

export async function exploreDataSource(id: string): Promise<{ job: JobRecord; message: string }> {
  const response = await fetch(`/v1/admin/data-sources/${encodeURIComponent(id)}/explore`, jsonRequest({}));
  return readJSON(response);
}

export type JobRecord = {
  id: number;
  instruction: string;
  pipeline: string;
  status: string;
  result?: string;
  error?: string;
  created_at?: string;
  updated_at?: string;
};

export async function fetchJobDetails(id: number): Promise<{ job: JobRecord; steps?: Array<Record<string, unknown>> }> {
  const response = await fetch(`/v1/jobs/${id}`);
  return readJSON(response);
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
