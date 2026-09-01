# Omnidex

**Current development release:** `v0.5.0` Charmeleon

Omnidex is a local-first AI workbench with a conversational surface and a server-authoritative execution core. Charmeleon extends the deterministic assembly line with repository intelligence, software-defined task context, and a domain-neutral cognition runtime for bounded work across long-lived environments.

Charmander established the bounded assembly-line foundation; its captured measurements and limitations remain in [docs/CHARMANDER_PROOF.md](docs/CHARMANDER_PROOF.md). Charmeleon is now the active development milestone. Its context architecture is in [docs/CHARMELEON_CONTEXT_SYSTEM.md](docs/CHARMELEON_CONTEXT_SYSTEM.md), its production cognition contract is in [docs/CHARMELEON_COGNITION_RUNTIME.md](docs/CHARMELEON_COGNITION_RUNTIME.md), and its code-owned prerequisite and narrow-inference boundary is in [docs/CHARMELEON_COGNITION_RESOLUTION.md](docs/CHARMELEON_COGNITION_RESOLUTION.md).

## Charmeleon in one sentence

Charmander made individual model jobs reliable. Charmeleon makes those bounded jobs cooperate through code-owned continuity, attention, action, evidence, revision, and completion across environments too large or long-lived for any one model context.

```text
conversation
    ↓
context facts when needed                code-owned acquisition + bounded semantic leaves
    ↓
untrusted requirement inventory          exact absence or 1..code-owned maximum
    ↓
code-owned authorization-first sieve      one candidate relation per call
    ↓ at least one accepted leaf
product / surface / deployment semantics  only at each first consumer
    ↓
one frozen task per accepted task-local requirement code-owned
    ↓
stack, target tree, workload + block graph code-owned
    ↓
static declarations + bounded source bodies code + source model(s)
    ↓
AST assembly + isolated full tests        code-owned
    ↓
authoritative writes + exact verification code-owned
    ↑ exact code-routed body defect
same persisted body job/model context
```

## What changed after Venusaur

| Venusaur-era failure | Charmander contract |
| --- | --- |
| One model carried project scope, workflow control, code, checking, and recovery. | Each model receives one small typed responsibility. |
| Raw memories and old work could redirect the active task. | The coding semantic model receives only current direct authority and ordered user feedback; memory is absent. |
| Correction arrived late or restarted the build. | Accepted blocks remain in memory and only one declared generated owner can be corrected. |
| Models chose commands, declared success, and confused advice with execution. | Code owns tools, mutation authority, verification, and completion. |
| Intermediate files were rejected because unfinished siblings did not compile. | Dependency waves complete before the whole staged program is compiled or tested. |
| Source workers received paths, trees, plans, and excessive project context. | One source job receives one signature for scope, one local behavior, direct declarations, and allowed symbols, then returns only an ordinary implementation body. A specific defect continues that same persisted job and model context. |
| Hidden ledgers and duplicate recovery systems accumulated stale state. | Git is the source history; Omnidex keeps one current workspace and one coding engine. |
| Long generic status streams obscured real progress. | Live events report phase, active station, target, accepted diff, diagnostic, and terminal outcome. |

## Coding assembly line

The runtime has deliberately unequal stations:

1. **Semantic intake (models, one responsibility at a time)** — after code resolves needed context facts, one bounded requirement-inventory call returns exactly `NO_RUNTIME_REQUIREMENT_CANDIDATES` or between one and the code-owned maximum positive atomic runtime-candidate lines. Every positive candidate then receives only its independently necessary candidate-bound relations. No call emits an authoritative application contract, plan, workflow decision, or completion state.
2. **Intent and workload compiler (code)** — parses and counts inventory lines mechanically; no semantic station pre-counts the inventory, and no pre-count receipt exists. The inventory is untrusted intake, not a completeness claim. Code owns its source-ordered queue; sends every positive candidate through authorization first; discards exact repeats, unrequested candidates, non-runtime candidates, and semantic duplicates; partitions only an authorized compound candidate; and projects each surviving task-local runtime outcome directly into one frozen task. Rejected candidates cannot block or reopen accepted state. Queue exhaustion freezes the current iteration without a completeness review. Optional rejected or speculative suggestions remain outside current authority.
3. **Consumer semantics, stack, tree, and graph compilers** — only after at least one requirement survives does code resolve product identity, delivery surface, deployment semantics, or another semantic leaf at its first actual consumer. Code then selects a registered stack, derives every mechanical target, parses the optional target-tree station's complete raw basename hierarchy, constructs all paths, and compiles exact block ownership and direct capabilities.
4. **Source-body function (model, only when required)** — returns ordinary plain text containing only one implementation body. It is never required to reproduce a signature, parameter list, schema, JSON packet, control label, AST, path, or framework grammar, and it never sees a document, project plan, queue, or filesystem operation.
5. **Stager and correction controller (code)** — place the body inside the code-owned declaration, parse and validate it, stitch and format complete documents, and run isolated verification. A specific body defect continues the same persisted job with the same model for at most three total attempts; every other accepted block remains untouched.
6. **Mutation, verification, and completion controllers (code)** — write only a verified staged program, emit reviewable diffs, verify exact workspace content, rerun fixed checks, and declare the outcome.

