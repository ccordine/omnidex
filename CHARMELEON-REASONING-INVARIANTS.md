Codex Advisement: Passive, Model-Agnostic Reasoning for Omnidex

Mission

Implement the new reasoning approach as a passive, non-authoritative advisory capability within the proven Charmander-first Omnidex architecture.

This is not a return to the rejected universal cognition loop, an adviser-controlled planner, a model council, or a tool-calling agent.

The governing principle is:

Models may generate potentially useful semantic material. Omnidex decides what is relevant, what is established, what becomes work, and what affects reality.

The advisory model may be Qwen, DeepSeek R1, Claude, GPT, Kimi, another local model, or any future provider. The architecture must not depend on a particular model family or on structured JSON output.

The model may return ordinary plain text. Omnidex treats that text as an untrusted information source, passes it through the existing retrieval and context-minification machinery, and exposes only relevant bounded fragments to later semantic stations.

Core architecture

The production flow is:

code-owned objective
        ↓
grounded authority and evidence
        ↓
code compiles one bounded advisory projection
        ↓
passive model returns free-form text
        ↓
raw advisory artifact
        ↓
chunk / normalize / tag / index
        ↓
context reduction and relevance selection
        ↓
bounded advisory capsules
        ↓
later Context Projections may include relevant capsules
        ↓
small semantic stations perform their existing bounded jobs
        ↓
code validates results and continues the workload

The advisory model is not in the execution path.

It cannot:

* select operations;
* select tools;
* run searches;
* read files;
* choose paths;
* create objectives;
* modify the workload graph;
* change the Working Set;
* update the Task Ledger;
* accept facts or decisions;
* generate repository mutations;
* select verification commands;
* declare completion;
* approve its own recommendations.

It provides text only.

Distinguish passive advice from typed semantic stations

Omnidex now has two different model boundaries.

Typed semantic station

Use this when code must consume an exact bounded result:

select one candidate ID
classify one relation
generate up to three search terms
return one source declaration
patch one exact semantic leaf

These stations may use JSON, enums, candidate IDs, or strict schemas because code acts on the validated result.

Passive advisory source

Use this when the model is asked to think broadly and provide potentially useful considerations:

risks
edge cases
ambiguities
alternative interpretations
questions worth checking
possible hidden constraints
verification ideas
architectural implications

Its output is plain text.

Code does not parse the text into operations or authority. The text enters the same information pipeline as a document, web result, historical note, or non-authoritative memory.

Do not force the advisory model into JSON merely for consistency.

Initial scope

Implement one narrow vertical first.

Trigger

Run an optional objective-level advisory pass only after the objective has a bounded grounded context and before the next major workload-compilation or semantic-execution stage.

For the first implementation, support one trigger:

post_grounding_objective_advisory

Do not add advisory calls after every task, operation, transition, model call, or failure.

The initial advisory request should contain only:

* the current code-owned objective;
* exact current user authority applicable to it;
* accepted constraints;
* grounded facts and evidence summaries;
* accepted decisions and invariants;
* explicitly unresolved semantic questions;
* a concise description of what kind of advice is useful.

It must not contain:

* the entire repository;
* the full workload graph;
* the chat transcript;
* previous prompts or responses;
* unrelated objectives;
* every artifact or failure;
* tool or operation schemas;
* mutation authority;
* completion state not relevant to the advisory question.

Suggested advisory instruction

The exact wording may be versioned in code, but the responsibility should remain equivalent to:

Review the objective and established evidence below.
Identify potentially useful implications, risks, edge cases, ambiguities,
alternative interpretations, hidden constraints, verification ideas, or
questions that subsequent work should keep in mind.
Do not issue commands.
Do not claim authority.
Do not assume unsupported facts.
Plain text is expected.

This is not a planner prompt.

Model-agnostic provider contract

The provider interface should accept:

objective projection
model/provider configuration
temperature and sampling configuration
input/output budget
trigger identity

and return:

plain-text final advisory content
provider/model identity
token and timing evidence where available
status

Do not require a model-specific output schema.

A high-temperature creative model may be configured for one advisory source. A lower-temperature analytical model may be configured for another. They must use the same passive contract.

Multiple configured advisors may run independently, but they must not converse, vote, debate, or recursively critique each other.

