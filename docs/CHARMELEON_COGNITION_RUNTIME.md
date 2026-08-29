# Omnidex Cognition Runtime contract — Charmeleon build

Status: normative target production contract. Conformance is recorded explicitly at
the end; unchecked behavior is not implemented or promoted.

Charmeleon is only the codename for this Omnidex build. The Cognition Runtime is a
domain-neutral Omnidex subsystem, not a codename subsystem or an autonomous agent. It
extends the authority model in
[`CHARMELEON_CONTEXT_SYSTEM.md`](CHARMELEON_CONTEXT_SYSTEM.md); the bounded coding
rules in [`CHARMANDER_ASSEMBLY_LINE.md`](CHARMANDER_ASSEMBLY_LINE.md) remain in force.

## Purpose

The Cognition Runtime coordinates code-owned progress across tasks whose relevant
state is larger or longer-lived than one model context within the current running
Omnidex service. It is not a cross-start state mechanism. Production vocabulary is
limited to:

- goal;
- observation;
- evidence;
- uncertainty;
- obligation;
- action;
- effect;
- failure;
- revision;
- completion predicate.

It has no production concept of rooms, doors, keys, treasure, mazes, or other
benchmark mechanics. Those concepts may exist in an injected environment adapter.
Repository investigation, research, project coordination, and infrastructure
inspection must use the same runtime contract rather than workload-specific branches.

The runtime is not a planner persona, transcript manager, or tool-calling agent. Code
owns the cognition loop. It restores authoritative state, evaluates completion,
resolves prerequisites, acquires deterministically available evidence, grounds
operation inputs, selects supporting evidence, executes transitions, and repeats.

Inference is an interrupt, not the loop. A model call is invalid unless code has first
exhausted registered deterministic work and persisted one precisely named semantic
uncertainty that it cannot resolve. The model crosses only that uncertainty and
returns one station-specific typed leaf. It never chooses an environment operation,
constructs an operation input, cites action evidence, predicts an effect, manages the
Working Set, proposes a plan while acting, or declares completion.

## Package boundary

The rejected universal cognition runtime, its tool-calling policy, replay/store/
transport sidecars, and its bespoke procedural gauntlet have been deleted. They are
not compatibility surfaces and must not return.

The former in-memory cognition reference tree has been deleted after its task-neutral
contracts were cut into the authoritative assembly-line, repository, queue, and
worker packages. Keeping it would create a second objective runtime and an
archaeological fallback. Architecture tests now require that parallel tree to remain
absent.

Production continues to use the existing task-state, Working Set, context-builder,
evidence, assembly-line, model, and queue contracts only where their behavior matches
this document. The incompatible production cutover must consume the proven behavior
directly; no production binary may gain a hidden benchmark route, oracle loader,
score reader, or workload-specific prompt.

## Environment Contract

An environment exposes registered operations, not a raw shell and not its complete
state. Every operation has both an execution schema and a public causal contract that
code can reason over. Hidden guards and latent world state remain private to the
environment. The target protocol is equivalent to:

```go
type Environment interface {
    Start(context.Context, ScenarioRef) (Transition, error)
    Apply(
        context.Context,
        EpisodeRef,
        Revision,
        RegisteredAction,
    ) (Transition, error)
}

type ScenarioRef struct {
    ID           ScenarioID
    PublicSHA256 Digest
}

type Revision struct {
    EpisodeID EpisodeID
    Number    uint64
    SHA256    Digest
}

type RegisteredAction struct {
    ID            ActionID
    Actor         AttemptRef
    SpecID        ActionSpecID
    Input         ValidatedActionInput
    RequestSHA256 Digest
}

type Transition struct {
    ActionID      *ActionID
    Previous      *Revision
    Current       Revision
    Observations  []Observation
    Cost          int64
    Terminal      bool
    PublicOutcome string
}

type Observation struct {
    ID        ObservationID
    EpisodeID EpisodeID
    ActionID  *ActionID
    Revision  Revision
    SpecID    ObservationSpecID
    Payload   ValidatedObservation
    SHA256    Digest
    ByteCount int64
}
```

