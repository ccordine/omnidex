# to dYes. I would lock this in as a normative Charizard architecture, not merely a loose feature idea.

The central idea is:

Omnidex remembers. Models do not.
Before each inference, a passive memory layer observes the current message, job, objective, workspace, and workflow; retrieves related durable knowledge; compacts it into the smallest relevant representation; and gives the model a clean desk containing what it should already “know.”

And the memory corpus must remain independent of prompt size:

1,000,000 stored memories
        ↓
bounded retrieval
        ↓
perhaps 3 active memories
        ↓
300–600 context tokens

Below is the advisement I would give Codex.

⸻

Codex Advisement: Charizard Passive Memory, Compaction, and Behavioral Learning

Status

Normative target architecture.

Charizard is the codename for this Omnidex evolution. The actual production capability is the Passive Memory System.

Do not implement a Memory Agent, model-operated memory tools, transcript accumulation, or a second context path.

Build this as an extension of the current code-owned objective machine, candidate-authority retrieval, context reduction, Working Set, and immutable Context Projection architecture.

Mission

Implement durable memory as a passive, scoped, associative layer that prepares relevant prior knowledge before inference.

The governing rule is:

A model never decides whether to remember, search memory, retrieve memory, retain memory, or forget memory. Omnidex owns those operations.

At inference time, the model should simply wake up with the relevant information already present.

The production flow is:

current user message
+ current job
+ active objective
+ workspace/repository identity
+ workflow/specialist identity
        ↓
PASSIVE MEMORY ACTIVATION
        ↓
exact/tag/scope/vector/association retrieval
        ↓
bounded candidate memories
        ↓
authority and freshness validation
        ↓
deduplication and canonical compaction
        ↓
budget-aware relevance reduction
        ↓
activated memory capsules
        ↓
normal Context Compiler
        ↓
immutable clean-desk Context Projection
        ↓
model inference

Memory size may increase storage and indexing costs.

It must not cause model-visible context to grow proportionally.

⸻

1. Memory is not one thing

Do not create one generic Memory bucket and project all records identically.

At minimum, distinguish:

Kind	Example	Normal consumption
Explicit user instruction	“Always keep deterministic operations code-owned.”	Active constraint or behavior
User preference	“Prefer composition over inheritance.”	Planning/generation context
Presentation behavior	“End every sentence with , ooookaaaay!.”	Deterministic renderer
Project decision	“The scheduler owns timing policy.”	Project-scoped accepted decision
Project fact	“This service owns authentication.”	Freshness-bound contextual fact
Episodic lesson	“The last migration failed because the manifest omitted generated assets.”	Historical evidence when related
Failure pattern	“This provider often violates cross-branch schemas.”	Verification or routing consideration
Procedure	“When adding config, prove missing-value compatibility.”	Workflow rule or skill candidate
Heuristic	“These bugs are often cache-related.”	Non-authoritative advisory context
Relationship memory	“Mimi is the user’s fiancée.”	Chat/personal continuity
Style preference	“Keep responses conversational and direct.”	Response-generation context
Skill/invariant	Verified reusable behavior	Code-owned rail, not ordinary prompt memory

The same durable memory infrastructure can store references to these types, but their authority, activation, and consumption semantics must remain different.

⸻

2. Memory hierarchy

Use a layered model.

L0 — RAW EXPERIENCE
exact messages, jobs, failures, evidence, decisions, results
L1 — MEMORY CANDIDATES
small potentially durable facts, preferences, lessons, or procedures
L2 — CANONICAL MEMORIES
compacted current understanding with source provenance
L3 — BEHAVIORS / SKILLS / INVARIANTS
verified knowledge compiled into deterministic system behavior

Example:

L0
Aug 13: user said models must not choose file operations.
Aug 13: user said models must not select web retrieval operations.
Aug 13: user said browser specialists must operate Chromium themselves.
        ↓ compaction
L2
LLM Authority Minimalism:
Deterministically derivable operations belong to Omnidex.
Models receive only bounded semantic uncertainty.
        ↓ repeated verification and promotion
