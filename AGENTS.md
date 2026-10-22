# Omnidex

These instructions apply to Codex developing this repository. The product workflow below describes Omnidex's runtime; it is not a prerequisite ledger or approval process for Codex to edit, test, or maintain Omnidex. Follow the owner's current request and the global working agreement. This file supersedes conflicting older project design guidance.

## Acceptance: it works in the actual software

- Implement the requested behavior and run the relevant automated checks. Then, before reporting completion, pilot Omnidex through a focused gauntlet that exercises the actual change.
- Build and run the version containing the change. Verify which executable/version is running; source-only changes or tests against an older installation cannot establish acceptance.
- Use the normal CLI/UI and the real providers and dependencies required by the changed workflow. Observe the actual outcome, relevant failure/cancellation paths, elapsed time, and how much user intervention or tweaking is required.
- If the requested behavior fails, fix the responsible implementation, rerun affected automated checks, and repeat the final pilot. Keep the work scoped to the owner's request.
- Record the version, scenarios, observed results, and intervention in the existing task/evidence record. A passing build or automated suite is insufficient; the changed behavior must actually work in Omnidex.
- If a necessary capability is inaccessible, report the exact blocker and incomplete acceptance. Do not present unpiloted behavior as finished. Documentation-only edits require content and consistency checks; runtime-affecting configuration or instructions must be exercised in their actual consumer.

## Product workflow and model boundaries

- Code owns authoritative state, identities, versions, queues, ordering, persistence, filesystem actions, tool dispatch, retries, cancellation, and verification. Model claims of success do not establish completion.
- Omnidex coding uses one finite, owner-approved PostgreSQL ledger. Code selects the first incomplete item, opens a fresh Codex execution session with that item's frozen context, then a fresh independent review session for the same item, records the result, and advances. Models do not select or expand the queue. Do not add another ledger, aggregate workload controller, or alternate completion path.
- Those product execution/review sessions may use tools within their task authority. Other model calls answer one necessary semantic question that code cannot determine, with only the relevant context. Do deterministic work in code; do not invent model stages merely to approve, classify again, restate, or manufacture convergence.
- Keep task instructions and their supporting details together on the ledger item: objective, owner context, constraints, file responsibilities, acceptance criteria, and verification when supplied. Code supplies identity, provenance, and lifecycle metadata. Do not require artificial field pairings or scan/group/re-extract pipelines to retain those details.
- Do not throw errors or stop work because AI output differs from a preferred format, grouping, ordering, schema, or code-owned structure. Code handles mechanical representation and retains usable meaning. A genuine unresolved semantic need must be addressed specifically without discarding accepted work, guessing success, or restarting the whole task.
- Do not introduce error-throwing content-size/count limits or active-work timeouts. Genuine provider failures and unavailable capabilities must be reported truthfully. Preserve explicit cancellation and authorization/lease validity checks without turning them into arbitrary work deadlines.
- Preserve the same authoritative job and accepted state through feedback, interruption, and corrections. Per-job model routing remains job-local and stable.
- Ordinary user text enters the established semantic request boundary unchanged; do not route meaning through keyword heuristics or invent side workflows.
- CLI Ctrl+C clears a nonempty draft first. With no draft, it retains the existing cancellation behavior.

## Existing implementation boundaries

- The core is Go; the web interface uses TypeScript with existing Stimulus, RecyclrJS, and Tailwind patterns. Use language-appropriate composition, not PHP trait or framework prescriptions.
- Server state is authoritative. Preserve server-rendered surfaces and the existing React/TypeScript Scrum card modals; do not impose one rendering rule on both or add competing renderers.
- PostgreSQL owns durable state across restarts. Redis/Valkey is ephemeral. Preserve tasks, accepted work, context, evidence, credentials, and user data.
- database/setup.sql remains the sole internal schema definition. Preserve existing state; do not reset databases to make a pilot pass, manually patch schemas, or invent an internal migration/upgrade system. Generated workload migrations remain workload-owned.
- Keep responsibility and typed boundaries clear. Remove only code superseded by the requested change; do not use this file as a reason for unrelated refactoring.
- Use calm, accessible UI and truthful progress. Loading feedback must respect reduced motion; every interaction does not need animation.

## Piloting workloads

- Codex changes Omnidex; Omnidex produces the application used to test it. Use ordinary requests and inspect the real result. Do not hand-build, scaffold, patch, or supply intermediate implementation hints to the workload.
- Fix task-neutral framework defects between runs. Do not change framework source during an evidence run or encode the current workload's domain into framework helpers, prompts, templates, or adapters.
- Use disposable workload fixtures without cloning the Omnidex source. For an autonomy claim, use a fresh workload workspace, keep evaluation criteria out of the builder's context, and record model inputs/results and interventions. A steered run is not evidence of autonomous completion.
- Functional interaction tests may deliberately exercise user edits, clarification, and cancellation; describe that interaction accurately. Keep elaborate autonomy-benchmark requirements scoped to autonomy claims.

## Reference material

Read only references relevant to the affected implementation. They provide context, not permission to expand scope or revive superseded size limits, timeouts, AI-format errors, mandatory semantic pipelines, or development approval gates. Build codenames are version labels, not subsystems.

- [Build, run, and usage](README.md)
- [Existing evidence record](docs/EVIDENCE_LEDGER.md)
- [Assembly-line design](docs/CHARMANDER_ASSEMBLY_LINE.md), [context system](docs/CHARMELEON_CONTEXT_SYSTEM.md), [cognition resolution](docs/CHARMELEON_COGNITION_RESOLUTION.md)
- [Job/objective/task design](docs/CHARMELEON-EMERGENT-ORCHESTRATION-INVARIANTS.md), [artifact graph design](docs/CHARMELEON-CONVERGENT-COGNITIVE-DEVELOPMENT-INVARIANTS.md)
- [Tree planning](docs/TARGET_TREE_PLANNING.md), [artifact adapters](docs/ARTIFACT_ADAPTERS.md), [roleplay simulation](docs/ROLEPLAY_SIMULATION.md)
