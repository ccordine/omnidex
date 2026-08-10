# Omnidex software-defined context — Charmeleon build

Status: authoritative target architecture; implementation and promotion gates are explicit below.

Charmeleon is only the codename for this Omnidex build. It is not a subsystem,
runtime, framework, worker, agent, or product. The bounded coding assembly line in
[`CHARMANDER_ASSEMBLY_LINE.md`](CHARMANDER_ASSEMBLY_LINE.md) remains the coding
authority foundation.

## Objective

Omnidex must continue a long task, understand a large repository, and construct one
small model call without making a model remember the repository or execution history.
Repository size may increase indexing and storage cost. It must not determine prompt
size. The requested change surface determines prompt size.

The context system has five separate authorities:

| Layer | Purpose | Lifetime | Authority |
| --- | --- | --- | --- |
| Repository Intelligence | Current files, symbols, edges, tests, routes, configuration, and hashes | Repository snapshot | Source and tool derived |
| Task Ledger | Goals, constraints, execution graph, decisions, questions, failures, and checkpoints | One job initially | Typed and provenance-labelled |
| Working Set | References currently resident for one scope | Call, step, task, objective, or job | Code managed |
| Context Projection | Exact immutable material rendered for one inference | One model call | Code selected and immutable |
| Durable Memory | Explicitly promoted cross-job preferences, references, and lessons | Cross-job | Reference only |

These layers must not share tables merely because all of them can be called memory.
Durable memory is historical reference. Repository intelligence is disposable,
hash-bound derived state. The task ledger is current execution state. A working set is
attention. A context projection is evidence of one call.

## Authority flow

```text
repository facts ───────┐
durable memory ─────────┤ acquire by typed provider
artifacts and evidence ─┤
user authority ─────────┘
                         ↓
                    Task Ledger
                         ↓
                    Working Set
                         ↓
                  Context Builder
                         ↓
                Context Projection
                         ↓
                       Model
                         ↓ bounded proposals/requests
                  Code coordinator
```

The model may forget everything after every call. Omnidex must not.

The domain-neutral coordinator that consumes these authorities is specified in
[`CHARMELEON_COGNITION_RUNTIME.md`](CHARMELEON_COGNITION_RUNTIME.md). Its first
procedural proof environment is the separately isolated offline laboratory defined in
[`LABYRINTH_GAUNTLET.md`](LABYRINTH_GAUNTLET.md). Neither benchmark mechanics nor
private evaluation authority may enter this context substrate.

## Task Ledger

PostgreSQL is canonical. One transaction updates normalized current state, appends one
audit event, and increments the ledger version. Optimistic version conflicts fail
explicitly. Pure event replay is an audit and recovery proof, not the normal read path.

The first supported owner is one job. Card- and project-scoped ledgers remain disabled
until job continuity is proven. Job enqueue creates the authoritative telemetry run,
the exact job/run-bound ledger, one code-owned root goal, and one hash-bound reference
to the unchanged user instruction in the same transaction; none may commit without the
others, and no separate ledger-creation API exists. Claim, success, failure, and
cancellation transition that root atomically with job authority.

The execution graph contains goals, objectives, tasks, checkpoints, and change groups.
Dependencies and verification relationships are explicit edges. Current status is one
of pending, ready, active, blocked, done, failed, or canceled.

Entries have epistemic type and provenance. Initial kinds are constraints, facts,
observations, hypotheses, decision candidates, accepted decisions, questions,
failures, checkpoints, notes, and typed user feedback. Authority is one of user, code, tool evidence, model
proposal, or accepted model decision.

Rules:

- A fact has at least one valid evidence reference.
- A model may propose hypotheses, questions, and decision candidates. It cannot create
  user, code, or tool authority.
- A model cannot accept its own decision, transition execution state, or declare work
  complete.
- Code records the policy and evidence that accept a model-originated decision.
- Rejected, resolved, and superseded entries remain in history and are omitted from
  normal active projections.
- Stable references identify repository facts, artifacts, evidence, memory, web
  evidence, and task state without copying their bodies.
- A source-bound reference whose version or hash no longer matches is invalidated and
  reacquired before projection.

The initial immutable plan remains evidence of what was authorized. Mutable task nodes
record execution under that authority; they do not rewrite the plan artifact. Direct
coding transports that do not emit an intent or plan artifact must project their
accepted application specification and requirements before generation begins; they may
not run without task authority merely because their transport is shorter.

Claim, completion, failure, cancellation, interruption, input, and replan transitions
are committed inside the queue transactions that own job and step state. Worker
telemetry and best-effort status events never drive the ledger. A command has a stable
ID and canonical hash: an exact retry returns its existing event, a reused ID with
different content fails as an invariant violation, and a distinct stale command fails
with a version conflict.

Replanning has an explicit generation and supersession boundary. A replan appends one
immutable generation record, retires the old pipeline tail, creates fresh step
identities, and records hash-bound user feedback in the Task Ledger in the same
transaction. Old artifacts, evidence, and subtask assignments remain history but
cannot be selected as current authority. Generation-scoped plan nodes are deliberately
not projected yet: they require typed node-generation scope and atomic node retirement
before a plan can become mutable current authority without surviving its generation.

