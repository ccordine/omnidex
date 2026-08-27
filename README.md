# Omnidex

**Current development release:** `v0.5.0` Charmeleon

Omnidex is a local-first AI workbench with a conversational surface and a server-authoritative execution core. Charmeleon extends the deterministic assembly line with repository intelligence, software-defined task context, and a domain-neutral cognition runtime for bounded work across long-lived environments.

Charmander established the bounded assembly-line foundation; its captured measurements and limitations remain in [docs/CHARMANDER_PROOF.md](docs/CHARMANDER_PROOF.md). Charmeleon is now the active development milestone. Its context architecture is in [docs/CHARMELEON_CONTEXT_SYSTEM.md](docs/CHARMELEON_CONTEXT_SYSTEM.md), its production cognition contract is in [docs/CHARMELEON_COGNITION_RUNTIME.md](docs/CHARMELEON_COGNITION_RUNTIME.md), and its code-owned prerequisite and narrow-inference boundary is in [docs/CHARMELEON_COGNITION_RESOLUTION.md](docs/CHARMELEON_COGNITION_RESOLUTION.md).

## Charmeleon in one sentence

Charmander made individual model jobs reliable. Charmeleon makes those bounded jobs cooperate through code-owned continuity, attention, action, evidence, revision, and completion across environments too large or long-lived for any one model context.

```text
conversation
    ↓
behavior class (five fields)             semantic model
    ↓
shape-specific semantic labels           semantic model
    ↓
expanded typed behavior                  code-owned
    ↓
validated adapter + block graph          code-owned
    ↓
static declarations + rare functions     code + fragment model(s)
    ↓
AST assembly + isolated full tests        code-owned
    ↓
authoritative writes + exact verification code-owned
    ↑ exact failure
one owning generated function only        fragment model
```

## What changed after Venusaur

| Venusaur-era failure | Charmander contract |
| --- | --- |
| One model carried project scope, workflow control, code, checking, and recovery. | Each model receives one small typed responsibility. |
| Raw memories and old work could redirect the active task. | The coding semantic model receives only current direct authority and ordered user feedback; memory is absent. |
| Correction arrived late or restarted the build. | Accepted blocks remain in memory and only one declared generated owner can be corrected. |
| Models chose commands, declared success, and confused advice with execution. | Code owns tools, mutation authority, verification, and completion. |
| Intermediate files were rejected because unfinished siblings did not compile. | Dependency waves complete before the whole staged program is compiled or tested. |
| Source workers received paths, trees, plans, and excessive project context. | A fragment model receives one signature, one behavior, direct declarations, allowed symbols, and optionally its own current function plus a path-free failure. |
| Hidden ledgers and duplicate recovery systems accumulated stale state. | Git is the source history; Omnidex keeps one current workspace and one coding engine. |
| Long generic status streams obscured real progress. | Live events report phase, active station, target, accepted diff, diagnostic, and terminal outcome. |

## Coding assembly line

The runtime has deliberately unequal stations:

1. **Semantic interpreter (model)** — performs two bounded jobs: a three-field schema/shape/language classification and one compact shape-specific label extraction. Code owns the surface and toolchain version. Neither response can express implementation or orchestration.
2. **Seed validator, expander, and adapter registry (code)** — reject malformed or unsupported seeds, expand types/roles/inputs/persistence guarantees, resolve opaque protected artifacts, and build one exact block graph.
3. **Block scheduler and parser (code)** — compute dependency waves, render all standard behavior directly, and derive the minimum declarations for any behavior code cannot render.
4. **Fragment transformer (model, only when required)** — returns exactly one function with an immutable signature. It never sees a path, document, project, job, plan, or filesystem operation.
5. **Stager and correction controller (code)** — stitch and format complete documents, run the isolated complete program, and map an exact failure to one declared generated owner. Static defects fail as adapter defects.
6. **Mutation, verification, and completion controllers (code)** — write only a staged program, emit reviewable diffs, verify exact workspace content, run fixed tests once more, and declare the outcome.

The detailed contract lives in [internal/worker/RUNTIME.md](internal/worker/RUNTIME.md).

## Existing-repository path (development)

Charmeleon's existing-repository workflow is deliberately separate from the
greenfield application builder. It currently supports Go repositories through one
server-owned path:

1. Git and the active worktree produce a complete, content-addressed snapshot.
2. Compiler-backed analysis stores files, symbols, direct edges, tests, and exact
   source spans as derived PostgreSQL facts.
3. Typed semantic-excerpt, declaration, and incoming-reference queries construct a
   bounded evidence pack; the model never receives a path or repository tree.
4. One bounded call extracts exact grounded change requirements, and code validates complete change-surface
   coverage into one source-snapshot-bound change contract and ordered verification
   plan.
