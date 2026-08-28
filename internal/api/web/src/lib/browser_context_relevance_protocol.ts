import type { ChatCompletionRequestNonStreaming } from "@mlc-ai/web-llm";

export const browserContextConfigSchema = "omnidex.browser-context-relevance-config.v1";
export const browserContextJobSchema = "omnidex.browser-context-relevance-job.v1";
export const browserContextResultSchema = "omnidex.browser-context-relevance-result.v1";
export const contextRelevanceStation = "context_relevance";
const webLLMEmptyThinkingPrefix = "<think>\n\n</think>\n\n";

export type BrowserContextConfig = {
  schema: typeof browserContextConfigSchema;
  enabled: boolean;
  station: typeof contextRelevanceStation;
  model?: string;
};

export type BrowserContextJob = {
  schema: typeof browserContextJobSchema;
  job_id: string;
  station: typeof contextRelevanceStation;
  model: string;
  prompt: string;
  prompt_hint: string;
  max_output_tokens: number;
};

export type BrowserContextSubmission = {
  schema: typeof browserContextResultSchema;
  job_id: string;
  model: string;
  raw_result?: string;
  error?: string;
};

export function requireBrowserContextConfig(value: unknown): BrowserContextConfig {
  const record = requireExactObject(value, ["schema", "enabled", "station", "model"], "browser inference config");
  if (record.schema !== browserContextConfigSchema || typeof record.enabled !== "boolean" ||
      record.station !== contextRelevanceStation) {
    throw new Error("Browser inference config does not match the context_relevance contract.");
  }
  if (record.enabled && !isBoundedText(record.model, 256)) {
    throw new Error("Enabled browser inference config requires one exact model.");
  }
  if (!record.enabled && record.model !== undefined && record.model !== "") {
    throw new Error("Disabled browser inference config cannot advertise a model.");
  }
  return record as BrowserContextConfig;
}

export function requireBrowserContextJob(value: unknown, model: string): BrowserContextJob {
  const record = requireExactObject(value, [
    "schema", "job_id", "station", "model", "prompt", "prompt_hint",
    "max_output_tokens",
  ], "browser context relevance job");
  if (record.schema !== browserContextJobSchema || record.station !== contextRelevanceStation ||
      record.model !== model || typeof record.job_id !== "string" ||
      !/^bcr_[0-9a-f]{32}$/.test(record.job_id) || !isBoundedText(record.prompt, 128 * 1024) ||
      !isBoundedText(record.prompt_hint, 1024) ||
      !Number.isSafeInteger(record.max_output_tokens) ||
      Number(record.max_output_tokens) < 1 || Number(record.max_output_tokens) > 4096) {
    throw new Error("Browser context relevance job is invalid or differs from configured authority.");
  }
  return record as BrowserContextJob;
}

export function browserContextCompletionRequest(
  job: BrowserContextJob,
): ChatCompletionRequestNonStreaming {
  return {
    stream: false,
    messages: [
      { role: "system", content: job.prompt },
      { role: "user", content: job.prompt_hint },
    ],
    temperature: 0,
    max_tokens: job.max_output_tokens,
    extra_body: { enable_thinking: false },
  };
}

export function browserContextSuccess(
  job: BrowserContextJob,
  rawResult: string,
): BrowserContextSubmission {
  if (!isBoundedText(rawResult, 16 * 1024)) {
    throw new Error("Browser context relevance produced an empty or oversized raw result.");
  }
  return {
    schema: browserContextResultSchema,
    job_id: job.job_id,
    model: job.model,
    raw_result: rawResult,
  };
}

// WebLLM seeds this exact empty block when enable_thinking is false and
// includes it in message.content. It is a provider envelope, not model-authored
// semantic output. No other thinking or prose is removed here.
export function browserContextProviderResult(rawResult: string): string {
  return rawResult.startsWith(webLLMEmptyThinkingPrefix)
    ? rawResult.slice(webLLMEmptyThinkingPrefix.length)
    : rawResult;
}

export function browserContextFailure(
  job: BrowserContextJob,
  reason: unknown,
): BrowserContextSubmission {
  const message = reason instanceof Error ? reason.message : String(reason);
  return {
    schema: browserContextResultSchema,
    job_id: job.job_id,
    model: job.model,
    error: truncateUTF8(message.trim() || "browser inference failed", 2 * 1024),
  };
}

function requireExactObject(
  value: unknown,
  allowed: string[],
  label: string,
): Record<string, unknown> {
  if (!isRecord(value)) throw new Error(`${label} must be one JSON object.`);
  const permitted = new Set(allowed);
  for (const key of Object.keys(value)) {
    if (!permitted.has(key)) throw new Error(`${label} contains unknown field ${key}.`);
  }
  return value;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isBoundedText(value: unknown, maximumBytes: number): value is string {
  return typeof value === "string" && value.trim() !== "" && !value.includes("\0") &&
    new TextEncoder().encode(value).byteLength <= maximumBytes;
}

function truncateUTF8(value: string, maximumBytes: number): string {
  let result = "";
  let bytes = 0;
  const encoder = new TextEncoder();
  for (const character of value) {
    const width = encoder.encode(character).byteLength;
    if (bytes + width > maximumBytes) break;
    result += character;
    bytes += width;
  }
  return result || "browser inference failed";
}