`ScenarioRef` contains only the public scenario identity and public artifact hash. It
cannot carry a seed, oracle identity, private storage locator, hidden label, or score.
Action specifications, inputs, and environment outputs are bounded, versioned types.
The required public requirements, causal edges, argument bindings, typed knowledge,
and observation reducers are defined by
[`CHARMELEON_COGNITION_RESOLUTION.md`](CHARMELEON_COGNITION_RESOLUTION.md). They expose
no private oracle state and are never inferred from workload prose.

Identity and transition rules are absolute:

- Code assigns episode, action, model-call, and obligation identities. `ActionID` is
  the sole idempotency identity; there is no second retry key.
- `RegisteredAction.Actor` is the current invocation fence, not part of semantic
  action identity. `RequestSHA256` binds the action ID, specification, and input. A
  replacement attempt may retry that same request with its current actor fence.
- A revision is an opaque monotonic number plus a digest of authoritative environment
  state. The model receives the identity, never the state used to derive it.
- `Start` returns a transition whose `Previous` and `ActionID` are absent. An `Apply`
  transition has both fields present, `Previous` exactly equals the requested
  revision, all revisions belong to the same episode, and `Current.Number` equals
  `Previous.Number + 1`.
- An observation is immutable and bound to its episode, revision, canonical payload
  digest, authority, byte cost, and optional producing action. Initial observations
  have no producing action.
- An exact `ActionID` retry with the same canonical request returns the already
  recorded transition. Reusing it with different canonical content fails explicitly.
  Actor-fence validation precedes replay; replay precedes the current-revision check.
- A previously unseen action accepts only the exact current revision. A stale
  revision or cross-episode revision fails without an effect.
- Preconditions, authorization, input validation, state mutation, transition
  recording, and the new revision commit atomically within the current database
  lifecycle.
- Before dispatch, code may prepare an action only from one registered public
  operation contract whose public requirements are satisfied. Every argument and
  evidence reference is derived from accepted typed knowledge or a registered fixed
  binding. Missing, conflicting, stale, or ambiguous grounding fails before dispatch.
- Invalid actions and failed preconditions do not mutate environment state.
- Pre- and post-revision hashes, the action request hash, observations, cost, and
  outcome are transactional evidence for the current database lifecycle.

Adapters may implement macro-actions. Code may perform deterministic low-level
transitions inside a registered macro, but reports station results, code-prepared
environment actions, and low-level transitions separately.

## Current-runtime transition protocol

PostgreSQL is canonical during one running service for cognition episode identity,
active lifecycle attempt, action request identity, ingested transition receipts,
obligation commands, Task Ledger events, Working Set state, Context Projections, and
terminal state. Redis may coordinate leases and progress but is not authoritative.
An environment host may commit its own state and transition receipts according to
its adapter contract, but that external state cannot revive an Omnidex episode after
an Omnidex startup.

The coordinator records a new `ActionID` and canonical request hash before dispatch.
The environment atomically validates the actor fence and expected revision, applies
the action, and records its transition under that ID. The coordinator then ingests
the returned transition exactly once and atomically updates its Task Ledger, Working
Set, obligation graph, and episode revision. While the same Omnidex service remains
running, an ambiguous dispatch result may be retried with the same `ActionID`; the
environment returns the prior transition. A conflicting result, missing prior
revision, nonconsecutive revision, cross-episode identity, or terminal episode
mutation is an invariant failure. If the Omnidex service stops, the episode ends and
is not reconstructed on startup.

