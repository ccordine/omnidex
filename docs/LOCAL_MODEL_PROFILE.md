# Local Model Profile

This profile is for the Framework 16 host used to run Omnidex. It is a
deployment choice, not model-aware application logic: every role remains an
explicit immutable route and no model is used as a fallback for another.

Parameter count is never a routing rule. Each station retains one stable semantic
contract, and deployment configuration resolves that station to one exact model only
after the candidate has demonstrated the required raw-leaf fidelity, semantic quality,
latency, and resource use. A smaller candidate that passes is valid; a larger candidate
that fails is not. Context and output numbers are operating targets and per-call
resource bounds, not arbitrary global correctness ceilings.

## Measured host boundary

The host has:

- an AMD Ryzen 9 7940HS;
- an RX 7700S with 8 GB GDDR6;
- one 48 GB DDR5-5600 SODIMM, leaving about 43 GiB visible to Linux; and
- Ollama's Vulkan backend pinned to the RX 7700S with one loaded model and one
  parallel request.

This is not a 64 GB machine. Partial offload is viable, but one SODIMM also
means system-memory-resident layers do not get the bandwidth of a populated
second channel. Framework documents two SODIMM slots and support for up to
2x48 GB on this generation:

- https://frame.work/products/ddr5-5600?v=FRANRM0003X2
- https://frame.work/laptop16?slug=laptop16-amd-7040

## Authoritative station-routed profile

Use one exact configured model per station. The active coding routes are deliberately
split between bounded semantic extraction, deployment-semantics classification, repair
guidance, and source-node generation:

```dotenv
OMNI_CODING_REQUIREMENTS_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_SERVICE_DEPLOYMENT_INTENT_MODEL=phi4:14b
OMNI_CODING_WORKLOAD_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_FRAGMENT_MODEL=qwen2.5-coder:7b
OMNI_CODING_FRAGMENT_REPAIR_GUIDANCE_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_FRAGMENT_CORRECTION_MODEL=qwen2.5-coder:7b

INFERENCE_CONTEXT_TOKENS=8192
CODING_FRAGMENT_CONCURRENCY=1
```

The complete exact station-key list is checked in to `default.env` and `.env.example`.
Bounded semantic stations, including requirement extraction and frozen workload
construction, use Qwen 3.5 9B. The target-tree station consumes that same
`OMNI_CODING_WORKLOAD_MODEL` route when a stack retains a genuine structural naming
question; current command-line and PHP stacks project their exact path grammars in
code, while TypeScript/React browser structure remains its current consumer. There
is no separate target-tree environment key. The continued-availability and conditional
persistence-destination stations share the explicit Phi-4 14B deployment-semantics
route. The established `OMNI_CODING_SERVICE_DEPLOYMENT_INTENT_MODEL` environment key
and `coding_service_deployment_intent_model` project-setting key are retained for
persisted configuration compatibility. They select only that shared provider route;
the two stations retain separate IDs, prompts, raw results, and conditional
dispatch, and the retired ternary deployment-intent station remains unavailable. The
repository and web claim-evidence review routes use `deepseek-r1:8b`; their live
provider identity must differ from the Qwen answer, synthesis, and correction routes.
Startup or the named gap fails loudly when an exact configured route is unavailable.
No route falls back to another model.

Qwen 3.5 9B is the practical bounded semantic and repair-guidance choice because its
Q4_K_M Ollama image is 6.6 GB, its live requirement/workload qualification converged,
and Qwen publishes strong instruction following, tool-use, and coding results for the
9B checkpoint. Qwen 2.5 Coder 7B owns raw fragment generation
and instruction-only repair execution because the live two-fixture compiler
qualification terminated promptly and compiled both guided edits. DeepSeek R1 8B is restricted to the two
independent evidence-review stations; it does not plan, synthesize, correct, or
select repository operations. Qwen3-Coder 30B is a 30.5B-total, 3.3B-active MoE
trained primarily on code and is non-thinking by design, which fits Omnidex's
bounded single-node output contract.

### Active local model inventory

File sizes below are the exact active Ollama tag sizes used for deployment capacity
planning. They are model-file sizes, not runner-allocation or latency measurements.

| Route | Exact model | Model file |
| --- | --- | ---: |
| Semantic stations, requirements, workload, target tree, and repair guidance | `qwen3.5:9b-q4_K_M` | 6.6 GB |
| Source generation and repair execution | `qwen2.5-coder:7b` | 4.7 GB |
| Service deployment semantics | `phi4:14b` | 9.1 GB |
| Independent repository/web evidence review | `deepseek-r1:8b` | 5.2 GB |
| Local embeddings | `nomic-embed-text` | 0.27 GB |

