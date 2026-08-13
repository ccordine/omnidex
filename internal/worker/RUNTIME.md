# Worker Runtime

The worker consumes server-authoritative queue steps. It owns execution, evidence,
failure, and completion. A model can return only the bounded semantic leaf defined
by a registered assembly-line station.

## Production entry points

The native runtime registers exactly two actions:

- `objective_resolve` runs the code-owned free-form objective workflow.
- `v3_coding` runs the deterministic source assembly line directly for an explicit
  coding transport.

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

## Coding assembly line

Code owns application structure, paths, declarations, dependencies, scheduling,
workspace mutation, command selection, retries, diagnostics, verification, and
completion. A coding station receives one exact source-node contract and returns
one parseable declaration.

Greenfield writes and verification commands are selected and invoked directly by
the coding workflow. They do not pass through a model-visible registry. Existing
repository changes retain their exact snapshot, evidence, change-contract,
staging, sandbox, mutation-authority, and post-mutation verification gates.

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
