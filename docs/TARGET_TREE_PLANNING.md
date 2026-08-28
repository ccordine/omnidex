# Target-tree synchronization contract

## Status

This is the sole path/tree-visible model boundary in the coding assembly line.
It answers one structural semantic question and has no filesystem or workflow
authority. Every source, declaration, repair, verification, and other semantic
model remains path- and tree-blind.

## One frozen workload, one complete answer

Target-tree resolution starts only after code has frozen and validated the
complete application workload. When the selected stack leaves a real naming
uncertainty, code makes one initial target-tree call for the whole workload:

> What complete managed workload file tree should exist for all accepted goals
> in the code-selected technical format?

The input contains only:

* every accepted goal in frozen order, without task IDs or ownership metadata;
* the selected stack's exact technical tree grammar and file-count constraints;
* the current managed workload tree, rendered from code-held normalized
  relative paths; and
* the code-reserved tree that workload output cannot claim.

This is a complete desired workload state, not a per-task fragment or change
list. There are no earlier-task calls, reusable-path prompts, path-union model
calls, file-to-task mapper calls, or model-authored operations.

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

Inference is forbidden when a registered stack has one exact structural
answer. Go, JavaScript, Rust, Java, PHP, and Laravel allocate neutral
implementation/verification pairs in code. Code performs the per-task
allocation against current, reserved, and already allocated paths, records the
exact task-to-pair provenance, and returns one sorted union. These stacks make
zero target-tree model calls.

A mechanical projection failure is terminal. There is no inference fallback.

## Code-owned coverage provenance

The TypeScript browser stack permits one implementation/test pair for the whole
workload. Because that stack requires exactly those two leaves, code binds every
frozen task to both returned leaves. This all-to-all coverage is mechanically
forced by the registered stack; it is not model planning or filename inference.

Mechanical stacks retain the task-to-pair mapping code created during
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

The target-tree diff supports scoped delete transitions. The coding driver
mechanically filters the current snapshot through the selected stack's artifact
adapters to form the managed workload set, then separately grants exactly that
deterministic set as deletion eligibility. The standalone plain-text creation
path grants none. Neither path derives eligibility from model output or omission.

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