The active model files total about 25.9 GB (24.1 GiB). Reserve additional space for
Ollama runtime files, application workspaces, PostgreSQL, backups, logs, and Docker
build cache.

Primary model sources:

- https://huggingface.co/Qwen/Qwen3.5-9B
- https://ollama.com/library/qwen3.5/tags
- https://huggingface.co/Qwen/Qwen2.5-Coder-7B-Instruct
- https://ollama.com/library/qwen2.5-coder
- https://huggingface.co/microsoft/phi-4
- https://ollama.com/library/phi4/tags
- https://ollama.com/library/deepseek-r1
- https://ollama.com/library/nomic-embed-text
- https://huggingface.co/Qwen/Qwen3-Coder-30B-A3B-Instruct
- https://ollama.com/library/qwen3-coder

## Live requirements and workload qualification

On 2026-08-28, the checked-in
`TestLiveCodingRequirementsAndWorkloadQualification` exercised three unrelated
immutable requests through the production raw-leaf renderers: a music studio, a
catalog, and an appointment scheduler. Qwen 3.5 9B completed every requirement
fixed point and every objective, behavior, and criterion leaf in 38 calls without a
correction call. All three frozen workloads passed their code-owned validation.

Provider identity was Ollama 0.24.0, model digest
`6488c96fa5faab64bb65cbd30d4289e20e6130ef535a93ef9a49f42eda893ea7`,
quantization `Q4_K_M`, with an 8192-token context. The complete qualification took
91.55 seconds. This is route qualification for task-neutral station contracts, not
application-specific framework behavior. The checked-in test proves bounded transport,
fixed-point termination, call shape, and frozen-workload validation; it does not use
keyword matching to pretend that code can score the semantic coverage of each fixture.

## Live deployment-semantics qualification

On 2026-08-27, the checked-in opt-in qualification sent six unrelated immutable
requests through the production continued-availability renderer and, only after an
affirmative result, the separate persistence-destination renderer. The exact
`phi4:14b` route made six first-stage calls and four second-stage calls with no
correction. The second-stage portable job binds the accepted affirmative first-stage
leaf, while its model-visible prompt contains only the destination question and the
immutable request. A destination that is separate, unstated, or ambiguously named
remains unresolved and therefore grants no current-host deployment authority.

Provider identity was Ollama 0.24.0, model digest
`ac896e5b8b34a1f4efa7b14d7520725140d5512484457fab45d2a4ea14c69dba`,
quantization `Q4_K_M`, and context 8,192 tokens. The complete run passed in
108.189 seconds.

| Semantic case | Availability / destination | Prompt/output tokens by stage | Provider/wall time by stage |
| --- | --- | ---: | ---: |
| command-line behavior only | not required / skipped | 161 / 42 | 8.931 / 9.043 s |
| browser behavior only | not required / skipped | 160 / 41 | 8.500 / 8.594 s |
| explicit build-environment persistence | required / build environment | 164 / 40; 178 / 40 | 8.753 / 8.841 s; 8.569 / 8.660 s |
| explicit separate destination | required / not established | 163 / 47; 177 / 43 | 9.798 / 9.907 s; 9.011 / 9.119 s |
| persistence with destination unstated | required / not established | 159 / 43; 173 / 44 | 8.957 / 9.042 s; 9.150 / 9.235 s |
| persistence on ambiguously named server | required / not established | 160 / 39; 174 / 44 | 8.326 / 8.423 s; 9.237 / 9.337 s |

These are station-call measurements, not model prewarm memory measurements. Re-run
the exact qualification with:

```bash
OMNIDEX_TEST_OLLAMA_URL=http://127.0.0.1:11434 \
OMNIDEX_TEST_OLLAMA_CONTEXT=8192 \
OMNIDEX_TEST_CODING_SERVICE_DEPLOYMENT_INTENT_MODEL=phi4:14b \
go test ./internal/worker \
  -run '^TestLiveApplicationServiceDeploymentSemanticSplitQualification$' \
  -count=1 -v -timeout=15m
```

## Live guided-repair qualification

