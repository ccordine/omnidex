# Browser inference provider

Status: the first `context_relevance` vertical is implemented and opt-in. The
browser transport works end to end, but no tested browser model is currently
qualified for production routing.

## Boundary

The browser is an inference coprocessor, not another Omnidex runtime:

```text
server-owned objective, state, retrieval, provenance, and station
                              ↓
                 one exact station packet
                              ↓
       browser Web Worker + WebLLM + WebGPU + OPFS
                              ↓
                   raw semantic result
                              ↓
          server-owned decode and validation
```

The browser receives the existing `context_relevance` prompt, code-owned raw
response transport, opaque job identity, exact configured model, and output limit. It receives no
tools, database authority, memory store, workflow, queue, objective state, or
provenance fields. It cannot update authoritative state. The same server decoder
that validates a server-provider result validates the browser result again.

Provider choice sits below the station contract. There is no
`browser_context_relevance` station. The configured route remains:

```text
context_relevance station contract
                ↓
explicit provider + exact configured model
```

There is no provider fallback. When browser execution is configured, a missing
browser, disconnect, model error, malformed result, unknown candidate ID, or
model mismatch fails that invocation explicitly.

## Runtime

Server execution remains the default:

```dotenv
OMNI_CONTEXT_RELEVANCE_PROVIDER=server
```

The experimental browser route is enabled only by both exact values:

```dotenv
OMNI_CONTEXT_RELEVANCE_PROVIDER=browser_webgpu
OMNI_CONTEXT_RELEVANCE_MODEL=<qualified WebLLM model ID>
```

After the UI loads, the body-scoped Stimulus bridge checks WebGPU support,
loads the exact registered WebLLM model in one dedicated Web Worker, caches its
artifacts in OPFS, and opens the same-origin WebSocket only after the model is
resident. The engine remains warm and resets chat state before every isolated
station call. Only one browser session can register with the current broker.

WebLLM seeds one exact empty `<think>` block when thinking is disabled. The
browser adapter removes only that documented provider envelope before sending
semantic bytes to the server. Non-empty reasoning, malformed tags, and invalid
raw semantic leaves are not repaired or hidden; server validation rejects them.

## Qualification

Model size is not routing authority. A candidate is qualified only for this
station contract, corpus version, browser/runtime, and explicit latency target.
The checked-in corpus has ten focused cases covering empty selection, negation,
numbers, dates, relationship direction, accepted versus rejected state,
fiction versus reality, source qualification, elliptical reference, and
multiple relevant facts.

The opt-in live test launches the real embedded UI in Chromium, waits for the
WebGPU model to connect, sends every case through the production broker and
station renderer, and writes one report containing:

- station and exact model;
- browser and corpus version/hash;
- pass/fail;
- measured median, maximum, and total latency;
- exact-match rate, micro-precision, and micro-recall; and
- the final `qualified` value.

Qualification requires every case to return the exact opaque ID set and the
measured median to meet the operator-supplied target. The profile directory is
explicit and reusable so OPFS caching is tested rather than discarded between
runs.

```bash
cd internal/api/web
npm run build
cd ../../..

OMNIDEX_TEST_BROWSER_CONTEXT_QUALIFICATION=1 \
OMNIDEX_TEST_BROWSER_CONTEXT_MODEL=<candidate WebLLM model ID> \
OMNIDEX_TEST_BROWSER_CONTEXT_REPORT=/tmp/context-relevance-report.json \
OMNIDEX_TEST_BROWSER_CONTEXT_PROFILE=/tmp/omnidex-browser-profile \
OMNIDEX_TEST_CHROMIUM_PATH=/usr/bin/chromium \
OMNIDEX_TEST_BROWSER_CONTEXT_MAX_MEDIAN_MS=10000 \
go test ./internal/api \
  -run '^TestLiveBrowserContextRelevanceQualification$' \
  -count=1 -v -timeout=35m
```

The initial 2026-08-20 candidates all failed the unchanged semantic contract:

| Model | Exact cases | Median | Precision | Recall | Qualified |
| --- | ---: | ---: | ---: | ---: | --- |
| `Qwen3.5-0.8B-q4f16_1-MLC` | 8/10 | 664 ms | 0.750 | 0.900 | no |
| `Qwen3.5-2B-q4f16_1-MLC` | 7/10 | 968 ms | 0.875 | 0.700 | no |
| `Qwen2.5-1.5B-Instruct-q4f16_1-MLC` | 7/10 | 688 ms | 0.727 | 0.800 | no |

The compact evidence is retained in
[`evidence/2026-08-20-browser-context-relevance-candidates.json`](evidence/2026-08-20-browser-context-relevance-candidates.json).
Its SHA-256 is `850b2604acc7d8966fbf1b634fa4638bf05dc992efabc5f56170da0c892df420`.
Do not enable one of these rejected profiles merely because its latency is low.
