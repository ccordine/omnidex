# Omnidex code-owned execution — Charmeleon build

Status: authoritative execution boundary, not a second runtime specification.

Charmeleon is only a build codename. Omnidex's existing queue, workers, assembly line,
artifact adapters, and evidence stores own execution. The former universal cognition
runtime and parallel in-memory reference tree are retired.

## Purpose

Code drives the current user objective toward functional, verified completion.
Models answer small semantic questions inside that workflow. They do not act as
workers, coordinators, tool users, state owners, or completion authorities.

The governing contracts are [CHARMANDER_ASSEMBLY_LINE.md](CHARMANDER_ASSEMBLY_LINE.md),
[CHARMELEON_CONTEXT_SYSTEM.md](CHARMELEON_CONTEXT_SYSTEM.md), and
[CHARMELEON_COGNITION_RESOLUTION.md](CHARMELEON_COGNITION_RESOLUTION.md).

## Production state

The [queue](../internal/queue) owns the job, generation, steps, execution attempts,
lifecycle operations, and accepted coding plan. The
[assembly line](../internal/assemblyline) owns typed work and deterministic artifact
construction rules. The [worker](../internal/worker) executes those rules and invokes
the selected provider only for an unresolved semantic leaf.

An LLM call is associated with the current job, generation, step, attempt, work kind,
and exact inputs. Database call IDs identify observations and correction lineage.
A model does not generate any of those identities.

These contracts do not require an abstract Episode, ScenarioRef, hash-bound Revision,
PreparedAction, causal-catalog digest, or environment journal. Do not rebuild the
deleted reference implementation under new names. A supported execution boundary
uses the existing concrete records that its consumer actually needs.

## Code-owned loop

1. Accept the unchanged request through its explicit production transport.
2. Load current state and validate the job generation and active attempt.
3. Resolve every available deterministic prerequisite through registered code.
4. If one necessary semantic uncertainty remains, render its minimal input and
   invoke only its registered station.
5. Capture the actual provider response, parse its semantic value, and apply the
   station's code-owned validation and state rule.
6. Perform the resulting construction, acquisition, execution, or verification work
   in code.
7. Reuse accepted state and continue until the current objective is verified or a
   specific unresolved failure prevents progress.

A stage name does not justify a model call. A model's label does not authorize a
transition. There is no mandatory planner, reviewer, approval model, adversarial
challenge, or completion restatement in the success path.

Code decides when tools run, which adapters run, their arguments, ordering, bounds,
and how results affect retained state. Tool catalogs and call schemas are not model
context. Models are not instructed to abstain from capabilities they do not have.

## Actual-value records

[Evidence](EVIDENCE_LEDGER.md) records what happened:

- model-visible input and the provider's actual response;
- commands, arguments, observed output, exit status, and duration;
- database statements, typed arguments, and returned rows;
- web discovery, HTTP acquisitions, and fetched source text;
- applied simulation effects and their before/after state.

A request hash, expected-effect description, source fingerprint, sealed projection,
or declared success is not execution evidence. Record references identify actual
observations; code compares their values and the state relevant to the consumer.

Authentication and third-party package-integrity protocols have separate purposes.
They must not be confused with internal content-addressed work authority.

## Continuity within one service

The [lifecycle-operation boundary](../internal/queue/lifecycle_operation_types.go)
retains exact commands under code-issued identities. An exact retry reuses the
recorded result; conflicting reuse fails. Feedback and replanning remain within the
same authoritative job and create only the code-owned generation transition.

A worker must present the current attempt for its job generation and step. Expired
or superseded attempts cannot write or complete work. Reclaiming an expired attempt
while the service remains running is not restart recovery.

Accepted source work is reused by its retained inputs and observed result. One
source-body defect may continue only the same persisted generation lineage and
immutable model route. Code supplies the exact defective span and question, applies
the replacement text to its retained base, and preserves all unrelated accepted work.

No transcript, exported file, hash, or newly invented model plan restores state.

## Construction and completion

The accepted current objective determines the workload. Code rejects unauthorized,
unnecessary, unsupported, malformed, or duplicate semantic candidates locally.
Discarded speculation does not become a completion obligation.

Code constructs the target surface, declarations, dependency projection, source
blocks, verification stages, and workspace mutations. Source models receive only
one path-blind implementation responsibility. They do not choose files, tools,
dependencies, failure owners, or subsequent work.

A real compiler or test failure may authorize inference only after code identifies
one owning mutable span and one necessary semantic question. Missing ownership is
an explicit failure, not a larger correction prompt.

Completion is a code-owned conclusion from the accepted objective and observed
behavior. Successful verification commands prove only what they actually check.
A later explicit user objective may extend an already completed product; the current
job does not need to anticipate every possible enhancement.

## Startup boundary

[database/setup.sql](../database/setup.sql) is the sole internal schema definition.
Service startup drops and recreates the configured dedicated schema and discards
previous internal jobs, attempts, evidence, memory, and accepted state.

There is no internal migration chain, historical manifest, cross-start takeover,
replay-from-database, or archive reconstruction contract. Ordinary user workspace
files may remain, but they do not restore a stopped job's identity or authority.
A new request evaluates that current workspace normally.

Workload-owned migrations, when required by an authorized workload and supported
adapter, are separate from Omnidex's internal database lifecycle.

## Scope and proof

Roleplay's typed fictional simulation follows [ROLEPLAY_SIMULATION.md](ROLEPLAY_SIMULATION.md).
Research and database work use their registered acquisition paths and evidence.
None may introduce a universal tool-calling model or workload-specific framework
branch.

Framework verification must show actual zero-call deterministic work, bounded
semantic calls, exact state transitions, preserved accepted work, loud failure,
and the fresh-start boundary through the production implementation.

An autonomy claim additionally requires an unchanged ordinary request, frozen
framework code and routing, a fresh workspace, no intermediate steering, exact
execution evidence, and evaluation only after the build stops. The evaluator's
criteria and private data must remain unavailable to the builder during the run.

Conformance is established by [the evidence ledger](EVIDENCE_LEDGER.md) and
[the autonomy proof ledger](CHARMANDER_PROOF.md), not by speculative checkboxes for a
second environment protocol. Unverified capabilities remain unclaimed.
