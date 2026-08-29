import assert from "node:assert/strict";
import test from "node:test";
import {
  newDelegatedAuthorityId,
  OmnidexApiError,
  OmnidexClient,
  validateDelegatedAuthorityId,
} from "../src/index.js";

const token = "integration-token-0123456789abcdef";

test("delegated registration carries no PostgreSQL credentials", async () => {
  const client = new OmnidexClient({
    baseUrl: "https://omnidex.internal", token,
    fetch: async (url, init) => {
      assert.equal(url, "https://omnidex.internal/v1/integrations/data-sources");
      assert.equal(init.headers.Authorization, `Bearer ${token}`);
      const body = JSON.parse(init.body);
      assert.deepEqual(body, {
        name: "Clinical", driver: "postgres", execution_mode: "delegated",
        host: "", port: 0, database_name: "", username: "", password: "", ssl_mode: "",
        use_dsn: false, dsn: "", authority_url: "https://application.internal",
        credential_env: "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN",
      });
      return jsonResponse(201, {
        source: {
          id: "source-1", name: "Clinical", driver: "postgres", execution_mode: "delegated",
          authority_url: "https://application.internal",
          credential_env: "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN", read_only: true,
        },
      });
    },
  });
  const source = await client.registerDelegatedDataSource({
    name: "Clinical", authorityUrl: "https://application.internal",
    credentialEnv: "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN",
  });
  assert.equal(source.id, "source-1");
});

test("message preserves exact prompt and authority", async () => {
  const authorityId = `dba_${"a".repeat(64)}`;
  const prompt = "  Find the knee collection.\nKeep context. ";
  const client = new OmnidexClient({
    baseUrl: "https://omnidex.internal", token,
    fetch: async (url, init) => {
      assert.equal(url, "https://omnidex.internal/v1/integrations/channels/clinical-chat/messages");
      assert.deepEqual(JSON.parse(init.body), {
        prompt, delegated_data_authority_id: authorityId,
      });
      return jsonResponse(202, {
        channel: { id: "clinical-chat", data_source_id: "source-1", mode: "assistant" },
        user_message: { id: 12, channel_id: "clinical-chat", role: "user", content: prompt, created_at: "2026-08-19T00:00:00Z" },
        job: { id: 73, instruction: prompt, pipeline: "chat" },
      });
    },
  });
  const result = await client.sendMessage("clinical-chat", { prompt, delegatedDataAuthorityId: authorityId });
  assert.equal(result.job.id, 73);
  assert.equal(result.user_message.content, prompt);
});

test("invalid authority fails before transport", async () => {
  let calls = 0;
  const client = new OmnidexClient({
    baseUrl: "https://omnidex.internal", token,
    fetch: async () => { calls += 1; throw new Error("must not run"); },
  });
  await assert.rejects(
    client.sendMessage("clinical-chat", { prompt: "question", delegatedDataAuthorityId: "invalid" }),
    /opaque dba_/,
  );
  assert.equal(calls, 0);
});

test("channel lookup uses the authenticated typed route", async () => {
  const client = new OmnidexClient({
    baseUrl: "https://omnidex.internal", token,
    fetch: async (url, init) => {
      assert.equal(url, "https://omnidex.internal/v1/integrations/channels/clinical-chat");
      assert.equal(init.method, "GET");
      assert.equal(init.headers.Authorization, `Bearer ${token}`);
      return jsonResponse(200, {
        channel: { id: "clinical-chat", scope: "user", data_source_id: "source-1", mode: "assistant" },
      });
    },
  });
  const channel = await client.getChannel("clinical-chat");
  assert.equal(channel.data_source_id, "source-1");
});

test("unknown responses and HTTP errors fail closed", async () => {
  const responses = [
    jsonResponse(200, { channel_id: "clinical-chat", messages: [], next_before_id: null, has_more: false, unknown: true }),
    jsonResponse(409, { error: "channel already has an active turn" }),
  ];
  const client = new OmnidexClient({
    baseUrl: "https://omnidex.internal", token, fetch: async () => responses.shift(),
  });
  await assert.rejects(client.listMessages("clinical-chat"), /unknown field/);
  await assert.rejects(
    client.sendMessage("clinical-chat", { prompt: "question" }),
    (error) => error instanceof OmnidexApiError && error.status === 409 &&
      error.apiMessage === "channel already has an active turn",
  );
});

test("configuration and generated authority are bounded", () => {
  assert.throws(() => new OmnidexClient({ baseUrl: "file:///tmp/omnidex", token }), /HTTP/);
  assert.throws(() => new OmnidexClient({ baseUrl: "https://omnidex.internal/", token }), /trailing slash/);
  assert.throws(() => new OmnidexClient({ baseUrl: "https://omnidex.internal", token: "short" }), /32/);
  validateDelegatedAuthorityId(newDelegatedAuthorityId());
});

test("delegated registration cannot select an arbitrary process secret", async () => {
  let calls = 0;
  const client = new OmnidexClient({
    baseUrl: "https://omnidex.internal", token,
    fetch: async () => { calls += 1; throw new Error("must not run"); },
  });
  await assert.rejects(client.registerDelegatedDataSource({
    name: "Clinical", authorityUrl: "https://application.internal", credentialEnv: "OPENAI_API_KEY",
  }), /dedicated namespace/);
  assert.equal(calls, 0);
});

test("response bodies are bounded while streaming without Content-Length", async () => {
  const chunk = new Uint8Array(1024 * 1024);
  let pulls = 0;
  let cancelled = false;
  const stream = new ReadableStream({
    pull(controller) {
      pulls += 1;
      controller.enqueue(chunk);
    },
    cancel() {
      cancelled = true;
    },
  });
  const client = new OmnidexClient({
    baseUrl: "https://omnidex.internal", token,
    fetch: async () => new Response(stream, { headers: { "content-type": "application/json" } }),
  });
  await assert.rejects(client.getChannel("clinical-chat"), /exceeds 16777216 bytes/);
  assert.ok(pulls >= 17 && pulls <= 18);
  assert.equal(cancelled, true);
});

function jsonResponse(status, value) {
  return new Response(JSON.stringify(value), {
    status, headers: { "content-type": "application/json" },
  });
}
