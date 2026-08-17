# Target-tree planning contract

## Status

This is a normative Charmeleon planning boundary. It defines the one narrow
exception to path-blind model context. It does not relax the coding assembly-line
boundary.

## Purpose

The target tree is a declaration of desired semantic artifact structure, not an
execution plan. Registered technical adapters may add their own deterministic
runtime/manifests/bootstrap artifacts after the declaration; a model never decides
those adapter artifacts. It lets Omnidex turn a user objective into a finite,
code-owned workload:

```text
immutable objective + verified context + current artifact inventory
    -> target-tree declaration
    -> code validation and structural diff
    -> code-owned artifact and declaration ledger
    -> bounded source generation and verification
```

The sole semantic uncertainty assigned to the target-tree station is: which
artifacts must exist, remain, move, or cease to exist for the accepted objective.
It returns typed artifact nodes, never operations.

## Model boundary

The target-tree declaration station may receive only:

* the immutable user objective and accepted requirements;
* compact, source-backed project facts and accepted memories that are relevant;
* a bounded, code-built inventory of existing artifact opaque IDs, normalized
  relative paths, and artifact kinds; and
* the response schema and hard size limits.

For every desired file node it may declare:

* a normalized relative target path;
* an artifact kind;
* a concise semantic purpose;
* opaque requirement IDs that the artifact serves; and
* either one existing opaque artifact ID or one new planner-local key.

It may not return or choose commands, filesystem operations, directory creation,
source content, imports, declarations, signatures, work items, ordering,
dependencies, tools, verification, or completion. It must not receive source,
workspace snapshots, task ledgers, tool catalogs, operation results, or coding
paths beyond the bounded artifact inventory.

All other semantic, coding, review, repair, and test-generation stations remain
path-blind.

## When it is legal to call

Code invokes this station only after deterministic bootstrap and context closure.
It is direct for an empty greenfield workspace because an initial desired structure
is a necessary unresolved semantic value.

For an existing workspace, code first uses a tiny topology-need station only when
it cannot determine whether the objective needs a structural artifact change. That
station returns one Boolean semantic value. A false result retains the current tree;
a true result opens this declaration station. Code never invokes either station when
the answer is already deterministic.

There is no mandatory tree review or accept/reject ritual. A valid tree is accepted
by code. Only an exact structural validation error can open a one-field semantic
correction, and that correction returns only the replacement field.

## Code-owned validation and execution

Code owns the current inventory and stable IDs. It rejects a declaration that has
an invalid path, duplicate target path, ambiguous existing ID, duplicate new key,
kind collision, missing purpose or requirement binding, forbidden/protected-path
transition, unsafe deletion, or any size-budget violation. Directory requirements
are derived from accepted file paths; no model creates directories.

Code compares the accepted target tree with current authoritative inventory and
derives every transition:

* `create` for a new artifact;
* `retain` for an unchanged artifact;
* `modify` when retained identity gains or loses requirement bindings/purpose;
* `move` for an existing opaque ID with a changed target path; and
* `delete` only when an existing ID is absent and deletion validation permits it.

The resulting artifact ledger, priorities, dependency order, declaration contracts,
test placement, writes, parsing, compilation, test commands, retries, and completion
are all code control flow. A model never sees or manages that ledger.

For each selected artifact, code derives the smallest declaration or block contract
that still requires semantic/source generation. A test-generation station, where
needed, receives only that exact test declaration contract; code chooses the test
artifact and runs the test.

## Completion and evidence

Target-tree success is not a model claim. It requires that the host workspace
inventory matches the accepted semantic tree plus code-derived registered adapter
artifacts, every derived artifact/declaration contract has been reconciled, and the
authoritative final verification succeeds. Container-only or in-memory artifacts are
not completion evidence.

Autonomy evidence records the untouched user request, the code-built inventory,
the exact target-tree envelope and response, validation/diff results, every derived
ledger transition, source/verification evidence, and final host paths. Codex must
never supply a target tree, path, source hint, or intermediate artifact to a proof
run.
