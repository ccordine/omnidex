import {
  validateAuthorityId,
  validateCanonicalId,
  validateChannel,
  validateConfiguration,
  validateDelegatedDataSource,
  validateDirectDataSource,
  validatePrompt,
} from "./validation.js";
import {
  apiError,
  channelResponse,
  dataSourceResponse,
  jobDetailsResponse,
  messagePageResponse,
  messageResponse,
  OmnidexApiError,
} from "./responses.js";

const MAX_RESPONSE_BYTES = 16 * 1024 * 1024;

export { OmnidexApiError };
export { validateAuthorityId as validateDelegatedAuthorityId };

export function newDelegatedAuthorityId() {
  if (!globalThis.crypto?.getRandomValues) throw new Error("Secure random generation is unavailable.");
  const bytes = new Uint8Array(32);
  globalThis.crypto.getRandomValues(bytes);
  return `dba_${Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("")}`;
}

export class OmnidexClient {
  constructor({ baseUrl, token, timeoutMs = 30_000, fetch: fetchImplementation = globalThis.fetch }) {
    validateConfiguration(baseUrl, token);
    if (!Number.isSafeInteger(timeoutMs) || timeoutMs < 1) throw new TypeError("timeoutMs must be a positive integer.");
    if (typeof fetchImplementation !== "function") throw new TypeError("A fetch implementation is required.");
    this.baseUrl = baseUrl;
    this.token = token;
    this.timeoutMs = timeoutMs;
    this.fetch = fetchImplementation;
  }

  async registerDirectDataSource(input, options = {}) {
    validateDirectDataSource(input);
    return dataSourceResponse(await this.request("/v1/integrations/data-sources", {
      method: "POST", signal: options.signal, expectedStatus: 201,
      body: dataSourceBody({
        name: input.name, executionMode: "direct", host: input.host, port: input.port,
        databaseName: input.databaseName, username: input.username, password: input.password ?? "",
        sslMode: input.sslMode, useDsn: input.useDsn, dsn: input.dsn ?? "",
      }),
    }));
  }

  async registerDelegatedDataSource(input, options = {}) {
    validateDelegatedDataSource(input);
    return dataSourceResponse(await this.request("/v1/integrations/data-sources", {
      method: "POST", signal: options.signal, expectedStatus: 201,
      body: dataSourceBody({
        name: input.name, executionMode: "delegated", authorityUrl: input.authorityUrl,
        credentialEnv: input.credentialEnv,
      }),
    }));
  }

  async createChannel(input, options = {}) {
    validateChannel(input);
    const value = await this.request("/v1/integrations/channels", {
      method: "POST", signal: options.signal, expectedStatus: 201,
      body: {
        id: input.id, name: input.name, tags: input.tags, workspace_root: input.workspaceRoot,
        data_source_id: input.dataSourceId, mode: "assistant",
      },
    });
    return channelResponse(value, input.id, input.dataSourceId);
  }

  async getChannel(channelId, options = {}) {
    validateCanonicalId("Channel ID", channelId, 96);
    const value = await this.request(`/v1/integrations/channels/${channelId}`, {
      method: "GET", signal: options.signal, expectedStatus: 200,
    });
    return channelResponse(value, channelId);
  }

  async sendMessage(channelId, input, options = {}) {
    validateCanonicalId("Channel ID", channelId, 96);
    validatePrompt(input?.prompt);
    const body = { prompt: input.prompt };
    if (input.delegatedDataAuthorityId !== undefined) {
      validateAuthorityId(input.delegatedDataAuthorityId);
      body.delegated_data_authority_id = input.delegatedDataAuthorityId;
    }
    const value = await this.request(`/v1/integrations/channels/${channelId}/messages`, {
      method: "POST", signal: options.signal, expectedStatus: 202, body,
    });
    return messageResponse(value, channelId, input.prompt);
  }

