# Target-tree planning contract

## Status

This is the normative structural front-door boundary. It is one narrow
exception to the path-blind coding-worker rule; it does not make the model a
planner, filesystem authority, content mapper, or coding worker.

## One frozen task, one focused structural answer

Target-tree resolution begins only after the application workload is frozen.
Code resolves one focused path set for each frozen task, in frozen task order.
It invokes the target-tree station only when the selected stack leaves this
semantic question unresolved:

> Which normalized relative workload file paths should exist for this frozen
> task in the code-selected technical format?

When inference is necessary, its input is code-built and task-local:

* the accepted product context, accepted requirement statement, and structural
  objective for that one frozen task;
* the one code-selected registered project stack and its exact path-count and
  root-location constraints required to choose compatible paths; and
* the real, bounded current workspace tree (file paths and directory paths),
  the retained path leaves from earlier frozen tasks projected as reusable or
  unavailable by code, and the exact selected-stack code-owned paths that are
  reserved for deterministic project artifacts.

Its complete response remains a path-only tree:

```json
{"schema":"omnidex.target-tree.v1","paths":["create.go","create_test.go"]}
```

The tree station returns no task or requirement IDs, artifact IDs, kinds,
purposes, ownership, source, declarations, commands, filesystem operations,
move/delete instructions, work items, ordering, dependencies, tests, tools, or
completion state. Two focused calls may independently return the same path;
that is a shared leaf, not a duplicate model-authored identity.

The selected stack is explicit technical context, not an instruction to write
source. The complete registered greenfield stacks are TypeScript/React for a
browser application; Go, JavaScript, Rust, and Java for command-line
applications; and PHP with NGINX and Docker Compose for an HTTP service. A
returned path is resolved by code through the selected stack's registered leaf
adapters. Parse-only or structural-only artifact support does not constitute a
complete project stack. The model never chooses an adapter, command, parser, or
validation operation.

Inference is forbidden when the stack grammar has one exact mechanical answer.
The Go, JavaScript, Rust, and Java command-line stacks allocate neutral
three-digit implementation/verification pairs in their registered package
layout. The PHP service stacks apply the same rule to `src/FeatureNNN.php` and
`tests/FeatureNNNTest.php`. A half-existing pair is preserved and skipped rather
than guessed or overwritten. Every projected pair passes the same adapter,
stack, ownership, diff, union, and coverage validation as an inferred path-only
result. A deterministic projection failure is terminal; there is no model
fallback. The TypeScript/React browser stack retains the target-tree station
because component placement remains a genuine structural naming question.

For a fresh workspace, the existing tree is empty. At this contract boundary an
existing tree can be input evidence, and an omitted existing path means
untouched; it never implies deletion. The current ordinary runtime still routes
an implementation-bearing workspace through its separate repository-change
pipeline before reaching this boundary; target-tree reconciliation alone is not
evidence of fresh/existing workflow parity.

One narrower existing-workspace case has a complete code-owned projection. If
lexical parsing and the artifact registry establish exactly one explicitly named,
absent plain-text path, a bounded semantic station may classify only whether the
intact request requires exactly that one standalone unstructured document with a
complete requested body and no other change. Code selects the `plain_text` adapter
before structural work, freezes one path-blind task bound to the immutable request,
projects the exact requested path without target-tree inference, supplies the full
bounded current tree to the ordinary target input, derives a create-only diff and
task coverage, and invokes the focused plain-text `SourceBlueprint` compiler. A
collision, ignored path, unsupported adapter, reconcile transition, or workspace
rejection is terminal; no replacement path is requested from a model.

When inference remains necessary, code projects path authority into three
separate model-facing facts. The exact
filesystem snapshot appears in `EXISTING_WORKSPACE_PATHS_JSON`; those paths may
be returned for reconciliation and omission remains non-destructive. Paths
accepted for earlier frozen tasks appear in `REUSABLE_ACCEPTED_PATHS_JSON` only
when the selected stack permits shared ownership. The selected stack's
code-owned model-addressable paths, plus earlier-task paths when the stack
requires exclusive ownership, appear in `FORBIDDEN_OUTPUT_PATHS_JSON` and
cannot be returned. A forbidden output path remains unavailable even when it
also exists in the workspace. Code derives these sets from the filesystem
snapshot, retained task union, and one stack registry before dispatch; the
model does not classify them. Only the exact filesystem snapshot participates
in create-versus-reconcile transitions. Reusable and forbidden paths never
become filesystem facts merely because they appeared in a prompt.

Candidate validation and every compiler entry consume the same reserved-path
collision function, while project stacks supply the sole compiler-owned path
registry. A model cannot claim a deterministic runtime, entrypoint, shell, or
matching code-owned test as a workload leaf. Prompt-only reservations never
enter the target union, task coverage, or filesystem transitions.