The checked-in opt-in qualification is an isolated framework-primitive test,
not an autonomy benchmark. On 2026-08-15 it sent three real TypeScript compiler
scenarios through the production repair-guidance renderer, the separate
instruction-only executor renderer, code-owned AST replacement, and the pinned
TypeScript compiler. The third scenario proves iterative convergence: it began
with two compiler errors, the first guided repair reduced that set to one, the
remaining compiler failure produced a new guidance/execution pair, and the
second repair compiled cleanly.

| Failure class | Guidance tokens in/out | Executor tokens in/out | Compiler result |
| --- | ---: | ---: | --- |
| nested lexical scope | 468 / 35 | 160 / 40 | 1 -> 0 |
| incompatible local value | 356 / 35 | 121 / 9 | 1 -> 0 |
| successive failures, iteration 1 | 373 / 36 | 122 / 9 | 2 -> 1 |
| successive failures, iteration 2 | 373 / 30 | 117 / 9 | 1 -> 0 |

The exact prepared requests, raw provider bodies, provider-identity evidence,
instructions, candidates, and compiler receipts are retained in
[`evidence/2026-08-15-guided-typescript-repair-iterative-live.jsonl`](evidence/2026-08-15-guided-typescript-repair-iterative-live.jsonl)
(SHA-256 `c1970966b4d47194787b528f624d602248d5653a13eb6fd64b54a3d9b799b78c`).
The synthetic frozen source points in that report were not dispatched; only the
derived guidance and execution jobs were model calls.

The qualification also rejected two prior routes loudly. `deepseek-r1:8b`
requires native thinking and did not match the tested repair-guidance transport.
The `qwen3.5:9b-q4_K_M` alias could not be attested as the
active canonical runner, and canonical Qwen 3.5 raw execution then ran for more
than six minutes without terminating a tiny edit. Neither remains a coding
repair default.

Re-run the qualification with:

```bash
OMNIDEX_TEST_OLLAMA_URL=http://127.0.0.1:11434 \
OMNIDEX_TEST_OLLAMA_CONTEXT=8192 \
OMNIDEX_TEST_TYPESCRIPT_REPAIR_GUIDANCE_MODEL=qwen3.5:9b-q4_K_M \
OMNIDEX_TEST_TYPESCRIPT_REPAIR_EXECUTOR_MODEL=qwen2.5-coder:7b \
OMNIDEX_TEST_TYPESCRIPT_REPAIR_REPORT=/tmp/guided-repair.jsonl \
go test ./internal/worker \
  -run '^TestLiveTypeScriptGuidanceExecutorCompilerConvergence$' \
  -count=1 -v -timeout=15m
```

## Browser context-relevance qualification

The browser WebGPU provider is implemented below the existing
`context_relevance` station, but remains disabled by default. Its small ten-case live
corpus and report format deliberately stop at the minimum useful qualification record:
station, exact model, corpus version/hash, pass/fail, measured latency, measured
quality, and `qualified`.

Three initial candidates completed real WebGPU inference through the production UI
bridge and server validator. All three met the supplied latency target and failed the
semantic contract, so none was promoted. The results and rerun command are in
[`BROWSER_INFERENCE.md`](BROWSER_INFERENCE.md); compact evidence is retained in
[`evidence/2026-08-20-browser-context-relevance-candidates.json`](evidence/2026-08-20-browser-context-relevance-candidates.json).

## Local measurements

Measurements use the same deterministic request shape now checked in as
`omni ollama:prewarm`: the fixed minimal prompt, thinking disabled, temperature
zero, a 64-token output ceiling, and runner inspection through Ollama `/api/ps`.
The current retained 8K baseline was measured on 2026-08-19:

| Model | Runner allocation | RX 7700S allocation | GPU offload | Cold probe | Warm probe | Warm decode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `qwen3.5:9b-q4_K_M` | 9.3 GiB | 7.0 GiB | 75% | 11.9 s | 2.4 s | 19.1 tok/s |
| `qwen2.5-coder:7b` | 5.0 GiB | 5.0 GiB | 100% | 14.2 s | 1.9 s | 48.3 tok/s |

No retained 8K prewarm allocation measurement is recorded here for
`phi4:14b` or `deepseek-r1:8b`. Their model-file sizes above and
the Phi-4 station-call measurements are not substitutes for runner-memory evidence.

The following historical context-scaling rows were recorded earlier. The measured
16K rows were verified by the command. The 2K comparison rows used equivalent direct
Ollama requests before the command was added; 2K is below Omnidex's current hard
inference-context minimum and cannot be selected through the command.

