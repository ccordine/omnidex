import { readJSON } from "./api";

export interface ChatComponentPage {
  next_offset?: number;
  has_more: boolean;
  html: { bundle: string };
}

export interface ChatChannelOptionsPage extends ChatComponentPage {
  default_channel_id?: string;
}

export interface ChatMemoryPage {
  memory?: { next_offset?: number; has_more: boolean };
  candidates?: { next_offset?: number; has_more: boolean };
  html: { bundle: string };
}

export async function fetchChannelOptionsPage(offset = 0, limit = 20): Promise<ChatChannelOptionsPage> {
  const payload = await fetchComponentPage(
    `/v1/ui/chat/channels?limit=${boundedPageInteger(limit, "channel page limit", 1, 50)}` +
      `&offset=${boundedPageInteger(offset, "channel page offset", 0, Number.MAX_SAFE_INTEGER)}`,
    "Channel options",
  );
  const defaultID = payload.default_channel_id;
  if (defaultID !== undefined && (typeof defaultID !== "string" || !/^[a-z0-9][a-z0-9_.:-]{0,95}$/.test(defaultID))) {
    throw new Error("Channel options returned an invalid default channel id.");
  }
  return { ...payload, default_channel_id: defaultID as string | undefined };
}

export async function fetchChatJobsPage(status: string, offset = 0, limit = 20): Promise<ChatComponentPage> {
  const query = componentPageQuery(offset, limit);
  if (status) query.set("status", status);
  return fetchComponentPage(`/v1/ui/chat/jobs?${query}`, "Chat jobs");
}

export async function fetchChatTimelinePage(offset = 0, limit = 20): Promise<ChatComponentPage> {
  return fetchComponentPage(`/v1/ui/chat/timeline?${componentPageQuery(offset, limit)}`, "Chat timeline");
}

export async function fetchChatMemoryPage(
  section: "all" | "memory" | "candidates",
  kind: string,
  offset = 0,
  limit = 20,
): Promise<ChatMemoryPage> {
  const query = componentPageQuery(offset, limit);
  query.set("section", section);
  if (kind) query.set("kind", kind);
  const response = await fetch(`/v1/ui/chat/memory?${query}`);
  const payload = await readJSON<Record<string, unknown>>(response);
  requireHTTPStatus(response, 200, "Chat memory");
  const html = requiredRecord(payload.html, "Chat memory HTML");
  const page: ChatMemoryPage = { html: { bundle: requiredBundle(html.bundle, "Chat memory") } };
  if (section === "all" || section === "memory") {
    page.memory = requiredSectionPage(payload.memory, "Chat memory page");
  }
  if (section === "all" || section === "candidates") {
    page.candidates = requiredSectionPage(payload.candidates, "Chat candidate page");
  }
  return page;
}

export function requireServerComponentBundle(payload: unknown, source: string, field = "html"): string {
  const record = requiredRecord(payload, source);
  const html = requiredRecord(record[field], `${source} ${field}`);
  return requiredBundle(html.bundle, source);
}

async function fetchComponentPage(url: string, source: string): Promise<ChatComponentPage & Record<string, unknown>> {
  const response = await fetch(url);
  const payload = await readJSON<Record<string, unknown>>(response);
  requireHTTPStatus(response, 200, source);
  const html = requiredRecord(payload.html, `${source} HTML`);
  const hasMore = payload.has_more;
  if (typeof hasMore !== "boolean") throw new Error(`${source} did not include has_more.`);
  const page: ChatComponentPage & Record<string, unknown> = {
    ...payload,
    has_more: hasMore,
    html: { bundle: requiredBundle(html.bundle, source) },
  };
  if (payload.next_offset !== undefined) {
    page.next_offset = boundedPageInteger(payload.next_offset, `${source} next_offset`, 1, Number.MAX_SAFE_INTEGER);
  }
  if (page.has_more !== (page.next_offset !== undefined)) {
    throw new Error(`${source} pagination fields are contradictory.`);
  }
  return page;
}

function requiredSectionPage(value: unknown, source: string): { next_offset?: number; has_more: boolean } {
  const raw = requiredRecord(value, source);
  if (typeof raw.has_more !== "boolean") throw new Error(`${source} did not include has_more.`);
  const page: { next_offset?: number; has_more: boolean } = { has_more: raw.has_more };
  if (raw.next_offset !== undefined) {
    page.next_offset = boundedPageInteger(raw.next_offset, `${source} next_offset`, 1, Number.MAX_SAFE_INTEGER);
  }
  if (page.has_more !== (page.next_offset !== undefined)) {
    throw new Error(`${source} pagination fields are contradictory.`);
  }
  return page;
}

function componentPageQuery(offset: number, limit: number): URLSearchParams {
  return new URLSearchParams({
    limit: String(boundedPageInteger(limit, "component page limit", 1, 50)),
    offset: String(boundedPageInteger(offset, "component page offset", 0, Number.MAX_SAFE_INTEGER)),
  });
}

function boundedPageInteger(value: unknown, source: string, minimum: number, maximum: number): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < minimum || value > maximum) {
    throw new Error(`${source} must be an integer between ${minimum} and ${maximum}.`);
  }
  return value;
}

function requiredRecord(value: unknown, source: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error(`${source} must be an object.`);
  return value as Record<string, unknown>;
}

function requiredBundle(value: unknown, source: string): string {
  if (typeof value !== "string" || !value.trim() || value.includes("\0")) {
    throw new Error(`${source} did not include its required server-rendered bundle.`);
  }
  return value;
}

function requireHTTPStatus(response: Response, expected: number, source: string): void {
  if (response.status !== expected) {
    throw new Error(`${source} expected HTTP ${expected}, received HTTP ${response.status}.`);
  }
}
