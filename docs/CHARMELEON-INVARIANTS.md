The correct agnostic architecture is a recursive, code-owned objective machine. The “agent” is the running workflow, not an LLM persona. Models are stateless semantic stations invoked only when the machine reaches a precisely named uncertainty it cannot resolve itself.

The operating principle is:

Compile a large problem into code-held objectives. Drive those objectives toward completion with deterministic machinery. Invoke an LLM only to cross one irreducibly semantic bridge, accept a tiny bounded result, and immediately return control to code.

The target may be 90% code and 10% inference, but the authority split is stricter:

State authority       100% code
Execution authority   100% code
Completion authority  100% code
Inference authority     0%

The model proposes information. It never makes reality happen directly.

⸻

1. The core machine

Every workload is represented by five fundamental objects.

Object	Meaning	Owner
Objective	Something that must become true	Code
Fact	Something currently established about reality	Code, tools, user authority
Operation	A deterministic capability with prerequisites and effects	Code
Gap	One precisely named uncertainty preventing deterministic progress	Code constructs; model may resolve
Artifact	A concrete output such as source, report, plan fragment, or decision	Code stores and verifies

A minimal conceptual model is:

type Objective struct {
    ID          ObjectiveID
    Desired     Predicate
    Acceptance  []Predicate
    Parent      *ObjectiveID
    DependsOn   []ObjectiveID
    Status      ObjectiveStatus
}
type Fact struct {
    ID          FactID
    Predicate   Predicate
    Value       Value
    Authority   Authority
    Evidence    []EvidenceRef
}
type Operation struct {
    ID          OperationID
    Requires    []Predicate
    Produces    []Predicate
    Invalidates []Predicate
    Executor    ExecutorID
    Cost        Cost
}
type CognitiveGap struct {
    ID          GapID
    ObjectiveID ObjectiveID
    Kind        GapKind
    Question    string
    Candidates  []Candidate
    Evidence    []EvidenceRef
}

Crucially, an Operation is never an LLM tool. It is a capability of the machine.

The model does not receive:

read_file
grep
run_tests
move_north
unlock
edit_file
format

The code machine invokes those when its state says they are required.

The model receives something like:

Which candidate interpretation best explains the observed behavior?
A. dispatches all invitations immediately
B. schedules invitations over a configurable interval

And returns:

B

That is one semantic bridge. The letters are call-local opaque identities. Code maps
`B` to its retained candidate; the model never reproduces an internal identity or JSON
packet.

⸻

2. The universal control loop

The machine runs deterministic closure before considering inference.

Select ready objective
        ↓
Evaluate current authoritative state
        ↓
Apply all code-derivable progress
        ↓
Objective complete?
    ├── yes → verify and close
    └── no
         ↓
Identify missing prerequisite
         ↓
Can code acquire or produce it?
    ├── yes → execute producer and continue
    └── no
         ↓
Can code prove one next transition dominates?
    ├── yes → execute and continue
    └── no
         ↓
Construct one named CognitiveGap
         ↓
Render one minimal clean-desk inference
         ↓
Model returns one bounded answer
         ↓
Code validates and incorporates it
         ↓
Resume deterministic closure

Go-like pseudocode:

func RunObjective(ctx context.Context, id ObjectiveID) error {
    for {
        state := loadAuthoritativeState(id)
        if completionSatisfied(state) {
            return completeObjective(ctx, id)
        }
        transition, progressed, err := reducer.Reduce(state)
        if err != nil {
            return err
        }
        if progressed {
            if err := executor.Apply(ctx, transition); err != nil {
                return err
            }
            continue
        }
        gap, err := gapBuilder.Build(state)
        if err != nil {
            return err
        }
        if gap == nil {
            return ErrObjectiveUnresolvable
        }
        projection := contextBuilder.BuildForGap(gap, state)
        answer, err := stationRegistry.Resolve(ctx, gap.Kind, projection)
        if err != nil {
            return err
        }
        validated, err := gapValidator.Validate(gap, answer)
        if err != nil {
            return err
        }
        if err := stateReducer.ApplyResolution(ctx, validated); err != nil {
            return err
        }
    }
}

The central invariant is:

A model call is illegal unless code can name the unresolved question that made deterministic closure stop.

No generic:

What should I do next?

No universal CognitionDecision.

No action + arguments + evidence + planning + memory-management bundle.

⸻

3. The workload lifecycle

The large problem moves through code-owned workflows:

INTAKE
  ↓
