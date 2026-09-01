# Emergent orchestration invariants

This document defines the code-owned coordination hierarchy for Omnidex. It
does not define a coordinator model, an agent loop, or a tool protocol.

## 1. Persisted hierarchy

The authoritative state is nested and durable:

```
JOB
  objective
    task
      cognition cycle
```

- A job retains the user authority, accepted context, and lifecycle state.
- An objective is one bounded outcome necessary for that job.
- A task is one bounded unit that can be observed, prepared, executed, and
  verified.
- The Task Ledger is the authoritative dependency graph and status record.
- The Working Set contains only the active task's necessary, accepted facts.

Code creates, schedules, transitions, and completes all of these records. A
model never creates nodes, edges, state transitions, scopes, retries, or
completion claims.

## 2. Cognition is a code state machine

Each active task repeatedly takes the smallest applicable transition:

```
restore authoritative state
  -> deterministic observation / validation / evidence acquisition
  -> named unresolved semantic uncertainty? -- no --> next deterministic transition
                                           -- yes -> one bounded semantic station
  -> validate and persist the returned typed leaf
  -> execute code-owned work
  -> verify reality
  -> task complete, blocked with an exact missing fact, or continue
```

No model call is permitted until code has exhausted the registered
deterministic work for that exact task and persisted the precise semantic
uncertainty. A model result is data for that station, never instructions for
the coordinator.

Finite closed choices obey their literal cardinality after code constructs the
applicable set: zero options take the station's explicit zero-option transition;
one option is consumed immediately with no model resolution, no model execution,
and no rejection; two or more options may open one bounded opaque-ID selection.
Code alone maps the returned ID to the retained value.

## 3. Planning hierarchy

Strategic planning establishes objectives. It does not decide files,
declarations, tools, source, or execution order several layers in advance.

Tactical planning occurs inside one task after its available facts are known.
It establishes only the next bounded semantic artifact that code cannot
derive. Code turns accepted artifacts into objectives, task dependencies,
artifact leaves, declaration leaves, and verification work.

## 4. Investigation is conditional

Code first restores exact project reality: workspace inventory, project stack,
manifests, accepted decisions, durable memory, current job state, and known
repository facts.

If those facts are sufficient, the task proceeds. If they are not, code
persists an evidence need with its question, relevance, permitted evidence
classes, and completion criterion. Code selects and runs the appropriate
repository, memory, web, runtime, or user-evidence workflow. Code owns every
acquisition query. A model may fill only a remaining semantic leaf, such as one
relevance relation or a choice between supplied opaque evidence candidates; it
never selects or calls a tool.

## 5. Raw target-tree boundary

The tree station runs only when naming remains genuinely unresolved after code
has frozen the complete workload. It receives all accepted goals in frozen
order, the code-selected technical tree grammar and constraints, and bounded
code-rendered current managed and reserved trees. It returns exactly one
complete expected workload tree in the raw `ROOT` node grammar. Each returned
node contains one directory or file basename; it does not return a normalized
path, ownership, purpose, requirements, content, declarations, commands,
operations, or filesystem authority.

Code then:

1. Parses every node and constructs and validates every normalized relative path.
2. Selects artifact adapters from each constructed path and the accepted project stack.
3. Diffs the complete expected workload paths against the authoritative managed tree.
4. Derives parent directories and creates one persisted leaf task for each
   directory and file transition.
5. Preserves the resulting order and requires code-owned filesystem evidence
   before closing each leaf.

Tree omission has no destructive authority by itself. It yields a delete
transition only when a separate code-owned rule supplies deletion eligibility
for that exact file and proves that it is present in the current managed tree.
Without that eligibility, omission yields no deletion.

## 6. Adapter baseline versus workload tree

An adapter may deterministically require manifests, runtime shells, generated
composition, bootstrap files, styles, and adapter tests. Those are code-owned
baseline artifacts, not missing tree-model output. Code adds them to the same
persisted filesystem workload as tree leaves, validates their exact bytes, and
records their creation or reconciliation like every other leaf.

The complete raw node tree remains the model's entire structural output. Code
never asks it to restate adapter mechanics it already owns.

## 7. Artifact bindings and cross-artifact coordination

Code derives every binding that topology and adapter facts force. For example,
one implementation leaf plus one verification leaf forces every accepted
requirement to that pair. Calling a model in this situation is prohibited.

When multiple artifacts leave a genuine ownership or interface uncertainty,
code must persist an explicit artifact-coordination task before any content or
source work depends on it. That task must state the exact unresolved relation,
the finite candidate artifacts, required verified interface evidence, and its
completion condition. It may use one path-blind semantic choice over opaque
artifact handles and code-projected declarations. It may not use a broad
requirement-to-path mapper, filename heuristics, another target-tree prompt, or a
whole-file worker.

Once an artifact's public declaration is generated and parser-validated, code
records that interface as verified fact. Dependent tasks receive only the
allowlisted symbol-level interface they require. They do not receive a sibling
file, project tree, or conversation history.

## 8. Execution boundary

For a coding task, code reduces accepted artifact work into declarations and
then exact mutable blocks. The coding assembly line owns signatures, paths,
imports, dependency order, local scopes, staging, writes, and verification.

The source model receives one code block's language, signature as scope context,
local behavioral contract, and allowlisted declarations and returns ordinary
implementation-body text. Code supplies the declaration and owns parsing and
validation. If code proves one specific defect and its exact mutable byte span,
only the same persisted source job and immutable model route may continue. The
model receives one necessary semantic question plus that span alone and returns
ordinary replacement text. Code digest-checks the retained base, splices only
the span, and reruns validation and reality. It never asks inference to preserve
the surrounding body and never opens guidance, executor, restart, or model-swap
paths.

## 9. Verification and completion

A generated block is accepted only after code validation. A filesystem leaf is
complete only after its exact transition is observed. An objective is complete
only after all prerequisite tasks and filesystem leaves are complete and the
selected real workspace verification passes.

Those predicates close only the frozen accepted objective for the current iteration.
They do not require every plausible product enhancement. Rejected, speculative, and
deferred intake candidates own no task or filesystem leaf and cannot block completion;
only a later explicit user objective may send one through the ordinary sieve.

Compiler, parser, test, runtime, and repository results are authoritative.
They either close a bounded task or create one exact next failure. They never
restart a completed project or cause a model to reconstruct accepted state.

## 10. Forbidden regressions

- No coordinator, reviewer, planner, content worker, or coder has tool access
  or execution authority.
- No mandatory approval/review call exists merely to emit `accept`.
- No model is called when code already knows the answer.
- No model returns filesystem actions, queue operations, state transitions,
  or completion.
- No model receives a workspace tree, file path, whole-file responsibility,
  task ledger, or orchestration state unless a separately specified invariant
  explicitly permits a narrower non-coding semantic projection.
- No model-to-model natural-language repair protocol replaces a typed code
  transition.
- No unsupported ambiguity is silently guessed. It remains an explicit
  persisted blocker until the registered task can resolve it.
