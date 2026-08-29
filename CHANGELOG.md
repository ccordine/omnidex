# Changelog

Release codenames follow Omnidex pride versioning based on National Dex order: Bulbasaur, Ivysaur, Venusaur, Charmander, and **Charmeleon** (current development release).

## v0.5.0 - Charmeleon (in development)

Charmeleon is the **repository-intelligence and software-defined context release**. It keeps the bounded assembly-line authority split while moving repository knowledge, job continuity, attention, and exact per-call context into separate code-owned primitives. Implementation is in progress; existing-repository mutation and large-codebase autonomy remain explicitly unclaimed until production vertical evidence passes.

### Context and repository control plane

- Added complete, content-addressed Git/worktree snapshots and normalized immutable repository facts.
- Added compiler-backed Go symbol and dependency analysis plus bounded, path-free evidence packs.
- Added a transactional job-scoped Task Ledger with append-only command events,
  generation-safe replanning, lifecycle operation identity, accepted-intent projection,
  and cursor-paginated history.
- Added generation-bound Working Sets with explicit budgets and immutable Context
  Projections. A model-visible projection is valid only when the production station
  consumes it to resolve its named semantic gap.
- Replaced the rejected universal cognition runtime and its model-driven procedural
  gauntlet with an isolated in-memory reference objective machine. The reference proves
  deterministic closure, one-leaf semantic gaps, repository discovery, bounded source
  generation handoff, and recursive semantic correction before durable cutover.
- Made memory-candidate promotion atomic and generation-aware so a retired job
  generation cannot publish durable memory.
- Added typed repository retrieval operations, complete requirement/change-surface
  coverage, hash-bound path-free fragment modification, exact full-tree staging,
  read-only network-isolated Go verification, structured target-bound test proof, and
  bounded single-owner correction.
- Added a durable prepared/applying/applied/indeterminate repository mutation journal
  with immutable sealed file authority, exact source/post recovery, atomic generated
  diff evidence, and real PostgreSQL plus bubblewrap workflow tests.
- Removed the production R1 requirement adviser, product-identity call, recursive residual partition, synthetic candidate choices, and `none` loop. Greenfield intake now uses one grounded aggregate over the intact request; each accepted requirement receives one local objective/criterion leaf, then code freezes and verifies the workload task by task. Existing-repository intake uses one separate grounded change aggregate.

## v0.4.0 - Charmander

Charmander is the **AI assembly-line release**. It replaces broad coding-agent loops with server-owned orchestration and small typed model jobs. Its initial three unattended junior-application baseline now passes; arbitrary-project support remains intentionally unclaimed.

### Coding control plane

- Split path-blind semantic interpretation into a five-field classification and one shape-specific label seed. Code validates and expands that seed into the hard-typed software contract; the model never emits the expanded contract.
- Added deterministic Go adapters for record, unit-conversion, and expense-report CLIs, including code-owned declarations, atomic persistence where required, and assertion-bearing tests.
- Added dependency-wave scheduling, bounded parallel function transforms, exact single-function AST validation, import derivation, document stitching, and block line maps.
- Added isolated complete-program staging before authoritative writes and exact test/compiler routing to one declared generated owner.
- Made actionable free-form turns use one cohesive objective, then bypassed planner, analysis, response, verifier, and memory model calls for deterministic coding results. Code now derives the final summary and verdict directly from the accepted subtask and persisted diff/test evidence.
- Removed phrase-based requirement compilers, model-generated manifests, whole-file source workers, file repair agents, LLM repair routers, source-version ledgers, and compatibility fallbacks.
- Removed greeting/action verb dictionaries, software-noun routing, token-overlap requirement scoring, and English sentence-prefix intent rewriting. Typed transport selects the pipeline; semantic intent remains a model task.
- Removed Project Planning and Scrum Coach slash-prefix mode routing. Their explicit request mode is now the sole transport authority, and free-form message text remains untouched.
- Removed content-scanned memory categories and assistant-text tool detection. Memory kind/tags and message roles are typed authority; model text cannot reroute storage or presentation.
- Removed cross-model contract-repair fallback and rejected-response replay. Bounded repairs use the configured model with current authority and the exact validation failure.
- Put semantic correction on deterministic one-field merge rails: code retains the decoded seed, the correction schema exposes one top-level field, and a structural diff rejects full retries, unrelated mutations, multiple nested changes, and no-ops.
- Deleted the unreachable legacy planner, tournament, memory-inference, response-rewrite, verification-guessing, and Ollama-restart engine instead of retaining compatibility paths; removed and explicitly reject its obsolete environment settings.
- Removed the separate keyword-routed research CLI, freshness ledger, dossier store, and official-source fallback. Free-form research now uses the same typed semantic-intent and bounded web-evidence stages as other requests.
- Added exact workspace reconciliation, protected opaque artifacts, fixed typed-program verification commands, reviewable diffs, and server-authoritative completion.
- Made per-job model routing immutable so concurrent swarm workers cannot overwrite or restore one another's model selections.
- Enforced same-job feedback, interruption, and replanning at both server and browser boundaries and deleted unreachable successor-job compatibility responses.
- Removed the CLI's write-only architect profile and planning, persistence, review, missing-tool, generic reasoning, autonomy, approval, verification, web, workspace, and external-agent controls. Stale top-level metadata now fails explicitly; only runtime-backed model and consumed research-query settings remain.
- Registered raw-function response mode for fragment stations; unknown model-call scopes now fail explicitly instead of inheriting a guessed format.
- Completed a fresh current-build set of three clean-workspace Go CLI proof runs in 53.812 seconds total with eight compact semantic calls, zero fragment-model calls, independently passing tests, and unchanged protected request artifacts. See `docs/CHARMANDER_PROOF.md` for the measured comparison, discarded regression run, and limitations.