The detailed contract lives in [internal/worker/RUNTIME.md](internal/worker/RUNTIME.md).

Completion is iteration-scoped: Omnidex must produce and verify a functional result for
the currently retained objective, not predict every feature the user may eventually
want. Rejected or speculative candidates do not enter the workload. If they are retained
as optional follow-up suggestions, they remain non-authoritative until a later explicit
user turn sends them through the ordinary candidate sieve.

## Reliability rules

- One authoritative implementation; no legacy fallback engine.
- Explicit errors for invalid provider configuration, schemas, paths, mutations, and completion claims.
- Git and the active worktree remain source authority. PostgreSQL repository snapshots are immutable, disposable, hash-bound derived facts—not alternate source history.
- The Task Ledger records typed job execution state only; it never versions or overrides source code.
- No rollback of valid file work because a later file fails.
- No deletion unless the current instruction authorizes deletion.
- No modification of protected instruction files.
- One bounded raw semantic result per inventory, classification, relation, selection, or value question; code alone assembles typed state. A coding model returns one ordinary plain-text implementation body.
- Closed choices are resolved by literal cardinality: zero options follow the station's explicit zero-option rule, one option is used immediately by code with no model call and no rejection, and only two or more options permit one opaque-ID selection call.
- Finite subset choices repeat that operation over a code-reduced candidate set: each accepted candidate is persisted and removed, each new round remaps opaque letters over only the remaining candidates plus a no-additional-choice alternative, and duplicate selection is mechanically impossible.
- A specific source-body defect continues the same persisted job and model context for at most three total attempts; unrelated accepted work is never reopened.
- Verification commands are selected from the accepted typed program, never inferred from prose or workspace guesses.
- Direct, exact diagnostic feedback reaches the next worker immediately.
- Source correction has no separate route or response protocol. Code retains the rejected body as evidence but exposes only the exact code-proven mutable span and one necessary semantic question to the same job/model continuation. The returned ordinary text replaces only that span by deterministic splice.

## Live operator experience

The CLI and web cockpit expose the same concise execution truth:

- interpreting, assembling, staging, constructing, verifying, completed, or failed phase;
- active semantic/fragment role and current semantic block;
- periodic alive heartbeat for slow local inference;
- accepted writes and reviewable diffs;
- exact rejected candidate or command diagnostic;
- queue position and final state.

Repeated polling state is coalesced. Full prompts and context remain available through explicit verbose/debug views instead of flooding the normal progress stream.

## Models

Ollama is the recommended explicit local provider. Omnidex has no implicit model
provider: deterministic work starts with both provider selections absent, and a
persisted named semantic or embedding need fails explicitly if its required
authority is still unconfigured. Bounded station routes are explicitly
configurable without giving any model control-plane authority:

```dotenv
LLM_PROVIDER=ollama
OMNI_CODING_SURFACE_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_REQUIREMENTS_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_REQUIREMENT_RESULT_RELATION_MODEL=phi4:14b
OMNI_CODING_SERVICE_DEPLOYMENT_INTENT_MODEL=phi4:14b
OMNI_CODING_WORKLOAD_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_ARTIFACT_HANDLING_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_CAPABILITY_RELATION_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_SKILL_SELECTION_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_FRAGMENT_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_REPOSITORY_CHANGE_SURFACE_MODEL=qwen3.5:9b-q4_K_M
INFERENCE_CONTEXT_TOKENS=8192
CODING_FRAGMENT_CONCURRENCY=1
```

