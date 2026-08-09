# Omnidex

**Current development release:** `v0.5.0` Charmeleon

Omnidex is a local-first AI workbench with a conversational surface and a server-authoritative execution core. Charmeleon extends the deterministic assembly line with repository intelligence and software-defined task context for bounded work in existing codebases.

Charmander established the bounded assembly-line foundation; its captured measurements and limitations remain in [docs/CHARMANDER_PROOF.md](docs/CHARMANDER_PROOF.md). Charmeleon is now the active development milestone. Its context architecture and uncompleted promotion gates are in [docs/CHARMELEON_CONTEXT_SYSTEM.md](docs/CHARMELEON_CONTEXT_SYSTEM.md).

## Charmeleon in one sentence

Omnidex stores repository truth and task continuity outside the model, then gives each Qwen call one immutable, bounded projection while code retains authority over acquisition, mutation, verification, and completion.

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
4. Code partitions every exact requirement and validates complete change-surface
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

Ollama is the default local provider. The only coding model roles are independently configurable without giving either one control-plane authority:

```dotenv
LLM_PROVIDER=ollama
OLLAMA_MODEL_SPECIALIST_CODING_SURFACE=qwen3.5:9b-q4_K_M
OLLAMA_MODEL_SPECIALIST_CODING_PRODUCT_IDENTITY=qwen3.5:9b-q4_K_M
OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_PARTITION=qwen3.5:9b-q4_K_M
OLLAMA_MODEL_SPECIALIST_CODING_ARTIFACT_HANDLING=qwen3.5:9b-q4_K_M
OLLAMA_MODEL_SPECIALIST_CODING_CAPABILITY_RELATION=qwen3.5:9b-q4_K_M
OLLAMA_MODEL_SPECIALIST_CODING_SKILL_SELECTION=qwen3.5:9b-q4_K_M
OLLAMA_MODEL_SPECIALIST_CODING_SKILL_PROCEDURE=qwen3.5:9b-q4_K_M
OLLAMA_MODEL_SPECIALIST_CODING_FRAGMENT=qwen3-coder:30b
OLLAMA_MODEL_SPECIALIST_CODING_FRAGMENT_CORRECTION=qwen3-coder:30b
INFERENCE_CONTEXT_TOKENS=16384
CODING_FRAGMENT_CONCURRENCY=1
```

The surface role classifies only browser, command-line, or service delivery. One stable feature-partition role extracts exact feature envelopes from the user authority and splits each accepted envelope to a code-owned fixed point. There is no production reasoning adviser and no separate split-model route. There is no model-authored kind, outcome, plan, or coverage verdict. Artifact handling is a separate token-blind classification job. For each local need, code either binds an exact active PostgreSQL skill, gives a selector at most five opaque purpose summaries, or asks a procedure worker to produce one bounded reusable instruction. The fragment role then receives one exact feature contract plus that procedure and returns one raw declaration. Every call is an immutable content-addressed work unit; identities, paths, imports, formatting, stitching, scheduling, commands, and completion remain code-owned. Local Ollama uses Qwen 3.5 9B for the bounded semantic stations and Qwen3-Coder 30B for fragment generation and correction. Keeping the profile to two models limits phase-boundary model swaps on a one-model Ollama runner. It defaults to one fragment lane because concurrent requests to one endpoint are contention, not distributed capacity; the explicit concurrency setting may be raised to at most four when real independent capacity exists. A missing model, context mismatch, or invalid capacity fails explicitly. See [docs/LOCAL_MODEL_PROFILE.md](docs/LOCAL_MODEL_PROFILE.md) for measured hardware limits and alternatives. Offline advisory-model experiments are isolated from production routing and documented in [docs/MODEL_GAUNTLETS.md](docs/MODEL_GAUNTLETS.md).

Hosted generation providers include Ollama, OpenAI, Azure AI, xAI, Google Gemini, Anthropic, Hugging Face, and custom OpenAI-compatible endpoints.

Chinese service integrations include DeepSeek; Alibaba Qwen / Model Studio; Moonshot / Kimi; Zhipu / BigModel / GLM; Z.AI; MiniMax; Baidu Qianfan / ERNIE; Tencent Hunyuan; ByteDance Doubao / Volcengine Ark; StepFun; 01.AI Yi; Baichuan; iFlytek Spark; SiliconFlow; ModelScope; Huawei ModelArts; Xiaomi MiMo; Meituan LongCat; Ant Ling / InclusionAI; and Tencent TokenHub.

Provider IDs, aliases, endpoints, credential keys, and embedding capabilities are centralized in [internal/llmprovider/catalog/definitions.go](internal/llmprovider/catalog/definitions.go). Selecting a service without its required key, model, endpoint, or separate embedding provider fails configuration validation.

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

- **Chat** — conversational entrypoint with model-owned semantic intent classification and code-owned typed transport; wording lists never select execution.
- **Projects** — workspace, model, agent, and codebase-map configuration.
- **Scrum** — Venusaur’s planner, review queue, board, card channel, and controlled execution surface.
- **Data** — read-only data-source channels with recorded SQL and result evidence.
- **Jobs** — live queue, station progress, failures, and final results.
- **Memory** — scoped reference/preference storage; never hidden prompt authority.

See [docs/SCRUM_PLANNER.md](docs/SCRUM_PLANNER.md) for the planner surface retained from Venusaur.

## Quick start

Requirements: Docker with Compose and an Ollama endpoint reachable from the core service.

```bash
cp default.env .env
docker compose up --build
```

Open `http://localhost:8090`.

The default compose topology keeps PostgreSQL and Redis on the internal backend network. The core API is the normal host-facing service.

For a host build:

```bash
go build ./cmd/core ./cmd/cli ./cmd/omni
cd internal/api/web
npm install
npm run build
```

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
- [internal/workspace](internal/workspace) — bounded snapshots, excerpts, and relevance ranking.
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
