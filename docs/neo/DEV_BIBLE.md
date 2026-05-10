# OmnidexNeo Dev Bible
## Deterministic Multi-LLM Coding Agent (Go + Ollama + PostgreSQL)

Prepared: 2026-05-07 (US/Eastern)  
Scope: Architecture constitution, standards, risk controls, and phased execution gates  
Status: Planning Authoritative v1.0

## 1) Purpose
This document is the governing development standard for OmnidexNeo.

It defines:
- What we are building.
- What we will not build.
- How determinism and LLM specialization are enforced in code.
- How safety, observability, and debuggability are guaranteed.
- What acceptance gates must pass before implementation progresses.

This is intentionally stricter than a typical roadmap. Any implementation decision that conflicts with this document requires an explicit amendment.

## 2) Product Mission
Build a CLI-first coding agent platform where multiple specialized LLM roles collaborate under deterministic orchestration.

Core mission constraints:
- No single model is trusted to solve end-to-end tasks alone.
- LLMs propose decisions within narrow contracts.
- Deterministic code executes policies, tools, retries, and state transitions.
- Every decision is logged deeply enough for full run replay and forensic debugging.
- The system remains maintainable as role count grows.

## 3) Non-Negotiable Principles
1. Deterministic control plane over stochastic model outputs.
2. Bounded role scope and minimal context per role.
3. Strict machine-validated output contracts for every role.
4. Policy gate before every side effect.
5. Replay-first architecture (event stream is source of truth).
6. Safety over convenience for command execution and external access.
7. Transparent, dense telemetry on every model and tool step.
8. Maintainable code shape: cohesive modules, small clear interfaces, low debt.
9. Fat domain models, skinny controllers.
10. Functional object-oriented style: pure decision logic + explicit effect boundaries.

## 4) Current-State Audit (Observed on 2026-05-07)

### 4.1 OmnidexNeo repository status
- Path: `/home/gryph/Projects/omnidexneo`
- Current state: docs-only project.
- Existing document: `docs/ROADMAP.md`.
- No Go module initialized yet.

### 4.2 Toolchain/runtime observations
- `go` is not installed in the current host environment (`go: command not found`).
- `ollama` is installed and reachable on localhost.
- Ollama version: `0.23.0`.
- Installed model inventory currently contains only `tinyllama:latest`.
- `tinyllama:latest` capabilities report `completion` only.

Implication:
- Immediate implementation cannot be executed locally until Go toolchain is installed.
- Current model inventory is insufficient for robust structured output/tool-routing workflows.

### 4.3 PostgreSQL container observations
- Active stack path: `/home/gryph/Projects/stack/container-pgsql`.
- Running container: `postgres_db` (`pgvector/pgvector:pg13`, healthy).
- PostgreSQL server version: `13.23`.
- `docker-compose.yml` uses `expose: 5432`, not host `ports` publishing.
- Host-level connection test to `127.0.0.1:5432` fails (`connection refused`).
- Container uses external Docker network `dev-net`.

Implication:
- A host-native CLI agent process cannot directly access Postgres over localhost with current compose config.
- Either host port publish or agent-in-container strategy is required.

### 4.4 Seed/config integrity observations
`databases.conf` entries with missing seed directories under `sql/`:
- `ai-rpg` (missing)
- `system_settings` (missing)
- `measure` (missing)
- `pros` (missing)
- `wai` (missing)
- `stats` (missing)

Observed DBs currently present include:
- `ai_rpg`, `digimon`, `engage`, `laravel`, `ncs_database`, `postgres`, `pros`, `stats`, `system_settings`, `wai_database`, `yomeru_database`

Implication:
- Environment likely carries historical state from prior seeding/restores.
- Bootstrap reproducibility risk is high unless seed path mapping is normalized.

### 4.5 Vector/pgvector observations
- `vector` extension currently present in `pros` only.
- `pros.public.ai_embeddings.embedding` is typed as `vector`.
- `pros.public.ai_embeddings` row count is currently `0`.
- No HNSW or IVFFlat index definitions were found in DB index metadata.

Implication:
- “Cosine matching is set up” is only partially true in current runtime snapshot.
- ANN indexing and populated embeddings are not currently established for this project’s memory workload.

