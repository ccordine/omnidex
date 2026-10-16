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
review state, checks, and completion. Planning uses normal unrestricted Codex tools
and receives the request, conversation, and one current remaining-ledger snapshot.
No planning response is
executable until deterministic validation and owner approval freeze a revision.

Execution receives only the selected task's goal, guidance, relevant files,
constraints, acceptance criteria, and checks. Each task gets its own normal Codex thread
with full access, native tools, and an ordinary text response. No Omnidex file tools
or structured executor response are supplied. The configured response parser reads
that final text and identifies completion, a question, a permission request, or an
error. Questions and permission requests wait for an owner reply on the same job
and task; they resume the same persisted native thread and do not reopen planning.
Native tool questions, approvals, and MCP elicitations retain the live turn while
the owner replies through the existing conversation. Permission grants require an
explicit owner choice; they are never inferred or automatically accepted.
Code owns every ledger transition.
The public waiting-state contract explicitly identifies reply versus replan;
clients do not infer that decision from a step name or response wording.

After reported completion, code runs the declared checks in the authoritative
workspace and dispatches a fresh reviewer with only that task and its observed
result. Execution, review, and checks use normal host tools, environment, and
network access. File lists are task context, not permission allowlists. Approved
execution survives a disconnected CLI while retaining its exact job/directory
identity, attempt lease, and explicit cancellation. A task advances only after its checks and review are recorded. Neither
executor output nor parser output alone marks ledger work complete.

Review returns ordinary Codex prose without an output schema. The same configured
response parser extracts its findings, reported verification, questions, permission
requests, or errors using only that response and the task's acceptance criteria.
Code binds the parsed result to the exact native review and frozen parser route.
Parser failures never replay completed native review work. A final review question
waits on the same job and task; the reply goes to a fresh disposable reviewer with
the task, previous question, and owner reply, not back to execution. The existing
end-of-run evidence audit is unchanged; no aggregate AI review is introduced.

Every new coding or parsing channel consumes the existing context compiler's
minification, without repeating raw history beside it or automatically injecting
project documents. Large inputs use its chunking rather than a native 128 KiB cap.
Exact source assignments and responses remain in the journal. The parser is pure
tool-free inference, not another executor. Its failures retain the queued job and
retry through the existing lease boundary without repeating completed native work.
Minification has no fixed result-size target or response ceiling. Its 2 KiB
threshold only skips unnecessary inference for already-small input; it does not
reject necessary context or force repeated compaction. Codex owns its real native
context window; the Ollama `INFERENCE_CONTEXT_TOKENS` setting cannot reject a
completed Codex turn. Exact prompts and their PostgreSQL evidence have no 128 KiB
input ceiling.

There is no three-round execution/review cutoff. The selected approved task
continues until verified, canceled, or in need of owner input. Task-local owner
clarifications reach execution, review, and continuation planning programmatically,
including after replanning.

Execution is never blindly replayed. Continuation metadata is journaled before
dispatch. Recovery reads the original native turn. Native context exhaustion,
compaction start, or interrupted work preserves the task as incomplete and opens
planning to inspect partial work and propose its remainder. An owner reply returns
the same job to mutable planning; execution cannot resume until the owner freezes
the ledger. The approved remainder, interrupted task, and prerequisites precede
untouched work, even after multiple planning turns. Exact accepted contracts carry
forward without reexecution. Long responses remain exact in the journal and project
atomically into bounded transcript parts. No aggregate AI reviewer is introduced.

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
