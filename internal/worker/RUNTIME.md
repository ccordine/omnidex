# Worker Runtime

The worker consumes server-authoritative queue steps. It owns execution, evidence,
failure, and completion. A model can return only the bounded semantic leaf defined
by a registered assembly-line station, using ordinary raw text which code alone
decodes into typed state.

## Production entry points

The native runtime registers exactly three actions:

- `objective_resolve` runs the code-owned free-form objective workflow.
- `v3_coding_plan` creates or revises one PostgreSQL planning ledger, then waits
  for the owner to approve and freeze it.
- `v3_coding` repeatedly queries the first incomplete approved ledger task, runs
  one fresh isolated execution, performs deterministic checks, runs one fresh
  isolated review, records acceptance, and queries again.

Every former conversation-stage action is rejected as unregistered. There is no
persona team, general planner, model verifier, model-selected operation, tool
catalog, or compatibility path in the worker.

## Objective resolution

The objective workflow preserves the exact instruction and queue identity. Code
opens only the station-specific semantic uncertainty needed to classify the
objective. It then dispatches a typed code-owned workflow, acquires required
evidence before synthesis, validates the bounded response, persists evidence, and
completes the same claimed step.

Models do not receive paths, queues, operations, tools, retries, completion state,
or unrelated context. Unsupported objectives and unavailable evidence providers
fail explicitly.

Before a closed semantic choice can reach a model, code resolves its applicable
option set. Zero options follow the station's explicit zero-option rule. One option
is consumed immediately with zero model-resolution and zero model-execution calls;
it is never rejected for being singular. Only a set of two or more options may be
rendered, and the model returns only one opaque option ID that code maps to the
already-known value.

When the unresolved result is a subset of code-known candidates, the worker repeats
that single-choice operation. It persists and removes each accepted candidate, then
renders the reduced set plus a semantic no-additional-choice alternative. Choice
letters are local to each reduced round; no model response carries the selected set.

## Coding task ledger

PostgreSQL owns the planning ledger, frozen task ledger, task order, dependencies,
review state, checks, and completion. Planning receives the broad request and the
bounded repository facts needed to construct the ledger. No planning response is
executable until deterministic validation and owner approval freeze a revision.

Execution receives only the selected task's goal, guidance, relevant files,
constraints, acceptance criteria, and checks. It never receives or controls the
remaining queue. Code owns workspace authority and every state transition. After
execution, code runs the declared checks from a sealed source tree and dispatches a
fresh reviewer with only that task and its observed result. A task advances only
after its checks and review are recorded. The executor and reviewer cannot select,
reorder, expand, reopen, or complete ledger work.

There is no alternate direct-coding pipeline. Missing ledger authority, invalid
routes, stale revisions, failed checks, and rejected review all fail explicitly.

## Authority and failure

- The claimed job, generation, step, attempt, and worker remain authoritative.
- Per-job model routing is immutable for the life of the claim.
- Every accepted side effect produces code-owned evidence.
- Invalid station output, missing evidence, stale authority, and unsupported
  actions fail loudly.
- Human correction updates the same job; it never creates a successor runtime.

## Extension rule

Add a strongly typed code-owned workflow or capability. If deterministic closure
stops at a genuine semantic uncertainty, add one small station contract for that
uncertainty. Never add a generic action selector, tool loop, role persona, planner,
or verifier.
