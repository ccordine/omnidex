import { SCRUM_CHANNEL_RESPONSE_MAX_BYTES } from "./api";

const encoder = new TextEncoder();
const decoder = new TextDecoder("utf-8", { fatal: true, ignoreBOM: true });
const GO_UNICODE_WHITESPACE = /^[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$/gu;

function trimGoUnicodeWhitespace(value: string): string {
  return value.replace(GO_UNICODE_WHITESPACE, "");
}

export function exactRecord(
  value: unknown,
  label: string,
  allowed: readonly string[],
  required: readonly string[] = allowed,
): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${label} must be one typed object.`);
  }
  const record = value as Record<string, unknown>;
  const unknown = Object.keys(record).find((key) => !allowed.includes(key));
  if (unknown) throw new Error(`${label} contains unknown field ${JSON.stringify(unknown)}.`);
  const missing = required.find((key) => !Object.prototype.hasOwnProperty.call(record, key));
  if (missing) throw new Error(`${label} is missing required field ${JSON.stringify(missing)}.`);
  return record;
}

export function exactString(value: unknown, label: string, options: {
  maxBytes?: number;
  nonblank?: boolean;
  canonical?: boolean;
} = {}): string {
  if (typeof value !== "string" || value.includes("\0")) {
    throw new Error(`${label} must be a NUL-free string.`);
  }
  const bytes = encoder.encode(value);
  if (decoder.decode(bytes) !== value) throw new Error(`${label} must contain valid Unicode.`);
  if (bytes.byteLength > (options.maxBytes ?? SCRUM_CHANNEL_RESPONSE_MAX_BYTES)) {
    throw new Error(`${label} exceeds its response bound.`);
  }
  const trimmed = trimGoUnicodeWhitespace(value);
  if (options.nonblank && !trimmed) throw new Error(`${label} must not be blank.`);
  if (options.canonical && value !== trimmed) throw new Error(`${label} must be canonical.`);
  return value;
}

export function exactInteger(value: unknown, label: string, maximum = Number.MAX_SAFE_INTEGER): number {
  if (!Number.isSafeInteger(value) || (value as number) < 0 || (value as number) > maximum) {
    throw new Error(`${label} must be a bounded non-negative integer.`);
  }
  return value as number;
}

export function exactTimestamp(value: unknown, label: string): string {
  const timestamp = exactString(value, label, { maxBytes: 64, nonblank: true, canonical: true });
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(timestamp) || !Number.isFinite(Date.parse(timestamp))) {
    throw new Error(`${label} must be one canonical UTC timestamp.`);
  }
  return timestamp;
}

export function exactMicrosecondTimestamp(value: unknown, label: string): string {
  const timestamp = exactTimestamp(value, label);
  const match = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.[0-9]{1,6})?Z$/.exec(timestamp);
  if (!match || (match[1]?.endsWith("0") ?? false)) {
    throw new Error(`${label} must be one canonical UTC microsecond timestamp.`);
  }
  const parsed = new Date(timestamp);
  if (!Number.isFinite(parsed.getTime()) || parsed.toISOString().slice(0, 19) !== timestamp.slice(0, 19)) {
    throw new Error(`${label} must be one canonical UTC microsecond timestamp.`);
  }
  return timestamp;
}