The established `OMNI_CODING_SERVICE_DEPLOYMENT_INTENT_MODEL` environment key
and `coding_service_deployment_intent_model` project-setting key are retained so
persisted configuration remains readable. They select one shared provider model
for the independent continued-availability and persistence-destination stations;
they do not restore the retired ternary deployment-intent station or combine the
two semantic responsibilities.

`context_relevance` executes only through its durable server-side exact-station
route. The retired browser/WebGPU provider key is rejected explicitly; the browser
has no inference transport, model lifecycle, or semantic-result channel.

### Deployment sizing

The checked-in profile is Qwen-led, but it is not a single-model deployment.
The active routes use Phi-4 14B only for the two candidate-bound coding
result-presence questions and the two service deployment semantics. Qwen 3.5 9B owns
the requirement inventory, authorization-first sieve, product and stack leaves, other
bounded semantic work, target-tree naming, and ordinary source-body generation and
same-context correction, while code alone constructs the workload. Exact live qualification
admits each model only to that bounded responsibility: Phi satisfies the named
operation/property result-relation boundary that Qwen does not, while Qwen satisfies
the strict multiline inventory and authorization boundary that Phi does not. The system
uses `nomic-embed-text` for local embeddings.
The complete route list is in
[`default.env`](default.env).

The following values are capacity-planning estimates, not universal performance
guarantees. Model-file sizes come from the exact local Ollama inventory and agree
with the current Ollama library entries. Runtime allocation and historical
generation-timing rows were measured on 2026-08-19 at the exact production
8,192-token context. The current `omni ollama:prewarm` command reproduces only
the mechanical model-load, context, memory, and offload checks; governed station
evidence owns current generation timing.

#### Model working set

| Route | Current model | Local model file | Runtime role |
| --- | --- | ---: | --- |
| Requirement inventory, sieve, product, and stack stations | `qwen3.5:9b-q4_K_M` | shared 6.6 GB image | One raw semantic result |
| Requirement result-presence station | `phi4:14b` | shared 9.1 GB image | One opaque candidate ID |
| Other semantic leaf, target-tree, database, and answer stations | `qwen3.5:9b-q4_K_M` | shared 6.6 GB image | One raw semantic result |
| Source-body generation and same-context correction | `qwen3.5:9b-q4_K_M` | shared 6.6 GB image | One ordinary plain-text implementation body |
| Service deployment semantics | `phi4:14b` | 9.1 GB | Conditional availability and destination leaves |
| Embeddings | `nomic-embed-text` | 0.27 GB | Retrieval vectors |

The exact set occupies about **16.0 GB (14.9 GiB) on disk**. Reserve at least
50 GB of fast local storage for the model set and runtime files. A practical
all-in-one host should have 100 GB free before adding workspace checkouts,
PostgreSQL growth, backups, logs, or Docker build cache.

Qwen 3.5 9B advertises a much larger native context, but Omnidex deliberately
uses 8,192 tokens. Its station envelopes fit that bound, and larger contexts
increase KV-cache memory and latency without making code-owned context selection
less necessary. Ollama also multiplies context memory by `OLLAMA_NUM_PARALLEL`,
so context and concurrency must be sized together.

#### Measured 8K baseline

The current measurement host is an 8-core/16-thread Ryzen 9 7940HS with 43 GiB
usable system RAM and an 8 GB Radeon RX 7700S using Ollama's Vulkan backend. One
model and one request were allowed at a time.

| Model | Runner allocation | GPU allocation | GPU offload | Cold probe | Warm probe | Warm decode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `qwen3.5:9b-q4_K_M` | 9.3 GiB | 7.0 GiB | 75% | 11.9 s | 2.4 s | 19.1 tok/s |
| `qwen2.5-coder:7b` | 5.0 GiB | 5.0 GiB | 100% | 14.2 s | 1.9 s | 48.3 tok/s |

