# Target-tree planning contract

## Status

This is the normative structural front-door boundary. It is one narrow
exception to the path-blind coding-worker rule; it does not make the model a
planner, filesystem authority, or coding worker.

## One question, one answer

The target-tree station answers exactly one unresolved semantic question:

> Which normalized relative file paths should exist to satisfy this objective
> in the code-selected technical format?

Its input is code-built:

* the accepted objective;
* only the technical context required to choose compatible paths; and
* the real, bounded current workspace tree (file paths and directory paths).

Its complete response is a path-only tree:

```json
{"schema":"omnidex.target-tree.v1","paths":["src/counter.tsx","tests/counter.test.tsx"]}
```

The tree station returns no artifact IDs, kinds, purposes, ownership,
requirement bindings, source, declarations, commands, filesystem operations,
move/delete instructions, work items, ordering, dependencies, tests, tools, or
completion state.

For a fresh workspace, the existing tree is empty. For an existing workspace,
the code-built existing tree is input evidence. An omitted existing path means
untouched; it never implies deletion.

## Code-owned transition

Code parses and validates the returned paths, derives every parent directory,
and compares paths with the authoritative filesystem snapshot:

* a missing parent becomes one `ensure_directory` transition;
* a returned absent file becomes one `create` transition;
* a returned existing file becomes one `reconcile` transition; and
* an omitted existing file receives no transition.

Directories are derived by code. The model never creates a directory or emits a
filesystem operation. Transitions are ordered parent directories first, then
file leaves. A transition is a code-owned ledger item, not a model instruction.

## Separate content boundary

Each returned file leaf opens a distinct file-content station. That station is
not the tree station and receives no tree or queue. Code determines the file's
technical category from the selected adapter and path grammar. The file-content
station returns only the bounded semantic coverage needed for that one file.

Code then maps that coverage to declaration/block contracts, schedules the
already-bounded coding workers, assembles source in memory, writes it to the
host workspace, parses/compiles/tests it, and verifies that every accepted tree
path exists on the host filesystem. A single file leaf may contain several
separate bounded source blocks; grouping those blocks is code assembly, not a
new responsibility for the tree model.

## Validation and correction

Code accepts a structurally valid tree directly. There is no ceremonial review
or model-authored accept/reject control plane. Only a concrete schema or path
validation error permits one bounded replacement of the path-only candidate.
No-op or repeated candidates are explicit validation failures; they are never
routed as JSON patching or as an instruction for a different model to invent
work.

## Completion evidence

Target-tree completion is real only when every returned file path has been
reconciled and is present in the host workspace, followed by the adapter's
deterministic verification. In-memory or container-only source is not proof.