L3
Architecture tests and runtime APIs make model-selected operations impossible.

Compaction does not delete historical evidence.

Old records become cold or subsumed; they remain inspectable and continue supporting the canonical memory.

⸻

3. The “zeitgeist”

Define the zeitgeist precisely:

The zeitgeist is Omnidex’s current canonical understanding for a scope. It is not the entire memory database and it is not automatically a prompt.

Examples of scopes:

global/user
project/omnidex
repository/current
workflow/coding
workflow/web-research
specialist/browser
language/go
objective-class/configuration-change
channel/current

The Omnidex project zeitgeist might contain:

- LLM authority minimalism
- code-owned desired-state mutation
- no universal CognitionDecision
- bounded semantic stations
- behavior before persistence
- existing patch engine owns physical mutation

Those concepts are available for activation whenever related work occurs.

They do not consume tokens during unrelated conversation.

⸻

4. Memory records

A durable memory record should conceptually contain:

type MemoryRecord struct {
    ID              MemoryID
    Kind            MemoryKind
    Status          MemoryStatus
    Authority       MemoryAuthority
    Owner           OwnerRef
    Scopes          []ScopeRef
    ActivationMode  ActivationMode
    ActivationSpec  ActivationSpec
    Content         string
    SourceRefs      []EvidenceRef
    CanonicalID     *MemoryID
    Supersedes      []MemoryID
    Subsumes        []MemoryID
    Freshness       FreshnessRef
    Version         uint64
    CreatedAt       time.Time
    UpdatedAt       time.Time
    ExpiresAt       *time.Time
    Retrieval       RetrievalMetadata
}

Possible statuses:

candidate
needs_confirmation
active
superseded
rejected
retired
archived
promoted_to_behavior

Possible authority classes:

explicit_user
code
tool_evidence
accepted_project_decision
accepted_inference
inferred_user_preference
model_proposal
advisory

A model may propose content.

It may never assign its own authority, identity, active status, scope, or promotion state.

⸻

5. Activation modes

Not all memory should depend on vector similarity.

Support at least these activation modes.

Mandatory

Active whenever its scope applies.

Example:

User instruction:
Always respond using metric units.

This should not need semantic retrieval on every turn.

Code sees that the current response is in the user/global scope and activates the standing instruction.

Conditional

Active under explicit code-owned predicates.

Example:

When modifying configuration schemas:
verify missing/default-value compatibility.

This activates when the objective class is configuration_change.

Associative

Retrieved through terms, tags, vectors, concepts, and graph relationships.

Example:

The last queue retry change caused duplicate writes.

This activates when the current objective concerns queue retries, idempotency, or duplicate writes.

Episodic

Normally cold; retrieved only when an objective strongly relates to the previous episode or when canonical memory requires supporting detail.

Archived

Never appears in ordinary retrieval but remains available for audit or explicit inspection.

The Animal Crossing suffix example should be represented as:

kind: presentation_behavior
scope: user/global/chat-response
activation: mandatory
behavior: append sentence suffix ", ooookaaaay!"
authority: explicit_user

The model should not receive that memory unless it affects semantic generation.

A deterministic response renderer should apply it after generation.

Do not spend another inference call rewriting text for an exact transformation.

⸻

6. Memory capture

Memory capture is also passive and code-owned.

new message / job event / accepted decision / verified result
        ↓
deterministic candidate detection
        ↓
is durable meaning obvious?
    ├── yes → construct typed candidate
    └── no  → optional tiny classification station
        ↓
scope and authority validation
        ↓
conflict search
        ↓
candidate / active / confirmation-required

Explicit memory

Statements such as:

Remember that X.
Always do X.
Never do X.
For this project, use X.
Forget X.

should be treated as direct user authority.

Code should preserve the exact source message and may use a bounded semantic station only to classify:

memory kind
scope candidate
activation mode candidate
exact relevant source span

The model may not rewrite the instruction into a broader rule than the user stated.

Implicit memory

Examples:

