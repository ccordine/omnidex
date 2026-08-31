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
| Durable Memory (historical name) | Explicitly promoted cross-job preferences, references, and lessons | Later jobs in the current service/database lifecycle | Reference only |

These layers must not share tables merely because all of them can be called memory.
The historically named durable memory is reference material only for the current
service/database lifecycle. Repository intelligence is disposable, hash-bound
derived state. The task ledger is current execution state. A working set is
attention. A context projection is evidence of one call. No internal layer survives
the next Omnidex startup.

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
                         ↓ one station-specific typed leaf
                  Code coordinator
```

The model may forget everything after every call. Within the current running service,
Omnidex must not. A service startup intentionally begins a new empty internal state
lifecycle.

The domain-neutral coordinator that consumes these authorities is specified in
[`CHARMELEON_COGNITION_RUNTIME.md`](CHARMELEON_COGNITION_RUNTIME.md), with its
code-owned prerequisite and named-uncertainty boundary in
[`CHARMELEON_COGNITION_RESOLUTION.md`](CHARMELEON_COGNITION_RESOLUTION.md). The
the sole production assembly-line and queue implementation owns the executable
contract. The former parallel in-memory reference tree has been retired; fixture
mechanics and private evaluation authority may not enter this context substrate.

## Task Ledger

### Nested direct-coding execution

The queue-owned job root remains the outer lifecycle authority. Once code has
accepted a bounded workload, it records a child objective and explicit child
tasks in that same ledger. A direct-coding task may be marked
`inline_execution` only when it is a `task` node created by the currently
running queue step. It is not a second queue, an agent, or a model-owned
transition: code activates it only after persisted dependencies are done,
records code-validated generated artifacts as its evidence, and marks the
objective done only after real workspace verification succeeds.

Ordinary queued tasks continue to require their own assigned queue step.
Inline execution exists solely for bounded child work that is deterministically
executed inside its already-authorized outer step. Both forms use the same
Task Ledger, verification requirements, and loud failure behavior.

PostgreSQL is canonical during the current running service. One transaction updates
normalized current state, appends one audit event, and increments the ledger version.
Optimistic version conflicts fail explicitly. Event reduction may verify the audit
log within that database lifecycle; it is not a startup recovery path.

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
observations, hypotheses, questions, failures, checkpoints, notes, and typed user
feedback. Authority is user, code, or tool evidence. Model-proposal and
accepted-model-decision authorities and entry kinds are retired. A model result remains
station evidence; only code may persist a validated bound value into an appropriate
entry under the registered rule and evidence references.

Rules:

- A fact has at least one valid evidence reference.
- A model may emit one hypothesis, question, or opaque candidate only through a
  registered station for a persisted named uncertainty. It cannot create user, code,
  or tool authority.
- A model cannot choose an environment operation, accept its own station result,
  transition execution state, or declare work complete.
- Code records the station policy and evidence that accept a model-originated value.
- Rejected, resolved, and superseded entries remain in history and are omitted from
  normal active projections.
- Stable references identify repository facts, artifacts, evidence, memory, web
  evidence, and task state without copying their bodies.
- A source-bound reference whose version or hash no longer matches is invalidated and
  reacquired before projection.

The initial immutable plan remains evidence of what was authorized. Mutable task nodes
record execution under that authority; they do not rewrite the plan artifact. Direct
coding transports that do not emit an intent or plan artifact must first acquire needed
context facts, then freeze task-local requirements through the ordinary inventory and
sieve. Only if a leaf survives may they resolve surface, product, or deployment
semantics, each at its first concrete consumer and before that consumer generates work.
They may not run without task authority merely because their transport is shorter.

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

The ledger records what must remain available during the job in the current running
service. A working set records what is
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

Normal station outputs cannot request retention or release. If later evidence proves
that deterministic retention is insufficient, one exceptional attention-advice role
may be introduced through a bounded role-specific schema. Code validates scope, kind,
budget, freshness, duplication, and reference existence. Release never deletes task
history.

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

Source models retain the strictest contract. Initial fragment generation receives only
its immutable signature, exact local behavior, direct allowed declarations/symbols, and
accepted local invariants. Repair guidance separately receives one exact mutable block,
the minimum diagnostic-analysis evidence, and at most one current path-free diagnostic.
Repair execution receives only the resulting instruction and exact mutable block. None
of these stations gains a ledger, repository browser, or free-form attention interface.

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

### Need-driven context compilation

Context compilation does not call a model merely because a context station exists.
Code checks whether the fixed provider has searchable authority. When it does, the
exact authoritative instruction is the sole retrieval query; when it does not,
retrieval receives an explicit empty query set. A model never formulates or rewrites
that environment-operation argument. Database queries, ranking, filtering, authority
checks, deduplication, ordering, hashing, provenance binding, and result combination
remain deterministic.

After code has acquired and validated candidates, it owns one source-ordered relevance
queue. `context_relevance` receives the exact instruction and exactly one untrusted
candidate content value and returns only the registered directly-relevant or
not-directly-relevant relation. Candidate identity, authority class, provenance, queue
state, and whether another candidate exists remain code-owned. A negative candidate
evaporates; a positive candidate is preserved in authoritative order and is never
reviewed again. Queue exhaustion ends relevance processing without a model-authored
absence, completeness, or continuation claim. Per-call byte budgets are local safety
bounds, not global correctness ceilings.

Selected content that fits the target projection budget is used verbatim with zero
minification calls. Content beyond the target first exhausts the available lossless
code-owned filtering and deduplication. Only then does one unresolved semantic
question exist—what smaller text preserves the selected authority for the current
instruction—and the `context_minification` station may run. Oversized sets are reduced
hierarchically in bounded groups. Each round must make measurable progress; an invalid
result or a no-progress round fails explicitly. One extra byte never creates an
artificial terminal failure, while a real per-call or provider resource limit still
fails loudly.

Model routes are qualified station profiles, not parameter-count classes. The station
contract stays stable while deployment configuration resolves it to one exact server
model. Context relevance has no browser inference provider or alternate result path.

## Human-readable projections

PostgreSQL remains authoritative until the service stops. For terminal jobs, an explicit server-authorized
operation may atomically generate disposable, read-only inspection files under
`.omni/runs/<job-id>/`, including a manifest, task-ledger state, and bounded artifact,
evidence, and call indexes.

These files are an inspection ABI for humans and external tools. Models do not edit
them, workers do not read them as authority, and deleting them does not delete state
during the current database lifecycle. Conversely, retaining them across startup
does not restore or reconstruct discarded internal state.
Default exports omit prompts, responses, native thinking, source excerpts, web bodies,
memory, diffs, command output, job metadata, and private benchmark evaluation. The
repository inventory and Git-state identity exclude `.omni/**` before counting and
hashing, so an inspection export cannot invalidate repository truth.

## Behavior-first implementation sequence

Context recording mirrors proven cognition behavior; it must not define that
behavior. New Task Ledger, Working Set, projection, same-runtime transaction, and
provenance work for the replacement cognition path is blocked until the in-memory objective
machine passes its executable gates:

1. **Deterministic closure** — a missing prerequisite is acquired through registered
   producers and the objective completes with zero model calls and no provider
   configured.
2. **One named semantic gap** — deterministic closure stops at one genuine ambiguity;
   one station returns one opaque candidate ID; code incorporates the typed fact and
   completes.
3. **Procedural transfer** — the same control rule drives a bounded world whose
   mechanics, legal transitions, backtracking, and completion remain code-owned.
4. **Read-only repository work** — exact snapshot, index, parser, symbol, reference,
   and test providers run under code control; inference can select only among an exact
   bounded semantic remainder.
5. **Bounded front door and workload compilation** — code first records exact workspace
   state as a typed, hashed fact. For an existing workspace, one bounded raw call returns a
   source-ordered repository-fact-question candidate inventory or the registered semantic
   absence. The inventory is untrusted candidate data; code owns the queue and exact
   deduplication. Each unique candidate first receives one necessity/unresolvedness relation
   against only the request, current facts, and candidate. A not-necessary or already-resolved
   candidate evaporates without a pairwise relation or evidence acquisition. Only a necessary
   unresolved candidate is compared with one accepted question at a time through a separate
   same-fact/distinct-facts relation. Accepted questions remain immutable, and a same-fact
   candidate evaporates. Code resolves each necessary distinct question exactly once through
   a registered deterministic provider, formalizes selected evidence as
   compact source-backed facts, and makes those facts visible to later queued candidates. Queue
   exhaustion—not a model absence or completeness claim—returns the context without a second review, completeness call, or bound-overflow
   request for more work. Exactly one bounded, source-ordered requirement-inventory call
   then returns either `NO_RUNTIME_REQUIREMENT_CANDIDATES` or between one and the
   code-owned maximum positive atomic finished-software runtime-outcome candidates. Code
   parses and counts the returned lines mechanically. No semantic station pre-counts the
   inventory, and no pre-count receipt exists. Inventory generation is
   untrusted candidate intake, not authorization or a completeness claim. Every positive
   candidate enters the ordinary authorization-first sieve. The inventory generator
   splits independent outcomes and may express only the
   literal core operation or governed result inherent in a purpose-denoting product or
   category name. Independently verifiable means that the governed result exists; it does
   not authorize an unstated presentation, delivery, storage, interface, or output format.
   It omits construction constraints, customary features, and speculative
   enhancements. Code owns exact deduplication and the queue.
   Every remaining candidate first receives one request-entailment authorization relation; an
   unauthorized candidate evaporates before classification. Only then does code ask kind and
   cardinality. An authorized candidate that still proves mixed or compound may return one bounded
   partition whose children re-enter the same queue and authorization boundary. A structurally invalid
   station response fails at that station. A structurally valid candidate whose semantic content
   remains malformed, cyclic, over-depth, or over-capacity dies without blocking an independent
   candidate or reopening accepted state. There is no later requirement generator or aggregate review.

   Product or category identity never licenses customary features, prerequisites, enabling behavior,
   or likely consequences. Its literal core action or governed result is proposed once at inventory
   intake and has no authority until its own candidate passes the ordinary sieve. Separately named
   controls, elements, states, persistence behavior, channels, formats, or other behavior still require
   their own authorized candidates. Candidate interpretation preserves semantic subjects: when the
   software produces a derived value from actor-selected or actor-supplied rule-bearing inputs, the
   application applies the rule and exposes the result; an actor or external source that supplies an
   already-derived value remains the source. Surface, technical or structural format, generic test or
   build, and deployment constraints are non-runtime and remain owned by their narrow code paths. A
   construction-workflow descriptor attached to the builder's act is non-runtime unless the request
   assigns that behavior or data flow to the completed application. A builder-directed test or
   verification clause adds no runtime outcome when it merely confirms that an accepted governed result
   is produced or conforms to the same determining rule. An explicitly different rule, external
   reference, scope, tolerance, event or observation time, retention boundary, time bound, delivery
   channel or recipient, output format, or state remains a distinct outcome when authorized.

   Only an authorized task-local one-outcome candidate is compared pairwise with one retained
   requirement at a time. The station returns only `SAME_RUNTIME_OUTCOME` or
   `DISTINCT_RUNTIME_OUTCOMES`; the model sees only the two statements, while code binds the job and
   result to the complete candidate kind/cardinality receipts and the retained requirement's
   result-relation receipt. Language that only restates conformance of the identical value to the
   identical determining rule is `SAME_RUNTIME_OUTCOME`; another determining rule, reference, scope,
   response, event, observation time, retention boundary, time bound, delivery channel, recipient,
   format, or state remains distinct. An exact or semantic duplicate evaporates, and the retained
   requirement is never reviewed or reopened. Only a distinct candidate proceeds to the separate three-way
   result-relation question. It returns only that the outcome needs no derived result, explicitly states an
   independently computable determining relation, or omits that relation. A named existing per-item grouping
   key completely determines group membership; its origin and unasserted ordering are not missing. An expression,
   formula, predicate, or named operation supplied, configured, or selected by an actor is a rule-bearing input.
   A named intrinsic or mechanically observable property such as a dimension, length, or count is determined by
   its governed object and that property; the candidate need not restate the property's measurement procedure.
   When an actor performs a calculation, its chosen operation and operands are observable runtime inputs; neither
   form must be fixed before runtime. A named family of result-bearing operations over governed inputs is one
   parametric determining relation; its concrete family member and operand values may be selected or supplied at
   runtime and need not be enumerated or fixed in the candidate. A bare quality claim or output described only as calculated, computed,
   evaluated, generated, or selected remains missing. Selection,
   ordering, transformation, aggregation, measurement, or decision can establish a derived relation even when that value is the only rendered output. A named result-bearing operation applied to its governed object still asserts the resulting value when phrased as an action; action form does not turn a transform, read, extraction, decode, ordering, calculation, or selection into a result-free event. Actions, controls, unchanged rendering, state transitions, artifact availability, and event occurrences are `NO_DERIVED_RESULT` when they assert only that behavior. A qualitative descriptor on an action, event, or message does not create a derived value. The result carries the exact
   candidate hash and hashes of both complete input receipts; code validates those identities before the
   relation can be retained. The last value first opens one separate entailment relation over only the
   immutable request, established verified application facts, exact current candidate, and complete
   missing-relation authority. The same minimal code-owned context projection used by requirement
   interpretation is rendered; source identities, evidence-need identities, and hashes stay code-only.
   Its receipt binds the request, validated `ApplicationContext`, candidate, and missing-relation receipt.
   `NO_EXACTLY_ONE_DETERMINING_RELATION_ENTAILED` discards only that candidate before correction and cannot authorize a guessed policy.
   Only `EXACTLY_ONE_DETERMINING_RELATION_ENTAILED` opens one exact candidate correction, and that positive
   receipt is mandatory correction input. Its context contains only the immutable semantic request,
   minimal verified facts, current candidate, code-established defect, and positive grounding relation.
   Code preserves every retained leaf and reruns exact deduplication, authorization, kind, cardinality, outcome-relation, and
   result-relation closure. A repeated omission exhausts the one-correction bound and discards only that candidate; there is no reviewer.
   Substring, interval, overlap, source-order, punctuation, and exact-quote allocation are not
   authority checks. Queue exhaustion freezes the currently accepted functional objective for this
   iteration without a coverage, review, completeness, or `REQUIREMENT_REMAINS` call; later objectives
   may continue iteratively from verified reality. Rejected, speculative, or over-capacity candidates
   may be retained only as non-authoritative follow-up suggestions outside the current ledger,
   workload, verifier, and completion criteria; a later explicit user objective must send one through
   the ordinary sieve before it can become work. Only when a task-local leaf survives does
   code resolve product, surface, or deployment semantics, and each runs only at its first
   concrete consumer; an empty accepted queue creates no downstream interpretation work.
   Code projects each accepted task-local runtime implementation requirement directly into one frozen task in
   accepted source order. The task contains only its code-owned task identity, requirement identity,
   and exact accepted requirement; there are no objective, behavior, acceptance-criterion,
   dependency, schedule, or completion model calls. Code validates every leaf and assembles the
   typed job specification itself. Invalid semantic leaves fail at their owning station; there is
   no generic response-correction station or retry path.
   Code preserves user authority separately from derived build decisions. A private authority pairs the
   raw production-request digest with the path-redacted semantic request, and only the redacted request is
   model-visible. Code rebinds accepted receipts to the raw digest after semantic resolution and rejects
   individual or coordinated digest drift. It assigns identity and order, and builds a separate
   result-relation validation plan bound to both that authenticated request digest and the frozen workload SHA.
   The plan has one task/requirement/receipt binding per frozen task, projects exactly one binding into
   the current task stage, and remains code-only: it is absent from the frozen task, task context, and
   every model envelope. Code freezes only validated state and executes one task at a time with
   the minimum sufficient authoritative task projection, and alone decides completion. A
   task-local source projection contains its exact accepted product identity, exact accepted
   requirement, and direct declared capabilities, but no derived aggregate product summary or
   sibling requirement. A
   static code-owned harness alone renders the exact public feature with its runtime and capability
   identity. The generated browser acceptance declaration receives no render, JSX, component, runtime,
   or source authority. Code first stages the accepted implementation without its unresolved verification
   declaration, closes that implementation-only projection through the real typechecker and registered
   deterministic compiler corrections, and revalidates its source and public-surface shape. Only then may
   code extract the declaration's sole implementation-derived projection: a bounded public-interaction
   receipt containing allowlisted intrinsic control roles, canonical counts/ordinals, literal accessible
   names and hints, value kinds, explicit public action claims, and named dynamic `<output>` selector facts.
   Each visible output has a unique exact literal accessible name, direct dynamic-only nonmixed content,
   code-proven dataflow from declared state/capabilities or event/prior-state-derived local state, and an
   implicit `status` role. Literal expressions, constant aliases, static memoization, and constant setter
   calls cannot obtain output authority. Its receipt fact contains only the selector name, never static text, a
   current value, a JSX expression, handler source, an expected result, or ordinary dynamic text outside
   a registered output.

   Extraction is fail-closed over registered intrinsic elements, an exact per-attribute grammar, accessible
   and available ancestry, and an allowlisted Tailwind utility grammar. Unknown, effectful, namespaced,
   duplicate, spread, or wrongly typed attributes fail; forms are unavailable and buttons require exact
   `type="button"`. Custom or unsupported intrinsics, dynamic visibility state,
   hidden/inert/ARIA-hidden or disabled ancestry, and any unproved
   opacity, pointer-event, clipping, transform, zero-size, screen-reader-only, or arbitrary Tailwind form
   fail. All unbound runtime identifiers require an exact deterministic ECMAScript/React allowlist. Code
   statically binds JSX event handlers and rejects browser-host, DOM-selection, navigation, network,
   storage, audio, scheduling, dynamic import, host metadata, dynamic-evaluation, reflection, alias,
   unresolved computed-property, computed-event, mutation, and event escape authority. Only literal or
   immutable numeric-constant domain indexing is admitted. Only a direct read-only `value` or `checked` leaf through `event.target` or
   `event.currentTarget` is admitted, represented by the canonical `event.target.value` and
   `event.currentTarget.checked` forms.

   Browser `state` and `capabilities` are immutable authority. Extraction rejects direct or aliased writes
   and mutators. Every `SharedValue` fallback and publication accepts only bounded dense arrays or plain
   records, atomically rejects cycles, accessors, hidden/symbol keys, custom prototypes, and unsupported values, and deep-freezes accepted graphs before exposure.

   Code freezes the receipt, result-relation receipt, and internal element-ID sequence before verification
   generation, then re-extracts and compares them after every staged attempt and before final execution.
   IDs are never model-visible, must remain globally unique across task surfaces, and cannot collide with
   reserved code-owned mount IDs. The verifier grammar is exhaustive: one executable function contains a
   non-empty flat sequence of direct registered `fireEvent` calls, direct screen-grounded expectations, or
   an awaited `waitFor` whose parameterless callback contains only direct expectations. Queries are direct
   throwing allowlisted `screen` calls with exact receipt roles, names, cardinality, and literal indexes;
   asynchronous queries and `waitFor` are explicitly awaited; events use compatible static payloads.
   Declarations, branches, loops, returns, helpers, nested/dead closures, aliases, optional chains, and
   every unregistered statement, query, event, matcher, or consumer fail.

   An explicit derived-result relation requires a singular receipt-named implicit-`status` output asserted
   with non-negated `toHaveTextContent` and an anchored literal regular expression, even when no event is
   needed; with interactions, that exact assertion must follow the final `fireEvent`. Any other interacting
   verification also needs a qualifying outcome after its final event. Only when no named output owns that
   outcome may a compatible checked, validity, value, or display-value control-state assertion qualify.
   Generic text and presence assertions do not prove a derived result. An action claim may select only among
   alternatives already established by the exact requirement and remains independent from static inputs;
   it is never a missing relation, proof, or expected-output authority. There is no acceptance-grounding
   reviewer. A generated verifier's receipt-grounding or grammar rejection, and any staged execution
   failure originating in the generated verification block, is terminal and cannot authorize
   implementation-repair inference. Code verifies the grounded artifact before advancing.
6. **Charmander handoff** — cognition produces one existing bounded declaration job;
   code parses, stitches, formats, stages, tests, applies, and reconciles it.
7. **Failure-specific replacement** — ordinary invalid semantic leaves fail explicitly and cannot
   dispatch a generic correction job. The target-tree station alone may replace its complete raw
   hierarchy once after one exact code-proven tree defect. Source repair remains the separate
   guidance-instruction then executor-node boundary. Neither path can reconstruct aggregate state,
   review valid output, return a merge patch, or author an acceptance control plane.
8. **Incompatible production cutover** — remove the universal model-action path and
   its schemas, recovery consumers, and provider eagerness. There is no fallback or
   feature flag.
9. **Current-runtime state** — only after the preceding behavior survives the
   production path do PostgreSQL Task Ledger state, accepted facts, gap records,
   artifacts, Working Set, projections, same-runtime transaction integrity,
   provenance, and fresh-start reset evidence become promotion work.

Existing ledger and context primitives remain useful foundations, but their existence
is not evidence that the replacement cognition behavior works. Current-runtime
recording may add authority and transactional integrity; it may not add startup
recovery, a planner, an alternate transition rule, or other behavioral semantics.
There is one implementation of each promoted primitive.

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

## Startup reset boundary

`database/setup.sql` is the one authoritative definition of Omnidex's internal
PostgreSQL schema. Every Omnidex process/service startup drops and recreates the
configured dedicated schema from that file. Previous jobs, ledger events and
materializations, Working Sets, projections, memory, evidence, attempts, leases, and
repository-mutation journal rows are intentionally discarded.

There is no internal database migration sequence, restart continuation, phase
takeover, PostgreSQL replay, or in-place upgrade contract. Filesystem mutations from
a stopped process may still exist as ordinary repository reality, but neither those
files nor exported `.omni` projections restore the previous job or its authority. A
subsequent request starts with new internal identity and proves the current repository
state normally.

During one running service, repository mutations may use a
prepared/applying/applied/indeterminate journal. The journal binds the exact job
generation, step, worker, source snapshot, contract, stage, full patch, and
source/post file states. A same-runtime retry classifies the complete current
repository inventory: exact source permits the same patch, exact post permits atomic
generated-diff evidence finalization, and any other state fails as indeterminate.
This journal provides transaction and retry safety only within the current database
lifecycle.

The cross-cutting step-attempt lease authority is likewise current-runtime only. Each
worker-originated write is bound to one monotonically increasing attempt; an expired
attempt may be reclaimed as a later attempt while the service remains running, and
the stale worker is fenced from subsequent writes and completion. Tests may prove
expiry, reclaim, same-runtime journal finalization, and stale-write rejection with
concurrent workers. They must not claim continuation after stopping and starting the
Omnidex service.

## Proof gates

The first proof is current-runtime continuity plus an exact startup reset, not a
large-repository edit:

- clear all model conversation state after every completed step while the service
  remains running;
- select the same next runnable task from code-owned current state;
- preserve active constraints, accepted decisions, unresolved failures, and completed
  work during that database lifecycle;
- never reuse rejected hypotheses or repeat completed work;
- stop and start the Omnidex service, then prove the schema was recreated from
  `database/setup.sql` and contains none of the prior internal rows.

Required promotion invariants are 100% same-runtime state validity, 100% fresh-start
reset, zero prior internal rows after startup, zero authority violations, zero stale
references admitted to model context, no end-to-end correctness regression, and a
material reduction in context and duplicate acquisition.

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
