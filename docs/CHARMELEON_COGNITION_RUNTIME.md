# Omnidex Cognition Runtime contract — Charmeleon build

Status: normative target production contract. Conformance is recorded explicitly at
the end; unchecked behavior is not implemented or promoted.

Charmeleon is only the codename for this Omnidex build. The Cognition Runtime is a
domain-neutral Omnidex subsystem, not a codename subsystem or an autonomous agent. It
extends the authority model in
[`CHARMELEON_CONTEXT_SYSTEM.md`](CHARMELEON_CONTEXT_SYSTEM.md); the bounded coding
rules in [`CHARMANDER_ASSEMBLY_LINE.md`](CHARMANDER_ASSEMBLY_LINE.md) remain in force.

## Purpose

The Cognition Runtime coordinates bounded model decisions across tasks whose relevant
state is larger or longer-lived than one model context. Production vocabulary is
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

The runtime is not a planner persona or a transcript manager. Code owns identity,
state transitions, budgets, evidence, action validation, side effects, retries, and
completion. A replaceable model policy chooses one bounded action and may submit typed
proposals.

## Package boundary

The intended package boundary is:

```text
internal/cognition/                 production, domain-neutral contracts and loop
    environment/                   Environment Contract and registered action types
    obligation/                    bounded graph commands and validation
    policy/                        model decision schema and immutable renderer

internal/taskstate/                durable goals, entries, generations, and events
internal/workingset/                code-owned attention lifecycle
internal/contextbuilder/            immutable per-call projection construction
internal/evidence/                  exact call and transition evidence

internal/labyrinth/                 benchmark-only world, adapters, and oracle handling
internal/cognitiongauntlet/         offline benchmark runner and evaluators only
gauntlets/labyrinth/                versioned public cases and private labels/oracles
```

These are target boundaries, not a statement that the packages exist today. The
production runtime may depend on the existing task-state, working-set,
context-builder, evidence, model, and queue contracts. Labyrinth may implement the
production Environment Contract; the offline cognition gauntlet may depend on both.

The reverse directions are forbidden:

```text
internal/cognition        -> internal/cognitiongauntlet
internal/cognition        -> internal/labyrinth
internal/worker           -> internal/cognitiongauntlet
internal/worker           -> internal/labyrinth
internal/api              -> internal/cognitiongauntlet
internal/api              -> internal/labyrinth
internal/omni             -> internal/cognitiongauntlet
internal/omni             -> internal/labyrinth
other production core     -> either benchmark package
```

Only an offline benchmark entrypoint may construct a Labyrinth adapter. Before any
benchmark source is added, a source-level architecture test must reject every
forbidden import. No production binary may gain a hidden benchmark route, oracle
loader, score reader, or workload-specific prompt.

## Environment Contract

An environment exposes registered operations, not a raw shell and not its complete
state. The target protocol is equivalent to:

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
An adapter registers each action schema before an episode; the model cannot create a
kind, alter a schema, or expand the action catalog.

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
  persistence, and the new revision commit atomically.
- Invalid actions and failed preconditions do not mutate environment state.
- Pre- and post-revision hashes, the action request hash, observations, cost, and
  outcome are durable evidence.

Adapters may implement macro-actions. Code may perform deterministic low-level
transitions inside a registered macro, but reports model decisions, environment
actions, and low-level transitions separately.

## Durable transition protocol

PostgreSQL is canonical for cognition episode identity, active lifecycle attempt,
action request identity, ingested transition receipts, obligation commands, Task
Ledger events, Working Set state, Context Projections, and terminal state. Redis may
coordinate leases and progress but is not recovery authority. An environment host
must durably commit its own state and transition receipts.

The coordinator records a new `ActionID` and canonical request hash before dispatch.
The environment atomically validates the actor fence and expected revision, applies
the action, and records its transition under that ID. The coordinator then ingests
the returned transition exactly once and atomically updates its Task Ledger, Working
Set, obligation graph, and episode revision. A crash between dispatch and ingestion
is recovered by retrying the same `ActionID`; the environment returns the prior
transition. A conflicting result, missing prior revision, nonconsecutive revision,
cross-episode identity, or terminal episode mutation is an invariant failure.