Each output becomes an independent advisory artifact.

Advisory artifact

Store or represent an advisory result with enough identity to preserve provenance:

advisory ID
objective ID
trigger ID and version
input Context Projection ID/hash
job/workload generation
provider
requested and effective model identity
sampling configuration
raw final-text hash
raw final text or artifact reference
byte/token count
created time
status
authority = non_authoritative_advisory

Do not store the text as a fact, accepted decision, user authority, project authority, or objective.

Do not request or persist hidden chain-of-thought as working context. The artifact is the model’s ordinary final advisory text.

Provider-native thinking metadata may remain diagnostic evidence if already supported, but it must never enter ordinary Context Projections or acquire authority.

Context reduction and relevance farming

Raw advisory text must never be dumped wholesale into every later prompt.

Feed it through the existing Omnidex context pipeline:

raw advisory
    ↓
bounded chunking
    ↓
deterministic metadata and tags
    ↓
exact / full-text / vector candidate retrieval
    ↓
context relevance reduction
    ↓
optional tiny relevance classification
    ↓
optional bounded minification
    ↓
advisory capsule

An advisory capsule must retain:

capsule ID
source advisory ID
source text span or chunk identity
objective/generation scope
plain-text minified content
source model/provider
authority = advisory only
relevance basis
byte/token cost

A later model call may receive:

ADVISORY — NON-AUTHORITATIVE
Preserve unconfigured default behavior; adding a new configuration
field may otherwise change existing clients.

It must not receive an undifferentiated multi-page reasoning transcript.

The Context Compiler decides whether a capsule is relevant to the current objective or semantic gap.

The advisory model does not decide what later workers should remember.

Authority rules

The following rules are absolute.

Advice is not truth

A statement such as:

The scheduler may own timing policy.

remains advisory.

It does not become:

FACT: scheduler owns timing policy

Code may use that statement to justify deterministic investigation:

inspect scheduler relationships
search timing-policy references
check applicable tests

Only repository/tool evidence and the normal acceptance policy may establish an authoritative fact.

Advice is not work

An advisory statement such as:

Check backward compatibility.

does not automatically create a workload objective.

In the first implementation, advisory text may only enter downstream context.

Do not implement automatic advisory-to-objective promotion in this change.

A later explicit code-owned boundary may determine whether a recurring or verified advisory concern should become work.

Advice is not an operation

If the advisory text contains:

run tests
read foo.go
delete the old file
use Chromium
execute rm -rf

those strings remain inert text.

No parser may convert arbitrary advisory prose into operations, paths, commands, mutations, or task transitions.

Advice cannot override authority

Precedence remains:

direct current user authority
code/platform invariants
accepted scoped decisions
tool/repository evidence
advisory material
model inference

Advice cannot contradict and replace higher authority merely because it sounds intelligent.

Modes

Implement explicit modes:

off
shadow
active

Off

No advisory call occurs. Existing behavior is byte-for-byte or semantically unchanged.

Shadow

The advisory call runs, raw text is stored, and capsules may be generated, but no advisory material enters downstream model context.

Use this mode for evaluation.

Active

Only Context Compiler-selected, budget-valid advisory capsules may enter later Context Projections.

Raw advisory text never enters directly.

Advisory failure must be recorded explicitly. Because advice is optional and non-authoritative, failure does not invent another provider or silently switch models. The baseline code-owned workflow continues without advice unless a future explicitly configured policy says otherwise.

Initial implementation order

Do not begin with migrations, replay, release sealing, multi-advisor orchestration, or broad production cutover.

Gate A — Minimal in-memory vertical

Using the current objective and context machinery:

1. Build one grounded objective projection.
2. Invoke one configured model with the passive plain-text advisory contract.
3. Receive real free-form text.
4. Store it as a non-authoritative advisory artifact in memory or the existing artifact mechanism.
5. Chunk and minify it.
6. Select one relevant capsule for one later bounded semantic station.
7. Prove that the later station receives the capsule under advisory authority.
8. Prove that no Task Ledger, objective graph, operation, mutation, or completion state changed because of the raw advice.

No new PostgreSQL schema is required for this gate.

Gate B — Shadow comparison

Run the same frozen workloads with:

advisory off
advisory shadow

Measure:

