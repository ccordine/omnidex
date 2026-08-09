The best implementation is a software-defined context system

After looking through reasoning-overhaul, I would not implement this as “better RAG,” a giant memory document, or an agent-maintained notebook.

I would build three new, distinct primitives:

1. Task Ledger — durable continuity: goals, current work, completed work, decisions, questions, failures, and checkpoints.


2. Working Set — the small collection of references currently worth keeping in attention.


3. Context Projection — the exact immutable subset rendered for one model call.



Those sit beside—not inside—two other systems:

Repository Intelligence — what is currently true about the codebase.

Durable Memory — what future jobs may want to recall.


Repository Intelligence       Durable Memory
 "what exists now?"           "what may matter later?"
           │                          │
           └──────────┬───────────────┘
                      │ acquire
                      ▼
 User authority ──► Task Ledger
                    "what are we doing,
                     what happened,
                     what remains?"
                      │
                      ▼
                  Working Set
              "what is active now?"
                      │
               Context Builder
                      │
                      ▼
             Context Projection
             "what this one call sees"
                      │
                      ▼
                    Model
                      │
          bounded requests/proposals
                      │
                      ▼
               Code-owned coordinator

That is basically virtual memory for model context.

The model’s native context is the fast, tiny cache. PostgreSQL, repository indexes, artifacts, and evidence are the backing store. Omnidex pages information in and out according to the task graph rather than hoping cosine similarity or a giant conversation summary gets lucky.

This fits Charmander’s actual philosophy. The current architecture explicitly keeps memory out of the coding semantic path, gives models bounded responsibilities, and leaves tools, mutation, verification, and completion under code authority. The branch already has typed artifacts, but PlanArtifact is still essentially a static goal/constraints/subtask structure, while step completion also puts an untyped string into the runtime context map. The missing layer is typed, evolving, job-scoped state, not more prompt text. 

Keep the five state types separate

Layer	What it stores	Lifetime	Authority

Repository Intelligence	Files, symbols, relationships, tests, routes, config, hashes	Repository snapshot	Source/tool derived
Task Ledger	Goals, constraints, decisions, questions, failures, progress	Job or card	Mixed, provenance-labelled
Working Set	Active references selected for current scope	Call, step, objective, or job	Code-managed
Context Projection	Exact content given to one inference	One model call	Immutable
Durable Memory	Cross-job preferences, references, established lessons	Long-term	Reference-only unless explicitly promoted


The existing memory_chunks system is already designed around cross-job retrieval, embeddings, tags, categories, trust ordering, and semantic correction. V3 then projects those chunks under the explicit authority historical_reference_only. That is exactly why task working memory should not be jammed into the same tables: it has a completely different lifetime and authority model. 

RAG is merely one possible acquisition provider:

working.acquire from:
    repository exact search
    repository graph
    full-text search
    vector retrieval
    command result
    compiler diagnostic
    test result
    web evidence
    durable memory
    prior artifact

The working set should not care whether an item came from rg, PostgreSQL, pgvector, a compiler, or a human. It cares about its identity, authority, scope, provenance, freshness, and cost.

1. PostgreSQL should be canonical; files should be projections

I would make PostgreSQL authoritative for task state because Omnidex already has durable jobs, steps, artifacts, evidence, and multiple workers. Redis remains appropriate for locks, progress, cache, pub/sub, and realtime fanout. 

But the literal-file idea is still good.

Use PostgreSQL as canonical state, then generate read-only, grep-friendly projections:

.omni/runs/<job-id>/
    events.jsonl
    state.json
    status.txt

    working/
        objective-1.json
        change-group-4.json

    contexts/
        call-00041.json
        call-00042.json

status.txt might contain:

GOAL       ACTIVE    goal-1    Add tenant-specific invitation windows
CONSTRAINT ACTIVE    con-3     Preserve existing eligibility behavior
TASK       DONE      task-4    Locate dispatch owner
TASK       ACTIVE    task-5    Identify configuration path
FACT       ACTIVE    fact-8    InvitationJob owns dispatch mutation [repo:sym-182]
DECISION   ACCEPTED  dec-2     Modify the job, not the scheduler [fact-8,fact-9]
HYPOTHESIS REJECTED  hyp-4     Scheduler owns state mutation [fact-8]
FAILURE    ACTIVE    fail-6    InvitationJobTest expected 3, received 4
QUESTION   OPEN      q-7       Does tenant configuration already support durations?

