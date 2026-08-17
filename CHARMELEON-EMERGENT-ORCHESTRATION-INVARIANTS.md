I think it'd be 

- User has a prompt
- recent message history / memory ( I maybe be iterating over the last prompt I gave )
- investigation / current status tree / research / etc
- planning creates objectives
- objectives loop per objective
-- cognition loop per task
--- ( this starts a cognition loop given a certain task to complete, then for each task, it does... )
--- cognition investigation/tree loop
--- cognition planning loop
--- cognition execution loop
---- the normal coding loop we have now
... etc

Yes — this hierarchy makes more sense than the architecture I just proposed. I was flattening cognition into a coordinator between tree and files, when what you’re describing is that cognition is the recurring task-solving engine of Omnidex itself.

The tree is only one thing a cognition cycle may need.

I think the architecture is closer to this:

USER REQUEST
    ↓
REQUEST CONTEXT
    current prompt
    + relevant recent conversation
    + durable memory
    + current project/job state
    ↓
JOB-LEVEL INVESTIGATION / CONTEXT BOOTSTRAP
    establish what environment/project/reality we're operating in
    ↓
STRATEGIC PLANNING
    produce OBJECTIVES
    ↓
OBJECTIVE QUEUE
    O1
    O2
    O3
    ...
    ↓
for each objective:
    OBJECTIVE
        ↓
    derive / select next TASK
        ↓
    ┌─────────────────────────────┐
    │      COGNITION LOOP         │
    │                             │
    │   INVESTIGATE / OBSERVE     │
    │          ↓                  │
    │   PLAN THIS TASK            │
    │          ↓                  │
    │   EXECUTE                   │
    │          ↓                  │
    │   VERIFY REALITY            │
    │          ↓                  │
    │   update task state         │
    │          ↓                  │
    │   complete? ── no ──────┐   │
    │      │                  │   │
    │     yes                 └───┘
    └──────┼──────────────────────┘
           ↓
       TASK COMPLETE
           ↓
    objective satisfied?
       │          │
      yes        no
       ↓          ↓
 next objective  next task

And inside that cognition loop, the exact machinery invoked depends on what that task actually needs.

For example, suppose the objective is:

Add the counter feature to the application.

The cognition loop might begin with:

TASK
integrate counter feature

Then investigation asks:

What do I already know?
What currently exists?
What facts are missing before I can sensibly act?

For a greenfield project, that might lead naturally to:

current filesystem tree
        ↓
TREE SEMANTIC JOB
"what tree does this task require?"
        ↓
returned tree
        ↓
CODE parses/diffs
        ↓
concrete filesystem workload

But in a mature existing repository, the same cognition investigation could instead be:

Where is filtering implemented?
        ↓
code-owned repository investigation
        ↓
three candidate symbols remain
        ↓
tiny semantic ownership decision
        ↓
verified owner

No tree LLM at all if the task doesn’t need a structural decision.

That is much more consistent with your prime directive.

⸻

There are also two very different kinds of planning here, which I think is important.

At the top:

STRATEGIC PLANNING
User:
"Add appointment scheduling to this application."
→ O1 establish scheduling data/domain behavior
→ O2 expose scheduling UI
→ O3 integrate scheduling into existing application
→ O4 prove accepted scheduling behavior

Those are objectives.

Then inside one cognition loop:

TACTICAL PLANNING
Task:
"Expose scheduling UI"
Known facts:
- SchedulingService already exists
- appointments route exists
- current UI lives under resources/views/appointments
- Stimulus is already used
Task plan:
- determine required view/controller structure
- establish affected artifacts
- modify them
- verify UI behavior

The strategic planner shouldn’t be deciding exact files and functions three levels in advance.

The cognition planner shouldn’t be redesigning the whole project.

Each works at its own level.

⸻

The cognition loop itself

I think the task state wants to look conceptually like:

TASK STATE
Objective:
    what must become true
Known:
    verified facts relevant to this task
Unknown:
    facts that genuinely matter and are unresolved
