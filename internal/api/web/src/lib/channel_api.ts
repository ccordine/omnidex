import { readJSON } from "./api";
import type {
  ChannelMessage,
  ChannelTranscriptPage,
  ChannelTurnJob,
  ChannelTurnAccepted,
  UserChannel,
} from "./types";

const JOB_STATUSES = ["pending", "running", "waiting_input", "completed", "failed", "canceled"] as const;

export type ChannelCreationContext =
  | { mode: "assistant"; roleplay_world_name?: never; roleplay_viewpoint_name?: never }
  | { mode: "roleplay"; roleplay_world_name: string; roleplay_viewpoint_name: string };

export async function fetchChannelTranscript(
  channelID: string,
  options: { limit?: number; beforeID?: number; requiredMessageID?: number } = {},
): Promise<ChannelTranscriptPage> {
  requireChannelID(channelID, "Channel message request");
  const limit = options.limit ?? 48;
  requireBoundedInteger(limit, "channel message limit", 1, 200);
  if (options.beforeID !== undefined) {
    requireBoundedInteger(options.beforeID, "channel message cursor", 1, Number.MAX_SAFE_INTEGER);
  }
  if (options.requiredMessageID !== undefined) {
    requireBoundedInteger(options.requiredMessageID, "required channel message", 1, Number.MAX_SAFE_INTEGER);
  }
  const query = new URLSearchParams({ limit: String(limit) });
  if (options.beforeID !== undefined) query.set("before_id", String(options.beforeID));
  if (options.requiredMessageID !== undefined) query.set("required_message_id", String(options.requiredMessageID));
  const response = await fetch(`/v1/channels/${encodeURIComponent(channelID)}/messages?${query}`);
  const payload = await readJSON<Record<string, unknown>>(response);
  requireStatus(response, 200, "channel transcript");
  if (payload.channel_id !== channelID) throw new Error("Channel transcript response changed the requested channel identity.");
  if (typeof payload.has_more !== "boolean") throw new Error("Channel transcript response did not include has_more.");
  const html = requireRecord(payload.html, "Channel transcript html");
  const bundle = requireNonblankString(html.bundle, "Channel transcript bundle");
  const page: ChannelTranscriptPage = {
    channel_id: channelID,
    has_more: payload.has_more,
    html: { bundle },
  };
  if (payload.next_before_id !== undefined) {
    page.next_before_id = requireBoundedInteger(
      payload.next_before_id,
      "channel transcript next_before_id",
      1,
      Number.MAX_SAFE_INTEGER,
    );
  }
  if (page.has_more !== (page.next_before_id !== undefined)) {
    throw new Error("Channel transcript pagination fields are contradictory.");
  }
  return page;
}

export async function sendChannelMessage(
  channelID: string,
  prompt: string,
): Promise<ChannelTurnAccepted> {
  requireChannelID(channelID, "Channel turn request");
  requireBoundedText(prompt, "Channel turn prompt", 4096, false);
  const response = await fetch(`/v1/channels/${encodeURIComponent(channelID)}/messages`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ prompt }),
  });
  const payload = await readJSON<Record<string, unknown>>(response);
  requireStatus(response, 202, "channel turn");
  const accepted: ChannelTurnAccepted = {
    channel: requireUserChannel(payload.channel, "Channel turn channel"),
    user_message: requireChannelMessage(payload.user_message, "Channel turn user_message"),
    job: requireChannelTurnJob(payload.job, "Channel turn job"),
  };
  if (accepted.channel.id !== channelID || accepted.user_message.channel_id !== channelID) {
    throw new Error(`Channel turn response identity does not match ${JSON.stringify(channelID)}.`);
  }
  if (accepted.user_message.role !== "user") {
    throw new Error("Channel turn response user_message role must be exactly user.");
  }
  if (accepted.user_message.content !== prompt) {
    throw new Error("Channel turn response did not preserve the exact prompt bytes.");
  }
  if (accepted.job.instruction !== prompt) {
    throw new Error("Channel turn job did not preserve the exact prompt bytes.");
  }
  return accepted;
}