Then these are genuinely useful:

rg '^TASK +ACTIVE' .omni/runs/123/status.txt
rg 'InvitationJob' .omni/runs/123
rg '^FAILURE' .omni/runs/123/status.txt

But the model should not edit these files.

They are:

generated atomically from canonical state;

human-readable;

useful for debugging and external integrations;

disposable;

permissioned appropriately;

never accepted as mutation authority.


Omnidex itself should normally use the typed API. The files are an inspection ABI, not a second database.

Why not make the files canonical?

Because eventually you will have:

concurrent workers;

retries;

step recovery;

task transitions;

reference counting;

atomic evidence attachment;

optimistic locking;

UI updates;

crash recovery.


You do not want to reinvent transactional integrity with flock, temporary JSON files, and prayers to a filesystem god who has already stopped listening.

2. Use a hybrid event log plus materialized current state

I would not use pure event sourcing, where every prompt rebuilds current state by replaying ten thousand events. That becomes its own artisanal nightmare.

Use both:

normalized tables for current state;

an append-only event stream for audit and replay;

one database transaction updates both.


command:
    accept decision D7

transaction:
    validate current state/version
    update task_entries
    insert task_event
    increment ledger version
    commit

The materialized tables answer runtime queries cheaply.

The events answer:

how did we reach this decision?

what did the system think at step 14?

did a model or code create this?

what was released from the working set?

can the run be reproduced?

did a retry overwrite anything?


That prevents the task ledger from becoming exactly the kind of hidden, stale state Charmander was designed to eliminate. Charmander rejected hidden ledgers because they accumulated alternate truths. This ledger is different: it is explicit, typed, operator-visible, source-bound, and scoped to one execution. Git remains source history. 

3. The minimum useful schema

I would start with these tables—not a giant universal graph schema.

task_ledgers
task_nodes
task_node_edges
task_entries
task_entry_refs
task_events
working_sets
working_set_items
context_projections

task_ledgers

job_id
run_id
owner_type          job | card | project
owner_id
version
status
created_at
updated_at
closed_at

Support card and project in the schema, but enable only job initially. Otherwise you will accidentally build cross-job autonomous project management before proving a task can remember what it was doing for twenty minutes.

task_nodes

The execution graph:

id
job_id
parent_id
objective_id
kind                goal | objective | task | checkpoint | change_group
title
status              pending | ready | active | blocked | done | failed | cancelled
priority
created_by
created_step_id
completed_step_id
acceptance_criteria JSONB
metadata            JSONB
created_at
updated_at

Dependencies go in task_node_edges:

from_node_id
to_node_id
kind                depends_on | blocks | decomposes_to | verifies

task_entries

The system’s notebook, with explicit epistemic type:

id
job_id
scope_node_id
kind
status
authority
content
content_hash
confidence
created_by
created_step_id
supersedes_id
created_at
updated_at
resolved_at

Useful kinds:

constraint
fact
observation
hypothesis
decision_candidate
accepted_decision
question
failure
checkpoint
note

Useful authorities:

user
code
tool_evidence
model_proposal
accepted_model_decision

The rules matter more than the names:

A fact requires one or more evidence references.

A model directly creates only a proposal, hypothesis, question, or attention request.

A model cannot create user authority.

A model cannot declare a task complete.

A model cannot silently turn a hypothesis into a fact.

A decision may be model-originated, but code records the policy that accepted it.

A rejected or superseded entry remains in history but is excluded from normal active projections.


Stable references instead of copied blobs

Use a common reference structure:

type Ref struct {
    URI      string `json:"uri"`
    Version  string `json:"version,omitempty"`
    Hash     string `json:"hash,omitempty"`
    Relation string `json:"relation,omitempty"`
}

Examples:

task://job/481/node/task-7
artifact://job/481/plan/01J...
evidence://job/481/9831
repo://snapshot/abc123/symbol/sym-842
repo://snapshot/abc123/file/file-91
memory://chunk/184
web://evidence/291

Store content only when the entry itself is the content. For repository source, test output, compiler output, and artifacts, prefer references.

That gives you freshness checking:

working item:
    repo://snapshot/S1/symbol/X
    source hash: abc

current task snapshot:
    S2

result:
    stale → invalidate → reacquire

4. The Working Set is the actual memory manager