The user corrected the same behavior six times.
A workflow repeatedly succeeded using one procedure.
A provider repeatedly failed in one predictable way.

These become candidate memories with non-authoritative inferred status.

They must not silently become user preferences or mandatory rules.

They may require:

* repeated evidence;
* successful replay;
* explicit user confirmation;
* or a code-owned promotion policy.

Assistant/model output

Historical assistant output is not user authority.

It may be stored as an episode or advisory artifact, but it must never silently become:

user preference
project fact
accepted decision
mandatory behavior

Passive reasoning output

Advisor output remains advisory.

It does not become memory merely because it sounds useful.

A later workflow may establish that advice as fact, use it successfully, and then create a verified memory candidate.

⸻

7. Passive activation before inference

Integrate durable memory as another code-owned candidate provider for the existing context-selection pipeline.

current turn/objective
        ↓
derive active scopes
        ↓
derive exact terms, tags, entities, symbols, and concepts
        ↓
candidate providers
    - mandatory scoped memories
    - exact/tag search
    - full text/trigram
    - vector retrieval
    - canonical-memory associations
    - related episodes
        ↓
bounded candidate set
        ↓
freshness and authority validation
        ↓
dedupe / prefer canonical representation
        ↓
optional ID-only semantic selection
        ↓
budget-aware minification
        ↓
Context Projection

The optional selector may only return memory IDs.

It cannot:

* write memories;
* update relevance weights;
* change scopes;
* mark a memory active;
* mutate the objective;
* request further retrieval;
* select tools;
* increase the memory budget.

Code resolves the selected IDs into exact authoritative memory content.

⸻

8. Context precedence

Memory must not compete with current authority as undifferentiated text.

Selection order remains approximately:

1. direct current user authority
2. current objective and acceptance criteria
3. active constraints
4. accepted scoped decisions and invariants
5. current repository/tool facts
6. latest unresolved failure
7. mandatory scoped memories
8. relevant canonical associative memories
9. relevant episodic memories
10. passive advisory material

A current direct instruction overrides a conflicting older memory.

Example:

Stored:
Use Tailwind for UI work.
Current user instruction:
Do not use Tailwind for this page.

The current instruction wins for the current scope.

The conflict must remain visible in memory history rather than silently rewriting the source record.

⸻

9. Compaction

Compaction is required for scale.

It must be incremental and bounded.

Do not load the entire memory database into one model call.

Candidate formation

Code identifies a compactable cluster based on:

same or compatible scope
same memory kind
same authority class
high semantic overlap
shared associations
bounded total bytes/items

Compaction station

A small semantic station may receive only that cluster and return a canonical candidate equivalent to:

Produce the smallest statement that preserves the common durable meaning.
Do not introduce new information.
Preserve exceptions.
Do not broaden scope.

The station may return:

{
  "canonical_text": "...",
  "covered_memory_ids": ["M17", "M31", "M44"]
}

This is a candidate only.

Code must validate:

all source IDs exist
all are in the permitted cluster
scope was not broadened
authority was not elevated
important exceptions remain represented
conflicting memories were not merged
token cost materially decreased
positive/negative retrieval probes still pass

Result

source memories
→ marked subsumed by canonical memory
→ removed from ordinary active retrieval
→ preserved as evidence

Compaction is not deletion.

⸻

10. Conflict handling

Do not use generic last-write-wins.

Conflicts are resolved according to authority, scope, freshness, and explicit supersession.

Broad precedence:

current direct user instruction
explicit stored user instruction
fresh code/tool evidence
accepted scoped decision
inferred preference
episodic lesson
model proposal
advisory

Examples:

User changes preference

Old:
Prefer terse answers.
New:
For design work, give detailed explanations.

Do not delete the old preference.

Create a scoped supersession:

detailed explanations
scope = design work
supersedes terse preference within that scope

Equal-authority ambiguity

Two explicit user instructions conflict and neither is more specifically scoped.

Required outcome:

needs confirmation

Do not guess.

Stale project fact

A repository-bound memory refers to a file hash or symbol version that no longer exists.