### 4.6 Privilege observations
- Superuser-style roles include `admin`, `postgres`, and `laraveluser` in this instance.
- `users.conf` carries many credentials inline.

Implication:
- Principle-of-least-privilege posture is currently weak for an autonomous agent context.
- Dedicated least-privilege app user and role scoping is mandatory before production use.

## 5) Gap Analysis Summary
Critical gaps to close before implementation begins:
1. Go toolchain unavailable.
2. Postgres not reachable from host CLI process by default.
3. Model inventory lacks tool-capable/structured-output-capable operational set.
4. Seed mappings inconsistent with declared DB paths.
5. Vector retrieval schema/index/population not production-ready.
6. DB privileges too broad for autonomous tool execution.

## 6) System Architecture Constitution

### 6.1 Hard separation of concerns
- Orchestrator (deterministic FSM): state progression, retries, policy, context packing.
- LLM Gateway: model I/O only (never direct side effects).
- Tool Runtime: executes deterministic tool handlers.
- Policy Engine: allow/deny/escalate decisions.
- Verifier Engine: acceptance checks and semantic validation.
- Memory/Retrieval Engine: embeddings, retrieval, memory lifecycle.
- Telemetry Engine: logs, traces, metrics, replay data.

No component may skip policy and call side-effecting operations directly.

### 6.2 Event-sourced backbone
- Every significant action emits an immutable `run_event`.
- Current run state is derived from event sequence.
- Replays must reproduce deterministic decisions given same event/artifact set.

### 6.3 Role-first specialist model
Each role is a first-class artifact with versioned contract:
- `role_id`
- `role_version`
- `purpose`
- `allowed_tools`
- `input_schema`
- `output_schema`
- `failure_modes`
- `telemetry_requirements`
- `memory_permissions`

## 7) Role Taxonomy (Initial)

### 7.1 Router and control roles
- `router_llm`: output only CSV tool list.
- `planning_specialist`: decomposes objective into milestones.
- `strategy_specialist`: selects optimal execution posture.
- `software_architect`: validates architecture-level choices.

### 7.2 Operational specialists
- `pgsql_expert`
- `web_researcher`
- `linux_expert`
- `coding_specialist`
- `vlc_specialist`

### 7.3 Large-context delegation roles
- `doc_manager`
- `doc_worker`

### 7.4 Verification roles
- `verification_llm`
- optional `verification_llm_secondary` for high-risk disagreement checks

### 7.5 Missing roles to add next (recommended)
Priority 1 roles:
- `migration_specialist`: writes and validates schema migrations with strict up/down contract.
- `schema_governor`: protects migration ordering, drift detection, and rollback safety.
- `memory_curator`: approves/rejects memory candidates and deduplicates memory graph entries.
- `retrieval_librarian`: optimizes retrieval plans (lexical vs vector mix, filters, rerank policy).
- `security_specialist`: focuses on injection, exfiltration, command abuse, and secret exposure risks.

Priority 2 roles:
- `test_specialist`: generates regression/fuzz/e2e test plans from run failures and new contracts.
- `incident_analyst`: performs failure forensics from telemetry and proposes deterministic fixes.
- `cost_latency_specialist`: tunes model routing and context budgets for cost/performance targets.
- `context_compressor`: compresses large artifacts into role-specific minimal context packs.

## 8) Router Contract (CSV-Only)

### 8.1 Canonical contract
Router response must be exactly:
- comma-separated tool IDs only
- lowercase ASCII IDs: `[a-z0-9_]+`
- no spaces
- no prose, JSON, markdown, or rationale

Examples:
- valid: `memory_lookup,pgsql_expert,verification_gate`
- valid (empty): `` (interpreted as no tool selected)
- invalid: `Tool: memory_lookup`
- invalid: `["memory_lookup"]`

### 8.2 Deterministic parser behavior
- unknown tool IDs: rejected
- duplicates: dedupe keep-first unless repeatable tool type
- over max tool count: reject
- parse error: deterministic retry path with explicit parse error code
- retry budget exhausted: transition to `ROUTER_FAILED`

## 9) Specialist Invocation Contract
Every tool call uses a deterministic envelope:
- `run_id`
- `step_id`
- `tool_id`
- `tool_version`
- `role_id`
- `role_version`
- `policy_snapshot_id`
- `input_artifact_refs`
- `deadline_ms`