The Task Ledger records everything worth retaining for the job.

The Working Set determines what remains “resident.”

working_sets
------------
id
job_id
scope_type
scope_id
status
repository_snapshot_id
max_items
max_bytes
version
created_at
updated_at

working_set_items
-----------------
working_set_id
ref_uri
role
retention
priority
state
source_hash
byte_cost
acquired_by
reason
last_used_at
use_count
created_at
released_at

Retention classes

call
step
phase
task
objective
job
pinned

The lifecycle is mostly code-owned:

call ends
    → release call-local items

step ends
    → release step-local items

task completes
    → release task-local items not shared elsewhere

source hash changes
    → invalidate source-bound items

decision superseded
    → remove old decision from active working sets

failure resolved
    → release failure from normal contexts,
      retain it in task history

Use semantic lifecycle first, then LRU-like behavior only within the same class.

Do not let the model freely pin things. Otherwise you recreate context hoarding in PostgreSQL:

> “This might be useful later.”



Six hundred entries later:

> “Good news, I preserved everything.”



The model can request retention, but code enforces:

allowed kinds;

scope;

maximum pinned bytes;

duplicate suppression;

freshness;

reference validity.


A working item shared by two active objectives should use reference counting or equivalent scope membership, so finishing one objective does not evict something another still needs.

Clearing memory should not delete history

When the model says “I no longer need this,” it should mean:

release from active working set

not:

erase every record that it ever existed

That preserves auditability while keeping future prompts clean.

5. Most memory should be recorded automatically

Do not ask the model to narrate its own work after every operation.

Model self-reporting is:

expensive;

incomplete;

prone to rewriting history;

another opportunity to hallucinate;

redundant when code already knows what occurred.


The coordinator already knows when:

an intent was accepted;

a task was created;

an artifact was written;

a repository query ran;

evidence was acquired;

a patch was applied;

a command failed;

a test passed;

a task completed.


Record those automatically.

The current V3 runtime already has central writeArtifact, writeEvidence, and complete paths. Those are ideal interception points for task-state events rather than making every worker manually maintain a diary. 

For example:

IntentArtifact written
    → goal.created
    → objective.created
    → constraint.recorded

PlanArtifact written
    → task.created
    → dependency.recorded

repository evidence written
    → evidence.acquired
    → optionally attach to current working set

tool call fails
    → failure.recorded

verification passes
    → acceptance_criterion.satisfied
    → task.completed

The model should only contribute state where fuzzy interpretation is genuinely needed:

“This appears to imply X.”

“I need callers of symbol Y.”

“Decision Z should be retained for the rest of this objective.”

“This hypothesis has been contradicted.”

“There is an unresolved question.”


6. Give selected models a bounded state protocol

Not every model gets this.

The fragment model should remain exactly as constrained as it is now. It does not need a notebook, a plan, or a toolbelt. The planner, conductor, repository investigator, and perhaps verifier are the roles that benefit from task continuity.

A coordinator-capable response could include:

{
  "result": {
    "summary": "The job owns the state mutation."
  },
  "state_proposals": [
    {
      "kind": "hypothesis",
      "scope_ref": "task://job/481/node/task-7",
      "content": "Tenant configuration may already expose a duration field.",
      "evidence_refs": [
        "repo://snapshot/abc/symbol/sym-91"
      ]
    },
    {
      "kind": "decision_candidate",
      "scope_ref": "task://job/481/node/task-7",
      "content": "Modify InvitationJob rather than InvitationScheduler.",
      "evidence_refs": [
        "repo://snapshot/abc/symbol/sym-182",
        "repo://snapshot/abc/symbol/sym-207"
      ]
    }
  ],
  "attention_requests": [
    {
      "operation": "repo.references",
      "target_ref": "repo://snapshot/abc/symbol/sym-182",
      "reason": "Confirm callers do not depend on the current return behavior."
    },
    {
      "operation": "working.retain",
      "target_ref": "repo://snapshot/abc/symbol/sym-207",
      "retention": "objective",
      "reason": "This test establishes the invariant being preserved."
    },
    {
      "operation": "working.release",
      "target_ref": "repo://snapshot/abc/symbol/sym-44",
      "reason": "The scheduler does not own the mutation."
    }
  ]
}

Hard limits:

