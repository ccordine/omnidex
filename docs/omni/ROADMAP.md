# Omnidex Roadmap
## Deterministic Agentic Runtime For Hot-Swappable LLM Systems

Prepared: 2026-05-07  
Status: Planning Baseline v0.1

## 1) Objective
Build an open source LLM orchestration system where:
- Any supported model, or series of models, can be given practical agentic capabilities.
- Models are specialized into narrowly scoped roles.
- Orchestration is deterministic and state-driven.
- Tool selection and execution happen through explicit contracts.
- Tool execution happens through constrained, auditable patterns.
- Every LLM/tool step is logged deeply enough to reconstruct failures end-to-end.
- The system can scale to many delegated specialists, skills, and tools without becoming hard to debug or maintain.
- Weak or narrow models are forced to operate within their actual capabilities instead of being trusted with broad autonomous control.

This roadmap is implementation guidance for future contributor LLMs and human developers.

## 2) Hard Requirements (From Project Direction)
- Use Go for orchestration.
- Support local and hosted model providers through role-specific routing.
- Use PostgreSQL for queue, memory, and service-backed runtime state where persistence is needed.
- Do not rely on a single general model for whole-task completion.
- Specialist LLM outputs must use machine-validated contracts.
- Tool and skill specialists must operate with tightly bounded context.
- Include a terminal-command specialist and an independent verification specialist.
- Add extremely verbose logging across the full chain.

## 3) Design Principles
- Deterministic control plane: all workflow transitions happen in explicit code (finite-state machine + event log).
- LLMs are advisors, not controllers: models propose, code enforces.
- Strict contracts: every model output has a machine-validated grammar.
- Small context windows by role: no model receives unnecessary chain history.
- Policy-first execution: risky actions pass static checks before execution.
- Observability by default: every significant step produces trace, metrics, and structured logs.
- Replayability: any run can be replayed from persisted events/artifacts.
- Functional-core/imperative-shell style: pure decision logic in small testable functions, side effects isolated.
- Rich domain models and thin controllers: behavior lives in typed components, orchestration glue stays minimal.
- Functional object-oriented implementation: immutable value objects + behavior-centric interfaces + explicit state transitions.
- Bounded module size: avoid oversized files and ultra-granular methods that obscure root-cause analysis.
- Low-code-debt bias: every new role/tool must include tests, contracts, and telemetry before promotion.

## 4) Research-Constrained Architecture Decisions

### 4.1 Model provider behavior to design around
- Provider capabilities vary. A model may be good at language, weak at JSON contracts, poor at tool planning, or excellent at summarization.
- Runtime configuration must allow different models for interpretation, planning, shell command selection, summarization, verification, and final response.
- A role should fail visibly when the selected model cannot meet its contract.

### 4.2 Ollama API behavior to design around
- `/api/chat` supports both tools and structured formats (`json` or schema object).
- Streaming returns NDJSON chunks by default on streaming endpoints.
- Stream failures can arrive as NDJSON `error` objects after partial output has already been sent.
- Usage timing/token metrics are returned (`total_duration`, `load_duration`, `prompt_eval_count`, etc.) and must be persisted.
- Structured outputs are documented for local Ollama, while Ollama Cloud currently does not support structured outputs.
- Parallel request settings affect memory footprint (`OLLAMA_NUM_PARALLEL * OLLAMA_CONTEXT_LENGTH`).

### 4.3 PostgreSQL behavior to design around
- `FOR UPDATE SKIP LOCKED` is suitable for queue-like multi-consumer workers.
- `LISTEN/NOTIFY` is transaction-aware; notifications deliver on commit and between transactions.
- Advisory locks are useful for app-level coordination; transaction-level advisory locks auto-release at transaction end.
- Keep transactions short to reduce lock contention and notification delays.

### 4.4 Go observability/testing facts to use
- `log/slog` gives first-class structured logging in stdlib.
- OpenTelemetry Go provides standard traces/metrics/logs instrumentation.
- Go fuzzing is built into `go test`; fuzz targets should be deterministic and fast.

## 5) Target System Architecture

### 5.1 High-level flow
1. Ingest user objective.
2. Router LLM returns CSV tool sequence.
3. Deterministic parser normalizes and validates tool IDs.
4. Orchestrator dispatches each tool in order (or policy-approved parallel groups).
5. Tool outputs become typed artifacts.
6. Verifier tool(s) evaluate results.
7. Orchestrator decides next state: continue, retry, escalate, or complete.
8. Persist all events, raw model I/O, artifacts, and metrics.