5. Before any fragment generation, the complete focused-plus-terminal-broad plan must
   pass in a disposable projection containing exactly the validated source-snapshot
   files. Git metadata, `.omni`, ignored files, and excluded paths are never mounted.
   A failing or drifting baseline stops with no generation, correction, or mutation
   authority.
6. Each fragment call receives only one function plus direct capabilities. Candidate
   declarations are AST-validated and applied to a complete disposable
   stage. Focused direct tests and a terminal broad test run through a read-only,
   network-isolated bubblewrap sandbox with structured `go test -json` proof. The
   sandbox receives a disposable checksum-validated projection of only the source
   module's resolved build-list dependencies; it never receives the host-wide Go
   module cache.
7. Only a uniquely owned, path-free ordinary test/compiler failure may correct one
   function. The complete candidate set is restaged, with two total correction rounds
   and explicit no-progress/oscillation rejection.
8. A PostgreSQL mutation journal binds the exact source snapshot, contract, stage,
   patch, source/post file states, claim, and generated-diff evidence. Exact source may
   apply once; exact post may finalize once; every other state is indeterminate and
   fails loudly. Authoritative verification uses a newly constructed exact post-state
   projection rather than the already-tested candidate stage or the live worktree.

These proofs establish a clean pre-change baseline, exact patch integrity, and
regression verification against the repository's existing tests. They do not prove
that an arbitrary new user requirement was satisfied: that requires an independent,
requirement-bound or held-out proof unavailable to the builder until it stops. The
existing-repository path therefore remains unpromoted as an autonomy claim. Context
Projection is also shadow-only, the index is not yet incrementally refreshed, and a
lost running worker cannot be reclaimed until all worker writes carry a monotonic
step-attempt lease. In addition, an `applied` mutation-journal row is not a resumable
authoritative-proof or completion checkpoint: interruption after patch finalization
can restart the semantic change workflow. The current path must not be described as
crash-safe end-to-end execution. The exact promotion gates and restart boundary are
documented in [docs/CHARMELEON_CONTEXT_SYSTEM.md](docs/CHARMELEON_CONTEXT_SYSTEM.md).

## Reliability rules