maximum state proposals:       4
maximum attention requests:    4
maximum acquisition rounds:    2
maximum proposal bytes:        fixed
allowed operations:            role-specific

The model never receives:

task.complete
task.delete
task.accept_fact
filesystem.write
sql.execute
arbitrary shell

The coordinator validates every request and either applies it or records a rejection.

7. Build a deterministic Context Builder

This is the part that makes the whole system more than a fancy TODO list.

Each station declares its context contract in code:

type ContextSpec struct {
    Name                 string
    Version              string
    ScopeRef             Ref
    Required             []Selector
    Optional             []Selector
    AllowedAuthorities   []Authority
    MaxItems             int
    MaxBytes             int
    MaxAcquisitionRounds int
}

Example planner projection:

required:
    root goal
    active objectives
    user constraints
    completed/blocked task states
    accepted decisions
    unresolved questions

optional:
    latest failures
    evidence-gap summaries

Example repository-investigation projection:

required:
    current requirement
    current repository snapshot
    exact terms already extracted
    prior accepted routing decisions

optional:
    acquired symbols
    direct graph neighbors
    relevant tests
    unresolved evidence gaps

Example fragment projection:

required:
    immutable signature
    exact behavior
    allowed symbols
    direct declarations
    accepted invariants
    latest path-free diagnostic

optional:
    nothing

Deterministic selection order

I would prioritize:

1. Direct current user authority.


2. Active objective and acceptance criteria.


3. Active constraints.


4. Accepted decisions and invariants in scope.


5. Latest unresolved failure.


6. Explicitly acquired repository/evidence references.


7. Direct dependencies and tests.


8. Historical durable memory only where permitted.


9. Semantic retrieval only when structured retrieval is insufficient.



Never start with:

embed the task
→ grab whatever looks similar
→ call it context

That is useful as a fallback, not as the operating system.

Every projection becomes immutable evidence

Store:

context_projections
-------------------
id
job_id
step_id
work_id
scope_ref
spec_name
spec_version
working_set_id
selected_refs JSONB
omitted_refs JSONB
omission_reasons JSONB
rendered_sha256
rendered_bytes
estimated_tokens
created_at

Then bind context_projection_id to the exact LLM call evidence.

The branch already records exact prompt/model/schema/call evidence, and it already tracks prompt characters, sent characters, context limits, utilization, shrink percentage, success, error class, latency, and per-scope context growth. This gives you most of the instrumentation needed to measure the new layer instead of inventing another telemetry island. 

This is crucial for debugging:

> Why did Qwen forget the backwards-compatibility rule?



You should be able to answer exactly:

Constraint con-8 existed.
It was active and objective-scoped.
Context spec fragment-v3 did not select constraints of class api_compatibility.
Projection CP-42 therefore omitted it.

Not:

> Maybe the context compaction made it forget?



8. Incorporating it into the current V3 runtime

The initial implementation can fit into the current architecture without rewriting Charmander.

At intent creation

The current IntentArtifact already contains the user goal, objectives, constraints, capabilities, completion criteria, memory mode, unresolved references, and ambiguities. Use it to seed the ledger. 

IntentArtifact
    ↓
root goal
objectives
constraints
completion criteria
unresolved questions

At planning

Keep PlanArtifact immutable as the initial plan.

Materialize its subtasks into mutable task_nodes.

The current direct coding plan may produce one coordinate_implementation subtask; the ledger can then hold its evolving internal change groups and checkpoints without mutating the original plan artifact. 

That creates a useful distinction:

PlanArtifact:
    what was initially authorized

Task Ledger:
    current execution state under that authority

At subtask execution

When runSubtask begins:

1. Open or resume the task node.


2. Create an objective/task-scoped working set.


3. Load required direct authority.


4. Acquire the minimum repository/evidence refs.


5. Build a station-specific context projection.


6. Invoke the model.


7. Validate state and attention proposals.


8. Record outputs and transitions.


9. Close or suspend the working set.



The current runtime already resolves and validates the authoritative subtask assignment before executing it, so the working set should attach after that validation—not before. 

At artifact and evidence creation

Automatically attach references to the current task scope.

Do not copy every artifact body into task memory.

artifact written
    → task event
    → task entry ref
    → optional working-set attachment

The existing artifact types already distinguish workspace excerpts, retrieval items, web evidence, subtask results, tool calls, tool results, analysis, drafts, verification, and memory candidates. That is a strong set of source objects for a reference-based ledger. 