## Working Set

The ledger records what must survive during the job. A working set records what is
currently resident. Items have a stable reference, role, retention scope, priority,
freshness identity, byte cost, acquisition provenance, and use counters.

Retention scopes are call, step, phase, task, objective, job, and pinned. Lifecycle is
primarily semantic:

- call-local items release when the call finishes;
- step- and task-local items release when their scope finishes;
- shared items remain while another active scope references them;
- stale source-bound items invalidate immediately;
- superseded decisions and resolved failures leave normal contexts but remain history;
- least-recently-used eviction is permitted only within the same retention class.

A model may request retention or release only through a bounded role-specific schema.
Code validates scope, kind, budget, freshness, duplication, and reference existence.
Release never deletes task history.

## Context Projection

Every model station declares a versioned context specification in code. The
specification states its scope, required and optional selectors, allowed authorities,
item and byte ceilings, and acquisition-round ceiling.

Selection begins with structured authority, never embeddings:

1. direct current user authority;
2. active objective and acceptance criteria;
3. active constraints;
4. accepted in-scope decisions and invariants;
5. latest unresolved failure;
6. explicitly acquired repository and evidence references;
7. direct dependencies and tests;
8. permitted durable historical references;
9. semantic retrieval only when structured retrieval is insufficient.

A projection records selected and omitted references, omission reasons, the working-set
version, renderer/spec versions, byte and token estimates, and the exact rendered hash.
The projection identity is bound to immutable LLM call evidence. If a required selector
cannot fit or resolve, context construction fails; it does not silently drop authority.

Fragment models retain the strictest contract. They receive only their immutable
signature, exact local behavior, direct allowed declarations/symbols, accepted local
invariants, and at most one current path-free diagnostic. They do not gain a ledger,
repository browser, or free-form attention interface.

## Acquisition providers

RAG is one provider, not the architecture. Typed providers may acquire candidates from:

- exact repository search and symbol lookup;
- structural graph expansion;
- PostgreSQL full-text and trigram search;
- semantic retrieval after exact and structural retrieval are insufficient;
- compiler and test diagnostics;
- command, web, artifact, and evidence records;
- deliberately promoted durable memory.

The working-set contract is independent of how a reference was acquired. It validates
identity, authority, scope, provenance, freshness, and cost.

## Human-readable projections

PostgreSQL remains authoritative. For terminal jobs, an explicit server-authorized
operation may atomically generate disposable, read-only inspection files under
`.omni/runs/<job-id>/`, including a manifest, task-ledger state, and bounded artifact,
evidence, and call indexes.

These files are an inspection ABI for humans and external tools. Models do not edit
them, workers do not read them as authority, and deleting them does not delete state.
Default exports omit prompts, responses, native thinking, source excerpts, web bodies,
memory, diffs, command output, job metadata, and private benchmark evaluation. The
repository inventory and Git-state identity exclude `.omni/**` before counting and
hashing, so an inspection export cannot invalidate repository truth.

## Implementation sequence

1. **Task Ledger kernel** — typed commands and transitions, normalized current state,
   append-only events, optimistic concurrency, restart/replay tests, and read-only
   exports. No prompt changes.
2. **Transactional lifecycle** — create the ledger with the job, then bind claim,
   completion, failure, and cancellation to task transitions inside the queue's
   existing transactions.
3. **Atomic authority cutovers** — migrate artifact/evidence writes, accepted intent,
   accepted plan and assigned steps, typed feedback, verification, and direct-coding
   specifications one authority class at a time. Writers and readers switch together;
   no model-authored autobiography and no context-map fallback remain for a promoted
   class.
4. **Working Set lifecycle** — acquire, attach, retain, release, scope completion,
   stale-hash invalidation, reference sharing, hard budgets, and metrics. Run in
   shadow mode first.
5. **Context Projection evidence** — build and persist proposed immutable projections
   beside current prompts without sending them to models. Compare omissions and size.
6. **First live consumer** — repository investigation, after shadow selectors prove
   complete. Greenfield fragment generation remains unchanged.
7. **Typed attention requests** — selected coordinator roles may request bounded
   acquisition, retention, and release. Code remains the authority.
8. **Semantic retrieval** — promote vectors only after measured exact, structural,
   full-text, and trigram baselines.

There is one implementation of each primitive. Shadow mode records a proposed
projection but does not create a fallback context path. Promotion replaces the prior
consumer after its gates pass.

## Existing-repository proof boundary

The Go change workflow derives one ordered focused-plus-terminal-broad verification
plan from the exact source snapshot and accepted change contract. Before a fragment
model can run, code executes that whole plan in a disposable projection containing
only the validated `Snapshot.Files` inventory. The live worktree, Git metadata,
`.omni`, ignored files, and snapshot exclusions are never bound into this model-adjacent
sandbox. Every command is preceded by exact live-source and projection assertions,
command execution must leave both exact states unchanged, and a distinct baseline
acceptance is recorded only after the final assertions. A dirty source baseline,
missing indexed dependency, or failing test therefore grants no fragment-generation,
correction, or mutation authority.