Every worker-originated read-modify-write is bound to job, run, Task Ledger
generation, step, monotonically increasing attempt, worker, and lease fence. This
includes model-call evidence, ledger commands, Working Set commands, Context
Projections, action reservation and dispatch, transition ingestion, obligation
changes, failures, and completion. Lease expiry invalidates an in-flight model result.
An old worker may not write evidence, mutate attention, execute an action, or complete
the goal after a replacement attempt exists.

## Deterministic closure and named cognitive gaps

The coordinator must exhaust deterministic prerequisite resolution before inference.
It either prepares one fully grounded operation, persists one registered named
semantic uncertainty, or fails explicitly. The sole resolver, typed public causal
surface, station boundary, recovery rules, forbidden model outputs, and removal of
the rejected universal decision path are normative in
[`CHARMELEON_COGNITION_RESOLUTION.md`](CHARMELEON_COGNITION_RESOLUTION.md).

The model never decides what operation to invoke. It may return only the one typed
leaf permitted by the station for the persisted uncertainty. Code records that value
and reruns deterministic closure.

## Obligation graph

Planning is a bounded graph of desired predicates rather than a model-authored prose
plan. Each obligation has one code-assigned identity, generation, parent, desired
predicate, status, dependencies, supporting references, and registered completion
check.

Code owns graph construction, prerequisite expansion, acyclicity, depth and node
ceilings, ready/blocked transitions, supersession, generation changes, and completion
checks. If a genuine semantic ambiguity prevents graph expansion, a dedicated station
may select one code-issued candidate predicate or interpretation. The model does not
invent an obligation graph while executing an operation. Replanning creates a new
generation; it never rewrites accepted history.

## Production cognition loop

```text
code establishes root goal and active obligation
        ↓
code restores typed authoritative state and applies deterministic retention
        ↓
code evaluates completion and runs prerequisite closure
        ↓
unique grounded operation? ── yes ──→ code prepares and dispatches it
        │
        no
        ↓
one registered named semantic uncertainty?
        ├─ no ──→ fail loudly
        └─ yes
              ↓
        Context Builder seals one station-specific projection
              ↓
        model returns one typed leaf
              ↓
        code validates and records it, then reruns closure
        ↓
environment commits one transition or one explicit failure
        ↓
code records observations, effects, and lifecycle changes
        ↓
code evaluates registered completion predicates
```

There is no full-transcript fallback. A required projection item that cannot be
resolved or fit is a hard context-construction failure. Confusion does not increase
the configured context or action budget.

Every call is bound to the exact named uncertainty, active attempt, episode revision,
obligation generation, public-causal-catalog version, candidate-set digest, Working
Set version, Context Projection identity, station kind and version, renderer version,
and hard input/output budgets. The accepted typed leaf binds that same projection and
uncertainty. A later operation is derived anew by code; it is not copied from the
model response. An unbound call or a response that arrives after any bound authority
becomes stale is rejected.

### Every model call gets a clean desk

The context window is reusable compute space, not accumulated memory. PostgreSQL,
the Task Ledger, the Working Set, and immutable evidence hold state for the current
service/database lifecycle. Only after deterministic closure yields a named uncertainty does code compile a new
disposable station Context Projection and load it by exact projection identity
immediately before inference. Deterministic operations create no fake model work and
require no model provider. Provider discovery, attestation, and process activation
are also deferred until that uncertainty exists. A fully deterministic episode starts
and seals without provider bootstrap, activation, projection, or call evidence. A
station retains no prior prompt, response, transcript tail, or message buffer after
the call.

Two budgets remain distinct:

- the episode budget limits how many model calls and environment actions the whole
  run may consume;
- the registered station budget independently limits input bytes/tokens, output
  bytes/tokens, evidence references, and typed output fields for every call.

Consuming one episode call decrements only the remaining-call allowance. It does not
shrink the next station's input or output capacity. Candidate selection, semantic
classification, one declaration, one repair-guidance instruction, and one
repair-executor source node may therefore each use their complete registered budget
while seeing different exact projections. Environment execution is not itself a model
station.