### 5.2 Core components
- `orchestrator` (Go): FSM, retries, policies, context packing.
- `tool-registry` (Go + DB): canonical tool metadata and execution permissions.
- `llm-gateway` (Go): Ollama client, streaming handler, schema validation, metrics capture.
- `executor` (Go): built-in tool runners, terminal command runner, DB/API tools.
- `verifier-engine` (Go + LLM tools + deterministic checks): pass/fail/confidence.
- `memory-store` (Postgres + pgvector): artifacts, embeddings, run history retrieval.
- `telemetry` (slog + OTel): structured logs, traces, metrics.

### 5.3 Role graph (specialized LLMs)
- `router_llm`: output only `tool_a,tool_b,...`.
- `tool_specialist_*`: each tool has one capability boundary.
- `command_llm`: outputs terminal commands only (no prose).
- `verification_llm`: evaluates tool outputs against criteria; no action execution.
- `summarizer_llm` (optional): produces compact human-readable run summary.

No role can bypass orchestrator policy checks.

### 5.4 Initial specialist catalog (delegation targets)
- `pgsql_expert`: writes read-focused SQL for run history, prompt/response traces, memory lookup, and diagnostics.
- `web_researcher`: acquires external information through approved scraping/search tools and emits citation artifacts.
- `linux_expert`: generates command plans for environment setup, diagnostics, builds, and runtime operations.
- `doc_manager`: decomposes large-document analysis tasks into worker assignments.
- `doc_worker`: parses assigned chunks, extracts claims/facts, and writes structured memory candidates.
- `vlc_specialist`: handles media-control and playback workflows via constrained tool APIs.
- `coding_specialist`: generates or refines code artifacts within repo policy boundaries.
- `software_architect`: evaluates architecture tradeoffs and emits design constraints/target interfaces.
- `planning_specialist`: converts objective into milestones, dependencies, and execution plans.
- `strategy_specialist`: optimizes sequencing, risk posture, and resource allocation across roles.

Role onboarding rules:
- Every role must define: `purpose`, `allowed_tools`, `input_contract`, `output_contract`, `failure_modes`, `telemetry_fields`.
- Every role must have a strict output grammar and deterministic parser.
- Every role must declare memory read/write permissions.
- Every role version must be immutable once promoted; upgrades create new versions.

### 5.5 Manager-worker delegation pattern
- Manager roles may split tasks only through deterministic chunking and explicit worker envelopes.
- Workers never self-spawn or change scope; they execute assigned slice and return typed artifacts.
- Manager merges worker artifacts through deterministic reducers (not free-form synthesis only).
- All delegation edges are persisted as graph links for replay and forensics.

## 6) Deterministic Contracts

### 6.1 Router CSV contract
- Allowed charset for tool IDs: lowercase `[a-z0-9_]+`.
- Separator: comma only.
- No spaces in canonical output.
- Empty output allowed and interpreted as `no_tool_selected`.
- Max tool count per routing step configured (example: 8).

Validation rules:
- Unknown tool IDs are rejected.
- Duplicate IDs are canonicalized (keep-first) unless tool is marked repeatable.
- Parse failure triggers deterministic retry path with explicit error feedback.
- After max retries, transition to `ROUTER_FAILED`.

### 6.2 Tool invocation contract
Internal envelope (not model-facing):
- `run_id`
- `step_id`
- `tool_id`
- `tool_version`
- `input_artifact_ids`
- `policy_snapshot_id`
- `deadline_ms`

Each tool writes:
- `output_artifact_ids`
- `status` (`success|fail|partial|blocked`)
- `error_code` (if any)
- `timings_ms`

### 6.3 Command tool contract
`command_llm` output:
- Shell command lines only.
- No markdown, no explanation, no comments.

Deterministic checks before execution:
- Tokenize + parse shell.
- Denylist patterns (destructive/exfiltration).
- Allowlist command families by trust tier.
- Path sandbox checks.
- Max runtime and output size guardrails.

Execution results pass into verifier before being accepted as final.

