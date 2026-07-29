# Worker Runtime

The worker is a server-authoritative, stage-driven control plane. A stage either produces its declared artifact or fails explicitly; it must not substitute another stage's output.

## Native V3 authority chain

1. `v3_intent_parse` — the only role that receives raw current user text; emits the typed objective ledger.
2. `v3_capability_audit` — derives availability from the callable tool registry and blocks missing capabilities.
3. `v3_workspace_research` — runs only when `workspace.read` is required.
4. `v3_memory_retrieval` — runs only when `memory.read` is required and emits a bounded reference-only projection.
5. `v3_external_research` — runs before planning when `web.search` is required.
6. `v3_planning` — binds every objective, priority, criterion, capability, role, and subtask.
7. `v3_subtask` — coordinates one typed assignment. Mutation objectives enter the direct coding loop described below.
8. `v3_analysis` — synthesizes typed artifacts and delegated results without reinterpreting the prompt.
9. `v3_response_draft` — composes from analysis and independent evidence references.
10. `v3_verification` — challenges every objective without planner rationale or memory authority.
11. `v3_memory_review` — promotes only preference/reference candidates after a clean verification pass.
12. `v3_finalize` — accepts the exact verified draft; it never rewrites, guesses, or falls back.

Conversation transports always enter semantic interpretation. Code never guesses intent from greeting, verb, noun, or preferred phrasing lists.

An actionable free-form user turn must resolve to exactly one cohesive objective. For a typed workspace-write plus command-execute objective, the planner is bypassed by a code-built one-subtask coordinator plan. After the assembly line succeeds, code derives analysis and the user-facing summary from the accepted subtask result, independently validates persisted diff/test evidence, and forbids memory candidates. Planner, analysis, response, verifier, and memory models receive zero calls on that route.

The CLI has no parallel natural-language action router and no research sidecar. Every non-command chat message is sent unchanged to this authority chain. Host operations remain explicit typed commands. A removed legacy runtime setting is rejected at startup rather than ignored.

Human corrections remain on the same job. Interrupting a running coding step cancels that invocation, appends the exact feedback to the step, and requeues the same step against the current workspace. Replan restarts from the active coding/subtask step when one exists, or from planning for non-coding work; it never creates a successor job or restarts intent parsing.

## Contracts

- Specialist messages use the `1.0` envelope and exact registered `role_id`.
- Blocked/failed specialist outcomes are operational failures, not schema-repair candidates.
- Contract repair is bounded by the role's explicit retry policy, stays on the configured model, and receives current authority plus the exact failure without replaying rejected output.
- Subtask results preserve role, objective ID, priority, capabilities, evidence sources, and summary as typed JSON.
- Raw memory artifacts never enter downstream prompts. Instruction memories are always rejected; procedural memories require explicit recall and remain inert references.
- Every `memory.retrieve` call is bound to the accepted intent and server-derived project/session scope. Callers cannot supply authority scopes, and the tool returns only bounded objective-relevant excerpts plus explicit omission counts.
- Memory ordering uses embedding similarity plus exact typed session/project/trust/kind gates. Memory categorization uses only hard-typed kind and explicit category/scope tags. Code does not tokenize the request, apply stopword lists, or search memory sentences for matching phrases.
- Memory/model judgments cannot support independent verification claims.
- Action objectives cannot finalize without generated-diff or explicit side-effect execution evidence.
- Scrum Play is transport-authoritative execution; an advice-only intent is rejected before planning. Scrum channel messages remain classifiable as conversation or action.

## Execution truth

Coding and Scrum Play use one `v3_coding` step. General V3 mutation objectives route their execute subtask into that same runtime. There is no alternate coding engine, source-version ledger, hidden rollback store, model-owned command loop, or compatibility fallback.

The coding runtime is an assembly line whose stations have deliberately unequal authority:

1. **Semantic classifier and seed extractor (model)** — two bounded calls receive only the current direct instruction, authoritative Scrum card text when applicable, ordered user feedback, and opaque artifact tokens. The first emits only schema, shape, and language. Code selects the fixed surface and toolchain version. The second emits only labels and shape-specific values. Neither schema contains a path, document, plan, memory, tool, implementation, or orchestration field.
2. **Semantic validator, expander, and adapter selector (code)** — strictly decode both small responses, reject incompatible values, resolve opaque protected artifacts, expand all field types and behavioral relationships, and select one registered adapter solely from typed enums and relationships. There is no phrase matcher, model-generated software contract, or planner fallback.
3. **Blueprint compiler and scheduler (code)** — own documents, paths, signatures, static declarations, dependency waves, repair targets, import allowlists, and test declarations.
4. **Fragment transformer (model, only when needed)** — receives exactly one function signature, local behavior, direct API declarations, allowed package symbols, and for correction only its current declaration plus one path-free failure. It returns one function declaration.
5. **Parser, composer, and stager (code)** — prove the exact AST shape, retain accepted nodes, derive imports, format documents, and run the complete program in an isolated temporary stage. Partial graphs are never compiled or tested.
6. **Node correction controller (code)** — maps a compiler/test line to one block and then to exactly one declared generated owner. Only that function can be regenerated. Ambiguous routing and defects in code-owned declarations fail loudly.
7. **Mutation, verification, and completion controllers (code)** — write only the accepted staged assembly, prove workspace bytes and source membership match it, run fixed typed-program test commands, verify protected paths, derive the final summary/evidence verdict, and declare success without post-build model judgment.

Writes are ordinary Git working-tree changes. The runtime has no generated version store and no post-write file agent. Repeated identical node diagnostics and unchanged function replacements fail explicitly after bounded attempts. Human feedback remains on the same job, but planner-generated constraints and memory do not become coding-model authority.

## Observability

Every step emits start, error, and completion events. Specialist dispatch, contract acceptance/rejection, capability blocking, memory projection, tool calls, and independent challenge have focused events. LLM and tool operations emit a coalescible heartbeat only after eight seconds and every fifteen seconds thereafter, so long work stays visibly alive without flooding the UI.

Model routing is resolved into an immutable value for each job. Worker goroutines never mutate shared routing state, so concurrent jobs cannot leak model overrides into one another.

Feedback endpoints require the repository to return the same job ID. There is no successor-job response, supersession event, or restart-from-zero compatibility branch.

Decoded semantic state is retained across correction. A correction call can return exactly one top-level field, and code rejects it unless the merge changes exactly one JSON leaf. It cannot repeat or regenerate the seed, mutate an unrelated operation label, alter several artifact values, or submit a no-op.

CLI and API metadata is not a pretend control panel. Every accepted control must have an authoritative consumer and a behavior test. Legacy or write-only profile, planning, persistence, review, missing-tool, generic reasoning, autonomy, approval, verification, web, and workspace controls are rejected at the runtime boundary. Active per-role model routing, typed external-agent configuration, and the explicit research query are separate, consumed fields.

## Extension rules

- Add a focused stage/service file rather than expanding a monolith.
- Add typed artifacts and strict schemas before wiring a new role.
- Derive tool access from objective capabilities and the live registry.
- Add success, failure, and forbidden-fallback tests.
- Remove replaced paths and stale references before completion.
