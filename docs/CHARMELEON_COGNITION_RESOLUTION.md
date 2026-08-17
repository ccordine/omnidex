# Omnidex code-owned cognition resolution contract — Charmeleon build

Status: normative sub-contract of
[`CHARMELEON_COGNITION_RUNTIME.md`](CHARMELEON_COGNITION_RUNTIME.md).

The assembly-line authority in
[`CHARMANDER_ASSEMBLY_LINE.md`](CHARMANDER_ASSEMBLY_LINE.md) is absolute: code does
everything it can derive reliably. Inference may fill only one explicitly named
semantic gap that deterministic machinery cannot cross.

## Sole cognition path

```text
restore typed authoritative state
        ↓
evaluate code-owned completion
        ↓
run deterministic prerequisite closure
        ├─ unique grounded operation → code prepares and executes it
        ├─ one registered semantic uncertainty → one tiny station call
        └─ no registered resolution → fail loudly
```

The model is an interrupt, not the loop. A model call without a persisted named
uncertainty is an invariant failure. There is no alternate model-agent runtime.

Provider discovery, bootstrap observation, attestation, process activation, and
station-call preparation are lazy. They occur only after the resolver persists a
named uncertainty for a registered station. A deterministic episode must start,
execute, seal, and recover with no provider configured or contacted and with no
fabricated provider-bootstrap, activation, projection, or call evidence. Encountering
a named uncertainty without its required provider authority fails explicitly.

## Application front-door closure

An ordinary coding request enters this same resolver contract. Code first hashes the
immutable request and bootstraps facts it can establish exactly: whether the workspace
is empty or existing, bounded accepted durable-memory authorities, and any verified
repository, runtime, or external evidence already acquired by registered providers.
It never asks a model what files exist, which command to run, or which provider to use.

Only after deterministic bootstrap may a context-sufficiency station identify zero
through three unanswered semantic evidence questions. Its output is a question set,
not an action set. Code maps each question to a registered evidence class, invokes the
deterministic provider with code-owned arguments, validates the result, and records a
compact fact with source identity and digest. Provider transcripts and broad search
results are not planning context. If no provider owns a required question, resolution
fails loudly. The promoted fresh-workspace vertical accepts only the zero-question
transition until additional provider mappings are proven in production.

Intent interpretation occurs only after that closure. The intent candidate contains
one product context and bounded semantic requirement statements, each ultimately bound
to the immutable request digest. Authority does not depend on reproducing a contiguous
substring or allocating non-overlapping text intervals. Code validates and retains a valid
candidate directly: an independent model is never called merely to accept, reject, or
cosmetically replace a leaf. Models see no operation catalog, repository tree, path, task
graph, or completion control.

For job specifications, code validates the complete candidate itself. Only an exact
deterministic schema failure creates a correction boundary. Code names the one mutable
field, applies the one-field correction itself, and verifies that exactly one leaf changed.
This is state grounding without a model-authored control plane.

## Autonomous substrate and subjective meaning

Turning inference off must not stop the world. Code-owned systems continue sensing,
reducing state, regulating numeric or categorical baselines, scheduling, traversing,
acquiring deterministic evidence, enforcing mechanics, executing uniquely determined
operations, and evaluating completion. The AI does not decide that a need exists,
reconstruct legal mechanics, or invoke the simulation's equivalent of a tool.

Parsers, compilers, type checkers, indexes, transaction managers, schedulers, graph
algorithms, rules engines, and environment mechanics are never model roles or
model-selected tools. Code invokes them whenever typed state requires them and treats
their validated output as authority. Inference receives only the semantic remainder
that those systems cannot compute exactly.

Models do not call tools. An LLM may not request deterministic machinery;
deterministic machinery runs whenever code-owned authoritative state requires it, and
inference receives only the unresolved semantic remainder produced after deterministic
closure. A tool registry, function-call catalog, shell operation,
repository operation, environment operation, or adapter command is never a
provider-visible capability. Code owns tool selection, arguments, ordering,
invocation, retries, budgets, and validation. For example, after code accepts one
model-generated declaration, code parses it, inserts it into an in-memory document,
formats it, compiles it, runs the predetermined checks, and routes an exact failure;
the model is never asked whether to perform any of those operations.

Some environments legitimately use inference to add semantic or narrative meaning to
authoritative events. That is a separate named gap, such as interpreting how one
bounded event relates to an existing trait, relationship, preference, or unresolved
belief. A role-specific appraisal station may return one bounded subjective value.
Its result:

- cites the exact authoritative state and event inputs it interprets;
- is stored with model-proposal or accepted-subjective authority, never as world fact;
- cannot negate, replace, or fabricate an authoritative observation;
- cannot directly mutate needs, mechanics, relationships, schedules, or baselines;
- influences later behavior only through a registered code-owned acceptance and
  bounded-effect policy; and
