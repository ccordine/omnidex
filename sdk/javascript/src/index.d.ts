export type DirectDataSourceInput = {
  name: string; host: string; port: number; databaseName: string; username: string;
  password?: string; sslMode: "disable" | "allow" | "prefer" | "require" | "verify-ca" | "verify-full";
  useDsn: boolean; dsn?: string;
};
export type DelegatedDataSourceInput = { name: string; authorityUrl: string; credentialEnv: string };
export type DataSource = {
  id: string; name: string; driver: "postgres"; execution_mode: "direct" | "delegated";
  read_only: true; [key: string]: unknown;
};
export type CreateChannelInput = {
  id: string; name: string; tags: string[]; workspaceRoot: string; dataSourceId: string;
};
export type Channel = {
  id: string; scope: "user"; name: string; tags: string[]; project_id: number;
  workspace_root: string; data_source_id: string; mode: "assistant"; created_at: string; updated_at: string;
};
export type ChannelMessage = {
  id: number; channel_id: string; role: "user" | "assistant"; content: string; created_at: string;
};
export type Job = {
  id: number; instruction: string; pipeline: string; status: string; result?: string; error?: string;
  current_generation: number; created_at: string; updated_at: string; completed_at?: string;
};
export type SendMessageResult = { channel: Channel; user_message: ChannelMessage; job: Job };
export type MessagePage = {
  channel_id: string; messages: ChannelMessage[]; next_before_id: number | null; has_more: boolean;
};
export type JobDetails = { job: Job; steps: Record<string, unknown>[]; contexts: Record<string, unknown>[] };
export type RequestOptions = { signal?: AbortSignal };
export class OmnidexApiError extends Error { readonly status: number; readonly apiMessage: string; }
export class OmnidexClient {
  constructor(options: { baseUrl: string; token: string; timeoutMs?: number; fetch?: typeof fetch });
  registerDirectDataSource(input: DirectDataSourceInput, options?: RequestOptions): Promise<DataSource>;
  registerDelegatedDataSource(input: DelegatedDataSourceInput, options?: RequestOptions): Promise<DataSource>;
  createChannel(input: CreateChannelInput, options?: RequestOptions): Promise<Channel>;
  getChannel(channelId: string, options?: RequestOptions): Promise<Channel>;
  sendMessage(channelId: string, input: { prompt: string; delegatedDataAuthorityId?: string }, options?: RequestOptions): Promise<SendMessageResult>;
  listMessages(channelId: string, options?: { limit?: number; beforeId?: number; signal?: AbortSignal }): Promise<MessagePage>;
  getJob(jobId: number, options?: RequestOptions): Promise<JobDetails>;
}
export function validateDelegatedAuthorityId(value: string): void;
export function newDelegatedAuthorityId(): string;