The probe is intentionally tiny, so its token rate is directional rather than a
full workload benchmark. Cold time includes loading the model; warm time is the
same bounded request with the runner already resident. The 9B model works on an
8 GB GPU through partial offload. A 12–16 GB GPU should hold that one runner
entirely, while 24 GB gives useful headroom to retain both Qwen runners and avoid
many model swaps. Detailed 2K/16K/32K measurements and the verified AMD backend
configuration are in [`docs/LOCAL_MODEL_PROFILE.md`](docs/LOCAL_MODEL_PROFILE.md)
and [`docs/OLLAMA_STABILITY.md`](docs/OLLAMA_STABILITY.md).

#### Minimum and recommended hosts

| Deployment | CPU | System RAM | GPU | Fast free storage | Intended load |
| --- | --- | ---: | ---: | ---: | --- |
| Control plane with remote Ollama | 4 vCPU | 8–16 GB | None | 30 GB plus PostgreSQL/workspace data | API, workers, PostgreSQL, and Redis only |
| CPU-only evaluation | 8 modern physical cores with AVX2 | 32 GB minimum | None | 50 GB | One queued turn; batch use, not an interactive latency target |
| Proven low-cost all-in-one | 8 modern cores | 32–48 GB | 8 GB VRAM | 100 GB | One active inference stream with partial 9B offload |
| Recommended interactive host | 8–16 modern cores | 64 GB | 16 GB VRAM | 100 GB | One fully offloaded model at a time; individual or small-team pilot |
| Recommended small-team GPU host | 12–24 modern cores | 64–128 GB | 24 GB VRAM | 150 GB | Two resident Qwen runners or queued multi-user work |
| Higher-throughput host | 16+ modern cores | 128 GB | 48 GB VRAM | 200 GB | Multiple resident routes and measured parallelism above one |

A GPU is therefore **not required for correctness**. CPU-only inference can run
the Q4 models if sufficient RAM is available, but multi-stage cognitive work
contains several sequential model calls. Without acceptable CPU prewarm results,
it should be treated as asynchronous batch processing. For a new Linux GPU
server, a supported NVIDIA CUDA GPU is the lowest-friction operational choice.
AMD ROCm or Vulkan can work, but the current host required explicit device
selection and Vulkan to avoid the recorded ROCm instability.

The VRAM tier matters more than raw model parameter count:

- 8 GB is viable because the 9B runner partially offloads to system RAM.
- 16 GB is the sensible floor for consistently interactive single-model use.
- 24 GB is the preferred value for a small team because it leaves useful
  headroom for route transitions, scheduler state, and driver allocation.
- 48 GB is the safer target before enabling several loaded models or parallel
  contexts on one Ollama server.

Do not enable parallel inference merely because multiple workers exist. The
default `WORKER_COUNT=3` permits code-owned jobs to progress and queue; it does
not make three model contexts free. Start with:

```ini
OLLAMA_NUM_PARALLEL=1
OLLAMA_MAX_LOADED_MODELS=1
OLLAMA_KEEP_ALIVE=5m
```

Keep `INFERENCE_CONTEXT_TOKENS=8192` and `CODING_FRAGMENT_CONCURRENCY=1`. On a
24 GB or larger GPU, test `OLLAMA_MAX_LOADED_MODELS=2` to retain the Qwen and
Phi-4 runners. Increase `OLLAMA_NUM_PARALLEL` only after the exact 8K prewarm and a
two-request load test prove the resulting allocation, latency, and stability.
Production station requests carry their own five-minute keep-alive; a server
default does not override that request-specific value.

#### Expected latency and workload

Use the measured probe instead of estimating from model-file size:

```text
turn time ≈ cold model loads + prompt evaluation + output tokens / decode rate
            + database, repository, compiler, and test time
```

On the measured 8 GB GPU, 100 output tokens take roughly 5 seconds of Qwen 3.5
decode after prompt evaluation. The retained Qwen 2.5 Coder row below is a
historical comparison, not an active route. Historical cold probes added roughly
12–14 seconds. Larger prompts,
additional bounded semantic leaves, and exact-span compiler-validation corrections add time
even when their output is short.

These ranges are deployment-planning estimates for one active inference stream:

| Workload | Warm GPU planning range | Main variables |
| --- | ---: | --- |
| Short conversational turn | 5–30 s | Context selection, answer length, memory retrieval |
| Simple database-RAG turn | 20–90 s | Relation count, schema-selection chunks, query-intent leaves, answer length |
| Small bounded coding change | 5–30 min | Requirements, generated bodies, model loads, compiler/test runs, exact-span corrections |
| Multi-feature repository build | Tens of minutes to hours | Task count, dependency waves, corrections, project build/test cost |