- One authoritative implementation; no legacy fallback engine.
- Explicit errors for invalid provider configuration, schemas, paths, mutations, and completion claims.
- Git and the active worktree remain source authority. PostgreSQL repository snapshots are immutable, disposable, hash-bound derived facts—not alternate source history.
- The Task Ledger records typed job execution state only; it never versions or overrides source code.
- No rollback of valid file work because a later file fails.
- No deletion unless the current instruction authorizes deletion.
- No modification of protected instruction files.
- Tiny typed JSON only for semantic classification and label extraction; one raw AST declaration only for a coding transform.
- Bounded fragment retries and node corrections; unchanged output and repeated failure stop loudly.
- Verification commands are selected from the accepted typed program, never inferred from prose or workspace guesses.
- Direct, exact diagnostic feedback reaches the next worker immediately.
- Bounded contract correction stays on the configured model and receives current authority plus the exact failure, never a fallback model or replayed rejected response.

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
OMNI_CODING_REQUIREMENTS_MODEL=llama3.2:3b
OMNI_CODING_SERVICE_DEPLOYMENT_INTENT_MODEL=phi4:14b
OMNI_CODING_WORKLOAD_MODEL=llama3.2:3b
OMNI_CODING_ARTIFACT_HANDLING_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_CAPABILITY_RELATION_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_SKILL_SELECTION_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_FRAGMENT_MODEL=qwen2.5-coder:7b
OMNI_CODING_FRAGMENT_REPAIR_GUIDANCE_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_FRAGMENT_CORRECTION_MODEL=qwen2.5-coder:7b
OMNI_CODING_REPOSITORY_SEARCH_TERM_MODEL=qwen3.5:9b-q4_K_M
OMNI_CODING_REPOSITORY_CHANGE_SURFACE_MODEL=qwen3.5:9b-q4_K_M
OMNI_REPOSITORY_GROUNDED_REVIEW_MODEL=deepseek-r1:8b
OMNI_WEB_CLAIM_EVIDENCE_REVIEW_MODEL=deepseek-r1:8b
INFERENCE_CONTEXT_TOKENS=8192
CODING_FRAGMENT_CONCURRENCY=1
```

The established `OMNI_CODING_SERVICE_DEPLOYMENT_INTENT_MODEL` environment key
and `coding_service_deployment_intent_model` project-setting key are retained so
persisted configuration remains readable. They select one shared provider model
for the independent continued-availability and persistence-destination stations;
they do not restore the retired ternary deployment-intent station or combine the
two semantic responsibilities.

The first opt-in browser WebGPU provider executes the existing
`context_relevance` station in a resident Web Worker while the server retains all
workflow, state, retrieval, provenance, and validation authority. No tested browser
model is currently qualified, so `OMNI_CONTEXT_RELEVANCE_PROVIDER=server` remains the
default. The exact boundary, live corpus, rejected candidate measurements, and rerun
command are in [docs/BROWSER_INFERENCE.md](docs/BROWSER_INFERENCE.md).

### Deployment sizing

The checked-in profile is Qwen-led, but it is not a single-model deployment.
The active routes use Qwen 3.5 9B for most bounded semantic work, Qwen 2.5
Coder 7B for source fragments and corrections, Llama 3.2 3B for requirement
extraction, Phi-4 14B for the two service deployment semantics, DeepSeek R1 8B for the
independently routed evidence reviews, and `nomic-embed-text` for local embeddings.
The complete route list is in
[`default.env`](default.env).

The following values are capacity-planning estimates, not universal performance
guarantees. Model-file sizes come from the exact local Ollama inventory and agree
with the current Ollama library entries. Runtime allocation and timing rows were
measured on 2026-08-19 using `omni ollama:prewarm` and the exact production 8,192-token
context. Re-run that command on every candidate server before accepting it.

#### Model working set

| Route | Current model | Local model file | Runtime role |
| --- | --- | ---: | --- |
| Most semantic, database, answer, and repair-guidance stations | `qwen3.5:9b-q4_K_M` | 6.6 GB | Structured semantic result |
| Fragment generation and correction | `qwen2.5-coder:7b` | 4.7 GB | One bounded source node |
| Requirement and workload extraction | `llama3.2:3b` | 2.0 GB | Small typed extraction |
| Service deployment semantics | `phi4:14b` | 9.1 GB | Conditional availability and destination leaves |
| Independent repository/web evidence review | `deepseek-r1:8b` | 5.2 GB | Separate review identity |
| Embeddings | `nomic-embed-text` | 0.27 GB | Retrieval vectors |

The exact set occupies about **27.9 GB (26.0 GiB) on disk**. Reserve at least
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
- 24 GB is the preferred value for a small team because the measured Qwen
  allocations total about 14.3 GiB before scheduler and driver headroom.
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
24 GB or larger GPU, test `OLLAMA_MAX_LOADED_MODELS=2` to retain the two Qwen
runners. Increase `OLLAMA_NUM_PARALLEL` only after the exact 8K prewarm and a
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
decode or 2 seconds of Qwen 2.5 Coder decode after prompt evaluation. A cold or
swapped model adds roughly 12–14 seconds in the current profile. Larger prompts,
JSON-constrained retries, and validation repairs add time even when their output
is short.

These ranges are deployment-planning estimates for one active inference stream:

| Workload | Warm GPU planning range | Main variables |
| --- | ---: | --- |
| Short conversational turn | 5–30 s | Context selection, answer length, memory retrieval |
| Simple database-RAG turn | 20–90 s | Relation count, schema-selection chunks, evidence rounds, answer length |
| Small bounded coding change | 5–30 min | Requirements, generated declarations, model swaps, compiler/test runs, repairs |
| Multi-feature repository build | Tens of minutes to hours | Task count, dependency waves, corrections, project build/test cost |

A simple database turn normally needs schema selection, typed query-intent
construction, an evidence-gap decision, and final grounded synthesis. Schemas
over 24 relations require additional bounded selection calls; ambiguous joins or
missing evidence add more rounds. PostgreSQL execution itself is bounded and is
usually not the dominant latency unless the application authority or database is
remote or slow.

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
ollama pull qwen2.5-coder:7b
ollama pull llama3.2:3b
ollama pull phi4:14b
ollama pull deepseek-r1:8b
ollama pull nomic-embed-text

omni ollama:prewarm --model qwen3.5:9b-q4_K_M --num-ctx 8192 --json
omni ollama:prewarm --model qwen2.5-coder:7b --num-ctx 8192 --json
omni ollama:prewarm --model llama3.2:3b --num-ctx 8192 --json
omni ollama:prewarm --model phi4:14b --num-ctx 8192 --json
omni ollama:prewarm --model deepseek-r1:8b --num-ctx 8192 --json
```