At verification

The verifier reads:

direct user objective;

acceptance criteria;

accepted changes;

authoritative evidence;

test and compiler outputs.


It does not trust the planner’s claim that something is done.

Only verification/code may transition:

active → done

At memory review

After clean completion, explicitly select which entries—if any—deserve promotion into durable memory.

Possible promotions:

established project preference
reusable validated procedure
stable project reference
user preference

Do not promote:

temporary hypotheses
one-run failures
working-set selections
superseded decisions
task-local notes

The ledger expires as job authority when the job closes. Future jobs see only deliberately promoted durable memory.

9. Package boundaries

I would add:

internal/taskstate/
    types.go
    store.go
    commands.go
    transitions.go
    events.go
    projector.go
    search.go
    export.go

internal/workingset/
    types.go
    store.go
    acquire.go
    lifecycle.go
    invalidation.go
    budget.go

internal/contextbuilder/
    spec.go
    selectors.go
    builder.go
    renderer.go
    evidence.go

Provider interfaces:

type AcquisitionProvider interface {
    Acquire(ctx context.Context, request Request) ([]Candidate, error)
}

type ContextSelector interface {
    Select(ctx context.Context, state StateView, spec ContextSpec) ([]Ref, error)
}

Providers can later include:

repository
taskstate
evidence
artifacts
durablememory
web

I would avoid calling the whole package memory. You already have memory, and it means something different.

10. Build it in narrow PRs

PR 1 — Task Ledger kernel

Add migrations, commands, transitions, append-only events, materialized state, and read-only exports.

No model behavior changes.

Acceptance:

deterministic state after replay;

optimistic concurrency;

invalid state transitions rejected;

generated status.txt and JSONL;

clean restart;

no prompts changed.


PR 2 — Automatic runtime recording

Wire:

IntentArtifact
PlanArtifact
writeArtifact
writeEvidence
tool results
step completion
step failure
user feedback
verification

into task events.

Still no model behavior changes.

This proves that the ledger can describe what Omnidex did without asking an LLM to write autobiography.

PR 3 — Working Set lifecycle

Implement:

acquire;

attach;

retain;

release;

scope completion;

stale-hash invalidation;

item and byte budgets;

metrics.


Run it in shadow mode. Continue using existing contexts for model calls.

PR 4 — Context Projection evidence

Build projections but do not send them to the model yet.

Compare:

current real prompt
vs
proposed deterministic projection

Record what would have been selected and omitted.

This will expose missing selectors before they can break a job.

PR 5 — First live consumer

Use it for repository investigation or the general V3 planner/subtask path, not the fragment model.

That is where continuity and evidence acquisition matter.

Keep greenfield Charmander generation unchanged.

PR 6 — Typed attention requests

Only after deterministic selection works, allow selected coordinator roles to ask for acquisition, retention, and release.

PR 7 — Semantic retrieval

Add vectors only after exact, structured, and full-text retrieval have a measured baseline.

Your R1 experiment already established the correct rule: additional machinery has to earn promotion. The existing gauntlet documentation likewise requires frozen cases, separate labels, repeated runs, paired fixes/regressions, validity, latency, tokens, and resource evidence before production changes are accepted. 

11. Proving that it is worth anything

You need to prove three separate claims:

Claim A: It preserves continuity

> Omnidex can resume a long task without relying on transcript memory.



Build a continuity gauntlet with:

50 tasks;

two repetitions;

10–20 steps per task;

early constraints needed only near the end;

decisions that supersede earlier decisions;

rejected hypotheses;

failures followed by corrections;

forced process restarts at randomized boundaries;

model conversation state cleared between calls.


Evaluate:

active goal recovered correctly
current task recovered correctly
constraints preserved
rejected hypothesis not reused
latest unresolved failure selected
completed work not repeated
restart produces same next runnable task

The strongest invariant:

Kill the worker after every completed step.
Restart it.
It must select the same next operation from PostgreSQL alone.

That proves external continuity rather than lucky model memory.

Claim B: It reduces context waste

Use the same model and tasks across these variants:

Variant	Description

A	Current runtime
B	Ledger recorded but never read
C	Ledger plus deterministic context projection
D	C plus scoped Working Set
E	D plus model attention requests
F	E plus semantic/vector retrieval


Variant B is important. It tells you whether merely adding persistence overhead affects anything.