Every tool result returns:
- `status` (`success|partial|fail|blocked`)
- `error_code` (if non-success)
- `output_artifact_refs`
- `timings`
- `verification_hints`

## 10) Deterministic Delegation Model (Manager/Worker)

### 10.1 Manager output contract
Manager emits worker envelopes only:
- `worker_role`
- `scope_ref`
- `acceptance_checks`
- `deadline_ms`
- `max_tokens`

### 10.2 Worker restrictions
- No self-spawn.
- No scope expansion.
- No direct side effects outside assigned tools.
- Must return provenance for all extracted claims.

### 10.3 Merge requirements
Manager merge reducer must be deterministic:
- stable sort order
- explicit conflict resolution rules
- rejection of artifacts missing acceptance criteria

## 11) Permission Model and Policy Engine

### 11.1 Permission modes (CLI/TUI-visible)
Mode A: `ask_permission` (default)
- Read operations are allowed.
- Any write/side-effect action requires explicit approval.
- Scope grants are supported (single action, category, or run-level).

Mode B: `full_access`
- Read and write actions are allowed without per-action prompts.
- Policy validation still runs and must log allow/deny reasoning.

### 11.2 Risk tiers for actions
Tier 0: read-only introspection  
Tier 1: local reversible edits  
Tier 2: local potentially destructive actions  
Tier 3: network/data exfiltration/security-sensitive operations

Policy requirement:
- Every command/tool mapped to tier.
- In `ask_permission`, tiers 1-3 require approval.
- In `full_access`, tiers 1-3 do not prompt but remain fully audited.

### 11.3 Mandatory policy checks before side effects
- allowlist gate
- denylist gate
- path boundary gate
- argument shape gate
- timeout and resource budget gate
- secret-exposure gate

## 12) Linux Command Specialist Pattern

### 12.1 Multi-stage command flow
1. `linux_expert` emits command lines only.
2. deterministic shell parser validates syntax and policy.
3. execution runtime invokes command with timeout/output caps.
4. verifier checks outputs against expected outcomes.

### 12.2 Execution hardening standards
- Prefer `os/exec` with argv-style execution.
- For shell features, parse first, then execute in controlled shell mode.
- Enforce max stdout/stderr bytes and line counts.
- Enforce wall-clock timeout and kill-on-timeout.
- Capture exact command text and environment snapshot hash.

### 12.3 Command parser recommendation
- Use a dedicated shell parser library (e.g. `mvdan.cc/sh/v3/syntax`) for deterministic AST checks before execution.

## 13) PostgreSQL Specialist Pattern

### 13.1 Default behavior
- Default to read-only SQL generation (`SELECT`/CTE only).
- Writes require explicit policy escalation and verifier consensus.

### 13.2 Query generation controls
- parameterized statements only
- allowlisted schemas/tables/views
- bounded result limits by policy
- max execution time per query

### 13.3 Queue and coordination strategy
For orchestrator work queues:
- use `SELECT ... FOR UPDATE SKIP LOCKED LIMIT n`
- keep transactions short
- use `LISTEN/NOTIFY` as wake signal, not sole source of truth
- use advisory locks sparingly and with bounded key spaces

## 14) Memory, Retrieval, and Knowledge Lifecycle

### 14.1 Memory classes
- ephemeral run memory
- candidate memory (pending approval)
- durable approved memory

### 14.2 Candidate-to-durable flow
1. specialist emits candidate memory with provenance.
2. deterministic validators check schema/provenance freshness.
3. verifier role approves/rejects.
4. approved memory written versioned + reversible.

### 14.3 Retrieval strategy
- hybrid retrieval: lexical + vector.
- deterministic reranking policy before context packing.
- strict max context bytes/tokens per role.

### 14.4 Vector standards
- explicit `CREATE EXTENSION vector` migration per target DB.
- explicit ANN index strategy per workload:
  - HNSW for low-latency approximate retrieval.
  - IVFFlat when tuning for larger result windows/population conditions.
- fallback exact search path when index unavailable or low-population.

## 15) CLI + TUI Product Spec

### 15.1 CLI invocation
Canonical command shape:
- `omnidex chat`
- `omnidex run <objective>`
- `omnidex replay <run_id>`
- `omnidex inspect <run_id>`
- `omnidex config permissions <mode>`