### 6.4 Specialist output contracts (examples)
- `pgsql_expert`: outputs a single SQL statement or deterministic statement list in approved grammar.
- `pgsql_expert` default mode is read-only (`SELECT`/CTE); write access requires explicit policy escalation.
- `pgsql_expert` must reference only allowlisted schemas/tables and parameter placeholders.
- `web_researcher`: outputs deterministic fetch plan IDs mapped to approved scraper/search tools.
- `linux_expert`: outputs command plan lines only and must pass command policy parser before execution.
- `planning_specialist`: outputs milestone graph in schema form (not prose-only free text).
- `strategy_specialist`: outputs ranked execution options with policy-compliant decision metadata.

### 6.5 Delegation contract
- Manager outputs worker assignment envelopes: `worker_role`, `scope_ref`, `acceptance_checks`, `deadline_ms`.
- Worker outputs must map to assigned scope only and include provenance back to source artifacts.
- Merge step accepts worker outputs only if all required acceptance checks passed.

## 7) PostgreSQL Data Model (Roadmap-Level)

### 7.1 Control and audit tables
- `runs`: one row per top-level request.
- `run_steps`: orchestrator state transitions.
- `run_events`: append-only event stream (source of truth for replay).
- `llm_calls`: model metadata, prompts/responses, token/timing metrics.
- `tool_registry`: tool definitions, policy tags, versioning.
- `role_registry`: specialist role definitions, contracts, and capability metadata.
- `role_versions`: immutable role/prompt snapshots for reproducibility.
- `tool_invocations`: execution attempts and outcomes.
- `delegation_edges`: manager-to-worker assignment graph per run.
- `artifacts`: typed outputs (text/json/diff/log/chunk/etc).
- `verification_results`: deterministic and LLM-based verification records.
- `error_catalog`: normalized error taxonomy.

### 7.2 Retrieval/memory tables
- `artifact_embeddings`: vector rows mapped to artifacts/chunks.
- `retrieval_queries`: what context retrieval asked for and returned.
- `context_packs`: exact slices delivered to each model call.
- `memory_entries`: approved durable memories with provenance links.
- `memory_candidates`: extracted candidates pending merge/approval.

### 7.3 Queue/orchestration tables
- `work_queue`: pending runnable tasks.
- `worker_leases`: active claim/heartbeat information.

Use queue polling pattern:
- `SELECT ... FOR UPDATE SKIP LOCKED LIMIT n`
- short transactions
- optional `LISTEN/NOTIFY` for wake-up signals.

Use advisory lock keys for singleton or sharded critical sections where needed.

## 8) State Machine

Recommended initial states:
- `RUN_CREATED`
- `ROUTING_REQUESTED`
- `ROUTING_PARSED`
- `ROUTING_INVALID`
- `TOOL_DISPATCHED`
- `TOOL_RUNNING`
- `TOOL_COMPLETED`
- `TOOL_FAILED`
- `DELEGATION_PLANNED`
- `WORKER_DISPATCHED`
- `WORKER_COMPLETED`
- `WORKER_FAILED`
- `MERGE_PENDING`
- `MERGE_COMPLETED`
- `VERIFY_PENDING`
- `VERIFY_PASSED`
- `VERIFY_FAILED`
- `RECOVERY_PENDING`
- `RUN_COMPLETED`
- `RUN_ABORTED`

Rules:
- Every transition must be explicit and persisted.
- Transition function must be pure relative to input state + event.
- Side effects (LLM calls, command execution, DB writes) happen via effect handlers and emit events.
- Idempotency keys required for all external calls.
- Role selection must be resolved from registry and policy, never from free-form role names in model text.
- Manager-to-worker fanout must be bounded by deterministic limits (`max_workers_per_step`, `max_depth`).

## 9) Logging and Observability (Mandatory Depth)

### 9.1 Per LLM call logging fields
- `run_id`, `step_id`, `llm_role`, `model`, `endpoint`
- `system_prompt_hash`, `input_context_hash`
- `raw_prompt` (stored securely)
- `raw_response` (stored securely)
- `parsed_output`
- `parse_success`, `parse_error_code`
- `request_started_at`, `request_finished_at`
- `prompt_tokens`, `completion_tokens`, `total_duration_ns`, `load_duration_ns`
- `retry_index`
- `policy_decisions`

### 9.2 Per tool execution logging fields
- `tool_id`, `tool_version`, `runner_type`
- `input_artifact_ids`, `output_artifact_ids`
- `stdout_ref`, `stderr_ref`, `exit_code`
- `started_at`, `finished_at`, `duration_ms`
- `resource_usage` (where available)
- `verification_status`

