import { jsonRequest, readJSON } from "./api";

export type DataSourceChannel = {
  id: string;
  data_source_id: string;
  name: string;
  created_at: string;
  updated_at: string;
};

export async function createDataSourceChannel(sourceID: string, name: string): Promise<DataSourceChannel> {
  const response = await fetch(`/v1/data-sources/${encodeURIComponent(sourceID)}/channels`, jsonRequest({ name }));
  const payload = await readJSON<{ channel: DataSourceChannel }>(response);
  return payload.channel;
}