### Models, UI, and operations

- Published the core through `https://omni.worknet` over WorkNet's Docker edge instead of claiming host port 8090; fixed standard-HTTPS URL normalization and pinned the edge subnet inside WorkNet's existing Docker-to-Ollama trust boundary.
- Added a centralized Chinese-service catalog covering DeepSeek, Qwen / Alibaba Model Studio, Moonshot / Kimi, Zhipu / GLM, Z.AI, MiniMax, Baidu Qianfan / ERNIE, Tencent Hunyuan, ByteDance Ark / Doubao, StepFun, 01.AI Yi, Baichuan, iFlytek Spark, SiliconFlow, ModelScope, Huawei ModelArts, Xiaomi MiMo, Meituan LongCat, Ant Ling / InclusionAI, and Tencent TokenHub.
- Replaced coding planner/source/reviewer roles with one semantic role and one optional fragment role. Tests and standard behavior are rendered by code; a fragment model can return only one immutable-signature function.
- Added English, Spanish, Simplified Chinese, Russian, and Japanese server-owned UI locale catalogs.
- Added concise live coding phases, model heartbeats, accepted-file events, reviewable diffs, exact verification failures, and terminal state reporting without replaying redundant status.

## v0.3.0 - Venusaur

Venusaur is the **augmented planner release**: research and architect at project scope, review AI-generated work in a persistent draft queue, promote approved cards to the scrum board, and let build agents execute only what you moved to Ready.

### Project planner

- **Project Chat** tab — productivity/planning AI (model + instant/thinking toggle, web research, board scan).
- **Research & draft (`/batch`)** — web research plus a batch of reviewable backlog cards in one pass.
- **Persistent draft queue** — `planning_draft_queue` on projects; drafts survive refresh with pending / added / dismissed states.
- **Bulk draft actions** — add one, add all, dismiss, clear history via `POST /v1/projects/{id}/planning-chat/drafts`.
- Planner memory notes stored for later retrieval (`project-chat`, `scrum`, `project:{id}` tags).

### Scrum board & execution

- **Flow metrics** — column churn, conversation depth, incomplete-work signals on cards and board summary.
- **Card Channel pilot** — minified channel context (LLM summary + memory lookup) instead of raw agent transcript truncation.
- **Card Coach** — per-card planning, card ticket drafts, memory notes.
- Channel scroll UX — open at bottom; live updates only when pinned.

### Docs

- The historical planner loop and its direct-inference API were retired by the Charmeleon authority cutover.

## v0.2.0 - Ivysaur

- Added core-owned research ingest and official-document memory storage.
- Added procedural success playbook memories for completed jobs.
- Added memory categories, indexed category tables, category backfill, and category-aware retrieval.
- Added provider support for Google Gemini, Anthropic Claude, Hugging Face, and xAI Grok.
- Improved structured command recovery, objective-ledger reconciliation, command repeat handling, and placeholder/dependency drift validation.
- Added Docker host-Ollama compose overlay and expanded setup documentation.

## v0.1.0-alpha - Bulbasaur

- First public alpha release.
- Included the initial Omnidex CLI, core queue/runtime, Ollama-backed model routing, memory storage, and local automation workflows.