DISCOVERY / EVIDENCE ACQUISITION
  ↓
CODE-OWNED OBJECTIVE / TASK COMPILATION
  ↓
EXECUTION
  ↓
DETERMINISTIC VERIFICATION
  ↓
FAILURE LOCALIZATION / RECONCILIATION
  ↓
COMPLETE

These are not rigid one-time phases. They are recursively callable workflow types.

Execution can establish that another registered prerequisite must be acquired.

A real verification failure can reopen only its code-proven owning objective or
generated block.

Evidence can reveal another code-owned prerequisite edge.

A child objective may itself run the entire lifecycle.

The machine owns that recursion. There is no model review phase, planner station, or
model-authored correction objective.

⸻

4. Intake: turn vague language into a workload

The input may be vague:

Invitations all go out at once. Make it configurable per client without breaking current defaults.

Code immediately preserves:

* the exact user instruction;
* explicit constraints;
* known project/repository identity;
* existing organizational policy;
* any directly derivable terms.

Then bounded Charmander-style semantic stations establish only the semantic leaves
that code cannot derive. During intake, each call has one result: one bounded atomic
runtime-candidate inventory, one candidate-bound relation, or one code-authorized
partition artifact. Product identity, delivery surface, deployment semantics, and other
downstream leaves are resolved only after a task-local requirement survives and only at
their first actual consumer.

Exactly one requirement-inventory station returns either
`NO_RUNTIME_REQUIREMENT_CANDIDATES` or between one and the code-owned maximum positive,
source-ordered runtime-outcome candidate lines. Code parses and counts those lines
mechanically. No semantic station pre-counts the inventory, and no pre-count receipt
exists. At this one intake boundary the inventory splits independent
outcomes and may express only the literal core operation or governed result inherent in a
purpose-denoting product or category name. It omits construction constraints, customary
features, and speculative enhancements. The inventory is not an accepted specification,
feature list, task queue, authorization, or completeness claim. Code validates the
inventory and creates the only authoritative candidate queue.

For each queued candidate, code first removes exact byte duplicates mechanically.
The first semantic call asks only whether the candidate's complete meaning is entailed
by the current request. A speculative or invented candidate is discarded before any
kind, cardinality, partition, or result analysis. An authorized candidate is then
classified by kind and, when it contains runtime meaning, cardinality. Only a
code-established mixed-kind or multiple-outcome result permits one bounded lossless
partition; its source-ordered children return to the same code-owned queue.

An authorized atomic runtime candidate is compared with retained requirements one
pair at a time through a same-or-distinct semantic relation. Exact and semantic
duplicates evaporate. Only a distinct, grounded functional outcome is retained and
projected into one frozen task in source order. Product identity, delivery surface,
technical and structural format, generic test/build/verification, and deployment
constraints remain outside this queue. Only after at least one task-local requirement
survives do their narrow code-owned consumers resolve the necessary semantic leaves from
the immutable request, each at its first use.

Rejected, speculative, duplicate, non-runtime, unresolved, or over-bound candidates
cannot block the queue, reopen accepted state, or manufacture another review. Queue
exhaustion freezes the retained functional objectives for the current iteration, including
an empty accepted set. A product or
category name does not imply customary extra features as prerequisites, and no later
product-name generator reopens the queue; a later user turn can request another bounded
iteration. If code retains a rejected or speculative candidate as an optional suggestion,
it remains provenance-bound but outside the current ledger, workload, verifier, and
completion predicates until that later user turn makes it authoritative through the
ordinary sieve. No model emits an objective graph, acceptance contract, dependency,
schedule, or plan. Text-span heuristics are not an authority substitute for semantic
validation.

⸻

5. Discovery and research

There is no “research model.”

Omnidex conducts research using code-owned providers:

exact text search
symbol lookup
reference traversal
dependency graph expansion
route/job/configuration discovery
full-text and trigram search
semantic index only after exact methods fail
web or external evidence providers

The workflow:

Objective requires evidence F
        ↓
Resolve the registered provider and exact code-owned query authority
        ↓
Run exact and structural providers
        ↓
Reduce and deduplicate results
        ↓
Enough evidence?
    ├── yes → inspect relevant material
    └── no
         ↓
Fail explicitly unless another registered deterministic evidence source is available

There is no search-term station. For web research, the exact initial query is an
explicit typed input consumed by code. For repository work, snapshot indexes,
parsers, symbols, references, and registered semantic-excerpt retrieval remain
code-invoked providers. A model never creates search strings, chooses a provider, or
constructs provider arguments.