export async function createUserChannel(input: {
  id: string;
  name: string;
  workspace_root?: string;
  tags?: string[];
  data_source_id?: string;
} & ChannelCreationContext): Promise<UserChannel> {
  requireChannelID(input.id, "Channel create request");
  requireExactText(input.name, "Channel create name");
  requireTags(input.tags ?? [], "Channel create tags");
  const workspaceRoot = input.workspace_root === undefined
    ? undefined
    : requireWorkspaceRoot(input.workspace_root, "Channel create workspace_root");
  const dataSourceID = input.data_source_id === undefined
    ? undefined
    : requireDataSourceID(input.data_source_id, "Channel create data_source_id");
  const creation = requireChannelCreationContext(input);
	if (creation.mode === "roleplay" && dataSourceID !== undefined) {
		throw new Error("Roleplay channel creation cannot bind a real-world data source.");
	}
  const response = await fetch("/v1/channels", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      id: input.id,
      name: input.name,
      tags: input.tags ?? [],
      ...(workspaceRoot === undefined ? {} : { workspace_root: workspaceRoot }),
      ...(dataSourceID === undefined ? {} : { data_source_id: dataSourceID }),
      ...creation,
    }),
  });
  const payload = await readJSON<{ channel?: unknown }>(response);
  requireStatus(response, 201, "channel create");
  const channel = requireUserChannel(payload.channel, "Channel create response");
  if (channel.id !== input.id || channel.name !== input.name ||
    (workspaceRoot !== undefined && channel.workspace_root !== workspaceRoot) ||
    (channel.data_source_id ?? "") !== (dataSourceID ?? "") || channel.mode !== creation.mode) {
    throw new Error("Channel create response changed the requested identity.");
  }
  return channel;
}

function requireUserChannel(value: unknown, source: string): UserChannel {
  const raw = requireRecord(value, source);
  const id = requireChannelID(raw.id, source);
  if (raw.scope !== "user") throw new Error(`${source} scope must be exactly "user".`);
  const name = requireExactText(raw.name, `${source} name`);
  const tags = requireTags(raw.tags, `${source} tags`);
  const dataSourceID = raw.data_source_id === undefined
    ? undefined
    : requireDataSourceID(raw.data_source_id, `${source} data_source_id`);
  const mode = requireChannelMode(raw.mode, `${source} mode`);
  const viewpointID = raw.roleplay_viewpoint_character_id === undefined
    ? undefined
    : requireRoleplayCharacterID(raw.roleplay_viewpoint_character_id, `${source} roleplay viewpoint`);
  if (mode === "assistant" && viewpointID !== undefined) {
    throw new Error(`${source} assistant mode cannot carry a roleplay viewpoint.`);
  }
  if (mode === "roleplay" && viewpointID === undefined) {
    throw new Error(`${source} roleplay mode requires its persisted viewpoint identity.`);
  }
	if (mode === "roleplay" && dataSourceID !== undefined) {
		throw new Error(`${source} roleplay mode cannot carry a real-world data source.`);
	}
  return {
    id,
    scope: "user",
    name,
    tags,
    project_id: requireBoundedInteger(raw.project_id, `${source} project_id`, 1, Number.MAX_SAFE_INTEGER),
    workspace_root: requireWorkspaceRoot(raw.workspace_root, `${source} workspace_root`),
    ...(dataSourceID === undefined ? {} : { data_source_id: dataSourceID }),
    mode,
    ...(viewpointID === undefined ? {} : { roleplay_viewpoint_character_id: viewpointID }),
    created_at: requireTimestamp(raw.created_at, `${source} created_at`),
    updated_at: requireTimestamp(raw.updated_at, `${source} updated_at`),
  };
}

function requireChannelCreationContext(input: {
  mode: unknown;
  roleplay_world_name?: unknown;
  roleplay_viewpoint_name?: unknown;
}): ChannelCreationContext {
  const mode = requireChannelMode(input.mode, "Channel create mode");
  if (mode === "assistant") {
    if (input.roleplay_world_name !== undefined || input.roleplay_viewpoint_name !== undefined) {
      throw new Error("Assistant channel creation cannot carry roleplay names.");
    }
    return { mode };
  }
  return {
    mode,
    roleplay_world_name: requireExactText(input.roleplay_world_name, "Channel create roleplay world name"),
    roleplay_viewpoint_name: requireExactText(input.roleplay_viewpoint_name, "Channel create roleplay viewpoint name"),
  };
}

function requireChannelMode(value: unknown, source: string): "assistant" | "roleplay" {
  if (value !== "assistant" && value !== "roleplay") {
    throw new Error(`${source} must be exactly assistant or roleplay.`);
  }
  return value;
}

function requireRoleplayCharacterID(value: unknown, source: string): string {
  if (typeof value !== "string" || !/^rpc_[0-9a-f]{32}$/.test(value)) {
    throw new Error(`${source} has an invalid canonical roleplay character id.`);
  }
  return value;
}

