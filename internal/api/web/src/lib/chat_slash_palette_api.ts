import { readJSON } from "./api";
import { requireServerComponentBundle } from "./chat_component_api";

const CANONICAL_CHANNEL_ID = /^[a-z0-9][a-z0-9_.:-]{0,95}$/;
const MAX_SLASH_COMMANDS = 97;

export interface SlashCommandComponent {
  channel_id: string;
  command_count: number;
  html: { bundle: string };
}

export async function fetchSlashCommandComponent(channelID: string): Promise<SlashCommandComponent> {
  if (!CANONICAL_CHANNEL_ID.test(channelID)) {
    throw new Error("Slash-command projection requires a canonical channel id.");
  }
  const query = new URLSearchParams({ channel_id: channelID });
  const payload = await readJSON<unknown>(await fetch(`/v1/ui/chat/slash-commands?${query}`));
  return decodeSlashCommandComponent(payload, channelID);
}

function decodeSlashCommandComponent(payload: unknown, requestedChannelID: string): SlashCommandComponent {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new Error("Slash-command component response is not an object.");
  }
  const record = payload as Record<string, unknown>;
  const fields = Object.keys(record).sort();
  if (fields.join(",") !== "channel_id,command_count,html") {
    throw new Error("Slash-command component response has fields outside its exact contract.");
  }
  if (record.channel_id !== requestedChannelID) {
    throw new Error("Slash-command component changed the active channel identity.");
  }
  if (!Number.isSafeInteger(record.command_count) || (record.command_count as number) < 0 ||
    (record.command_count as number) > MAX_SLASH_COMMANDS) {
    throw new Error(`Slash-command component count is outside 0..${MAX_SLASH_COMMANDS}.`);
  }
  return {
    channel_id: requestedChannelID,
    command_count: record.command_count as number,
    html: { bundle: requireServerComponentBundle(record, "Slash-command component") },
  };
}