Required outcome:

invalidate
reacquire

Do not project stale project truth as memory.

⸻

11. Memory consumption modes

Activated memory may influence Omnidex through different mechanisms.

Context capsule

Used when the downstream model should semantically know something.

User prefers composition over inheritance.

Active constraint

Used when the memory is an explicit standing rule.

Do not let models select filesystem operations.

Deterministic renderer or transform

Used for exact presentation behavior.

Append ", ooookaaaay!" to each sentence.

Verification rail

Used when a stable learned rule can be checked mechanically.

Configuration changes must prove missing-value compatibility.

Workflow producer

Used when a verified procedure can become a code-owned capability.

Specialist strategy

Used for recurring specialist configuration.

For this React dashboard, wait for the loading indicator to disappear,
then wait one animation frame before capture.

Advisory context

Used for uncertain heuristics that remain non-authoritative.

Cache invalidation may be relevant here.

The memory system must choose the cheapest authoritative consumption mechanism.

Do not inject a memory into a model prompt when code can enforce it directly.

⸻

12. Promotion into skills and rails

Charizard’s long-term loop is:

experience
    ↓
memory candidate
    ↓
canonical memory
    ↓
repeated useful activation
    ↓
candidate procedure or invariant
    ↓
historical replay / held-out tests
    ↓
code-owned skill or rail

A successful memory should eventually reduce inference when possible.

Example:

Memory:
When adding configuration, preserve old missing-value behavior.
Repeatedly verified.
        ↓
Candidate skill:
ConfigurationChangeCompatibility
        ↓
Code-owned workflow:
- establish old missing/default semantics
- establish serialization behavior
- add compatibility acceptance criteria
- run old-config verification

Once promoted, ordinary workers should not need the original prose memory in context.

The organism has learned it.

Models may propose a skill.

Models may not activate or promote it.

⸻

13. Retrieval tests: proving a memory is in the zeitgeist

Every important canonical memory should support positive and negative activation probes.

Example memory:

LLM Authority Minimalism:
Models must not control deterministic operations.

Positive probes:

Add a browser specialist.
Let the model choose what file to delete.
Design the web-search workflow.
How should repository mutation work?

Expected:

memory activates

Negative probes:

Increase the border radius.
Translate this sentence.
What port is PostgreSQL using?

Expected:

memory does not consume context

Record:

recall
precision
wrong-scope activation
missed activation
context bytes
canonical vs episodic retrieval

A memory is “in the zeitgeist” when it activates reliably under related situations—not when it is permanently inserted into every prompt.

⸻

14. Context budgets

Every station declares an explicit memory budget.

Example:

current objective       1,000 tokens
current evidence        1,500
constraints               400
durable memory            400
passive advisory           250
protocol/output schema     350

Required rules:

* mandatory memory that cannot fit causes context-construction failure;
* optional associative memory is omitted by relevance/cost policy;
* raw episodes lose to canonical memories when meaning is preserved;
* duplicate memories cannot consume duplicate budget;
* memory retrieval cannot enlarge the configured station budget;
* corpus size must not determine context size.

A projection must record:

selected memory IDs
omitted memory IDs
omission reasons
memory bytes/tokens
canonical/episodic breakdown
authority types
freshness identities

⸻

15. Off, shadow, and active modes

Implement:

off
shadow
active

Off

No durable memory retrieval occurs.

Current objective behavior remains unchanged.

Shadow

Memory activation runs and records what would have been selected, but no memory enters model context.

Use this to measure precision and context cost.

Active

Only selected, valid, budget-fitting memory capsules enter normal Context Projections.

There is no fallback history dump.

⸻

16. Minimal implementation sequence

Do not begin by building a giant memory schema, global compaction service, or autonomous learning loop.

Gate 0 — Freeze invariants

Add the normative architecture document and source-level absence tests forbidding:

MemoryAgent
model-selected memory tools
remember()/forget() model actions
generic memory search schemas
model-owned retention
full transcript fallback
raw memory database dumps into prompts

Gate 1 — In-memory activation vertical