advisory calls
advisory tokens and latency
raw advisory bytes
capsules produced
capsules selected
capsules rejected as irrelevant
potential downstream context cost

Shadow mode must not change task behavior or outcomes.

Gate C — One active consumer

Promote advisory context for exactly one bounded semantic station.

Suggested first consumer:

semantic review

or:

bounded workload-compilation ambiguity

Do not enable it globally.

Run paired cases:

without advisory
with selected advisory capsule

Measure fixes and regressions.

Gate D — Persistence

Only after Gates A–C demonstrate correct behavior should advisory artifacts become durable using the existing artifact/evidence authority.

Prefer existing durable primitives.

Do not add a new advisory subsystem with its own scheduler, task lifecycle, or alternate context store.

Required vertical tests

Model independence

Run the same advisory boundary with at least two providers or models.

The surrounding workflow must remain unchanged.

Plain-text acceptance

Prove that useful free-form output does not need JSON or schema repair.

False advisory

The advisor states a false repository claim.

Required outcome:

no fact accepted
no objective created
no operation executed
no mutation authorized

Malicious advisory

The text contains prompt injection, commands, paths, or demands for more authority.

Required outcome:

stored only as inert advisory text
no capability created
no authority changed

Irrelevant advisory

The advisor returns material unrelated to the later semantic gap.

Required outcome:

context reducer omits it
downstream Context Projection contains zero advisory bytes

Relevant advisory

The advisor identifies a useful edge case relevant to a later semantic gap.

Required outcome:

one bounded capsule enters context
capsule retains source provenance
later worker still returns its normal typed result
code remains completion authority

Conflicting advisors

Two advisors disagree.

Required outcome:

both remain non-authoritative
no model vote
no automatic winner
context may expose the bounded conflict when relevant
code/evidence decides what becomes established

Oversized output

An advisor exceeds its output budget.

Required outcome:

explicit invalid/truncated status
not active context
no silent acceptance

Baseline preservation

With mode off, the production workload behaves exactly as before.

Evaluation

The previous R1 experiment proved that “more reasoning” can reduce quality when it enters the authority path.

This implementation must demonstrate that passive reasoning adds useful context without becoming authority.

Report:

baseline successes
advisory-active successes
baseline pass → advisory fail regressions
baseline fail → advisory pass rescues
invalid advisory outputs
selected advisory capsules
unused advisory capsules
context-token increase
wall-time increase
downstream model-call changes

Do not promote based on anecdotes or one successful example.

The system should make it cheap to compare:

same code-owned workload
same worker models
same context budgets
advisor off
advisor active

Forbidden architecture

Do not introduce any of the following:

* ReasoningAgent;
* AdvisorAgent;
* a revived universal CognitionDecision;
* model-selected operations;
* model-selected tools;
* model-owned planning;
* model-owned review loops;
* model-owned context or memory management;
* model-generated Task Ledger commands;
* arbitrary advisory text parsed into executable work;
* advisor-to-advisor conversation;
* model voting;
* whole advisory transcripts dumped into downstream prompts;
* fallback from a failed advisor to another model without explicit policy;
* persistence/replay work before the behavioral vertical passes;
* changes to the proven Charmander/Charmeleon authority split.

Do not resurrect the rejected final-partition adviser protocol as production behavior.

Reuse only the general lesson that reasoning may be useful when it is non-authoritative and independently evaluated.

End state

The desired result is:

Omnidex has a grounded objective.
A configurable model thinks broadly about it and returns ordinary text.
Omnidex stores that text as non-authoritative advisory material.
The context compiler mines and minifies it.
Later tiny semantic workers receive only the few relevant considerations.
Code continues to own objectives, workflows, operations, evidence,
repository state, verification, correction, and completion.

The advisory system should make weaker local workers better informed.

It must not make the advisory model into the new brain of Omnidex.

Required implementation report

When complete, report:

files changed
new interfaces/types
trigger implemented
off/shadow/active behavior
models/providers tested
raw advisory size
minified capsule size
capsules selected/omitted
downstream context delta
behavioral fixes
behavioral regressions
authority violations, expected 0
model-selected operations, expected 0
new generic agent loops, expected 0

Do not describe the feature as implemented until the minimal vertical, false-advisory, malicious-advisory, relevance, and baseline-preservation tests pass.