### 15.2 TUI layout requirements
Terminal UI should present:
- conversation pane (user + assistant summaries)
- live action timeline pane (tool calls/state transitions)
- approval prompt pane (when needed)
- status/footer pane (mode, model, queue, budgets)

### 15.3 Stylized but truthful output rules
- clearly separate “thinking summary” from “actions executed”.
- never display fabricated execution events.
- render each side effect with timestamp + policy decision + result.
- keep display deterministic and replay-consistent.

### 15.4 User control affordances
- approve/deny once
- approve all in category for current run
- pause run
- abort run
- switch permission mode mid-session (with audit event)

### 15.5 Framework guidance
- CLI shell: Cobra (commands/flags/completions/help).
- TUI loop: Bubble Tea (MVU event model).
- TUI components: Bubbles (input, viewport, list, help).
- Styling: Lip Gloss.

### 15.6 Invocation contract (`odn`) and session persistence
Invocation behavior:
- Command alias: `odn` (short for `omnidexneo`).
- Default workspace context is the exact current working directory where `odn` is invoked.
- Session/chat persistence is workspace-scoped and reused on next invocation in the same workspace.
- Session identity is deterministic from workspace path + profile (no random workspace mapping).

Startup interaction:
- On startup, user selects permission mode: `ask_permission` or `full_access`.
- If no explicit selection, default is `ask_permission`.

Acceptance scenario (must pass):
1. User runs `odn` inside a target folder.
2. User prompt: “make a test project here in go lang and html”.
3. In `ask_permission` mode, agent asks for write permission before creating files/directories.
4. Once permitted, agent creates project files in current workspace context and verifies result.
5. Agent prints deterministic action timeline (planned -> approved -> executed -> verified).
6. On next `odn` run in same workspace, chat/run context is available for continuation.

Permission semantics for this scenario:
- `ask_permission`: reads allowed, writes blocked until approved.
- `full_access`: reads/writes allowed with full audit logging, no per-action prompt.

### 15.7 Intent gating (conversation vs execution)
The agent must distinguish between:
- `conversation_mode`: user is discussing architecture, brainstorming, or asking explanatory questions.
- `execution_mode`: user is requesting concrete actions (create/edit/run/fetch/deploy/etc.).

Deterministic requirement:
- A pre-router intent gate runs before tool routing.
- Output must be one of: `conversation_mode`, `execution_mode`, `ambiguous`.
- `conversation_mode` must not dispatch side-effecting tools.
- `execution_mode` may dispatch tools subject to permission mode and policy checks.
- `ambiguous` must trigger a short clarification question before any side effects.

Default behavior:
- If the user message contains direct action verbs plus target scope (for example “create X here”, “run tests”, “fix this file”), classify as `execution_mode`.
- If the user message is conceptual/spec-focused without a direct action request, classify as `conversation_mode`.
- If confidence is below configured threshold, classify as `ambiguous`.

Audit requirement:
- Every user turn logs `intent_classification`, `confidence`, and `reason_codes`.
- Any side-effecting run must include the intent-gate event ID in its run trace.

## 16) Observability and Forensics Standard

### 16.1 Every LLM call must log
- role/model/version
- full prompt hash + stored prompt reference
- full response hash + stored response reference
- parser result
- retry index
- latency and token metrics
- policy decisions linked to that call

### 16.2 Every tool execution must log
- normalized command or action descriptor
- policy check outcomes
- stdout/stderr artifact references
- exit code / status
- runtime resource usage
- verifier outcome

### 16.3 Telemetry stack baseline
- `log/slog` structured JSON logs
- OpenTelemetry traces and metrics
- correlation IDs across run/step/tool/llm spans

### 16.4 Debugging requirements
Given only persisted telemetry, an engineer must be able to answer:
1. Why was this tool chosen?
2. Why was this action allowed?
3. Which exact output failed parsing/verification?
4. Which retry path executed and why?
5. What minimal fix would have prevented failure?

## 17) Security and Privacy Standards

### 17.1 Secret handling
- Never place raw secrets in model prompts unless explicitly required and approved.
- Redact secrets from displayed TUI output.
- Store secret-bearing payloads encrypted at rest with restricted access.

