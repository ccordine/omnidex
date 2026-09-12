# Target-tree synchronization contract

## Status

This specifies the sole permitted tree-visible model boundary, not an available
station in the current registry. Current stacks resolve their trees mechanically.
A future tree station is never normalized-path-visible: it answers one structural
semantic question and has no filesystem or workflow authority. Every source,
source-body correction, verification, and other semantic model remains path- and
tree-blind. Code alone supplies source declarations.

## One frozen workload, one complete answer

Target-tree resolution starts only after code has frozen and validated the
complete application workload. Every currently registered stack has an exact
mechanical tree projection and therefore makes no target-tree model call. A
future stack may use the raw-tree station only when its registered technical
grammar leaves a real naming uncertainty that code cannot resolve. That one
call is for the whole workload:

> What complete managed workload file tree should exist for all accepted goals
> in the code-selected technical format?

The input contains only:

* the exact immutable request, without task IDs or ownership metadata;
* the selected stack's exact technical tree grammar and file-count constraints;
* the current managed workload tree, rendered from code-held normalized
  relative paths; and
* the code-reserved tree that workload output cannot claim.

This is a complete desired workload state, not a per-task fragment or change
list. There are no earlier-task calls, reusable-path prompts, path-union model
calls, file-to-task mapper calls, or model-authored operations.
The immutable request stops at this structural boundary and is never projected
into a declaration or source-generation prompt.

The model tree excludes paths code already determines exactly, including
runtime shells, entrypoints, manifests, generated composition, styles, and
adapter-owned verification artifacts. Code unions the accepted workload tree
with those deterministic outputs before constructing the physical desired
output tree. The model is never asked to repeat deterministic adapter facts.

## Exact raw tree grammar

The target-tree response is raw text, never JSON:

    ROOT
      D src
        F counter.tsx
      D tests
        F counter.test.tsx

ROOT is the exact first line. Each other line uses exactly two spaces per depth
and is one of:

    D <single basename>
    F <single basename>

Names are single basenames. They cannot contain a slash, backslash, traversal,
an absolute or drive identity, leading/trailing whitespace, or control bytes.
Files cannot have children. Directories cannot be empty. Duplicate siblings,
file/directory collisions, blank lines, CR line endings, skipped depths, JSON,
Markdown fences, flat path lists, and prose are invalid.

The model does not return normalized paths. The parser alone walks the accepted
tree and constructs the sorted normalized relative file-path set. Code then
applies the selected file-count, root-location, reserved-leaf, existing
directory, adapter-recognition, and stack-pair validators.

The prompt's current and reserved facts use the same canonical rendering:
directories first, files second, with each group ordered by basename. An empty
fact set is exactly ROOT. Physical empty directories are code-only collision
evidence and do not enter the managed workload tree.

The response contains no artifact IDs, task IDs, kinds, purposes, ownership,
source, declarations, commands, operations, dependencies, ordering, tools, or
completion state.

## Mechanical stacks

Inference is forbidden when a registered stack has one exact structural answer.
TypeScript browser applications allocate one neutral numbered source/test pair
for the complete frozen workload and bind every task to that pair. Occupied
filenames do not establish managed ownership: either occupied half advances the
whole allocation to a free pair. Browser collisions with fixed source, static,
manifest, or generated-tool paths fail before source dispatch or mutation.

Go allocates a task-local implementation/test pair. JavaScript, Rust, and Java
allocate one task-local implementation path; code owns their verification
declarations and runtime support. Allocation consumes code-held occupation and
reserved-path facts, retains exact task-to-path coverage, and returns one sorted
union. These stacks make zero target-tree model calls. The current filesystem
occupation snapshot is implemented for the browser stack; this document does
not claim broader existing-repository reconciliation support.

A mechanical projection failure is terminal. There is no inference fallback.

## Code-owned coverage provenance

The TypeScript browser stack permits one implementation/test pair for the whole
workload. Because that stack requires exactly those two leaves, code binds every
frozen task to both returned leaves. This all-to-all coverage and the neutral
pair allocation are mechanically forced by the registered stack; neither is
model planning nor filename inference.

Mechanical stacks retain the task-to-path mapping code created during
allocation. In both cases the coverage plan proves:

* every target file appears exactly once;
* every file has a registered implementation or verification kind;
* every task is covered in frozen order; and
* provenance contains only code-owned frozen task IDs.

A future inferred stack is unsupported unless code can derive coverage from an
equally explicit registered rule. It must not add a mapper model.

## Code-owned synchronization diff

Code derives every filesystem transition from the parsed tree and authoritative
filesystem facts:

* a missing parent yields ensure_directory;
* an expected absent file yields create;
* an expected existing file yields reconcile; and
* an omitted current file yields delete only when code separately supplies
  that exact file in the deletion-eligible set.

The model never sees or returns deletion eligibility. An omission alone has no
destructive authority. Eligibility must be code-owned, normalized, duplicate
free, and limited to files proven present in the current managed snapshot.
Unmanaged and protected paths remain outside this diff.

Before coverage or source generation, code also proves file-hierarchy closure.
A target leaf cannot occupy an existing directory, cross a reserved/static
file boundary, or use an existing regular file as an ancestor. Exact overlap
is allowed only for one current managed leaf that will be reconciled in place.
The same closure check runs inside the inferred candidate validator before any
future raw-tree result can be persisted as accepted, and again at the final
tree boundary.

Current target-tree resolution constructs paths and coverage; it does not grant
deletion eligibility. The coding driver receives deletion paths from separately
accepted artifact directives. An adapter's ability to recognize a file is not
ownership or permission to delete it. The retired standalone plain-text creation
workflow is not another source of deletion authority. No current or future path
may derive that authority from tree omission alone.

## Validation correction

A valid complete tree advances directly after deterministic validation. A
concrete validation defect may trigger a bounded replacement of that same
complete raw tree; correction never becomes a per-task call or patch.

When the candidate is structurally safe, code canonicalizes it before including
it in correction context. When syntax contains an absolute identity, traversal,
slash-bearing name, or another unsafe shape, code omits the candidate entirely
and supplies only a bounded grammar defect. Unsafe raw model bytes are never
echoed into another prompt.

## Completion

A parsed tree, coverage plan, or transition list is not completion. Completion
requires code-owned source assembly, deterministic filesystem mutation,
presence and absence verification for every authorized transition, and the
selected stack's compiler and test commands against the authoritative
workspace.
