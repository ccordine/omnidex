# Worker Runtime

The worker is a server-authoritative, stage-driven control plane. A stage either produces its declared artifact or fails explicitly; it must not substitute another stage's output.

## Native V3 authority chain

1. `v3_intent_parse` — the only role that receives raw current user text; emits the typed objective ledger.
2. `v3_capability_audit` — derives availability from the callable tool registry and blocks missing capabilities.
3. `v3_workspace_research` — runs only when `workspace.read` is required.
4. `v3_memory_retrieval` — runs only when `memory.read` is required and emits a bounded reference-only projection.
5. `v3_external_research` — runs before planning when `web.search` is required.
6. `v3_planning` — binds every objective, priority, criterion, capability, role, and subtask.
7. `v3_subtask` — coordinates one typed assignment. Mutation objectives are decomposed into a durable implementation ledger; each model invocation receives one file/review/triage contract rather than the whole app.
8. `v3_analysis` — synthesizes typed artifacts and delegated results without reinterpreting the prompt.
9. `v3_response_draft` — composes from analysis and independent evidence references.
10. `v3_verification` — challenges every objective without planner rationale or memory authority.
11. `v3_memory_review` — promotes only preference/reference candidates after a clean verification pass.
12. `v3_finalize` — accepts the exact verified draft; it never rewrites, guesses, or falls back.

Low-signal greetings use the deterministic `v3_chat_fastpath` and do not invoke memory or model roles.

Human corrections never attach to whichever specialist happens to be running. Interrupt, replan, and non-approval feedback cancel the immutable prior run, create a linked authority revision, and restart at `v3_intent_parse`. Ordered `v3_authority_directives` preserve current-thread user authority without replaying compiled channel history. Artifacts, evidence, claims, and bot outputs remain isolated under their original job IDs. Scrum card job binding changes in the same PostgreSQL transaction as the revision.

## Contracts

- Specialist messages use the `1.0` envelope and exact registered `role_id`.
- Blocked/failed specialist outcomes are operational failures, not schema-repair candidates.
- One schema repair is allowed only for malformed contract output when the role explicitly declares `one_repair_pass`.
- Subtask results preserve role, objective ID, priority, capabilities, evidence sources, and summary as typed JSON.
- Raw memory artifacts never enter downstream prompts. Instruction memories are always rejected; procedural memories require explicit recall and remain inert references.
- Every `memory.retrieve` call is bound to the accepted intent and server-derived project/session scope. Callers cannot supply authority scopes, and the tool returns only bounded objective-relevant excerpts plus explicit omission counts.
- Memory/model judgments cannot support independent verification claims.
- Action objectives cannot finalize without generated-diff or explicit side-effect execution evidence.
- Scrum Play is transport-authoritative execution; an advice-only intent is rejected before planning. Scrum channel messages remain classifiable as conversation or action.

## Execution truth

The internal coding engine has no synthetic coder, no no-op test writer, and no always-pass validator. It requires concrete configured implementations. If none are available, the coding job fails explicitly instead of reporting fake completion.

Mutation objectives have one durable, server-owned implementation ledger. The manifest planner receives the objective and a current workspace snapshot once. Every file worker receives only its file contract, target, declared dependency files, and latest direct correction. A separate model reviews that candidate. The server alone writes the assigned file and runs the ledger's shell-free, allowlisted verification command. Failed command output goes to a bounded triage model, which names exactly one authorized file owner; only that item reopens, while unrelated completed items remain intact. No memory, conversation replay, planner rationale, free-form debate, concurrent writer, or hidden legacy execution path enters this loop.

Each ledger transition is appended as an artifact. Files have one writer and a content hash. Completion requires every file contract, an independent review, a successful verification item, and unchanged server-observed file hashes. Invalid manifests, unsafe commands, context overflow, path drift, placeholders, ambiguous ownership, and exhausted item budgets fail explicitly.

## Observability

Every step emits start, error, and completion events. Specialist dispatch, contract acceptance/rejection, capability blocking, memory projection, tool calls, and independent challenge have focused events. LLM and tool operations emit a coalescible heartbeat only after eight seconds and every fifteen seconds thereafter, so long work stays visibly alive without flooding the UI.

## Extension rules

- Add a focused stage/service file rather than expanding a monolith.
- Add typed artifacts and strict schemas before wiring a new role.
- Derive tool access from objective capabilities and the live registry.
- Add success, failure, and forbidden-fallback tests.
- Remove replaced paths and stale references before completion.
