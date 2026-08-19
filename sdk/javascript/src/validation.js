const CANONICAL_ID = /^[a-z0-9][a-z0-9_.:-]*$/;
const AUTHORITY_ID = /^dba_[0-9a-f]{64}$/;
const ENVIRONMENT_NAME = /^OMNIDEX_DELEGATED_AUTHORITY_[A-Z][A-Z0-9_]{0,93}_TOKEN$/;
const SSL_MODES = new Set(["disable", "allow", "prefer", "require", "verify-ca", "verify-full"]);

export function validateConfiguration(baseUrl, token) {
  exactString(baseUrl, "Omnidex base URL");
  let parsed;
  try {
    parsed = new URL(baseUrl);
  } catch {
    throw new TypeError("Omnidex base URL must be one HTTP(S) origin or canonical base path.");
  }
  if (!["http:", "https:"].includes(parsed.protocol) || parsed.username || parsed.password || parsed.search || parsed.hash || baseUrl.endsWith("/")) {
    throw new TypeError("Omnidex base URL must be one HTTP(S) origin or canonical base path without a trailing slash.");
  }
  validateToken(token);
}

export function validateToken(token) {
  if (typeof token !== "string" || token.length < 32 || token.length > 4096 || !/^[\x21-\x7e]+$/.test(token)) {
    throw new TypeError("Omnidex integration token must contain 32..4096 exact visible ASCII bytes.");
  }
}

export function validateCanonicalId(label, value, maximum) {
  if (typeof value !== "string" || value.length < 1 || value.length > maximum || !CANONICAL_ID.test(value)) {
    throw new TypeError(`${label} is not canonical.`);
  }
}

export function validateAuthorityId(value) {
  if (typeof value !== "string" || !AUTHORITY_ID.test(value)) {
    throw new TypeError("Delegated authority must be one opaque dba_ identity.");
  }
}

export function validatePrompt(value) {
  if (typeof value !== "string" || !value.trim() || new TextEncoder().encode(value).length > 4096 || value.includes("\0")) {
    throw new TypeError("Prompt must contain 1..4096 exact nonblank UTF-8 bytes without NUL.");
  }
}

export function validateDirectDataSource(input) {
  exactString(input?.name, "Data-source name");
  if (!Number.isSafeInteger(input.port) || input.port < 1 || input.port > 65535) {
    throw new TypeError("PostgreSQL port must be between 1 and 65535.");
  }
  if (!SSL_MODES.has(input.sslMode)) throw new TypeError(`PostgreSQL SSL mode ${JSON.stringify(input.sslMode)} is unsupported.`);
  if (input.useDsn) {
    exactString(input.dsn, "PostgreSQL DSN");
  } else {
    exactString(input.host, "PostgreSQL host");
    exactString(input.databaseName, "PostgreSQL database");
    exactString(input.username, "PostgreSQL username");
  }
}

export function validateDelegatedDataSource(input) {
  exactString(input?.name, "Data-source name");
  validateConfiguration(input.authorityUrl, "12345678901234567890123456789012");
  if (!ENVIRONMENT_NAME.test(input.credentialEnv ?? "")) {
    throw new TypeError("Credential environment variable is outside the dedicated namespace OMNIDEX_DELEGATED_AUTHORITY_*.");
  }
}

export function validateChannel(input) {
  validateCanonicalId("Channel ID", input?.id, 96);
  validateCanonicalId("Data-source ID", input?.dataSourceId, 128);
  exactString(input.name, "Channel name");
  exactString(input.workspaceRoot, "Channel workspace root");
  if (!Array.isArray(input.tags) || input.tags.length > 32) throw new TypeError("Channel tags must be an array with at most 32 entries.");
  const seen = new Set();
  for (const tag of input.tags) {
    if (typeof tag !== "string" || !tag || tag !== tag.trim() || tag !== tag.toLowerCase() || tag.length > 64 || seen.has(tag)) {
      throw new TypeError(`Channel tag ${JSON.stringify(tag)} is not canonical and unique.`);
    }
    seen.add(tag);
  }
}

function exactString(value, label) {
  if (typeof value !== "string" || !value || value !== value.trim() || value.includes("\0")) {
    throw new TypeError(`${label} must be exact nonblank text.`);
  }
}
