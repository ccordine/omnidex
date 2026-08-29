const encoder = new TextEncoder();
const decoder = new TextDecoder();
const CURSOR_PREFIX = "scrumchat_v1_";
const MAX_CURSOR_ORDINAL = BigInt(Number.MAX_SAFE_INTEGER);

export function scrumChannelCursorOrdinal(cursor: string): bigint {
  const match = /^scrumchat_v1_([1-9a-z][0-9a-z]*)$/.exec(cursor);
  if (!match) return 0n;
  let ordinal = 0n;
  for (const character of match[1]) {
    const code = character.charCodeAt(0);
    const digit = code >= 48 && code <= 57 ? code - 48 : code - 87;
    ordinal = ordinal * 36n + BigInt(digit);
    if (ordinal > MAX_CURSOR_ORDINAL) return 0n;
  }
  return ordinal;
}

export function validateScrumChannelCursor(value: unknown, label: string, allowEmpty: boolean): string {
  if (typeof value !== "string" || value.includes("\0") || value !== value.trim()) {
    throw new Error(`${label} requires one exact canonical cursor.`);
  }
  const bytes = encoder.encode(value);
  if (bytes.byteLength > 128 || decoder.decode(bytes) !== value) {
    throw new Error(`${label} requires one exact canonical cursor.`);
  }
  if (value === "") {
    if (allowEmpty) return value;
    throw new Error(`${label} requires one exact canonical cursor.`);
  }
  if (!value.startsWith(CURSOR_PREFIX) || scrumChannelCursorOrdinal(value) === 0n) {
    throw new Error(`${label} requires one exact canonical cursor.`);
  }
  return value;
}