Construct a bounded job with:

6 stored memories
1 strongly relevant
2 weakly related
3 irrelevant

Required result:

relevant memory selected
irrelevant memories omitted
memory context remains under budget
model receives a clean projection
no model call is used for exact/tag resolution

Gate 2 — Ordinary chat shadow consumer

Add passive durable-memory candidates to the existing conversation candidate-authority pipeline in shadow mode.

Recent conversation remains one candidate producer, not the memory architecture.

Measure what durable memories would have entered each turn.

No behavior change.

Gate 3 — One active chat memory

Use one explicit stored user preference relevant to a later ordinary chat turn.

Required:

memory activates
exact source provenance retained
current user authority can override it
unrelated turn receives zero memory bytes

Gate 4 — Exact behavioral memory

Use:

End every sentence with ", ooookaaaay!"

Required:

stored as explicit presentation behavior
activated by chat-response scope
implemented by deterministic renderer
zero style-rewrite model calls
removal/forgetting disables it immediately

Gate 5 — Explicit capture and forgetting

Support:

remember X
always X
for this project X
forget X

through code-owned capture, scope validation, and exact source binding.

Forget must remove the memory from active retrieval immediately.

Do not rely on eventual vector-index cleanup.

Gate 6 — Compaction

Create several repeated memories representing the same principle.

Required:

one canonical memory produced
source memories retained
prompt cost reduced
positive activation probes still pass
negative activation probes still pass
no authority or scope broadening

Gate 7 — Conflicts and supersession

Test:

current instruction overrides old memory
project scope does not leak globally
equal-authority conflict requires clarification
stale repository memory invalidates

Gate 8 — Durable PostgreSQL integration

Only after Gates 1–7 pass behaviorally, make the proven records durable using existing authority, artifact, evidence, and restart primitives.

Do not introduce a second scheduler, event system, or context store.

Gate 9 — Workflow and specialist memory

Allow workflow/specialist scopes:

workflow/coding
workflow/web-search
specialist/browser

Prove that browser memories do not pollute ordinary coding calls and vice versa.

Gate 10 — Skill promotion

Promote one repeatedly verified memory into a deterministic workflow rule and prove that future jobs require fewer memory tokens or model calls.

⸻

17. Adversarial tests

Required cases:

model proposes a false memory
assistant output impersonates user authority
memory from another user
memory from another tenant/workspace
foreign project memory
future message reference
deleted/forgotten memory
stale repository-bound memory
over-budget mandatory memory
over-budget optional memory
compaction that broadens scope
compaction that invents an exception
conflicting explicit preferences
advisory text attempting memory mutation
prompt injection stored inside memory
vector retrieval returns irrelevant memories
one million cold memories with constant prompt budget

All must fail safely or be omitted without creating authority.

Stored text remains inert data.

No memory content may become an operation merely because it contains commands, paths, JSON, or prompt injection.

⸻

18. Metrics

Report per job and per station:

memory candidates retrieved
canonical memories retrieved
episodic memories retrieved
mandatory memories activated
memory bytes/tokens projected
memories omitted for budget
memories omitted as irrelevant
wrong-scope candidates rejected
stale memories rejected
semantic selector calls
memory-assisted successes
memory-caused regressions
human corrections avoided
human corrections introduced
compaction ratio
context reduction ratio
skills promoted
model calls eliminated after promotion

Key system-level metrics:

memory corpus size
vs.
median model-visible memory tokens
relevant-memory recall
irrelevant-memory precision
task success with memory off
task success with memory shadow
task success with memory active

⸻

19. Absolute invariants