### 9.3 Telemetry stack
- Structured app logs via `log/slog` JSON handler.
- Distributed tracing via OpenTelemetry (run -> step -> llm/tool spans).
- Metrics for:
  - router parse failure rate
  - unknown tool rate
  - tool success/failure rate
  - verification pass rate
  - retries per run
  - p50/p95 latency per role/tool
  - token usage by role/model

## 10) Safety and Reality Anchoring

### 10.1 Terminal execution guardrails
- Default deny for non-allowlisted commands.
- Block shell metacharacter chains unless explicitly policy-approved.
- Enforce working-directory boundaries.
- Enforce maximum command count per step.
- Enforce timeout, output truncation, and memory limits.
- Snapshot every command before execution for audit.

### 10.2 Verification layering
- Deterministic validators first (exit codes, schema checks, file existence, git diff sanity).
- Then verification LLM for semantic checks.
- Optional second verifier model for disagreement resolution on high-risk actions.
- Final accept/reject remains deterministic policy code.

### 10.3 Failure taxonomy
Define stable error classes:
- `parse_error.*`
- `policy_violation.*`
- `tool_unavailable.*`
- `execution_timeout.*`
- `execution_nonzero_exit.*`
- `verification_failed.*`
- `llm_transport_error.*`
- `llm_schema_error.*`

All retries keyed by error class + retry budget policy.

### 10.4 Maintainability guardrails
- Keep files intentionally sized; split modules when a file becomes difficult to reason about.
- Prefer cohesive packages over deeply fragmented helpers.
- Keep controller/orchestrator code focused on state transitions and coordination only.
- Keep business logic in typed role/tool components with unit tests.
- Require lint, tests, and telemetry checks for any new role/tool before merge.
- Require architecture decision records for cross-cutting changes to prevent drift and hidden debt.

## 11) Memory and Context Strategy

### 11.1 Context minimization
Each role receives only:
- role-specific instruction
- current objective fragment
- bounded artifact excerpts
- relevant tool capability cards

No role gets full chain transcript by default.

### 11.2 Retrieval strategy
- Store artifact chunks and embeddings in Postgres/pgvector.
- Prefer cosine search (`vector_cosine_ops`) for semantic retrieval.
- Keep lexical fallback (keyword match) for exact identifiers.
- Re-rank retrieved snippets deterministically (scoring policy) before packing context.

### 11.3 pgvector tuning starting points
- Use HNSW for low-latency recall tradeoff where suitable.
- Tune `hnsw.ef_search` per query profile.
- Consider iterative index scans for filtered ANN queries.
- For IVF use-cases, create index after sufficient data population.

### 11.4 Memory lifecycle
- Specialized roles (`web_researcher`, `doc_worker`, `pgsql_expert`) write `memory_candidates`, not direct durable memory.
- Deterministic validators and verifier role approve/reject candidate memories.
- Approved memories are merged into `memory_entries` with source artifact links.
- Memory writes are versioned and reversible for forensic rollback.

## 12) Delivery Roadmap (Phased)

### Phase 0: Foundations and Contracts
Deliverables:
- Finalized role definitions and output grammars.
- Role registry schema and versioning policy.
- Error taxonomy and retry matrix.
- Tool registry schema.
- Security policy baseline for command execution.

Exit criteria:
- All contracts documented and test vectors approved.

### Phase 1: Orchestrator Skeleton
Deliverables:
- Go project scaffold.
- FSM engine with persistent `run_events`.
- Basic `runs` and `run_steps` tables.

Exit criteria:
- Deterministic replay of synthetic runs works.

### Phase 2: Ollama Gateway + Router
Deliverables:
- Ollama chat client wrapper.
- Router role prompt + CSV parser/normalizer.
- Parse error handling and retry logic.

Exit criteria:
- Router can select valid tools with >99% parse success on fixture suite.

### Phase 3: Tool Registry + Execution Core
Deliverables:
- Tool metadata and permission model.
- Role-to-tool permission mapping.
- Executor framework for built-in tools.
- Queue worker loop (`SKIP LOCKED`) with leases.

Exit criteria:
- Parallel workers process queue without duplicate execution.

### Phase 3.5: Delegation Engine (Manager/Worker)
Deliverables:
- Deterministic chunk planner for large-context/document tasks.
- Worker envelope schema and assignment graph persistence.
- Deterministic merge reducers for worker outputs.

