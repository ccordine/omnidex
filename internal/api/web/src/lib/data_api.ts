import { readJSON, jsonRequest } from "./api";
import type { DataSource } from "./admin_api";

export type DataSourceChannel = {
  id: string;
  data_source_id: string;
  name: string;
  created_at: string;
  updated_at: string;
};

export type DataSourceChannelMessage = {
  id: number;
  channel_id: string;
  role: string;
  content: string;
  payload?: DataSourceChannelPayload;
  job_id?: number;
  created_at: string;
};

export type ChartSpec = {
  type: "bar" | "line" | "pie" | string;
  title?: string;
  label_key?: string;
  value_key?: string;
  series?: Array<{ label: string; value: number }>;
};

export type DataSourceChannelPayload = {
  query?: {
    question?: string;
    sql?: string;
    answer?: string;
    columns?: string[];
    rows?: Array<Record<string, unknown>>;
    count?: number;
    hard_facts?: string[];
    text_insights?: Array<{ field: string; samples: number; summary: string; themes?: string[] }>;
    evidence?: QueryEvidence;
  };
  chart?: ChartSpec;
  evidence?: QueryEvidence;
  job_id?: number;
};

export type QueryEvidence = {
  question: string;
  summary: string;
  step_count: number;
  row_count: number;
  confidence: string;
  steps?: Array<{ step: number; purpose?: string; sql: string; row_count: number; facts?: string[] }>;
  citations?: string[];
};

export type JobRecord = {
  id: number;
  status: string;
  result?: string;
  error?: string;
};

export async function fetchDataSourcesPublic(): Promise<DataSource[]> {
  const response = await fetch("/v1/data-sources");
  const payload = await readJSON<{ sources: DataSource[] }>(response);
  return payload.sources ?? [];
}

export async function fetchDataSourceChannels(sourceID: string): Promise<DataSourceChannel[]> {
  const response = await fetch(`/v1/data-sources/${encodeURIComponent(sourceID)}/channels`);
  const payload = await readJSON<{ channels: DataSourceChannel[] }>(response);
  return payload.channels ?? [];
}

export async function createDataSourceChannel(sourceID: string, name: string): Promise<DataSourceChannel> {
  const response = await fetch(`/v1/data-sources/${encodeURIComponent(sourceID)}/channels`, jsonRequest({ name }));
  const payload = await readJSON<{ channel: DataSourceChannel }>(response);
  return payload.channel;
}

export async function deleteDataSourceChannel(sourceID: string, channelID: string): Promise<void> {
  const response = await fetch(
    `/v1/data-sources/${encodeURIComponent(sourceID)}/channels/${encodeURIComponent(channelID)}`,
    { method: "DELETE" },
  );
  await readJSON(response);
}

export async function fetchDataSourceChannelMessages(sourceID: string, channelID: string, limit = 80): Promise<DataSourceChannelMessage[]> {
  const response = await fetch(
    `/v1/data-sources/${encodeURIComponent(sourceID)}/channels/${encodeURIComponent(channelID)}/messages?limit=${limit}`,
  );
  const payload = await readJSON<{ messages: DataSourceChannelMessage[] }>(response);
  return (payload.messages ?? []).map(normalizeChannelMessage);
}

export async function sendDataSourceChannelMessage(
  sourceID: string,
  channelID: string,
  prompt: string,
): Promise<{ user_message: DataSourceChannelMessage; job: JobRecord; message: string }> {
  const response = await fetch(
    `/v1/data-sources/${encodeURIComponent(sourceID)}/channels/${encodeURIComponent(channelID)}/messages`,
    jsonRequest({ prompt }),
  );
  const payload = await readJSON<{ user_message: DataSourceChannelMessage; job: JobRecord; message: string }>(response);
  return {
    ...payload,
    user_message: normalizeChannelMessage(payload.user_message),
  };
}

export async function fetchJobRecord(id: number): Promise<{ job: JobRecord }> {
  const response = await fetch(`/v1/jobs/${id}`);
  return readJSON(response);
}

function normalizeChannelMessage(message: DataSourceChannelMessage): DataSourceChannelMessage {
  if (!message) return message;
  let payload: DataSourceChannelPayload | undefined;
  const raw = message.payload as unknown;
  if (typeof raw === "string") {
    try {
      payload = JSON.parse(raw) as DataSourceChannelPayload;
    } catch {
      payload = undefined;
    }
  } else if (raw && typeof raw === "object") {
    payload = raw as DataSourceChannelPayload;
  }
  return { ...message, payload };
}

export function parseJobResultPayload(raw: string): DataSourceChannelPayload | null {
  const trimmed = raw.trim();
  if (!trimmed) return null;
  try {
    const parsed = JSON.parse(trimmed) as DataSourceChannelPayload;
    if (parsed?.query) return parsed;
    const legacy = parsed as unknown as DataSourceChannelPayload["query"];
    if (legacy && Array.isArray((legacy as { columns?: string[] }).columns)) {
      return { query: legacy as DataSourceChannelPayload["query"] };
    }
    return parsed;
  } catch {
    return { query: { answer: trimmed, columns: [], rows: [], count: 0 } };
  }
}