### 17.2 Data exfiltration controls
- Web/network write actions always tiered high risk by default.
- Mandatory domain allowlist for web tools.
- Block direct uploads of local artifacts without explicit approval.

### 17.3 Prompt injection posture
- Treat all retrieved/web content as untrusted.
- Never allow retrieved text to alter policy, role contracts, or runtime mode.
- Keep system contracts out-of-band from retrieved context.

## 18) Failure Taxonomy
Stable error categories:
- `parse_error.*`
- `policy_violation.*`
- `tool_unavailable.*`
- `tool_timeout.*`
- `tool_nonzero_exit.*`
- `db_query_error.*`
- `verification_failed.*`
- `llm_transport_error.*`
- `llm_schema_error.*`
- `replay_divergence.*`

Retries are policy-mapped by error class with explicit max budgets.

## 19) Engineering Standards

### 19.1 Code organization
- Keep package responsibilities crisp.
- Avoid giant files and micro-fragmented helper sprawl.
- Prefer explicit interfaces over reflection-heavy indirection.

### 19.2 Testing matrix
- unit tests for all parsers, policies, reducers, transitions
- fuzz tests for CSV parser and shell parser
- integration tests for Ollama/DB adapters (mock + real)
- deterministic replay regression suite
- permission mode behavior tests

### 19.3 Performance SLO seeds
- router parse success >= 99.5%
- tool invocation success (excluding policy blocks) >= 98%
- replay decision consistency = 100% on golden runs
- failed-run root cause from telemetry < 10 minutes

## 20) Canonical Data Model (Planning Level)
Control/audit tables:
- `runs`
- `run_steps`
- `run_events`
- `llm_calls`
- `tool_registry`
- `role_registry`
- `role_versions`
- `tool_invocations`
- `delegation_edges`
- `artifacts`
- `verification_results`
- `error_catalog`

Retrieval/memory tables:
- `artifact_embeddings`
- `retrieval_queries`
- `context_packs`
- `memory_candidates`
- `memory_entries`

Queue/orchestration tables:
- `work_queue`
- `worker_leases`

### 20.1 Migration framework standard (Laravel-style behavior)
Migration system requirements:
- Timestamped ordered migration files (example: `20260507113000_create_runs_table.sql`).
- Each migration has explicit `up` and `down` operations.
- Migrations execute in tracked batches, enabling batch rollback.
- Applied migration metadata includes: `migration_id`, `batch`, `checksum`, `applied_at`, `applied_by`.
- Applied migrations are immutable; never edit historical migration files after apply.
- Seeders are separate from migrations; schema history and data bootstrap history must stay distinct.

Control table:
- `schema_migrations` (authoritative migration history for this project DB).

CLI contract target:
- `omnidex migrate` (apply pending `up` migrations)
- `omnidex migrate:status` (show applied/pending)
- `omnidex migrate:rollback` (rollback latest batch using `down`)
- `omnidex migrate:rollback --batch=<n>` (rollback target batch)
- `omnidex migrate:verify` (checksum and drift checks)

Safety rules:
- Default each migration to single-transaction apply when SQL allows.
- Preflight checks must fail fast on checksum mismatch or out-of-order history.
- Rollback must be tested for every migration before promotion to shared environments.

## 21) Critical Pitfall Register

### P0 (must fix before implementation starts)
1. Host cannot connect to Postgres (`expose` only, no host `ports`).
2. Go toolchain absent in active environment.
3. Ollama model inventory insufficient (`tinyllama` completion-only).
4. Seed path mappings inconsistent with existing DB declarations.

### P1 (must fix before beta)
1. Vector memory pipeline not provisioned (extension/index/population standards incomplete).
2. Excess superuser privileges for agent-reachable users.
3. Credentials-in-config workflow needs tightening for autonomous execution.
4. Migration discipline not yet established (no project-local migration ledger yet).

### P2 (track and mitigate)
1. Large SQL dump seed files can cause non-idempotent bootstrap drift.
2. Listeners/notifications can drift if long transactions hold queue cleanup.
3. Advisory lock misuse can exhaust lock pool under uncontrolled keying.

## 22) Pre-Implementation Readiness Gates
Implementation work is blocked until all of the following are true:

Gate A: Environment
- Go toolchain installed and version pinned.
- Ollama reachable and model set approved for role contracts.
- Postgres connectivity pattern selected and tested (host-published or containerized app).