Investigation:
    evidence currently being acquired
Plan:
    current bounded approach to completing this task
Work:
    concrete executable work derived from that plan
Results:
    generated/modified reality
Verification:
    what reality now says
Status:
    unresolved | ready | executing | blocked | complete

And code owns that entire object.

The LLM never says:

"now enter planning mode"
"now use repository search"
"now execute tool X"

Instead code sees:

task has unresolved evidence need
→ enter investigation workflow
task has enough evidence but no plan
→ formulate one bounded planning problem
task has executable work
→ enter execution workflow
execution produced reality
→ verify

So the cognition loop is a code-owned state machine, with various LLM semantic functions appearing inside it.

⸻

And execution contains the coding machine you’ve already built

This is important because you shouldn’t replace what you’ve already proven.

Inside:

COGNITION EXECUTION

for a coding task, you enter:

concrete artifact/file workload
        ↓
file-content semantic station
        ↓
declaration / responsibility queue
        ↓
coder
        ↓
parser/compiler
        ↓
acceptance/runtime verifier
        ↓
analyst → correction → coder
        ↓
verified source

That’s just an execution strategy available to cognition.

Likewise, another task might execute:

filesystem tree transition

or:

configuration mutation

or:

repository investigation

without pretending every operation is “coding.”

⸻

Where the coordinator emerges

So I think you were right to say some coordinator should emerge, but the coordinator isn’t really a separate layer or model.

It’s the nested state hierarchy:

JOB
 ├── Objective 1
 │    ├── Task 1
 │    │    └── cognition loop
 │    ├── Task 2
 │    │    └── cognition loop
 │    └── ...
 │
 ├── Objective 2
 │    └── ...
 │
 └── Objective N

Code knows:

what objective is active
what tasks satisfy it
what task is currently active
what the task currently knows
what remains unknown
what work is ready
what reality returned

That is the coordinator.

No coordinator LLM needs a gigantic prompt describing everything.

⸻

And memory/history belongs above cognition

I agree with the first part of your outline too:

User prompt
+
recent relevant conversation
+
durable memory

because a user’s current request may be:

“Yeah, do that, but use PostgreSQL instead.”

The immutable semantic authority isn’t just that sentence. Code needs to recover the relevant preceding intent.

But I would still avoid literal:

last 20 messages

as the architecture.

More like:

current message
+
messages that establish unresolved/current authority
+
accepted prior decisions
+
relevant durable project/user memory

So the initial request context itself is software-defined.

⸻

Then the overall machine becomes

CONVERSATION
    ↓
REQUEST CONTEXT COMPILATION
    ↓
JOB CONTEXT / INVESTIGATION
    ↓
OBJECTIVE PLANNER
    ↓
OBJECTIVE LEDGER
    ↓
TASK SELECTION
    ↓
COGNITION LOOP
    ├── investigate
    ├── plan
    ├── execute
    │     └── coding loop when appropriate
    ├── verify
    └── revise
    ↓
TASK COMPLETE
    ↓
OBJECTIVE COMPLETE
    ↓
NEXT OBJECTIVE
    ↓
FINAL JOB VERIFICATION
    ↓
COMPLETION

That feels much closer to the general-purpose Omnidex architecture, because neither trees nor coding are artificially elevated into the central abstraction.

The central abstraction is:

Given one task and authoritative state, repeatedly investigate, plan, act, and verify until that task is actually complete.

The cognition gauntlet was essentially a toy world for proving exactly that loop.

And then Charizard becomes very natural later: it doesn’t change this loop. It simply gives each cognition cycle better prior state:

Charmeleon:
solve task from present evidence.
Charizard:
solve task from present evidence
+ verified things previously learned.

So I think your hierarchy is stronger than the artifact-coordinator framing I gave before.

The tree isn’t the coordinator. The coding pipeline isn’t the coordinator. The LLM isn’t the coordinator. The persisted nested job → objective → task → cognition state machine is the coordinator, and everything else is machinery it invokes.