Conversation history is never selected by `last N`. The current direct instruction,
active accepted constraints, authority referenced by the active obligation, and
explicitly relevant changed directives may enter as source-bound entries. Recency may
order already relevant authority but cannot make unrelated chat relevant. A worker
normally receives the accepted typed constraint; it receives original message bytes
only when the station specification explicitly requires that source. Missing required
authority or projection overflow fails before inference; neither causes transcript
fallback or a larger budget.

## Authority and epistemic rules

World truth, exposed observations, and Omnidex belief state are distinct:

- Environment state is authoritative for effects and revisions but is not model
  context.
- Only the environment may emit an authoritative observation about a transition.
- An observation can support a fact without becoming that fact.
- A fact requires immutable evidence and a code-owned acceptance policy appropriate to
  that evidence. A model assertion alone remains a proposal or hypothesis.
- Contradiction rejects or supersedes a belief entry; it does not rewrite the source
  observation or environment history.
- Accepted decisions record their acceptance policy and supporting references.
- Failures remain explicit until resolved or superseded.
- Completion is the result of a registered code-owned predicate against an
  authoritative transition. Model completion claims are invalid.

Attention policy is deterministic. The root goal, current obligation, active
constraints, current revision summary, ready blockers, and latest unresolved failure
remain resident. Evidence remains while it causally supports an active obligation.
Raw evidence may release after a compact evidence-bound fact is accepted; completed,
rejected, superseded, resolved, or stale material leaves normal projections while its
history remains available for the current database lifecycle. Normal station outputs cannot retain, release, pin, or select
Working Set entries. A future exceptional attention-advice experiment requires its
own role-specific contract and evidence that deterministic policy cannot perform the
job; it cannot be added to every station response.

## Public and private evaluation authority

The runtime receives an opaque scenario reference, registered action catalog, legal
observations, public transition outcomes, and no evaluation score. It must not receive
a generator seed, latent solution, shortest path, relevance labels, hidden task
archetype, oracle quality, or final score.

For serious promotion runs, the model-visible coordinator, environment host, and
post-run evaluator are separate processes with separate credentials and storage:

1. The environment host owns transition state and exposes only the Environment
   Contract.
2. The coordinator owns cognition state and can call only that contract.
3. After the episode is sealed and all model calls stop, the evaluator opens the
   private oracle and scores the immutable trace.

The evaluator cannot send feedback into a running episode. The coordinator cannot
open oracle storage. Process separation is a required promotion property, not an
optimization.

## Startup reset boundary

`database/setup.sql` is the sole authoritative definition of the internal Omnidex
schema. Every Omnidex process/service startup drops and recreates the configured
dedicated schema from that file. The startup intentionally discards all prior
episodes, jobs, Task Ledger events and materializations, Working Sets, Context
Projections, obligations, action receipts, evidence, memory, and terminal state.

There is no restart, takeover, resume, replay-from-PostgreSQL, or in-place database
upgrade contract. A stopped episode is stopped; external environment or filesystem
state does not reconstruct its former internal authority. A new request creates new
identity and state in the fresh database lifecycle.

Worker attempts, lease fencing, idempotent `ActionID` retry, and stale-write
rejection still apply while the same Omnidex service remains running. Tests for those
rules must not stop and restart the service. Startup tests instead prove that the
schema exactly matches `database/setup.sql` and no row from the previous lifecycle
is visible.

## Environment transfer and coding boundary

Environment adapters may change surface vocabulary and deterministic mechanics; they
may not change cognition state, resolver semantics, station schemas, retention policy,
projection rules, or completion authority. A transfer claim requires at least two
held-out surfaces using identical production cognition code and station renderer
versions.

