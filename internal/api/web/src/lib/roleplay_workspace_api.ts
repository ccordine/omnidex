import { readJSON } from "./api";
import { requireServerComponentBundle, type ChatComponentPage } from "./chat_component_api";

export async function fetchRoleplayWorldsPage(offset = 0): Promise<ChatComponentPage> {
  return fetchPage(`/v1/ui/roleplay/worlds?limit=20&offset=${pageOffset(offset)}`, "Roleplay worlds", 200);
}

export async function fetchRoleplayLibraryPage(channelID: string, offset = 0): Promise<ChatComponentPage> {
  const query = new URLSearchParams({ limit: "20", offset: String(pageOffset(offset)) });
  if (channelID) query.set("channel_id", exactChannelID(channelID));
  return fetchPage(`/v1/ui/roleplay/library?${query}`, "Character library", 200);
}

export async function createRoleplayLibraryCharacter(name: string, channelID: string): Promise<ChatComponentPage> {
  const query = channelID ? `?channel_id=${encodeURIComponent(exactChannelID(channelID))}` : "";
  return fetchPage(`/v1/ui/roleplay/library${query}`, "Create library character", 201, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name: exactName(name) }),
  });
}

function exactChannelID(value: string): string {
  if (!/^[a-z0-9][a-z0-9_.:-]{0,95}$/.test(value)) {
    throw new Error("Selected roleplay world lacks a canonical channel identity.");
  }
  return value;
}

async function fetchPage(
  url: string,
  source: string,
  expectedStatus: number,
  init?: RequestInit,
): Promise<ChatComponentPage> {
  const response = await fetch(url, init);
  const payload = await readJSON<Record<string, unknown>>(response);
  if (response.status !== expectedStatus) {
    const message = typeof payload.error === "string" && payload.error.trim() ? payload.error : `${source} failed.`;
    throw new Error(message);
  }
  if (typeof payload.has_more !== "boolean") throw new Error(`${source} omitted has_more.`);
  const page: ChatComponentPage = {
    has_more: payload.has_more,
    html: { bundle: requireServerComponentBundle(payload, source) },
  };
  if (payload.next_offset !== undefined) page.next_offset = pageOffset(payload.next_offset);
  if (page.has_more !== (page.next_offset !== undefined)) {
    throw new Error(`${source} returned contradictory pagination.`);
  }
  return page;
}

function pageOffset(value: unknown): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) {
    throw new Error("Roleplay page offset must be a nonnegative integer.");
  }
  return value;
}

function exactName(value: string): string {
  if (!value || value !== value.trim() || value.includes("\0") || new TextEncoder().encode(value).byteLength > 256) {
    throw new Error("Character name must be exact nonblank text of at most 256 UTF-8 bytes.");
  }
  return value;
}
