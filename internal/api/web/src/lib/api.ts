export async function readJSON<T = Record<string, unknown>>(response: Response): Promise<T> {
  const text = await response.text();
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
    throw new Error(message);
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