A simple database turn normally needs schema selection, typed query-intent
construction, one code-owned bounded query execution, and final grounded synthesis.
The validated execution evidence completes acquisition; it is not reopened by a
post-query completeness or refinement model. Schemas over 24 relations require
additional bounded selection calls, and more complex typed intents may require more
intent-leaf calls. PostgreSQL execution itself is bounded and is usually not the
dominant latency unless the application authority or database is remote or slow.

CPU-only timing is intentionally not guessed. Run the same prewarm command on the
candidate CPU host and substitute its reported decode rate into the formula. For
example, a measured 4 tok/s means 100 output tokens require about 25 seconds of
decode for each sequential call, before prompt evaluation or model loading.

With `OLLAMA_NUM_PARALLEL=1`, simultaneous user turns queue and tail latency grows
approximately with the number of active model calls. If interactive latency for
several users matters, prefer more VRAM and measured parallelism rather than
raising `WORKER_COUNT` alone.

#### Combined or split deployment

An all-in-one host is simplest for one user: Docker runs core, PostgreSQL, and
Redis while Ollama runs on the host GPU. For a server deployment, separating the
control plane and inference host is usually cleaner:

```text
application/API clients -> Omnidex core -> private Ollama GPU host
                                  |-----> PostgreSQL + Redis
```

Set the core deployment to the one exact private endpoint:

```dotenv
OLLAMA_BASE_URL=http://ollama.internal:11434
```

Configure Ollama to listen on the private interface, permit only the Omnidex core
host through the firewall, and do not publish port 11434 to the Internet. For PHI,
keep prompts, evidence, Ollama traffic, PostgreSQL, Redis, logs, backups, and TLS
termination inside the governed environment. Use a private encrypted network or
a private TLS proxy when Ollama is on another physical server.

The split control-plane row above does not include application database capacity.
If PostgreSQL is colocated, add the actual dataset, index, WAL, backup, and growth
budget separately. Delegated database mode leaves the protected application data
in its owning application and sends Omnidex only permission-filtered bounded
evidence.

#### Deployment acceptance check

Pull the exact configured models, then profile each routed generation model on
the candidate host:

```bash
ollama pull qwen3.5:9b-q4_K_M
ollama pull phi4:14b
ollama pull nomic-embed-text

omni ollama:prewarm --model qwen3.5:9b-q4_K_M --num-ctx 8192 --json
omni ollama:prewarm --model phi4:14b --num-ctx 8192 --json
```

Accept a host only after the probe reports the exact 8,192-token context, memory
fits without sustained swap, the intended GPU is used, and repeated loads
produce no runner restart or GPU error. Then run one ordinary
chat/database turn and one representative coding job while recording queue time,
model loads, tokens per second, peak RAM/VRAM, and end-to-end duration. Published
parameter counts and context windows do not replace that application-level check.