| Model | Context | Runner allocation | RX 7700S allocation | Warm evaluation |
| --- | ---: | ---: | ---: | ---: |
| `qwen2.5-coder:7b` | 2K | 4.81 GB | 4.81 GB | 51.67 tok/s |
| `qwen2.5-coder:14b` | 2K | 9.91 GB | 7.32 GB | 7.86 tok/s |
| `qwen3.5:9b-q4_K_M` | 16K | 13.82 GB | 7.53 GB | 11.10 tok/s |
| `deepseek-r1:8b` | 16K | 13.82 GB peak in paired trial | recorded in trial evidence | independent evidence review |
| `qwen3-coder:30b` | 2K | 18.98 GB | 7.56 GB | 14.85 tok/s |
| `qwen3-coder:30b` | 16K | 22.41 GB | 7.28 GB | 13.12 tok/s |
| `qwen3-coder:30b` | 32K | 26.24 GB | 7.28 GB | 11.06 tok/s |

The 30B MoE is almost twice as fast as the old 14B dense correction model on
this host despite the larger total checkpoint. At 32K it also pushed an
already busy host deep into swap. The observed semantic envelopes and the
fragment prompt/output limits also fit inside the exact 8K minimum, so the
checked-in local profile uses 8K rather than paying the 16K/32K allocation and
latency cost. Omnidex does not interpret prompt bytes as tokens: the exact
request disables provider truncation, declares the native input/output ceilings,
and validates Ollama's returned native token counts. Byte limits are only
coarse transport and resource-safety bounds.

Run the same exact load check after any model, context, backend, or memory change:

```bash
omni ollama:prewarm --model qwen3.5:9b-q4_K_M --num-ctx 8192 --json
omni ollama:prewarm --model qwen2.5-coder:7b --num-ctx 8192 --json
omni ollama:prewarm --model phi4:14b --num-ctx 8192 --json
omni ollama:prewarm --model deepseek-r1:8b --num-ctx 8192 --json
```

## Other genuinely viable choices

| Choice | Status on this host | Reason |
| --- | --- | --- |
| Qwen 3.6 35B-A3B | On-demand only after freeing disk and memory headroom | The official Q4_K_M GGUF is 20.4 GB and the Ollama Q4 image is about 24 GB. It should run with partial offload, but it leaves too little headroom on the current busy 43 GiB host to make it the production default. |
| Qwen 3.6 27B | Not recommended locally | The Ollama Q4 image is about 17 GB, but all 27B parameters are active. Qwen's coding results are excellent, while dense system-memory inference is the wrong latency tradeoff for this single-DIMM machine. |
| Qwen3-Coder-Next | Does not fit | Ollama's Q4 image is about 52 GB before context and runtime allocations, which exceeds usable physical memory. |
| DeepSeek R1 Distill Qwen 14B | Technically viable, not preferred | It has the same dense-memory bottleneck measured on the old 14B model, and a reasoning-first response style is a poor match for tiny exact raw-leaf stations with thinking disabled. |

Sources for the alternatives:

- https://github.com/QwenLM/Qwen3.6
- https://huggingface.co/Qwen/Qwen3.6-27B
- https://huggingface.co/ggml-org/Qwen3.6-35B-A3B-GGUF
- https://ollama.com/library/qwen3.6/tags
- https://ollama.com/library/qwen3-coder-next
- https://github.com/ggml-org/llama.cpp

If the machine receives a second 48 GB SODIMM, retest Qwen 3.6 35B-A3B at 16K.
Do not change the production route based on parameter counts or published
benchmarks alone; compare cold load, warm throughput, exact raw-leaf acceptance,
correction rate, and a fresh uncontaminated Omnidex run.

## Hosted capability ceiling

Hosted generation is intentionally not a production station transport. The
known-provider registry preserves identities and rejected environment keys, and
the production catalog exposes a hosted provider only for a consumed embedding
transport. Supporting hosted station inference later requires a provider-specific
exact prepared contract with the same immutable request and evidence guarantees;
generic chat-completions compatibility is insufficient. Current external model
documentation may inform that future isolated integration work:

- https://qwenlm.github.io/qwen-code-docs/en/users/configuration/model-providers/
- https://api-docs.deepseek.com/quick_start/pricing
- https://docs.z.ai/guides/develop/http/introduction

No hosted route was activated or claimed as validated here because no provider
credentials were supplied. A local throughput probe is also not an autonomy
result: capability claims require a fresh ordinary request through the
production boundary and the complete uncontaminated evidence required by the
assembly-line architecture.