- is recalled by deterministic scope, relevance, freshness, and budget rules rather
  than by transcript replay or model-managed memory.

This is the permitted "yes-and" role: code generates reality; inference adds bounded
meaning consistent with that reality; code incorporates the accepted meaning and
continues. A free-form narrative that has no registered downstream consumer is
model-authored autobiography and is forbidden.

## Public causal surface

An environment registers an execution schema and a public causal contract for every
operation. The causal contract uses opaque typed identities; code must not infer
causality by scanning requirement names, observation prose, or workload nouns.

The contract is equivalent to:

```go
type RequirementID string

type RequirementKind string
type RequirementLifetime string

const (
    RequirementPredicate RequirementKind = "predicate"
    RequirementValue     RequirementKind = "value"
)

type PublicRequirement struct {
	ID          RequirementID
	Version     string
	Kind        RequirementKind
	Lifetime    RequirementLifetime // episode-immutable or exact-current-revision
	ValueSchema *ValidatedValueSchema
	SHA256      Digest
}

type ArgumentBinding struct {
    Argument ActionArgumentName
    Source   RequirementID
}

type PublicOperationContract struct {
    SpecID         ActionSchemaID
    Requires       []RequirementID
    Provides       []RequirementID
    Achieves       []Predicate
    Bindings       []ArgumentBinding
    FixedArguments []ActionArgument
}

type ReducerSourceKind string

const (
    ReducerSourceInitial   ReducerSourceKind = "initial"
    ReducerSourceOperation ReducerSourceKind = "operation"
)

type ObservationReducerContract struct {
    Ref       ObservationReducerRef
    InputKind ObservationKind
    Source    ObservationReducerSource // initial or exact ActionSchemaRef
    Outputs   []PublicRequirementRef
    SHA256    Digest
}
```

Requirements, value schemas, operation contracts, reducers, and their canonical
digests are frozen before an episode. A reducer implementation identity binds its
exact named function to the digest of the running executable measured by code; an
adapter cannot assert an expected source digest or substitute another code authority.
The model cannot create or modify any of them.

The public causal surface contains only mechanics legitimately available to the
coordinator. It must not expose latent solutions, hidden guards, private facts,
unobserved entities, oracle labels, generator seeds, or scores. Environment-private
preconditions remain private and are enforced atomically when an operation is
applied.

Public observations become prerequisite state only during ingestion of an exact
validated environment journal state. The sole code-owned reducer registry executes
one implementation bound to one exact reducer reference and contract. Callers cannot
pass raw observations, synthesize an action, or register a second function under the
same identity. Initial reduction consumes only the journal's exact Start transition.
Operation reduction consumes only the journal's exact successful Apply receipt and
its registered action. Reducer input observations are selected from that transition
and canonically ordered by their complete evidence identity before code executes.

A reducer contract binds its exact input observation kind, exact versioned+hashed
output requirements, and either initial-transition authority or one exact producing
action schema. The accepted value and immutable evidence lineage are outputs of that
code path; callers cannot construct or mutate accepted knowledge fields directly.
Opaque JSON, natural-language guidance, arbitrary supplied values, and model
assertions are not machine knowledge.

Every requirement declares its validity lifetime. Episode-scoped knowledge is
permitted only for facts whose truth is immutable for that episode. Revision-scoped
knowledge must be derived from evidence at the exact current world revision; advancing
the world invalidates it. A requirement without an explicit registered lifetime is
invalid. This first resolver is deliberately monotonic: applicability, consumption,
negative effects, or mutable baselines require an explicit later contract and may not
be inferred from absence.

The current bounded environment journal authenticates evidence membership only from
the exact observations contained in its Start transition and successful current Apply
receipt. A matching episode/revision tuple—including the current receipt's expected
revision—is not observation ancestry. Accepted knowledge citing any other observation
fails loudly even when its episode, revision number, and revision digest appear
plausible. Numeric ordering is not ancestry. Supporting deeper history requires a
bounded, exact observation-membership authority restored by the production store; the
resolver must never infer that authority from `revision.Number < current.Number`.

The catalog is causally total. Every referenced prerequisite has at least one exact
materialization route: an initial reducer or a registered providing operation. Every
operation-provided requirement has one exact reducer bound to that producer; initial
reducers are registered separately. Missing, orphaned, duplicate, cross-producer, or
prior-version reducer authority makes the catalog invalid before execution. An
operation that depends on accepted knowledge may not declare evidence forbidden and
silently erase the causal lineage.

## Deterministic prerequisite resolver

The resolver receives one immutable, model-free code-owned resolution state restored
from the exact environment journal, obligation graph, active attempt, public-causal
catalog and accepted knowledge. It does not depend on a Context Projection, provider
identity, or model-call budget. The first implementation accepts only an active goal
containing exactly one positive `All` predicate and derives that predicate itself.
`Any`, `Not`, or compound goals fail loudly until a separate code-owned scheduler can
prove their exact semantics. The resolver never flattens a goal, chooses the first item
in a caller-provided list, or accepts a caller-selected target. It walks only
registered public causal edges under hard node and depth limits. It:

