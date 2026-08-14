export const DEFAULT_JSON_RESPONSE_MAX_BYTES = 8 * 1024 * 1024;
// A byte-bounded Scrum channel window can contain 4 MiB of control characters,
// each of which expands to six bytes in JSON. The remaining allowance covers
// the closed card projection and response envelope without dropping history.
export const SCRUM_CHANNEL_RESPONSE_MAX_BYTES = 32 * 1024 * 1024;

export class HTTPResponseError extends Error {
  constructor(readonly status: number, message: string) {
    super(message);
    this.name = "HTTPResponseError";
  }
}

async function readBoundedResponseText(response: Response, maxBytes: number): Promise<string> {
  if (!Number.isSafeInteger(maxBytes) || maxBytes <= 0) {
    throw new Error("JSON response byte bound must be a positive safe integer.");
  }
  const contentLength = response.headers.get("Content-Length");
  if (contentLength !== null) {
    if (!/^(0|[1-9][0-9]*)$/.test(contentLength)) throw new Error("Response Content-Length is not canonical.");
    const declaredBytes = Number(contentLength);
    if (!Number.isSafeInteger(declaredBytes)) throw new Error("Response Content-Length exceeds the safe integer bound.");
    if (declaredBytes > maxBytes) throw new Error(`JSON response exceeds the ${maxBytes}-byte transport bound.`);
  }
  if (!response.body) return "";
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let totalBytes = 0;
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    totalBytes += value.byteLength;
    if (totalBytes > maxBytes) throw new Error(`JSON response exceeds the ${maxBytes}-byte transport bound.`);
    chunks.push(value);
  }
  const bytes = new Uint8Array(totalBytes);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch {
    throw new Error("JSON response is not valid UTF-8.");
  }
}

export async function readJSON<T = Record<string, any>>(
  response: Response,
  maxBytes = DEFAULT_JSON_RESPONSE_MAX_BYTES,
): Promise<T> {
  const text = await readBoundedResponseText(response, maxBytes);
  const trimmed = text.trim();
  let payload: Record<string, unknown> = {};
  if (trimmed) {
    try {
      payload = parseFirstJSONValue(trimmed) as Record<string, unknown>;
    } catch (error) {
      const snippet = trimmed.length > 160 ? `${trimmed.slice(0, 160)}…` : trimmed;
      const detail = error instanceof Error ? error.message : String(error);
      throw new Error(snippet ? `${detail}: ${snippet}` : detail);
    }
  }
  if (!response.ok) {
    const message =
      (typeof payload.error === "string" && payload.error) ||
      (typeof payload.message === "string" && payload.message) ||
      `HTTP ${response.status}`;
    throw new HTTPResponseError(response.status, message);
  }
  return payload as T;
}

function parseFirstJSONValue(text: string): unknown {
  let index = 0;
  while (index < text.length && /\s/.test(text[index] ?? "")) index += 1;
  const start = text[index];
  if (start !== "{" && start !== "[") {
    throw new Error("Response was not JSON");
  }
  const open = start;
  const close = start === "{" ? "}" : "]";
  let depth = 0;
  let inString = false;
  let escaped = false;
  for (let i = index; i < text.length; i += 1) {
    const ch = text[i] ?? "";
    if (inString) {
      if (escaped) {
        escaped = false;
        continue;
      }
      if (ch === "\\") {
        escaped = true;
        continue;
      }
      if (ch === '"') inString = false;
      continue;
    }
    if (ch === '"') {
      inString = true;
      continue;
    }
    if (ch === open) depth += 1;
    if (ch === close) {
      depth -= 1;
      if (depth === 0) {
        if (text.slice(i + 1).trim() !== "") {
          throw new Error("Response contained trailing data");
        }
        return JSON.parse(text.slice(index, i + 1));
      }
    }
  }
  throw new Error("Incomplete JSON response");
}

export function jsonRequest(body: unknown): RequestInit {
  return {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body ?? {}),
  };
}

export function jsonPut(body: unknown): RequestInit {
  return {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body ?? {}),
  };
}