Upstream capacity references: the
[Qwen 3.5 9B model card](https://huggingface.co/Qwen/Qwen3.5-9B),
[Ollama Qwen 3.5 tags](https://ollama.com/library/qwen3.5/tags),
[Ollama Qwen 2.5 Coder tags](https://ollama.com/library/qwen2.5-coder/tags),
[Ollama Llama 3.2 tags](https://ollama.com/library/llama3.2/tags),
[Microsoft Phi-4 model card](https://huggingface.co/microsoft/phi-4),
[Ollama Phi-4 tags](https://ollama.com/library/phi4/tags),
[Ollama nomic-embed-text](https://ollama.com/library/nomic-embed-text),
[Ollama concurrency and memory FAQ](https://docs.ollama.com/faq), and
[Ollama GPU support matrix](https://docs.ollama.com/gpu).

Before intent interpretation, code hashes the immutable request and records the exact
workspace state as a typed fact. Repository, runtime, and external facts are acquired by
registered deterministic providers when authoritative state requires them. There is no
application-context-question model station or model-authored context-completeness claim.

After context closure, exactly one bounded requirement-inventory call returns either
`NO_RUNTIME_REQUIREMENT_CANDIDATES` or between one and the code-owned maximum positive,
source-ordered atomic finished-software runtime-outcome candidates. Code parses and counts
those lines mechanically. No semantic station pre-counts the inventory, and no pre-count
receipt exists. The inventory is candidate generation, not authority or a completeness
claim. At this boundary, a purpose-denoting product or category name may contribute only its
literal core operation or governed result; the inventory splits independent outcomes and
omits construction constraints, customary features, and speculative enhancements.

Code owns the candidate queue. It closes exact repeats mechanically, authorizes each positive
candidate against the immutable request before spending downstream inference on it,
classifies only an authorized candidate, partitions only an authorized candidate that still
proves mixed or multi-outcome, and compares one byte-different runtime candidate with one
retained outcome at a time only to remove duplicates. A rejected, unrequested, non-runtime,
malformed-after-bounded-correction, or duplicate candidate dies at that boundary and cannot
open a later product-name generator. Accepted outcomes are never globally reviewed or
reopened. Queue exhaustion, not a model-authored completion label, freezes the current typed
intent for this iteration. An invalid station grammar or receipt still fails at its owning
boundary; a valid negative candidate relation is an ordinary sieve result, not an error or
retry trigger. Rejected or speculative candidates may be retained only as non-authoritative
follow-up suggestions outside the current ledger, workload, verifier, and completion criteria.

Only after at least one task-local requirement survives does code invoke product-context,
delivery-surface, deployment, or other downstream semantics at the first consumer that needs
that exact leaf. Product context remains limited to product/domain identity and outside each
task-local source contract. Source boundaries receive one exact accepted requirement, never
a derived aggregate summary that can supply sibling requirements. Delivery surface,
technical and structural format, generic tests/builds, and deployment never become fake
application features. A functional verified result satisfying the current accepted objective
is a successful iteration; speculative enhancements require a later explicit user objective.
Requirements are bound to the immutable request digest; exact substrings, quote intervals,
source order, punctuation, disjointness, and overlap are not authority gates.

For each accepted task-local runtime implementation requirement, code creates exactly one frozen task containing the
code-owned task identity, requirement identity, and unchanged accepted requirement.
Code assigns source order and freezes the workload hash. There are no model-authored
objectives, behaviors, acceptance criteria, dependencies, schedules, tools, paths, or
completion state. Artifact handling remains a separate token-blind classification job.
Code may bind one independently accepted
PostgreSQL skill and may expose only direct pairwise capability APIs. After code selects
the stack, it projects an exact target tree mechanically when the registered grammar is
deterministic and invokes the target-tree station only for a genuine structural naming
uncertainty. That one call covers the complete frozen workload and returns the raw
objective by receiving the exact immutable request, while selected technical context
remains a separate code-owned projection. It returns the raw `ROOT` node grammar; code
constructs and validates all normalized relative paths. The raw request stops at this
structural boundary and is never forwarded to a source call. The
selected compiler turns the accepted tree and frozen workload into bounded source-block
responsibilities. Each source call returns one ordinary path-blind implementation
body; code owns the signature and declaration, parsing, document construction, imports, formatting,
stitching, isolated checks, and final verification.

A specific validation defect continues the same immutable, persisted source-body work
unit with the same model only after code proves its exact mutable byte span. The model
receives one necessary semantic question and that span alone and returns ordinary
replacement text. Code checks the retained base and span, performs the splice, and keeps
the signature, declaration, parser state, application state, and every other accepted
block. It accepts only a newly validated assembled declaration and stops loudly after the
bounded attempt limit. There is no repair model, replacement work kind, response packet,
or textual correction protocol. Local Ollama station models are selected through
environment-backed routing, so structurally qualified local models can be measured
against the same unchanged gates without application changes. A missing model, context
mismatch, or invalid capacity fails explicitly. The current boundary is documented in
[docs/CHARMANDER_ASSEMBLY_LINE.md](docs/CHARMANDER_ASSEMBLY_LINE.md#production-flow).

Production station inference currently requires Ollama's exact prepared contract.
OpenAI, Azure AI, Google, Hugging Face, and compatible services appear only when
they provide the explicitly selected embedding transport; generic hosted
generation is not a production compatibility path.

Every bounded station must use an exact prepared-inference contract. Provider identity and request-specific authority are established at the station boundary; there is no process-wide cognition brain or universal cognition policy.

Known provider identities and rejected legacy environment keys remain centralized
in [internal/llmprovider/catalog/definitions.go](internal/llmprovider/catalog/definitions.go).
Only transports implementing the exact prepared station contract or the selected
embedding interface appear in the production provider catalog. Provider selection,
credentials, endpoint validation, construction, and discovery occur lazily at the
first actual provider operation after its named need is persisted. Unsupported or
incomplete authority fails that gap without provider dispatch; it never falls back
to Ollama or another transport.

## Localization

The cockpit ships server-validated catalogs for:

- English (`en`)
- Spanish (`es`)
- Simplified Chinese (`zh-Hans`)
- Russian (`ru`)
- Japanese (`ja`)

The server owns the selected locale. `Accept-Language` seeds the first visit; `?locale=...` explicitly changes it. Unsupported explicit locales return a validation error instead of silently falling back.

## State and UI boundaries

- PostgreSQL owns jobs, steps, projects, cards, messages, and memories while the
  current core is running. Development startup deliberately discards that state:
  the core drops and recreates the exact configured runtime schema from
  `database/setup.sql` every time it starts.
- Redis owns ephemeral coordination, progress, pub/sub, locks, and realtime fanout.
- The server owns lifecycle and mutation truth.
- RecyclrJS is the page-scoped realtime bridge.
- Stimulus coordinates interactions and loading states; it is not an application component renderer.
- Reusable application UI is server-rendered. Scrum card modals remain the explicit React + TypeScript typed-JSON exception.

## Product surfaces

- **Chat** — conversational entrypoint with code-owned objective state and exact typed semantic stations; wording lists never select execution.
- **Projects** — workspace, model, and codebase-map configuration.
- **Scrum** — review queue, board, typed card channel, and explicit controlled execution surface.
- **Data** — deterministic read-only data-source queries with recorded SQL and result evidence.
- **Integration API** — authenticated typed chat, data-source registration, transcript, and job endpoints with SDKs for Go, JavaScript, PHP, Java, and Rust.
- **Jobs** — live queue, station progress, failures, and final results.
- **Memory** — scoped reference/preference storage; never hidden prompt authority.

The integration packages and the delegated-data security boundary are documented in
[sdk/README.md](sdk/README.md). The API remains disabled until an exact
`OMNIDEX_INTEGRATION_API_TOKEN` of 32–4096 visible ASCII bytes is configured.

## Quick start

Requirements: Docker with Compose and an Ollama endpoint reachable from the core service.

```bash
cp default.env .env
# Set HOST_UID=$(id -u), HOST_GID=$(id -g), and
# DOCKER_GID=$(stat -c '%g' /var/run/docker.sock) in .env.
./up.sh --build
```

Open `http://localhost:8090`.

Omnidex currently has no internal database migration or data-upgrade path. Every
core startup drops and recreates the exact non-public schema named by
`DATABASE_SCHEMA` from [database/setup.sql](database/setup.sql). All existing
Omnidex rows in that schema are intentionally discarded. Use a dedicated schema
whose contents may be destroyed; never point Omnidex at a schema whose data must
survive.

The default compose topology keeps PostgreSQL and Redis on the internal backend network. The core API is the normal host-facing service.
`up.sh` and `down.sh` require `DOCKER_CONTEXT=default`, clear ambient Docker
endpoint overrides, and use the exact `COMPOSE_PROJECT_NAME` in `.env`. Rootless
Docker is forbidden because its UID translation breaks host-authoritative bind
ownership. See [docs/ROOTFUL_DOCKER.md](docs/ROOTFUL_DOCKER.md). Do not run
ambient `docker compose` commands, which can point at a different Docker engine
and create a separate empty database.
The core image runs as the configured `HOST_UID`/`HOST_GID`, so files written
through the direct workspace bind retain the caller's host ownership. Compose
adds only the exact numeric `DOCKER_GID` as a supplementary group, allowing the
non-root core process to use the default `/var/run/docker.sock`. The mounted
socket is used only for code-owned generated-workload verification and
deployment; it does not hold or copy the workspace.

For a host build:

```bash
make build
```

The core embeds the production GUI, so every supported core build runs
`scripts/build-ui.sh` first. That builder requires Node and npm, installs the exact
lockfile with `npm ci`, and fails if the production bundle is incomplete.

## Install and update

Run the installer from a clean Git checkout with an `origin` remote:

```bash
cp default.env deployment.env
# Edit deployment.env explicitly for this host before installing.
./install.sh --env-file deployment.env --yes
```

The installer stages a complete checkout, builds the GUI and the authoritative
`bin/omnidex` server and `bin/omni` CLI, verifies their build identity, requires the
explicit deployment environment to be a regular file, and then swaps the finished
checkout into `~/.omnidex`. `default.env` is a template and is never silently
promoted to active authority. An existing install's regular `.env` is preserved
byte-for-byte; supplying `--env-file` during replacement is rejected. This
environment-file rule does not preserve database state: when the installed server
next starts, it rebuilds the configured Omnidex schema from `database/setup.sql`
and discards its old rows.

From any directory, update the installed host checkout and binaries with:

```bash
"${OMNIDEX_DIR}/update.sh" --host-only
```

Run `"${OMNIDEX_DIR}/update.sh"` without `--host-only` to also rebuild and restart
the configured Compose service. Updates fast-forward a sibling staged checkout and
do not replace the active install when fetching, GUI compilation, Go compilation,
or staged-layout validation fails.

A native binary release archive uses the same environment rule but does not
contain a Git checkout. After extracting it, prepare and review a deployment file,
then install it atomically:

```bash
cp default.env ../omnidex-deployment.env
# Edit ../omnidex-deployment.env explicitly for this host.
./install-release.sh --env-file ../omnidex-deployment.env --yes
```

The archive never contains an active `.env`, and its template is never promoted
implicitly. Replacing a prior binary-release install preserves that install's
regular `.env` byte-for-byte and rejects a replacement `--env-file`.

## Use the CLI

Build the CLI:

```bash
make omni
```

From the project directory that should be changed, start the typed chat transport:

```bash
CORE_URL=http://localhost:8090 /path/to/omnidex/bin/omni chat
```

The CLI streams the server-authoritative session and submits each free-form turn
through the ordinary production request boundary.

## Configuration

Start with [default.env](default.env) or [.env.example](.env.example). Important groups are:

- fresh-database setup, Redis, and listener settings;
- generation and embedding provider selection;
- per-role model routing;
- worker count, polling interval, and request timeout;
- workspace/host bridge boundaries;
- bounded web-search, semantic-memory retrieval, and context limits;
- realtime/UI Redis requirements.

Optional systems are disabled explicitly; a requested capability that is unavailable fails instead of pretending to run.

## Development verification

Backend:

```bash
go test ./cmd/omni ./cmd/omnidex ./internal/...
go vet ./cmd/omni ./cmd/omnidex ./internal/...
```

Frontend:

```bash
cd internal/api/web
npm test
npm run typecheck
npm run build
```

Release identity:

```bash
go run ./cmd/omni version --json
./scripts/build-release.sh --version v0.5.0 --codename Charmeleon
```

## Architecture map

- [internal/queue](internal/queue) — durable typed job transport, feedback continuity, and memory boundaries.
- [internal/worker](internal/worker) — V3 typed orchestration and the bounded coding assembly line.
- [internal/repository](internal/repository) — hash-bound repository snapshots, compiler-derived facts, retrieval, and evidence packs.
- [internal/taskstate](internal/taskstate) — job-scoped task continuity and state-transition authority.
- [internal/queue](internal/queue) — authoritative job and step lifecycle.
- [internal/llmprovider/catalog](internal/llmprovider/catalog) — provider capability registry.
- [internal/api](internal/api) — API, project/scrum services, and server-owned UI state.
- [internal/api/web/locales](internal/api/web/locales) — locale catalogs.
- [internal/version](internal/version) — embedded release identity.

## Release line

| Version | Codename | Meaning |
| --- | --- | --- |
| `v0.1.0-alpha` | Bulbasaur | Initial alpha. |
| `v0.2.0` | Ivysaur | Provider, memory, and runtime growth. |
| `v0.3.0` | Venusaur | Planner, draft queue, and human-controlled scrum execution. |
| `v0.4.0` | Charmander | Deterministic distributed coding assembly line. |
| `v0.5.0` | Charmeleon | Repository intelligence and software-defined task context. |

See [docs/RELEASE_VERSIONING.md](docs/RELEASE_VERSIONING.md) and [CHANGELOG.md](CHANGELOG.md).

License: MIT.