Every worker-originated read-modify-write is bound to job, run, Task Ledger
generation, step, monotonically increasing attempt, worker, and lease fence. This
includes model-call evidence, ledger commands, Working Set commands, Context
Projections, action reservation and dispatch, transition ingestion, obligation
changes, failures, and completion. Lease expiry invalidates an in-flight model result.
An old worker may not write evidence, mutate attention, execute an action, or complete
the goal after a replacement attempt exists.

## One bounded model decision

The model-visible contract is equivalent to:

```go
type CognitionDecision struct {
    ObligationID   ObligationID
    Action         ActionRequest
    EvidenceRefs   []EvidenceRef
    ExpectedEffect string
    Proposals      []LedgerProposal
    Attention      []AttentionRequest
}
```

`ExpectedEffect` is a short bounded prediction, not chain-of-thought and not evidence
that an effect occurred. The decision must reference the active obligation and enough
currently projected evidence to authorize the chosen action. Code rejects missing,
stale, out-of-scope, duplicate, oversized, or unsupported references.

The model may propose an observation claim, hypothesis, question, decision candidate,
candidate obligation, or bounded retain/release request. A proposed observation is
stored with `model_proposal` authority and never becomes an environment observation
merely because it uses the observation entry kind. The model may not assign durable
IDs, create authoritative observations or facts, mutate the Task Ledger or Working
Set, change a budget, bypass the environment, or declare completion.

The first policy uses one configured model. Planner/actor committees, advisers, and
model-authored reviews are separate experiments, not assumed production features.
Each additional call or role must beat the single-policy baseline without weakening
validity, authority, or budgets.

## Obligation graph

Planning is a bounded graph of desired predicates rather than a model-authored prose
plan. Each obligation has one code-assigned identity, generation, parent, desired
predicate, status, dependencies, supporting references, and registered completion
check.

Code owns graph acyclicity, depth and node ceilings, ready/blocked transitions,
supersession, generation changes, and completion checks. A model may propose a new
obligation after observing a blocker. Code validates and materializes it or records an
explicit rejection. Replanning creates a new generation; it never rewrites accepted
history.

## Production cognition loop

