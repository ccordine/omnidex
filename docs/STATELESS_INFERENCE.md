# Stateless integration inference

`go run ./cmd/omni-inference` serves one explicitly configured model transport
without starting the persistent channel/workspace runtime. This executable is
for applications that supply one bounded semantic question and must not retain
conversation history. It exposes no tools, jobs, filesystem, or model-selected
actions. It does not provide the Omnidex workbench UI or channel endpoints.

Required environment:

```dotenv
OMNIDEX_INFERENCE_LISTEN=127.0.0.1:8097
OMNIDEX_INFERENCE_PROVIDER=ollama
OMNIDEX_INFERENCE_MODEL=qwen3.5:9b
OLLAMA_BASE_URL=http://localhost:11434
OMNIDEX_INTEGRATION_API_TOKEN=<a private random token of at least 32 characters>
```

The Ollama model must already be installed. Alternatively, choose a native HTTP
model service explicitly:

```dotenv
OMNIDEX_INFERENCE_PROVIDER=http
OMNIDEX_INFERENCE_MODEL=<the backend's exact model identifier>
OMNIDEX_INFERENCE_BACKEND_URL=https://model.example
OMNIDEX_INFERENCE_BACKEND_TOKEN=<the backend's private bearer token>
```

For HTTP mode, remove `OLLAMA_BASE_URL`; for Ollama mode, do not set the native
backend variables. Missing or unknown provider selection fails at startup. There
is no automatic provider fallback. Native services must use HTTPS; plain HTTP is
accepted only for loopback tests. Redirects are rejected.

The process accepts loopback or private-IP
listeners and rejects wildcard/public listeners. A private listener can sit behind
a TLS gateway; every inference request still requires the integration token.
The Worknet deployment is https://inference.omni.worknet.
Each request is authenticated using the integration token and receives an
independent bounded inference call. There is no database, transcript log,
channel, job, memory write, retained provider context, or automatic retry. The
model must complete within 45 seconds; truncated, malformed, or excessive
responses fail with an error instead of becoming semantic output.

`POST /v1/integrations/infer`, with `Authorization: Bearer <token>`:

```json
{
  "question": "The one unresolved semantic question, including its result form.",
  "context": "Only the data needed to answer that question.",
  "max_output_tokens": 256
}
```

Returns `text`, `model`, and `duration_ms`. The Go SDK exposes `Client.Infer`. The standalone service contract is
[`sdk/contract/inference.openapi.yaml`](../sdk/contract/inference.openapi.yaml).
Question and context limits are 4096 and 12000 UTF-8 bytes, output is 1–1024
tokens, and at most two requests may be in flight. Inputs are never logged;
provider errors are reduced to a content-free API error. `GET /healthz` reports
process readiness and the configured model without invoking it.

## Native HTTP provider contract

The provider is a generic transport. Model-specific interpretation of the primary
`question` input and `context` belongs to the model service and its caller, outside
Omnidex. Omnidex forwards the three request fields unchanged to `POST /infer`,
with the private backend bearer token. It neither wraps a prompt nor interprets
the input as commands, tools, or workflow controls.

The backend returns this code-produced envelope:

```json
{"text":"semantic result","model":"configured-model-id","output_tokens":4,"finish_reason":"stop"}
```

`output_tokens` must reflect the actual decoder token count within the requested
limit. `finish_reason` must be `stop`, based on the decoder's termination condition.
Missing, unknown, oversized, incomplete, or mismatched results fail without a
retry; provider content is not returned in errors. These metadata fields are
produced by provider code, not requested as model-authored JSON. Startup checks
`GET /healthz` for `status=ready` and the exact configured `model` identifier.

The caller owns validation, routing, ephemeral history, expiry, and all effects.
The caller must be able to explain the semantic uncertainty, why exact code
cannot resolve it, the minimum necessary context, the one required result, and
the deterministic consumer of that result. This API is not an agent or a
replacement for the code-owned job runtime.