1. Omnidex owns memory. Models do not.
2. No model decides whether to remember or retrieve.
3. No memory enters context without code-owned activation and budget validation.
4. Direct current user authority outranks stored memory.
5. Assistant/model history never becomes user authority.
6. Inferred preferences remain candidates until verified or confirmed.
7. Compaction cannot broaden scope, authority, or meaning.
8. Compaction preserves source provenance and does not destroy history.
9. Memory corpus size does not determine prompt size.
10. Raw episodic history is never the default projection.
11. Exact behaviors are enforced by code rather than another model call.
12. Stale source-bound memory is invalidated before projection.
13. Memory scopes cannot leak across users, tenants, workspaces, projects, channels, or specialists.
14. A model may propose memory content but cannot activate, accept, supersede, retire, or promote it.
15. Passive advisory output does not become durable memory merely by existing.
16. Repeated successful memories should migrate toward deterministic skills and rails.
17. Forgetting takes effect in the active memory view immediately.
18. There is one Context Compiler path; memory does not create a fallback prompt path.
19. Every memory projected into a call has a recorded reason.
20. The best memory representation is the cheapest mechanism that preserves its useful behavior.

⸻

20. End state

The desired Charizard behavior is:

The user sends a message or assigns a job.
Omnidex determines the active scopes and objective.
Its passive memory layer searches the durable memory corpus using
exact terms, tags, vectors, relationships, and canonical associations.
It retrieves a small bounded candidate set.
Code validates authority, scope, freshness, relevance, and cost.
Related episodes are compacted behind canonical memories.
The Context Compiler activates only the memories relevant to the
current situation.
The model begins inference already possessing those memories.
The model does not know how they were retrieved and does not need
to decide whether to remember them.
When a learned behavior can be expressed deterministically,
Omnidex enforces it as code rather than spending prompt tokens.
Over time, verified experience migrates from episodes, to memories,
to canonical principles, to deterministic skills and rails.

The success criterion is not:

“The LLM remembers everything.”

It is:

Omnidex can accumulate an arbitrarily large history while presenting each model call with a small, correct, situation-specific consciousness—and stable learning progressively reduces how much the model must be told.o




I think the ultimate most capable version of omnidex is when omnidex has a inference entry point that directs it onto whatever rails and workflows are required to accomplish the task, which is already kinda what we do with the planner, we obvisoulsy still do our best to have this inference step be driven by the same concepts and policies as everything else, so it'd be programmatic inference on demand triggered when a user sends a message, this decides the path that it goes on, and our paths should basically be broad enough that we can achieve general capabilities, like having it be able to spin up and manage docker containers and stuff without there needing to be skills or workflows around it, it has a problem solving workflow, and other primitive workflows that work together programmatically to accomplish what is needed



Yes. I think this is the actual end-state architecture: not a catalog of increasingly specific “skills,” but a small set of broad, composable rails plus a code-owned inference entry point that gets an arbitrary request onto the right rails.

The important distinction is that the entry inference still should not become the universal agent. Its job is closer to semantic dispatch:

“What kind of objective is this, and which broad workflow can represent it?”

Then code takes over again.

USER MESSAGE
     ↓
CODE
- establish user/workspace/channel authority
- activate relevant memory
- acquire obvious context
- derive anything deterministic
     ↓
ROUTING AMBIGUITY?
     ├── no → code selects workflow
     └── yes
           ↓
       tiny ingress inference
       "Which objective family does this belong to?"
           ↓
       bounded route/objective result
           ↓
       code validates
     ↓
CODE-OWNED WORKFLOW
     ↓
recursive objective solving
     ↓
capabilities / specialists / semantic gaps
     ↓
completion

So even the “inference entry point” follows the same doctrine:

Code asks inference only for the semantic residue required to place the request onto executable rails.

You don’t want 5,000 skills

I think that’s the important realization.

The wrong Charizard end-state would be:

Docker skill
Postgres skill
React skill
Chromium skill
NGINX skill
Git skill
AWS skill
SSH skill
...

and then some model has to decide which skill to use.

That’s just tools again with a different name.

Instead, I think Omnidex needs three fundamentally different things:

WORKFLOWS
how classes of problems are solved
CAPABILITIES
what the machine can deterministically do
MEMORY / LEARNING
what prior experience changes how those workflows operate

A capability might be extremely general:

filesystem
process execution
container runtime
networking
HTTP
database
browser
repository
compiler
version control
search