When inference remains necessary, the selected stack also supplies
`CODE_SELECTED_PATH_CONSTRAINTS_JSON`. Its
exact focused path count is applied as the response schema's `minItems` and
`maxItems`; root-only stacks additionally receive a no-slash item pattern whose
repetition contains the path-length bound. The bound is part of the pattern
because the deployed structured-output converter gives `pattern` precedence
over a sibling `maxLength`; an unbounded repetition would otherwise permit one
path to consume the model's entire context.
Candidate decoding and deterministic tree projection apply those same typed
constraints. At compiler entry, code reconstructs every focused tree from the
workload-bound coverage plan and applies the constraints again; the complete
union receives only invariants that remain true after unioning. Cardinality and
root location are therefore code-owned facts, not prose the model must
reconstruct from a stack description. Stack-specific validators retain the
narrower implementation/verification pairing and naming grammar.

## Code-owned union and coverage provenance

Code validates each focused result against the selected stack before retaining
it. It records the already-known frozen task ID beside those paths, then
computes the sorted set union of every focused result. That union
is the one authoritative `TargetTree` and is bound by code to the selected stack
ID and its compatible project-version-profile ID. Neither identity is target-tree
model output. No model is asked to restate, merge, or infer task ownership.

Code resolves each union path to its deterministic implementation or
verification kind and constructs one workload-hash-bound coverage record for
it. The coverage plan must prove all of the following:

* every canonical union path appears exactly once;
* every path has one registered artifact kind and at least one frozen task as
  provenance;
* task provenance contains only known task IDs in frozen order, without
  duplicates; and
* every frozen task is covered by at least one union path.

The neutral coverage plan permits plural files, implementation-only files, and
files shared by several tasks. A project stack may impose a narrower per-task
source-role rule at its own compiler boundary. It must not reinterpret the
global union as one universal implementation/test pair or recover provenance
from filename semantics.

## Code-owned transition

Code derives every parent directory and compares the canonical union with the
authoritative filesystem snapshot:

* a missing parent becomes one `ensure_directory` transition;
* a returned absent file becomes one `create` transition;
* a returned existing file becomes one `reconcile` transition; and
* an omitted existing file receives no transition.

Directories are derived by code. The model never creates a directory or emits a
filesystem operation. Transitions are ordered parent directories first, then
file leaves. A transition is a code-owned ledger item, not a model instruction.

## Neutral source-node boundary

Coverage provenance is not file content. The selected project-stack compiler
consumes the frozen workload, capability graph, target-tree union, and coverage
plan and creates a language-neutral `SourceBlueprint`. Its source nodes are:

* `SourceDocument`, which owns one path, adapter identity, preamble fragments,
  and ordered blocks; and
* `SourceBlock`, which has exactly one static or generated authority, a bounded
  API, explicit dependency and direct-capability edges, and optional code-owned
  frozen-task ownership and implementation, verification, or support role.

Code validates the neutral dependency graph, task ownership, and project-stack
constraints. It resolves each document path to an adapter and requires that
adapter's registered composer. TypeScript/TSX, Go, JavaScript, Rust, Java, PHP,
and unstructured plain-text source documents have focused composers; an adapter with only a leaf parser
cannot silently enter this source pipeline.

Before source generation starts, code rebuilds the exact artifact-identity
provenance boundary from the accepted target union, compiled document paths,
and task-neutral static-file paths. This is a code-owned validation input only;
the paths never enter a generated-source envelope. A path literal returned by
a source model is therefore rejected even when that path was selected during
the current run rather than present in the initial workspace.

There is no whole-file or file-content model call to rediscover code-owned
coverage. Each generated source call receives only one exact source-block
signature, its local behavior contract, and its direct declarations and
symbols. Code assembles static and accepted generated blocks into complete
documents in memory, records their exact source spans, and runs the selected
stack's parsers, compiler, and tests.

## Validation and correction

Code accepts each structurally valid focused tree directly. There is no
ceremonial review or model-authored accept/reject control plane. For a
model-resolved tree, only a concrete schema, path, or selected-stack validation
error permits a bounded complete replacement of that focused path-only
candidate. No-op or repeated candidates are explicit validation failures; they
are never routed as JSON patching or as an instruction for another model to
invent work. A code-projected tree is validated and either accepted or fails
loudly; it never creates a replacement inference call.

## Completion evidence

Target-tree completion is real only when every path in the canonical union has
been reconciled and is present in the host workspace, followed by the selected
stack's deterministic verification. Focused candidates, a coverage plan, or
in-memory/container-only source is not proof.
