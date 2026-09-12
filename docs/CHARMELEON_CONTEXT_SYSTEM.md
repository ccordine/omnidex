# Omnidex software-defined context — Charmeleon build

Status: authoritative context boundary. Implementation claims require the production
code and behavioral evidence described below.

Charmeleon is only a build codename. This document does not define another runtime,
memory framework, or persistence layer. The coding contract remains
[CHARMANDER_ASSEMBLY_LINE.md](CHARMANDER_ASSEMBLY_LINE.md).

## Purpose

Repository size may increase acquisition and parsing work. It must not determine
model context size. Code resolves the facts it can determine and supplies only the
information needed for one remaining semantic question.

A model is not the owner of task state, context selection, retention, identity, or
completion. Its ordinary text result returns to code for interpretation and use.

## Current authorities

| Data | Authoritative owner | Use |
| --- | --- | --- |
| Request, generation, step, attempt, and accepted coding plan | Queue and current PostgreSQL schema | Continuity, execution eligibility, and completion |
| Repository files and symbols | Actual workspace, parsers, and registered artifact adapters | Construction and source validation |
| Acquired context | Fixed code-owned providers and the context compiler | The smallest relevant station input |
| Model input, provider response, and validation outcome | Job-owned LLM call evidence | Observation and same-runtime reuse |
| Commands, database reads, and web acquisitions | Their actual execution records | Verification and grounded citations |
| Fictional state and character knowledge | Roleplay's current-service records | Bounded, visibility-correct narrative context |

These are responsibilities of the existing production packages. The former generic
Task Ledger, Working Set, and Context Projection registries are not required
subsystems to restore. When those terms describe task state or active context in
other design documents, they do not authorize duplicate tables, event frameworks,
or model-managed memory.

PostgreSQL is authoritative during one service lifecycle. Startup recreates the
dedicated schema from [database/setup.sql](../database/setup.sql). Internal records
are not an archive, migration obligation, or restart-recovery contract.

## Request and task continuity

Code retains the unchanged user request separately from the path-redacted semantic
input. Explicit transport fields select coding, chat, roleplay, or other supported
boundaries; keyword matching must not reroute free-form text.

Accepted requirements and coding-plan decisions belong to the current job and
generation. Feedback and replanning update that same job through its queue-owned
transition. A model cannot invent a successor job or replace accepted obligations.

Within that lifecycle, an exact retry compares retained inputs and values. Operation
IDs and database row IDs identify records; hashes do not authenticate their meaning
or prove execution. Stale attempts and conflicting reuse fail explicitly.

## Need-driven acquisition

The [context compiler](../internal/contextcompiler/compiler.go) receives a fixed
provider and an explicit scope. Code inspects whether that provider can search.
When it can, the exact instruction is the retrieval query; otherwise the provider
receives the explicit empty query set. Models neither select providers nor formulate
those operation arguments.

Code validates acquired candidates, required/optional membership, source references,
and any exact grouping relationship. Required authority remains required.

For optional context, one relevance call receives one candidate and the instruction
needed to judge that candidate's relevance. Candidate IDs, authority namespaces,
grouping, source order, and acquisition machinery remain in code. A negative
candidate is omitted without reopening accepted candidates.

Selected content that fits is used without minification inference. Only when code
cannot fit the needed content through its deterministic reductions may a bounded
minification call answer what smaller text retains the necessary meaning. After
each reduction, code recombines that result with the untouched remaining groups
before considering another call. If the combined text fits, it is retained exactly
without further inference. Reduction must make progress and stay within the
station's hard limits. It does not add a review, completeness, or approval call.

Source records remain unchanged when their model-visible projection is reduced.
Fetch truncation, context reduction, and citation excerpt truncation are different
observations and must not be conflated.

## One call, one responsibility

Before dispatch, code must identify:

1. the exact unresolved semantic question;
2. why deterministic machinery cannot answer it;
3. the minimum input that question needs;
4. the single semantic result expected; and
5. the code that will validate and consume that result.

The station-specific work kind and inputs express that responsibility. A generic
uncertainty ledger or sealed projection object is not a prerequisite for inference.
The current five-answer descriptions are keyed by that work kind, without a
separate versioned identity. Their presence is not proof of execution or semantic
necessity; the actual input and deterministic consumer must satisfy the boundary.

Provider input is the versioned renderer's ordinary text. Provider output is ordinary
text, not a model-authored schema, identity, command, control decision, or state patch.
Code alone parses the result into the station-specific type.

For closed choices, code enumerates the complete applicable set. Zero options follow
the registered zero-option behavior; one is consumed without inference. Only genuine
plurality can require an opaque-letter choice. Subset selection uses separate
single-choice rounds over the still-unselected candidates.

## Source-generation cuts

A source-body call receives the exact language/dialect, declaration as lexical scope,
one local behavioral responsibility, and directly allowed declarations or symbols.
It receives no path, tree, queue, sibling work, workspace snapshot, or aggregate plan.
Code owns every structural byte around the returned implementation body.

The target-tree boundary is the narrow exception described in
[TARGET_TREE_PLANNING.md](TARGET_TREE_PLANNING.md). Code still constructs paths and
filesystem transitions from its result; omission alone has no deletion authority.

A real body defect may continue the same persisted generation job and model route
only after code proves the exact mutable span and necessary semantic question.
The continuation returns replacement text for that span. Code compares the current
source with its retained base, splices the result, and reruns validation. Accepted
bytes outside that span and unrelated jobs remain unchanged.

Package names are technical workspace-local compiler values. They are not hashes of
directory names or product titles. Product semantics remain separate.

## Evidence and freshness

The [call records](../internal/queue/llm_call_evidence_types.go) retain actual model
input, provider response, route, timing, and outcome. Root calls retain their exact
initial work inputs; correction calls identify their parent and mutable span.
[Reuse](../internal/queue/llm_call_evidence_recovery.go) is scoped to the same job,
generation, and step and compares actual retained inputs.

Freshness checks compare the values relevant to the consumer:

- source bytes and structural ownership for a source splice or workspace mutation;
- actual query arguments, returned columns, and rows for database evidence;
- recorded URL, fetched text, time, and projection for web citations;
- exact persona, scene, visible facts, and contributing records for roleplay.

An unrelated change is not a reason to reopen accepted work. A relevant mismatch is
an explicit failure or the exact locally authorized transition, not a global review.

A display index is disposable. The [workspace index](WORKSPACE_INDEX.md) and
[project map](CODEBASE_MAP.md) do not create persistent hash-addressed snapshots.
The runtime does not require custom inspection manifests or exported context files
to authorize work.

## Verification and limits

Context correctness requires inspecting actual model-visible bytes and proving the
consumer used the resulting value. A generated ID, digest, populated table, or model
activity count cannot establish that the objective was accomplished.

Framework tests must cover deterministic zero-call cases, minimal semantic calls,
bounds, changed relevant values, exact same-runtime reuse, and startup reset.
Repository-scale claims additionally require equal relevant work surrounded by
increasing unrelated source without growing model-visible context.

Current scoped evidence is described in [EVIDENCE_LEDGER.md](EVIDENCE_LEDGER.md).
An autonomous-build claim requires the unsteered production request and post-run
evaluation in [CHARMANDER_PROOF.md](CHARMANDER_PROOF.md). Neither this document nor a
passing primitive test substitutes for that evidence.