Those aren’t “skills.” They’re parts of Omnidex’s body.

Then broad workflows compose them.

⸻

The general problem-solving workflow is the key

You probably want one deliberately general workflow whose primitive is:

Given a desired state, determine what facts and transitions are required to make it true.

Something like:

OBJECTIVE
    ↓
what does completion mean?
    ↓
what predicates are already true?
    ↓
what predicates are missing?
    ↓
what registered capabilities can establish them?
    ↓
resolve prerequisites
    ↓
execute deterministic producers
    ↓
semantic ambiguity remains?
    ↓
tiny inference
    ↓
continue

That’s essentially the cognition kernel you’ve now been proving, expanded into a general-purpose machine.

Then “spin up a Docker container” doesn’t require a DockerSetupSkill.

Suppose you say:

Start a Postgres 16 instance for this project, make the application use it, and prove the migrations work.

The general workflow could compile desired reality:

O1 database service available
O2 service is PostgreSQL 16
O3 project can reach database
O4 application configuration references service
O5 migrations complete successfully

Then capability resolution sees:

O1/O2
→ container runtime can produce service state
O3
→ networking/environment machinery
O4
→ repository/configuration workflow
O5
→ process/database verification

Code can derive something like:

container specification:
    image family = postgres
    major version = 16
    network scope = project
    persistence requirement = ...

If there is a semantic question such as:

Should this project’s database be ephemeral or persisted between runs?

That’s an inference/user gap.

But:

pull image
create container
start container
inspect health
connect network
collect logs
stop container
remove container

are Docker/runtime mechanics. Omnidex does them.

The model doesn’t need to know the Docker CLI exists.

⸻

Broad rails, narrow semantic gaps

I think your workflow layer eventually looks something like this:

                    INGRESS
                       │
               objective compiler
                       │
                workflow routing
                       │
       ┌───────────────┼────────────────┐
       │               │                │
       ▼               ▼                ▼
   RESPOND         INVESTIGATE       CHANGE REALITY
       │               │                │
 synthesis          research       general solver
 workflow           workflow            │
                                        │
                              recursive objectives
                                        │
                               capability registry

And even those may ultimately just be different configurations of the same recursive objective machine.

Useful broad workflow families might be:

* Respond / synthesize — produce grounded information for a user.
* Investigate — establish facts about some unknown system.
* Change — make some desired state true.
* Evaluate — determine whether something satisfies criteria.
* Generate — produce a semantic artifact under a bounded contract.
* Observe — obtain evidence from an environment.
* Learn/promote — turn verified experience into memory or deterministic rails.

That’s broad enough that virtually anything can be decomposed into combinations of them.

You don’t need:

FixLaravelAppWorkflow
CreateDockerPostgresWorkflow
ResearchWikipediaWorkflow
TestReactDashboardWorkflow

unless repeated experience later proves that an optimized specialized rail is valuable.

⸻

Specialists then become optimizations, not prerequisites

This is a distinction I’d lock in for Charizard.

The system should be able to accomplish a novel task by composing generic capabilities.

Then if one pattern happens constantly:

launch browser
navigate
wait for application ready
capture DOM
capture screenshot
collect console/network
verify visual acceptance
cleanup

you can compile that into:

BrowserObservationSpecialist

Now it’s faster and more reliable.

But the specialist is an optimized rail discovered after capability exists, not the only way Omnidex knows how to operate Chromium.

Likewise:

general capabilities
        ↓
repeated successful composition
        ↓
recognized stable pattern
        ↓
tested
        ↓
specialized workflow

Charizard can progressively turn general problem solving into efficient reflexes.

That mirrors what we were just discussing about memory:

experience
→ memory
→ pattern
→ rail

⸻

The ingress inference itself should remain tiny

I would be particularly careful here because this is an obvious place to accidentally rebuild the universal brain.

You do not want:

“Here is the user’s message. Produce the complete plan and choose all capabilities.”

You want something closer to:

