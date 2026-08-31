import { jsonPut, jsonRequest, readJSON } from "./api";

export async function saveNetworkSettings(values: { host: string; port: number }): Promise<void> {
  await readJSON(await fetch("/v1/settings/network", jsonPut(values)));
}

export async function ingestDocuments(
  files: FileList | File[],
  options: { stage?: string; kind?: string; tags?: string },
): Promise<void> {
  const form = new FormData();
  for (const file of Array.from(files)) form.append("files", file, file.name);
  if (options.stage) form.append("stage", options.stage);
  if (options.kind) form.append("kind", options.kind);
  if (options.tags) form.append("tags", options.tags);
  await readJSON(await fetch("/v1/ingest/documents", { method: "POST", body: form }));
}

export type DataSource = {
  id: string;
  name: string;
  driver: string;
  execution_mode: "direct" | "delegated";
  host: string;
  port: number;
  database_name: string;
  username: string;
  ssl_mode: string;
  use_dsn: boolean;
  authority_url: string;
  credential_env: string;
  read_only: boolean;
  password_set: boolean;
  password_hint: string;
  last_test_status?: string;
  last_test_message?: string;
  last_test_at?: string;
  created_at: string;
  updated_at: string;
};

export type DataSourceUpsertPayload = {
  name: string;
  driver: string;
  execution_mode: "direct" | "delegated";
  host?: string;
  port?: number;
  database_name?: string;
  username?: string;
  password?: string;
  ssl_mode?: string;
  use_dsn?: boolean;
  dsn?: string;
  authority_url?: string;
  credential_env?: string;
};

export async function createDataSource(input: DataSourceUpsertPayload): Promise<DataSource> {
  const payload = await readJSON<{ source: DataSource }>(await fetch("/v1/admin/data-sources", jsonRequest(input)));
  return payload.source;
}

export async function updateDataSource(id: string, input: DataSourceUpsertPayload): Promise<DataSource> {
  const payload = await readJSON<{ source: DataSource }>(await fetch(`/v1/admin/data-sources/${encodeURIComponent(id)}`, jsonPut(input)));
  return payload.source;
}

export async function deleteDataSource(id: string): Promise<void> {
  await readJSON(await fetch(`/v1/admin/data-sources/${encodeURIComponent(id)}`, { method: "DELETE" }));
}

export async function testDataSource(id: string): Promise<void> {
  await readJSON(await fetch(`/v1/admin/data-sources/${encodeURIComponent(id)}/test`, jsonRequest({})));
}
