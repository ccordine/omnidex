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
* the one code-selected registered project stack required to choose compatible
  paths; and
* the real, bounded current workspace tree (file paths and directory paths).

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
The PHP service stack therefore does not call the target-tree station: code
allocates the first free three-digit feature number whose implementation and
matching verification paths are both absent from the current/reserved tree. A
half-existing pair is preserved and skipped rather than guessed or overwritten.
The resulting pair passes the same adapter, stack, ownership, diff, union, and
coverage validation as an inferred path-only result. A deterministic projection
failure is terminal; there is no model fallback.

For a fresh workspace, the existing tree is empty. For an existing workspace,
the code-built existing tree is input evidence. An omitted existing path means
untouched; it never implies deletion.

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
adapter's registered composer. TypeScript/TSX, Go, JavaScript, Rust, Java, and
PHP source documents have focused composers; an adapter with only a leaf parser
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