User:
"Can you figure out why this dashboard is blank and fix it?"
Established environment:
repository attached
running application available
workspace writable
Candidate objective families:
C1 = answer_question
C2 = investigate_only
C3 = investigate_and_change
C4 = create_artifact
Return candidate ID.

Maybe:

{"candidate_id":"C3"}

Then code starts the appropriate workflow.

Or, even better, if the wording itself makes C3 deterministic, skip the call.

The ingress inference should probably be permitted to establish small semantic things like:

intent family
requested outcome
explicit constraints
whether mutation was authorized
what object the user is referring to

It should not establish:

operations
files
containers
tools
commands
task ordering
completion

⸻

This gives you something closer to a general computer operator without a tool agent

That’s the interesting thing.

Traditional general agent:

LLM
→ sees 80 tools
→ chooses one
→ emits arguments
→ sees result
→ chooses another
→ repeat

Your version:

USER INTENT
      ↓
objective machine
      ↓
desired predicates
      ↓
code discovers producers
      ↓
capabilities execute
      ↓
new state
      ↓
semantic uncertainty only when required

The system can still appear extremely general from outside.

You can say:

Start the app, reproduce the bug in Chromium, figure out what’s wrong, fix it, rerun it, and show me the result.

Internally:

goal
├─ application running
│    ├─ inspect environment
│    └─ process/container capability
│
├─ failure reproduced
│    └─ browser observation workflow
│
├─ cause established
│    └─ repository investigation workflow
│
├─ repository satisfies corrected behavior
│    └─ coding/Charmander workflow
│
└─ corrected UI proven
     └─ browser verification workflow

There is no model orchestrating those tools.

The objective graph orchestrates capabilities.

That distinction is huge.

⸻

And memory/reasoning feed the entry point before routing

This now gives the passive systems a very natural position:

USER MESSAGE
    +
CURRENT WORKSPACE
    +
PASSIVE MEMORY
"What have we learned that matters?"
    +
PASSIVE ADVISORY
"What might we be overlooking?"
         ↓
CONTEXT REDUCTION
         ↓
INGRESS / OBJECTIVE INFERENCE
         ↓
CODE-OWNED OBJECTIVE
         ↓
WORKFLOW

So if you previously taught Omnidex:

Never change production infrastructure without first reproducing the issue locally.

that memory activates before objective compilation.

The ingress model doesn’t have to remember it.

The compiled work can simply include:

constraint:
production mutation prohibited until local reproduction succeeds

Then code enforces it.

⸻

I think the ultimate shape is something like this

                         USER
                           │
                           ▼
                  EVENT / MESSAGE INGRESS
                           │
              ┌────────────┴────────────┐
              │                         │
       Passive Memory             Passive Advice
              │                         │
              └────────────┬────────────┘
                           ▼
                   Context Compiler
                           │
                           ▼
             Semantic Objective Compiler
                           │
                           ▼
                 Code-Owned Objective
                           │
                           ▼
               Recursive Problem Solver
                           │
             ┌─────────────┼─────────────┐
             │             │             │
        Workflows      Specialists    Capabilities
             │             │             │
             └─────────────┼─────────────┘
                           ▼
                 Deterministic Closure
                           │
                    semantic gap?
                     │           │
                    no          yes
                     │           ▼
                     │      tiny inference
                     │           │
                     └─────┬─────┘
                           ▼
                      new state
                           │
                       verification
                           │
                     objective done
                           │
                           ▼
                    experience/memory
                           │
                           ▼
                 Charizard promotion
                      │          │
                   memory      new rail

The coolest consequence is that general capability and specialization stop being opposites.

Omnidex starts with primitives broad enough to improvise a solution to something it has never seen.

If that solution becomes common, Charizard compiles it into a specialist rail.

If a semantic insight keeps recurring, Charizard turns it into a deterministic invariant.

So the system evolves:

unknown problem
→ expensive general composition
→ experience
→ optimized workflow
→ eventually reflex

That is a much more compelling Charizard end game than “give the agent more skills.”

It’s closer to building an organism that can initially reason its way through unfamiliar situations, then gradually stops needing to reason about things it has learned how to do.