If bounded plausible results remain, code walks them in their authoritative source
order. Until the configured citation cap is reached, one tiny station resolves one
candidate-bound relation at a time:

Is this exact evidence candidate directly relevant to this exact requirement?
Return DIRECTLY_RELEVANT or NOT_DIRECTLY_RELEVANT.

Code validates each pair-bound result, retains the candidate identity only for a
direct relation, and closes the sieve when the cap is reached or its code-owned queue
is exhausted. The model never sees other candidates, accepted identities, queue
state, or an absence/completion choice.

The model does not decide to search, read, traverse, or stop researching.

Research completion is code-owned:

implementation owner known
configuration owner known
relevant direct dependencies known
applicable tests known
required semantic uncertainties explicitly represented

⸻

6. Planning: compile objectives, not prose

Planning is not an LLM writing a grand master plan.

Code derives all plan structure it can from:

* objectives;
* known predicates;
* prerequisites;
* operation producers;
* required verification;
* dependency edges;
* current blockers.

Suppose code knows:

Configuration change must precede scheduler change.
Scheduler change must precede behavioral verification.
Compatibility verification is independently required.

It builds those edges itself.

The direct coding path does not ask a model to plan or decompose the workload. It
freezes one task per accepted atomic functional requirement. A partition call is legal
only for one candidate whose mixed kind or multiple runtime outcomes were already
established, and it returns only narrower candidate text to the code-owned queue.
Other cognition consumers may ask one bounded semantic question among code-enumerated
candidates only when one relation or fact is genuinely unresolved; the result binds
that one leaf and cannot expand the objective graph.

There is no planner model. Code alone owns decomposition, scheduling, task state,
retries, and execution.

⸻

7. Execution: code runs until it reaches a semantic hole

For every objective, code first attempts deterministic closure.

Repository example:

Need implementation owner
→ exact/structural search
→ three candidates
Need candidate source
→ inspect all three
Need signatures/imports/references
→ parser and index derive them
Need applicable tests
→ graph lookup and naming conventions derive them
Semantic ambiguity remains:
Which candidate actually owns invitation timing?
→ tiny model selection

Once the change surface is known, cognition hands the bounded source-generation problem to Charmander:

target declaration
exact immutable signature
desired behavior
accepted semantic decisions
direct dependencies
allowed symbols
applicable invariants

The generation station returns one ordinary plain-text implementation body. Code places
it inside the exact immutable declaration; the model never reproduces the signature,
parameters, schema, AST structure, or another mechanically known byte.

Then code owns:

parse
structural validation
replacement
formatting
type checking
compilation
focused tests
broad tests
artifact persistence

The cognition machine does not turn into a coding agent.

It navigates the workload until it can create a valid Charmander job.

⸻

8. Verification and failure reconciliation

Verification consists of exact checks:

AST changes
forbidden-state checks
signatures
types
compiler
tests
acceptance predicates
expected files
unexpected files

Valid state advances without a semantic review call. A real parser, compiler, test,
proof-obligation, or workspace mismatch is reduced by code to the smallest owning
objective or generated block. Accepted state is preserved.

For a supported source-body failure, code must prove one exact path-free defect and its
exact mutable byte span. Only the same persisted generation job and same model context
may continue. It receives one necessary semantic question and that span alone and
returns ordinary replacement text for the span, not another complete body, repair
instruction, or replacement packet. Code verifies and splices into its retained base,
then repeats declaration assembly and validation for at most three total attempts while
every accepted byte outside the span and every sibling remain untouched. There is no
generic response correction, critique station, accept-or-replace gate, alternate model,
or review-again loop.

⸻

9. The small model stations

There should be no general “researcher,” “planner,” “coder,” or “reviewer” persona.

There should be small, consistent station contracts:

Station	Input	Output
InventoryRequirementCandidates	Exact immutable request	Registered semantic absence or bounded 1..N source-ordered atomic runtime-outcome candidates
AuthorizeRequirementCandidate	Exact request, established facts, and one untrusted candidate	One entailment relation
ClassifyRequirementKind	One authorized candidate	One registered kind
ClassifyRequirementCardinality	One authorized runtime candidate	One registered cardinality
PartitionRequirementCandidate	One candidate and one established compound relation	Bounded source-ordered child candidates
RelateRequirementDuplicate	One atomic candidate and one retained requirement	One same-or-distinct relation
RelateContextCandidate	Exact instruction, one candidate content	One relevance relation
ResolveReference	One phrase and candidate symbols	One candidate ID or none
ClassifyRelationship	Two bounded facts	One relation enum
GenerateImplementationBody	Exact local source responsibility	Ordinary implementation-body text
InventoryAnswerParagraphs	One exact requirement and selected evidence	Bounded untrusted paragraph candidates
AuthorizeAnswerParagraph	One candidate paragraph, exact requirement, and the complete supplied evidence set	One responsiveness-and-collective-support relation
RelateParagraphEvidence	One authorized candidate paragraph and one evidence capsule	One citation-attribution relation