  async listMessages(channelId, { limit = 24, beforeId, signal } = {}) {
    validateCanonicalId("Channel ID", channelId, 96);
    if (!Number.isSafeInteger(limit) || limit < 1 || limit > 200 ||
      (beforeId !== undefined && (!Number.isSafeInteger(beforeId) || beforeId < 1))) {
      throw new TypeError("Message page bounds are invalid.");
    }
    const query = new URLSearchParams({ limit: String(limit) });
    if (beforeId !== undefined) query.set("before_id", String(beforeId));
    const value = await this.request(`/v1/integrations/channels/${channelId}/messages?${query}`, {
      method: "GET", signal, expectedStatus: 200,
    });
    return messagePageResponse(value, channelId);
  }

  async getJob(jobId, options = {}) {
    if (!Number.isSafeInteger(jobId) || jobId < 1) throw new TypeError("Job ID must be positive.");
    const value = await this.request(`/v1/integrations/jobs/${jobId}`, {
      method: "GET", signal: options.signal, expectedStatus: 200,
    });
    return jobDetailsResponse(value, jobId);
  }

  async request(path, { method, body, signal, expectedStatus }) {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(new Error("Omnidex request timed out.")), this.timeoutMs);
    const abort = () => controller.abort(signal.reason);
    signal?.addEventListener("abort", abort, { once: true });
    try {
      const response = await this.fetch(this.baseUrl + path, {
        method, redirect: "error", signal: controller.signal,
        headers: {
          Authorization: `Bearer ${this.token}`, Accept: "application/json",
          ...(body === undefined ? {} : { "Content-Type": "application/json" }),
        },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
      const raw = await readBoundedResponse(response);
      let value;
      try { value = JSON.parse(raw); } catch { throw new TypeError("Omnidex returned invalid JSON."); }
      if (response.status !== expectedStatus) throw apiError(response.status, value);
      if (response.headers.get("content-type")?.split(";", 1)[0].trim().toLowerCase() !== "application/json") {
        throw new TypeError("Omnidex returned a non-JSON response.");
      }
      return value;
    } finally {
      clearTimeout(timeout);
      signal?.removeEventListener("abort", abort);
    }
  }
}

async function readBoundedResponse(response) {
  const declaredLength = response.headers.get("content-length");
  if (declaredLength !== null) {
    if (!/^(0|[1-9][0-9]*)$/.test(declaredLength)) {
      throw new TypeError("Omnidex returned an invalid Content-Length header.");
    }
    const length = Number(declaredLength);
    if (!Number.isSafeInteger(length) || length > MAX_RESPONSE_BYTES) {
      throw new Error(`Omnidex response exceeds ${MAX_RESPONSE_BYTES} bytes.`);
    }
  }
  if (response.body === null) return "";
  const reader = response.body.getReader();
  const decoder = new TextDecoder("utf-8", { fatal: true });
  const parts = [];
  let total = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      if (!(value instanceof Uint8Array)) throw new TypeError("Omnidex returned an invalid response stream.");
      total += value.byteLength;
      if (total > MAX_RESPONSE_BYTES) {
        await reader.cancel();
        throw new Error(`Omnidex response exceeds ${MAX_RESPONSE_BYTES} bytes.`);
      }
      parts.push(decoder.decode(value, { stream: true }));
    }
    parts.push(decoder.decode());
    return parts.join("");
  } finally {
    reader.releaseLock();
  }
}

function dataSourceBody(input) {
  const direct = input.executionMode === "direct";
  return {
    name: input.name, driver: "postgres", execution_mode: input.executionMode,
    host: direct ? input.host ?? "" : "", port: direct ? input.port : 0,
    database_name: direct ? input.databaseName ?? "" : "", username: direct ? input.username ?? "" : "",
    password: direct ? input.password ?? "" : "", ssl_mode: direct ? input.sslMode ?? "" : "",
    use_dsn: direct ? input.useDsn : false, dsn: direct ? input.dsn ?? "" : "",
    authority_url: direct ? "" : input.authorityUrl, credential_env: direct ? "" : input.credentialEnv,
  };
}