Go dependency authority is projected separately. Code resolves the source module's
exact offline build list, validates every cached archive and `go.mod` checksum, and
constructs a capped disposable module view. The verification sandbox receives only
that read-only view with workspace and network lookup disabled; the host-wide module
cache, unrelated cached modules, and missing dependency authority are never exposed or
silently substituted.

Post-change proof has separate staged and authoritative scopes. Their evidence binds
the source snapshot, contract, ordered plan, exact stage, patch hash, and complete
expected post state. After mutation, authoritative proof constructs a fresh exact
post-state snapshot projection; it neither reuses the candidate stage nor mounts the
live worktree. This establishes exact patch integrity and regression verification
against the repository's existing code-owned tests. It does not establish arbitrary
requirement satisfaction when the existing suite does not encode the new behavior.

Existing-repository autonomy remains unpromoted until an independent requirement-bound
or held-out evaluator, unavailable to the builder until the run stops, proves the
requested behavior without model-authored self-tests or benchmark-specific framework
logic. A passing regression suite alone is not a requirement-completion claim.

## Current process-restart boundary

Repository mutations use a durable prepared/applying/applied/indeterminate journal.
The journal binds the exact job generation, step, worker, source snapshot, contract,
stage, full patch, and source/post file states. A retry classifies the complete current
repository inventory: exact source permits the same patch, exact post permits atomic
generated-diff evidence finalization, and any other state fails as indeterminate. An
unresolved command can be loaded before repository indexing, but that read does not
claim or transfer its running step.

The journal currently ends at exact patch application. Its `applied` state is not a
durable acceptance of the subsequent authoritative verification plan, refreshed
repository index, or completed task. An interruption after exact-post finalization and
before those later phases cannot resume at the missing proof boundary; the current
runtime may begin semantic routing and change generation again from the post-patch
repository. Therefore the mutation journal must not be cited as crash-safe end-to-end
existing-repository execution. Promotion requires a code-owned phase checkpoint that
binds and resumes the baseline, staged proof, mutation, authoritative proof, refresh,
and completion sequence without rerunning an already applied request.

The cross-cutting step-attempt lease cutover is implemented. Every worker-originated
durable write is bound to one monotonically increasing attempt, an expired attempt is
reclaimed only as a later attempt, and the stale worker is fenced from subsequent
writes and completion. The former lease-required error and its unfenced writer path
are removed and must not return.

The implemented lease authority:

- add a monotonic attempt identity and one expiring active lease to each claimed step;
- carry the exact job, generation, step, attempt, and worker identity through every
  worker-originated durable write, model-call record, lifecycle operation, working-set
  mutation, memory decision, tool result, and domain side effect;
- use one job-then-step-then-attempt lock order before any ledger or working-set lock;
- reject every stale attempt through one typed error and remove every old writer
  signature rather than retaining compatibility overloads;
- allow repository-journal recovery under a later attempt only through an explicit
  actor-attempt field while preserving the immutable attempt that prepared the patch;
- prove expiry and reclaim with two workers, including rejection of the old worker's
  writes and completion, safe waiting-for-feedback behavior, and post-state mutation
  finalization after a real repository-process restart.

This lease authority does not by itself promote end-to-end repository-process restart.
That claim remains blocked by the missing phase checkpoint described above: the
baseline, staged proof, mutation, authoritative proof, refreshed index, and completion
sequence must resume from PostgreSQL without rerunning an already accepted phase.
Omnidex fails loudly at that boundary instead of treating attempt fencing as proof of
phase-level continuity or inventing a repository-only takeover path.

## Proof gates

The first proof is continuity, not a large-repository edit:

- kill a worker after every completed step;
- clear all model conversation state;
- restart from PostgreSQL alone;
- select the same next runnable task;
- preserve active constraints, accepted decisions, unresolved failures, and completed
  work;
- never reuse rejected hypotheses or repeat completed work.

Required promotion invariants are 100% state validity and forced-restart recovery, zero
authority violations, zero stale references admitted to model context, no end-to-end
correctness regression, and a material reduction in context and duplicate acquisition.

Repository scaling is measured with the same relevant module surrounded by increasing
amounts of unrelated source. Index and storage cost may grow. Model-visible context and
retrieval rounds must remain approximately constant at equal task correctness.

Historical-commit routing is evaluated separately using parent revision plus an issue
or commit description, with the actual diff withheld. File recall, symbol recall, test
recall, irrelevant bytes, evidence-pack bytes, retrieval rounds, and stale references
are reported for exact, structural, full-text, and optional vector variants.

No capability claim is valid until the ordinary production request boundary, frozen
code and model routing, exact call evidence, unsteered execution, and withheld
evaluation requirements all satisfy the existing assembly-line proof discipline.