Each station gets:

* one verb;
* one subject;
* only the minimum evidence needed for that question;
* one ordinary semantic result, except that a genuine closed choice returns one opaque ID;
* no authority.

Closed-choice cardinality is literal: zero options use the station's explicit
zero-option behavior, one option is used immediately by code with zero model calls and
no rejection, and two or more options may invoke one opaque-ID selection call.

Each station has one immutable configured route whose model has qualified for that
exact contract. Models do not select routes, and an unavailable route fails instead
of falling back. Changing model capacity never enlarges the station responsibility.

⸻

10. Scaling to big problems

Large scope is handled by recursively compiling it into smaller objective graphs.

Top-level objective
├── Research objective
│   ├── Consume exact typed query authority
│   ├── Identify architecture
│   └── Establish current behavior
├── Code-owned compilation objective
│   ├── Determine change surface
│   └── Determine verification
├── Execution objective
│   ├── Generate implementation body A
│   ├── Generate implementation body B
│   └── Apply configuration change
└── Verification objective
    ├── Exact verification
    └── Failure localization

The scheduler only works ready nodes.

Each node has explicit completion predicates.

Every blocker must do one of four things:

resolve deterministically
spawn a child objective
open one named cognitive gap
fail explicitly

No work item is allowed to remain “the model is thinking about it.”

The active Context Projection contains only the current local objective and its necessary state. The overall workload can be enormous while every model call remains tiny.

⸻

11. Context is compiled per gap

For a gap, the projection includes only:

current objective
exact unresolved question
active constraints
candidate IDs
evidence supporting the candidates
output contract

It does not include:

chat history
previous model responses
the entire workload graph
unrelated objectives
repository tree
all search results
all prior failures
generic model instructions
unused capabilities

The exact provenance questions should be:

Why was this fact included?
Which objective needs it?
Which evidence supports it?
Why were the other candidates omitted?
What uncertainty is this call resolving?

Provenance serves the behavior. It does not postpone the behavior.

⸻

12. Domain-agnostic operation

The core runtime does not know whether the environment is a repository, maze, game, or simulation.

The domain adapter supplies facts, operations, and completion predicates.

Domain	Code owns	Model crosses
Maze	Map, legal movement, visited cells, forced moves, goal	Strategic choice at a true unresolved fork
Repository	Search, reads, parsing, references, tests, edits	Semantic interpretation or bounded source generation
Chess	Board, legal moves, checks, captures, rules	Strategic selection among unresolved candidates
Poker	Legal bets, pot, cards, odds, history	Opponent/strategy interpretation
Blackjack	Rules and optimal basic strategy	Usually nothing
Sims-like simulation	Needs, state, schedules, interactions	Narrative interpretation and personality-consistent semantic choices
Web research	Query construction, fetching, indexing, citation, deduplication	Candidate relevance and semantic synthesis

The same runtime loop remains:

objective
→ deterministic closure
→ named gap
→ tiny inference
→ deterministic closure
→ completion

⸻

13. What 90% code / 10% LLM means operationally

It should be measured, not just described.

For every workload record:

deterministic operations executed
facts acquired without inference
objectives completed without inference
cognitive gaps opened
model calls
model tokens
artifact-generation calls
same-job source-body continuation calls by exact defect

Useful target metrics:

≥ 90% state transitions are deterministic
100% side effects are code-executed
100% completion decisions are code-owned
100% model calls bind to one named gap
0 generic "what next?" model calls
0 LLM-selected tools
0 LLM-managed memory or task state

The ratio will vary by workload. A highly semantic task may use more inference. The important rule is:

No inference is spent where code can compute the result exactly.

And authority remains 100% code even when inference volume rises.

⸻

14. Persistence comes after behavior

The in-memory kernel should prove the semantics first.

Then persistence mirrors the proven objects:

objectives
facts
operations executed
gaps opened
gap resolutions
artifacts
verification outcomes

