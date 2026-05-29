import { readJSON } from "./api";
import type { ChannelMessage, UserChannel } from "./types";

export async function fetchUserChannels(limit = 100): Promise<UserChannel[]> {
  const response = await fetch(`/v1/channels?limit=${limit}&scope=user`);
  const payload = await readJSON<{ channels: UserChannel[] }>(response);
  return payload.channels ?? [];
}

export async function fetchChannelMessages(channelID: string, limit = 48): Promise<ChannelMessage[]> {
  const response = await fetch(`/v1/channels/${encodeURIComponent(channelID)}/messages?limit=${limit}`);
  const payload = await readJSON<{ messages: ChannelMessage[] }>(response);
  return payload.messages ?? [];
}

export async function sendChannelMessage(
  channelID: string,
  prompt: string,
): Promise<{ output: string; model?: string; latency_ms?: number }> {
  const response = await fetch(`/v1/channels/${encodeURIComponent(channelID)}/messages`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ prompt }),
  });
  return readJSON(response);
}

export async function createUserChannel(input: {
  id: string;
  name: string;
  persona?: string;
  system?: string;
  tags?: string[];
}): Promise<UserChannel> {
  const response = await fetch("/v1/channels", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      id: input.id,
      name: input.name,
      persona: input.persona || "assistant",
      system: input.system || "You are a helpful assistant.",
      tags: input.tags ?? [],
      context: {},
    }),
  });
  const payload = await readJSON<{ channel: UserChannel }>(response);
  return payload.channel;
}

/** User-facing channels only — excludes internal thought-channel tags if any leak in. */
export function isUserChannel(channel: UserChannel): boolean {
  const id = (channel.id || "").toLowerCase();
  if (id.startsWith("thought_") || id.startsWith("internal-")) return false;
  const tags = (channel.tags ?? []).map((tag) => tag.toLowerCase());
  return !tags.includes("thought-channel") && !tags.includes("internal:thought");
}