Exit criteria:
- Large input tasks execute through bounded fanout and produce replayable merge decisions.

### Phase 4: Command Specialist + Guarded Runner
Deliverables:
- Command-only LLM role.
- Static command policy engine.
- Sandboxed execution adapter with captured artifacts.

Exit criteria:
- Unsafe command fixtures are blocked deterministically.

### Phase 5: Verification Layer
Deliverables:
- Deterministic validators (schema/exit/diff checks).
- Verification LLM role.
- Final policy gate (accept/reject/retry/escalate).

Exit criteria:
- Verification catches seeded failure cases in eval suite.

### Phase 6: Memory + Retrieval
Deliverables:
- Artifact chunking and embedding ingestion.
- pgvector retrieval APIs.
- Context packer for role-specific minimal context.
- Memory candidate pipeline and approval/merge flow.

Exit criteria:
- Retrieval quality and latency meet target SLOs.

### Phase 7: Observability and Debug UX
Deliverables:
- Full slog + OTel integration.
- Run timeline viewer API (or query layer).
- Failure forensics report generator.

Exit criteria:
- Any failed run is explainable from stored traces/events only.

### Phase 8: Hardening and Beta
Deliverables:
- Load test, chaos test, and fuzz test coverage.
- Operational playbooks (oncall, rollback, incident triage).
- Versioned migration and upgrade process.

Exit criteria:
- Stable beta with defined SLIs/SLOs and release checklist.

## 13) Test and Evaluation Strategy

### 13.1 Deterministic unit tests
- CSV parser and canonicalizer.
- FSM transition legality.
- Policy engine command parsing.
- Retry policy matrix.

### 13.2 Fuzz tests
- Router CSV parser fuzzing.
- Shell command parser fuzzing.
- Event replay integrity fuzzing.

### 13.3 Integration tests
- Ollama mock and real-instance smoke tests.
- DB queue concurrency tests with multiple workers.
- End-to-end tool-chain scenarios with seeded failures.

### 13.4 Golden run replay
- Persisted historical runs re-executed against same artifacts.
- Assert deterministic orchestrator decisions are unchanged.

## 14) Initial SLO Candidates
- Router parse success: `>= 99.5%`.
- Tool invocation success (excluding policy blocks): `>= 98%`.
- Verification false-accept rate: `< 1%` on curated regression suite.
- Time-to-root-cause for failed run from logs: `< 10 min`.
- Replay determinism: `100%` decision consistency for golden fixtures.

## 15) Immediate Next Actions
1. Approve/adjust role list, role contracts, and CSV grammar constraints.
2. Approve initial command allowlist/denylist policy.
3. Create DB migration set for core audit/control/role/memory tables.
4. Implement Phase 1 scaffold (FSM + event persistence) before any advanced tool logic.

## 16) Reference Sources
- Ollama API Introduction: https://docs.ollama.com/api/introduction
- Ollama Chat API: https://docs.ollama.com/api/chat
- Ollama Tool Calling: https://docs.ollama.com/capabilities/tool-calling
- Ollama Structured Outputs: https://docs.ollama.com/capabilities/structured-outputs
- Ollama Streaming Capability: https://docs.ollama.com/capabilities/streaming
- Ollama API Streaming: https://docs.ollama.com/api/streaming
- Ollama API Usage Metrics: https://docs.ollama.com/api/usage
- Ollama API Errors: https://docs.ollama.com/api/errors
- Ollama FAQ (concurrency, queueing, keep_alive, context sizing): https://docs.ollama.com/faq
- pgvector repository/readme: https://github.com/pgvector/pgvector
- PostgreSQL SELECT / SKIP LOCKED: https://www.postgresql.org/docs/current/sql-select.html
- PostgreSQL LISTEN: https://www.postgresql.org/docs/current/sql-listen.html
- PostgreSQL NOTIFY: https://www.postgresql.org/docs/current/sql-notify.html
- PostgreSQL Explicit Locking (advisory locks): https://www.postgresql.org/docs/current/explicit-locking.html
- PostgreSQL system admin lock functions: https://www.postgresql.org/docs/current/functions-admin.html
- Go structured logging (`slog`): https://go.dev/blog/slog
- OpenTelemetry for Go: https://opentelemetry.io/docs/languages/go/
- Go fuzzing guidance: https://go.dev/doc/security/fuzz/