Persistence must not introduce new behavioral semantics.

The order should remain:

prove in-memory behavior
→ harden types
→ persist
→ restart
→ replay
→ provenance
→ promotion evidence

Never again:

design
→ persist
→ prove persistence
→ prove replay
→ finally discover behavior was wrong

⸻

15. Concrete implementation ladder

Gate 1 — deterministic closure

Terminal example:

inspect_hint produces code
unlock requires code
unlock produces terminal.unlocked

Required result:

goal complete
LLM calls = 0

Already demonstrated in the new reference path.

Gate 2 — one genuine semantic gap

Code reaches one real ambiguity among at least two options. Qwen sees only the bounded
semantic context and opaque option descriptions and returns one opaque letter.

Required result:

goal complete
LLM calls = exactly 1

Also demonstrated in the reference path.

Gate 3 — procedural maze

Code owns mechanics and forced transitions. Qwen is invoked only at genuine unresolved route choices.

Run repeated fresh cases and measure:

deterministic transitions
semantic forks
model calls
wrong strategic choices
context tokens
completion

Gate 4 — read-only repository workload

Examples:

Find the implementation responsible for invitation timing.
Identify which configuration controls it.
Identify applicable tests.
Explain the relationship between two symbols.

No source mutation.

Gate 5 — bounded semantic intake and code-owned workload compilation

Take a vague request through one bounded requirement inventory that returns the exact
registered semantic absence or 1..N positive candidates within the code-owned maximum.
Code parses and counts its lines mechanically; no semantic station pre-counts the inventory,
no pre-count receipt exists, and inventory generation is not a completeness claim. Code owns the
source-order candidate queue. Every positive candidate enters the ordinary sieve.
Authorization precedes candidate semantic classification;
candidate kind/cardinality receipts permit only bounded candidate partitioning, and
pairwise relations remove semantic duplicates. Queue exhaustion closes intake. Code must
project each accepted task-local runtime requirement into exactly one frozen task and
work that deterministic graph to completion without a planner call. Rejected
candidates never reopen accepted work; the current accepted functional objective owns
completion, while a later user turn begins another bounded iteration. Product, surface,
deployment, and other downstream semantic leaves run only after a surviving requirement
exists and only at their first consumer.

Gate 6 — Charmander generation handoff

Cognition establishes the exact change surface and behavior. Charmander requests bounded
implementation bodies; code supplies declarations and verifies them.

Gate 7 — deterministic failure localization and bounded source-body correction

Code maps a real defect to one owning body job. Only that same persisted job/model
context may continue; valid retained state is preserved and no review model is invoked.

Gate 8 — persistence and recovery

Only now make the proven architecture durable.

⸻

16. Non-negotiable anti-drift rules

These should be architecture tests:

1. No universal CognitionDecision model contract.
2. No model-visible generic tool/action catalog.
3. No model call without a persisted or in-memory named gap.
4. No inference while deterministic progress exists.
5. No model-originated side effect.
6. No model-controlled Working Set, ledger, scheduler, or completion.
7. No raw repository navigation by the model.
8. No broad role persona such as “research agent” when a tiny station suffices.
9. Every behavioral change must first pass a vertical test before persistence work.
10. Repeated successful gap resolutions become candidates for deterministic skills later.

That final rule is the Charizard path:

model resolves recurring gap
        ↓
verified history accumulates
        ↓
one-need procedure station returns one candidate instruction leaf
        ↓
replay and held-out testing
        ↓
skill becomes a registered code-owned producer
        ↓
future workload needs fewer model calls

The system should become less dependent on inference as it learns, not more.

⸻

The architecture in one diagram is:

                  VAGUE PROBLEM
                       ↓
                OBJECTIVE COMPILER
                       ↓
               CODE-HELD WORKLOAD
                       ↓
             DETERMINISTIC CLOSURE
          ┌────────────┴────────────┐
          │                         │
    progress available        semantic gap
          │                         │
          ▼                         ▼
      CODE ACTS              TINY LLM STATION
          │                         │
          │                  candidate / leaf
          │                         │
          └────────────┬────────────┘
                       ▼
                 CODE VALIDATES
                       ↓
               OBJECTIVE UPDATED
                       ↓
          GENERATE / VERIFY / RECONCILE
                       ↓
                    COMPLETE

That is the agnostic machine: big problems become evolving graphs of tiny code-held objectives; deterministic workflows do nearly all the work; models fill only the exact semantic holes that remain.