```text
code establishes root goal and active obligation
        ↓
Working Set applies deterministic retention policy
        ↓
Context Builder seals one immutable projection
        ↓
model returns one bounded CognitionDecision
        ↓
code validates identity, evidence, budget, and action schema
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

Every call is bound to the exact active attempt, episode revision, obligation
generation, action-catalog version, Working Set version, Context Projection identity,
renderer version, and hard input/output/tool budgets. The accepted response and any
subsequent action bind that same projection hash. An unbound call or a response that
arrives after any bound authority becomes stale is rejected.

### Every model call gets a clean desk

The context window is reusable compute space, not accumulated memory. PostgreSQL,
the Task Ledger, the Working Set, and immutable evidence hold long-lived state. For
each bounded station, code compiles a new disposable Context Projection from that
state and loads it by exact projection identity immediately before inference. The
policy retains no prior prompt, response, transcript tail, or message buffer after
the call.

Two budgets remain distinct:

- the episode budget limits how many model calls and environment actions the whole
  run may consume;
- the registered station budget independently limits input bytes/tokens, output
  bytes/tokens, evidence references, and typed decision fields for every call.

Consuming one episode call decrements only the remaining-call allowance. It does not
shrink the next station's input or output capacity. Planning, one execution step, one
declaration, and one correction may therefore each use their complete registered
workspace while seeing different exact projections.

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

Default attention policy is deterministic. The root goal, current obligation, active
constraints, current revision summary, ready blockers, and latest unresolved failure
remain resident. Evidence remains while it causally supports an active obligation.
Raw evidence may release after a compact evidence-bound fact is accepted; completed,
rejected, superseded, resolved, or stale material leaves normal projections while its
history remains durable. A model retention request is advisory and subject to scope,
freshness, pin, item, and byte ceilings.

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

## Restart and takeover contract

Recovery reconstructs an episode from PostgreSQL and the environment's durable
revision, never from a conversation transcript, generated inspection file, Redis
cache, or model recollection. Before the first post-restart model call, the following
must match an uninterrupted execution at the same boundary:

- environment revision and registered action catalog;
- Task Ledger materialized state and replay hash;
- Working Set contents, scopes, freshness, and version;
- active obligation, dependencies, generation, and status;
- exact Context Projection identity and rendered hash;
- completed action receipts and remaining budgets.

The next stochastic model choice need not be byte-identical. The deterministic state
presented before that choice must be identical. Interruption tests cover no kill, one
random kill, five random kills, a kill after every model decision, lease expiry during
inference, and an old worker waking after takeover. The stale worker must fail every
ledger, Working Set, call-evidence, environment-action, and completion write.

## Environment transfer and coding boundary

Environment adapters may change surface vocabulary and deterministic mechanics; they
may not change cognition state, model decision schemas, retention policy, projection
rules, or completion authority. A transfer claim requires at least two held-out
surfaces using identical production cognition code and renderer versions.

Repository intelligence is a future environment consumer, not a relaxation of the
coding assembly line. A cognition policy may request registered, bounded repository
search, symbol inspection, and reference traversal. It cannot give a coding model a
path, workspace, plan, or whole-file responsibility. Repository mutation must still
flow through the existing parser-, capability-, stage-, and proof-owned coding
boundary. The first repository consumer is investigation-only shadow execution; no
mutation is authorized by Labyrinth success alone.

## Ordered implementation and promotion

0. **PR 0:** Freeze these contracts and enforce the production-to-gauntlet import
   prohibition.
1. **PR 1:** Build and property-test the deterministic Labyrinth kernel and sealed
   oracle split.
2. **PR 2:** Add solution-first generation and prove every case with an optimal or witness
   oracle.
3. **PR 3:** Add the isolated filesystem adapter and bounded registered actions.
4. **PR 4:** Add the minimal single-policy coordinator with durable transition ingestion.
5. **PR 5:** Integrate Task Ledger recording in shadow without changing prompts.
6. **PR 6:** Measure deterministic Working Set retention and release in shadow.
7. **PR 7:** Promote immutable Context Projection for one isolated suite and remove its prior
   transcript consumer.
8. **PR 8:** Add the obligation graph, generation-safe replanning, and contradiction handling.
9. **PR 9:** Add real process death, monotonic attempt takeover, replay, and stale-writer
    rejection.
10. **PR 10:** Prove scale and transfer on frozen held-out cases.
11. **PR 11:** Run the combined long-horizon Rogue Suite.
12. **PR 12:** Add a repository-investigation shadow consumer without mutation.

No later stage supplies evidence for an earlier missing gate. Shadow execution is
never a fallback path, and a promoted consumer has one authoritative implementation.

## Conformance status

Only checked items may be cited as implemented. A checkbox may be changed only in the
same reviewed change that adds the production code, success/failure/forbidden-path
tests, and exact evidence proving it.

- [x] Production packages exist and forbidden gauntlet imports fail architecture tests.
- [ ] Environment actions are schema-validated, revision-fenced, transactional, and
  idempotent solely by `ActionID`.
- [ ] Cognition transitions, obligations, and terminal state survive PostgreSQL-only
  recovery.
- [ ] Monotonic attempts fence every worker-originated read and write.
- [ ] One bounded model decision is projection-bound and cannot mutate authority.
- [ ] Task Ledger integration has one authoritative writer and exact replay.
- [ ] Deterministic Working Set lifecycle is live without a transcript fallback.
- [ ] Context Projection is live for a promoted consumer and every call is bound.
- [ ] Obligation generations, contradiction, and replanning pass transition tests.
- [ ] Restart state is identical and every stale-worker write is rejected.
- [ ] Two held-out environment surfaces pass without production changes.
- [ ] Repository investigation passes in shadow without weakening coding boundaries.
- [ ] Every applicable promotion gate in
  [`LABYRINTH_GAUNTLET.md`](LABYRINTH_GAUNTLET.md) has sealed evidence.

Checked items above correspond only to the implementation and exact tests in this
change. Existing Task Ledger, Working Set, Context Projection, and
repository-intelligence primitives remain foundations rather than proof of any
unchecked cognition or restart guarantee.