Accept a host only after the probe reports the exact 8,192-token context, memory
fits without sustained swap, the intended GPU is used, warm latency is acceptable,
and repeated runs produce no runner restart or GPU error. Then run one ordinary
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
[Ollama DeepSeek R1 tags](https://ollama.com/library/deepseek-r1/tags),
[Ollama nomic-embed-text](https://ollama.com/library/nomic-embed-text),
[Ollama concurrency and memory FAQ](https://docs.ollama.com/faq), and
[Ollama GPU support matrix](https://docs.ollama.com/gpu).

The surface station classifies only browser, command-line, or service delivery.
Before intent interpretation, code hashes the immutable request and records exact
workspace state plus bounded accepted durable memory as typed facts. A
context-sufficiency station can return only zero through three missing evidence
questions, never operations or tool calls; registered code-owned providers resolve
those questions and formalize selected results into source-backed facts. The
currently promoted fresh-workspace vertical requires zero questions and fails loudly
otherwise. An intent station then derives one concise product context plus one through
ten semantic requirement statements. Code validates and retains a complete candidate
directly. Only one exact deterministic structural defect permits a bounded replacement
of that defective leaf; there is no model call merely to accept or review a valid
candidate. Requirements are bound to the immutable request digest; exact substrings,
quote intervals, source order, punctuation, disjointness, and overlap are not authority
gates.

For each accepted requirement, one job-specification station receives the typed
surface, product context, complete accepted requirement set, and focused requirement.
It returns one objective, one through four required behaviors, and one through four
observable acceptance criteria. Code validates the complete candidate, assigns task
identity and source order, and freezes the workload hash. Dependencies, scheduling,
tools, paths, and completion remain outside model authority. Artifact handling remains
a separate token-blind classification job. Code may bind one independently accepted
PostgreSQL skill and may expose only direct pairwise capability APIs. After code selects
the stack, it projects an exact target tree mechanically when the registered grammar is
deterministic and invokes the path-only tree station only for a genuine structural
naming uncertainty. The selected compiler turns the accepted tree and coverage into
bounded source-block responsibilities. Each source call returns one exact path-blind
declaration or source node; code owns document construction, imports, formatting,
stitching, isolated checks, and final verification.

A mapped source failure is diagnosed by the separately routed repair-guidance model
into one self-contained instruction; the correction model sees only that instruction
and the exact mutable source block. Code applies and verifies the result, and any new
failure starts a new guidance/execution iteration. Every call is an immutable
content-addressed work unit. Local Ollama station models are selected through
environment-backed routing, so structurally qualified local models can be measured
against the same unchanged gates without application changes. A missing model, context
mismatch, or invalid capacity fails explicitly. The guided compiler qualification and
its exact evidence are documented in
[docs/LOCAL_MODEL_PROFILE.md](docs/LOCAL_MODEL_PROFILE.md#live-guided-repair-qualification).

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

- PostgreSQL owns durable jobs, steps, projects, cards, messages, and memories.
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
# Set DOCKER_CONTEXT, HOST_UID=$(id -u), and HOST_GID=$(id -g) in .env.
./up.sh --build
```

Open `http://localhost:8090`.

The default compose topology keeps PostgreSQL and Redis on the internal backend network. The core API is the normal host-facing service.
`up.sh` and `down.sh` use the exact `DOCKER_CONTEXT` and
`COMPOSE_PROJECT_NAME` in `.env`; do not run ambient `docker compose` commands,
which can point at a different Docker engine and create a separate empty
database.
The core image runs as the configured `HOST_UID`/`HOST_GID`, so files written
through the direct workspace bind retain the caller's host ownership. The
mounted rootless Docker socket is used only for code-owned generated-workload
verification and deployment; it does not hold or copy the workspace.

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

The installer stages a complete checkout, builds the GUI and all host binaries,
validates the explicit deployment environment, then swaps the finished checkout
into `~/.omnidex`. `default.env` is a template and is never silently promoted to
active authority. An existing install's regular `.env` is preserved byte-for-byte;
supplying `--env-file` during replacement is rejected. PostgreSQL and Redis data
remain in their Docker named volumes.

From any directory, update the installed host checkout and binaries with:

```bash
omni update --host-only
```

Use `omni update` without `--host-only` to also rebuild and restart the configured
Compose service. Updates fast-forward a sibling staged checkout and do not replace
the active install when fetching, GUI compilation, or Go compilation fails.

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

## Run a coding job

Build the API CLI:

```bash
make cli
```

From the project directory that should be changed:

```bash
CORE_URL=http://localhost:8090 /path/to/omnidex/bin/agent-cli run \
  "Build the requested feature, include focused tests, and run the project test suite."
```

The CLI prints live phases, file events, diagnostics, and the final state. Direct correction stays on the job:

```bash
CORE_URL=http://localhost:8090 /path/to/omnidex/bin/agent-cli feedback JOB_ID \
  "Fix the failing boundary test specifically; preserve the accepted implementation."
```

## Configuration

Start with [default.env](default.env) or [.env.example](.env.example). Important groups are:

- database, Redis, listener, and migration settings;
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
go test ./...
go vet ./...
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
go run ./cmd/cli version
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