Gate B: Contracts
- Router CSV grammar finalized.
- Role registry schema finalized.
- Permission mode matrix finalized.
- Error taxonomy and retry matrix finalized.
- Migration contract finalized (`up/down`, batching, checksums, rollback policy).

Gate C: Safety
- Command policy allow/deny baseline finalized.
- Secret redaction policy finalized.
- Web/domain allowlist policy finalized.

Gate D: Observability
- Required log fields locked.
- Trace/span naming convention locked.
- Replay artifact retention policy locked.

## 23) Phased Execution Plan

### Phase 0: Contract Freeze
Deliverables:
- contract specs for router + all initial specialists
- policy matrix
- schema migration plan

Exit:
- all readiness gates A-D accepted

### Phase 1: Core Scaffold
Deliverables:
- Go module scaffold
- FSM + event store
- run lifecycle APIs

Exit:
- deterministic replay of synthetic runs

### Phase 2: Ollama + Router
Deliverables:
- llm gateway
- router parser + retries
- role registry lookup path

Exit:
- fixture suite parse pass target met

### Phase 3: Tool Runtime + Permissions
Deliverables:
- tool registry
- command policy engine
- approval workflow

Exit:
- unsafe command fixtures blocked deterministically

### Phase 4: Verification Layer
Deliverables:
- deterministic validators
- verification specialist integration

Exit:
- seeded failure scenarios detected reliably

### Phase 5: Memory and Retrieval
Deliverables:
- embedding pipeline
- vector/lexical retrieval
- context packer

Exit:
- retrieval latency/quality targets achieved

### Phase 6: TUI Experience
Deliverables:
- chat + timeline + approval panes
- run replay mode
- permission-mode visibility controls

Exit:
- end-to-end local dogfooding in CLI/TUI

### Phase 7: Hardening
Deliverables:
- fuzz + load + chaos tests
- operational playbooks
- incident response workflows

Exit:
- beta readiness sign-off

## 24) Immediate Next Planning Outputs
No implementation yet. Next planning artifacts to produce from this bible:
1. `CONTRACTS.md` (all role I/O schemas and examples).
2. `POLICY_MATRIX.md` (permission modes x risk tier decisions).
3. `DB_MIGRATION_PLAN.md` (Laravel-style migration strategy, target schema, ordering, rollback flows).
4. `TUI_SPEC.md` (screen layout, keymaps, interaction flows).
5. `TEST_STRATEGY.md` (unit/fuzz/integration/replay suites and gates).

## 25) External References (Verified)
- Ollama API intro: https://docs.ollama.com/api
- Ollama chat endpoint: https://docs.ollama.com/api/chat
- Ollama structured outputs: https://docs.ollama.com/capabilities/structured-outputs
- Ollama tool calling: https://docs.ollama.com/capabilities/tool-calling
- Ollama streaming: https://docs.ollama.com/api/streaming
- Ollama usage metrics: https://docs.ollama.com/api/usage
- Ollama errors: https://docs.ollama.com/api/errors
- Ollama FAQ (queue/parallel/context): https://docs.ollama.com/faq
- PostgreSQL SELECT locking clause: https://www.postgresql.org/docs/current/sql-select.html
- PostgreSQL LISTEN: https://www.postgresql.org/docs/current/sql-listen.html
- PostgreSQL NOTIFY: https://www.postgresql.org/docs/current/sql-notify.html
- PostgreSQL explicit locking/advisory locks: https://www.postgresql.org/docs/current/explicit-locking.html
- PostgreSQL advisory lock functions: https://www.postgresql.org/docs/current/functions-admin.html
- PostgreSQL lock settings: https://www.postgresql.org/docs/current/runtime-config-locks.html
- pgvector: https://github.com/pgvector/pgvector
- Go `slog` blog: https://go.dev/blog/slog
- Go fuzzing: https://go.dev/doc/security/fuzz/
- Go `os/exec`: https://pkg.go.dev/os/exec
- Bubble Tea: https://github.com/charmbracelet/bubbletea
- Bubbles: https://github.com/charmbracelet/bubbles
- Lip Gloss: https://github.com/charmbracelet/lipgloss
- Cobra: https://github.com/spf13/cobra