Repository intelligence is a future environment consumer, not a relaxation of the
coding assembly line. Code requests registered, bounded repository search, symbol
inspection, and reference traversal whenever authoritative state and prerequisites
determine that they are needed. A model may resolve one semantic ambiguity among
code-issued opaque candidates; it cannot choose whether to read or search, receive a
path, workspace, plan, or whole-file responsibility. Repository mutation must still
flow through the existing parser-, capability-, stage-, and proof-owned coding
boundary. A procedural reference proof alone authorizes no repository mutation. The rejected
output-blind repository cognition shadow has been removed. A production repository
consumer must return exact authority, such as a change contract, that the downstream
workflow actually consumes; a sidecar whose result is discarded is forbidden.

## Ordered implementation and promotion

The replacement runtime is developed brittle-first. Its behavior must be cheap to
change or delete until the claimed control loop works vertically. The mandatory order
is:

1. prove in memory that registered prerequisite producers complete an objective with
   zero inference;
2. prove in memory that one genuine named uncertainty causes exactly one minimal
   station call and that ordinary deterministic closure resumes afterward;
3. transfer that rule to procedural mechanics, then to read-only repository
   investigation, without exposing operations or hidden authority to the model;
4. compile unchanged ordinary text into a recursive code-owned objective graph and
   work that graph to code-owned completion;
5. hand one exact source leaf to the existing Charmander generation contract and keep
   parsing, formatting, testing, mutation, and completion in code;
6. prove deterministic verification routes one real failure to the smallest owning
   objective or source block; inference remains forbidden unless that route exposes a
   separate named semantic uncertainty;
7. replace the production universal decision loop incompatibly and prove that a
   deterministic production workload can run without provider configuration or
   contact; and only then
8. record the proven objectives, facts, operation receipts, named gaps, accepted
   semantic leaves, artifacts, and verification results for the current database
   lifecycle; add same-runtime transaction, provenance, fresh-start reset, scale,
   transfer, and promotion evidence in that order.

A vertical failure stops the sequence. Persistence design does not continue around a
failed behavior gate. No later stage supplies evidence for an earlier missing gate.
Shadow execution is never a fallback path, and a promoted consumer has one
authoritative implementation.

## Conformance status

Only checked items may be cited as implemented. A checkbox may be changed only in the
same reviewed change that adds the production code, success/failure/forbidden-path
tests, and exact evidence proving it.

- [x] The rejected universal runtime, model-driven gauntlet, and alternate sidecars
  are absent, and source-level architecture tests forbid their return.
- [ ] Environment actions are schema-validated, revision-fenced, transactional, and
  idempotent solely by `ActionID`.
- [ ] Cognition transitions, obligations, and terminal state are transactionally
  authoritative within one running service.
- [ ] Monotonic attempts fence every worker-originated read and write.
- [ ] Public operation contracts, typed knowledge reducers, grounding, and evidence
  lineage are versioned and contain no private oracle state.
- [ ] A unique prerequisite producer or goal-achieving operation executes with zero
  model calls throughout the current service lifecycle.
- [ ] Every model call is caused by one persisted named uncertainty and returns only
  its station-specific typed leaf.
- [ ] Universal model-owned action, argument, evidence, expected-effect, proposal,
  attention, and completion fields are absent from production source and schemas.
- [ ] Task Ledger integration has one authoritative writer and exact same-runtime
  materialization.
- [ ] Deterministic Working Set lifecycle is live without a transcript fallback.
- [ ] Context Projection is live for a promoted consumer and every call is bound.
- [ ] Obligation generations, contradiction, and replanning pass transition tests.
- [ ] A service startup recreates the schema from `database/setup.sql`, retains zero
  prior internal rows, and every same-runtime stale-worker write is rejected.
- [ ] Two held-out environment surfaces pass without production changes.
- [ ] Repository investigation passes in shadow without weakening coding boundaries.

Checked items above correspond only to the implementation and exact tests in this
change. Existing Task Ledger, Working Set, Context Projection, and
repository-intelligence primitives remain foundations rather than proof of any
unchecked cognition guarantee.