Measure:

prompt/sent characters
estimated and actual tokens
peak context utilization
duplicate repository reads
duplicate search queries
repeated tool calls
working-set peak bytes
working-set cache hits
evictions
reacquisitions
thrashing
model calls
wall time

Omnidex already records most of the context-size, utilization, shrink, success, and latency data needed for this comparison. 

A reasonable pre-registered efficiency gate might be:

≥ 40–50% reduction in average model-visible context
≥ 25% reduction in duplicate retrieval/tool work
no decrease in task correctness

Claim C: It improves or preserves end-to-end competence

Same:

repository snapshot;

task;

model;

quantization;

context limit;

command policy;

clean worktree;

evaluation rubric hidden until both variants stop.


Compare baseline versus new context system.

Measure:

hidden tests passed
complete task success
direct-pass → working-memory-fail regressions
direct-fail → working-memory-pass rescues
lost constraints
out-of-scope edits
stale-reference attempts
correction loops
retrieval rounds
model calls
context tokens
model time
wall time
resume success

The binary result should be analyzed as paired outcomes, just like your requirement experiment:

baseline pass  → new pass
baseline pass  → new fail     regressions
baseline fail  → new pass     rescues
baseline fail  → new fail

Use the task as the statistical unit. Two repetitions measure stability; they do not magically turn 50 tasks into 100 independent observations.

12. Prove large-repository scaling directly

There is an especially good benchmark for your actual thesis.

Take the same target task and run it against:

Repository A:
    relevant module only

Repository B:
    same relevant module
    + 10× unrelated source

Repository C:
    same relevant module
    + 100× unrelated source

The correct behavior is:

index/storage cost:
    grows with repository size

model-visible context:
    stays roughly constant

retrieval rounds:
    stays roughly constant

task correctness:
    stays constant

The key metric is:

context bytes per relevant change surface

not:

context bytes per repository

Your defining scalability claim should be:

> Repository size does not determine model context size. The requested change surface determines model context size.



That is far more meaningful than advertising support for some context-window number.

Historical-commit routing gauntlet

For repository intelligence:

input:
    parent revision
    issue or commit message

withheld:
    actual diff

system must identify:
    relevant files
    relevant symbols
    likely tests

Measure:

file recall@5 and @10
symbol recall
test recall
irrelevant bytes
evidence-pack bytes
retrieval rounds
stale references

Compare in isolation:

exact lexical
exact + symbols
exact + structural graph
exact + graph + full-text
exact + graph + vectors

Commit diffs are only weak labels, so maintain a smaller curated gold set with verified relevant symbols and acceptable alternate routes.

Vector retrieval gets promoted only if it improves over exact and structural retrieval. It does not get tenure because pgvector was already standing around in the building.

Promotion criteria

I would require all of these before replacing current context behavior:

State validity:
    100%

Forced restart recovery:
    100%

Authority violations:
    0

Stale references admitted to model context:
    0

End-to-end correctness:
    not below baseline

Paired regressions:
    0, or explicitly justified by a much larger verified gain

Context reduction:
    materially below baseline

Duplicate acquisition:
    materially below baseline

Cost per successful task:
    improved or acceptably traded for a proven success increase

And I would not bundle:

task ledger
working set
repository graph
model attention API
vector search

into one giant experiment.

That would tell you only whether the entire pile worked, not which part helped or harmed it. The R1 run already showed why seductive pilot improvements need isolated controls and larger frozen tests.

The strongest first milestone

The first impressive demonstration is not yet “Omnidex edits a million-line repository.”

It is:

> Omnidex starts a 15-step maintenance task, is killed after random steps, resumes without conversation history, never loses the original constraints or accepted decisions, never repeats completed work, and keeps each model call under a fixed context budget.



Then add repository routing.

Then code changes.

Then scale the irrelevant repository around the same task and show that the prompt stays the same size.

That proves the architecture in layers.

The core idea is:

Repository Intelligence = external knowledge
Task Ledger            = continuity
Working Set            = attention
Context Projection     = one call's prompt
Durable Memory         = selected cross-job history

The planner does not need to be a giant model carrying the whole plan.

The conductor does not need to remember everything.

The coordinator does not need to dump the whole task history.

Code owns continuity. Code owns attention. Code owns lifecycle.

The model may forget everything after every invocation.

Omnidex does not.