function requireDataSourceID(value: unknown, source: string): string {
  if (typeof value !== "string" || !/^[a-z0-9][a-z0-9_.:-]{0,127}$/.test(value)) {
    throw new Error(`${source} has an invalid canonical data-source id.`);
  }
  return value;
}

function requireChannelMessage(value: unknown, source: string): ChannelMessage {
  const raw = requireRecord(value, source);
  const role = raw.role;
  if (role !== "user" && role !== "assistant") {
    throw new Error(`${source} has unsupported role ${JSON.stringify(role)}.`);
  }
  return {
    id: requireBoundedInteger(raw.id, `${source} id`, 1, Number.MAX_SAFE_INTEGER),
    channel_id: requireChannelID(raw.channel_id, source),
    role,
    content: requireBoundedText(raw.content, `${source} content`, role === "user" ? 4096 : 32768, false),
    created_at: requireTimestamp(raw.created_at, `${source} created_at`),
  };
}

function requireChannelTurnJob(value: unknown, source: string): ChannelTurnJob {
  const raw = requireRecord(value, source);
  const status = raw.status;
  if (typeof status !== "string" || !JOB_STATUSES.includes(status as (typeof JOB_STATUSES)[number])) {
    throw new Error(`${source} has invalid status ${JSON.stringify(status)}.`);
  }
  return {
    id: requireBoundedInteger(raw.id, `${source} id`, 1, Number.MAX_SAFE_INTEGER),
    instruction: requireBoundedText(raw.instruction, `${source} instruction`, 65536, false),
    pipeline: requireExactValue(raw.pipeline, "chat", `${source} pipeline`),
    status: status as ChannelTurnJob["status"],
  };
}

function requireChannelID(value: unknown, source: string): string {
  if (typeof value !== "string" || !/^[a-z0-9][a-z0-9_.:-]{0,95}$/.test(value)) {
    throw new Error(`${source} has an invalid canonical channel id.`);
  }
  return value;
}

function requireTags(value: unknown, source: string): string[] {
  if (!Array.isArray(value) || value.length > 32) throw new Error(`${source} must be an array of at most 32 tags.`);
  const tags = value.map((item, index) => requireBoundedText(item, `${source} item ${index}`, 64, true));
  if (tags.some((tag) => tag !== tag.toLowerCase())) throw new Error(`${source} must contain only lowercase tags.`);
  if (new Set(tags).size !== tags.length) throw new Error(`${source} contains duplicate tags.`);
  return tags;
}

function requireExactText(value: unknown, source: string): string {
  return requireBoundedText(value, source, 256, true);
}

function requireWorkspaceRoot(value: unknown, source: string): string {
  const root = requireBoundedText(value, source, 4096, true);
  if (!root.startsWith("/") || root.includes("//") || (root !== "/" && root.endsWith("/"))) {
    throw new Error(`${source} must be an exact absolute canonical path.`);
  }
  if (root.split("/").some((segment) => segment === "." || segment === "..")) {
    throw new Error(`${source} must not contain dot path segments.`);
  }
  return root;
}

function requireNonblankString(value: unknown, source: string): string {
  if (typeof value !== "string" || !value.trim() || value.includes("\0")) {
    throw new Error(`${source} must be a non-blank string without NUL.`);
  }
  return value;
}

function requireBoundedText(value: unknown, source: string, maxBytes: number, exact: boolean): string {
  const text = requireNonblankString(value, source);
  if (new TextEncoder().encode(text).byteLength > maxBytes) throw new Error(`${source} exceeds ${maxBytes} UTF-8 bytes.`);
  if (exact && text !== text.trim()) throw new Error(`${source} must not have surrounding whitespace.`);
  return text;
}

function requireTimestamp(value: unknown, source: string): string {
  const timestamp = requireNonblankString(value, source);
  if (Number.isNaN(Date.parse(timestamp))) throw new Error(`${source} must be a valid timestamp.`);
  return timestamp;
}

function requireExactValue<T extends string>(value: unknown, expected: T, source: string): T {
  if (value !== expected) throw new Error(`${source} must be exactly ${JSON.stringify(expected)}.`);
  return expected;
}

function requireBoundedInteger(value: unknown, source: string, minimum: number, maximum: number): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < minimum || value > maximum) {
    throw new Error(`${source} must be an integer between ${minimum} and ${maximum}.`);
  }
  return value;
}

function requireRecord(value: unknown, source: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error(`${source} must be an object.`);
  return value as Record<string, unknown>;
}

function requireStatus(response: Response, expected: number, operation: string): void {
  if (response.status !== expected) {
    throw new Error(`${operation} expected HTTP ${expected}, received HTTP ${response.status}.`);
  }
}
