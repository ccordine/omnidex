# Worker Runtime

The worker runtime is a stage-driven execution path used by `internal/worker.Service.processStep`.

## Goals
- Keep queue/API contracts stable (`job_steps.action`, `step_contexts` keys).
- Make agent behavior easier to reason about with one clear input/output per stage.
- Preserve compatibility heuristics already covered by tests.

## Stage Contract

Each action reads prior step contexts and writes one canonical context key:

- `tooling` -> `tooling`
- `workspace_scan` -> `workspace`
- `tag` -> `tags`
- `retrieve` -> `retrieval`
- `plan` -> `plan`
- `web_search` -> `web_search`
- `analyze` -> `analyzer`
- `assist|roleplay|narrate` -> same action key
- `verify` -> `verification`

The final job result still comes from the final `verify` step output.

## Orchestration Notes

- Search remains explicit: `web_search=off|auto|on`.
- Retrieval remains scoped: session/project tags + embedding search.
- Plan remains JSON-first and deterministic fallback is always available.
- Response always appends a `Sources` section when missing.
- Verification runs grounding checks and can trigger `replan` when persistent execution is enabled.

## Extending Runtime

Add logic in `runtime_v2.go` by stage (`runTooling`, `runPlanning`, etc.) rather than expanding a monolithic switch body.

Guidelines:
- Keep each stage idempotent.
- Emit concise step events for observability.
- Prefer completion with explicit skip reasons over silent no-ops.
- Use existing helper heuristics for consistency with tests and CLI behavior.