1. evaluates requirements against accepted typed knowledge and public state;
2. locates registered producers for missing requirements;
3. recursively resolves uniquely determined acquisition operations;
4. binds arguments from accepted values or registered fixed values;
5. selects supporting evidence from the value's immutable lineage;
6. prepares a request only when every public requirement and binding is exact; and
7. blocks on ambiguity unless a separate concrete station contract has been
   registered and persisted by the production coordinator.

Its result is a closed tagged type:

```go
type ResolutionKind string

const (
    ResolutionExecute ResolutionKind = "execute"
    ResolutionBlocked ResolutionKind = "blocked"
)

type Resolution struct {
    Kind    ResolutionKind
    Action  *PreparedAction
    Failure *ResolutionFailure
}
```

`PreparedAction` is entirely code-constructed. It binds the exact resolution-state
digest, catalog, active obligation, current revision, operation-contract digest and
resolution digest. The action journal assigns its durable identity after verifying
that binding against the same restored state.

No producer, a causal cycle, a missing binding, an exceeded bound, or ambiguity
without a registered station returns an explicit `blocked` failure. Conflicting or
stale accepted knowledge makes resolution-state restoration invalid before planning;
it returns an error and no plausible resolution value. None of these conditions
falls through to a general model call.

## Named cognitive gaps

The prerequisite core has no generic uncertainty DTO and does not fabricate one from
ambiguity. Until a concrete station is registered with a complete persisted authority,
ambiguity is `blocked`. Adding a station adds its own exact gap type rather than
expanding `Resolution` into a speculative universal union.

The first permitted inference role is bounded candidate selection:

```go
type CandidateSelectionCall struct {
    UncertaintyID UncertaintyID
    Question      BoundQuestion
    Evidence      []BoundEvidence
    Candidates    []OpaqueCandidate
    Projection    ContextProjectionRef
}

type CandidateSelection struct {
    CandidateID CandidateID
}
```

The provider-visible response contains only one candidate ID from the exact bounded
set. Code owns the ID mapping, validates and persists the result as typed state, and
reruns deterministic closure.

Classification, extraction, hypothesis selection, declaration generation, and
critique require separate hard-typed ports and minimal schemas if later justified.
There is no generic station result union. A station cannot emit:

- an environment operation or its arguments;
- action evidence or an expected effect;
- a Task Ledger or obligation-graph mutation;
- a Working Set retain, release, pin, or selection request;
- a durable identity; or
- a completion result.

Every call binds the named uncertainty, exact candidates, selected evidence, active
attempt, episode revision, obligation generation, public-causal-catalog version,
Working Set version, projection, station and renderer versions, and hard budgets. A
later operation is derived anew by code; it is never copied from a station response.

## Recovery

Recovery restores either deterministic resolver state, one unresolved station call,
or one already prepared code-owned action. It never reconstructs cognition from a
transcript or asks a model what happened. After removing only actor-fence fields, the
named uncertainty, candidate set, evidence, and rendered station context must match an
uninterrupted run at the same boundary.

## Rejected experimental path

The universal `CognitionDecision` path is rejected. Its model-owned action,
arguments, evidence references, expected effect, proposals, attention requests, and
completion-shaped fields are not compatibility surfaces.

The production cutover must remove that path, its provider schema, its persistence
types, and its recovery route. Old serialized usage fails explicitly. There is no
old/new feature flag, adapter, alias, shadow fallback, or semantic backfill.

## Required behavioral gates

The first gates run through the isolated in-memory executable specification. They are
required before freezing a persistence representation or implementing recovery:

- a unique goal-achieving operation completes with zero model calls;
- a missing typed prerequisite invokes its unique producer, reduces the resulting
  observation, grounds the consumer operation, and completes with zero model calls;
- both deterministic cases run with no provider configured and contain no provider
  discovery, bootstrap, activation, projection, or call records;
- two genuinely ambiguous opaque candidates cause exactly one candidate-selection
  call whose payload contains no environment-action or memory-management surface;
- missing, contradictory, cyclic, ambiguous-unregistered, or stale state fails
  loudly;
- procedural mechanics, read-only repository traversal, recursive workload
  compilation, bounded declaration generation, and semantic review each preserve the
  same code-owned operation and completion authority; and
- source-level absence tests reject the universal decision APIs, renderer instruction,
  JSON fields, persistence consumers, compatibility aliases, and fallbacks.

After those behaviors pass through the one production path, durability may mirror
them. Its additional gates are exact restart after prerequisite acquisition, the same
next deterministic operation or named gap after recovery, stale-writer rejection,
replay equality, and complete provenance. Persistence is not permitted to introduce a
new transition rule, planner, inference role, or fallback.
