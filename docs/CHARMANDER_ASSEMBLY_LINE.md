# Omnidex coding assembly line — Charmander build

Status: authoritative coding architecture.

Charmander is only the codename for this Omnidex build. It is not a subsystem, runtime, framework, worker, or product.

## Objective

Omnidex is a code-owned document assembly line. It accepts an ordinary user request, derives bounded local work, asks replaceable models only for semantics or source code that deterministic code cannot supply, constructs complete documents in memory, verifies the complete assembly, and writes the authoritative workspace only after isolated verification passes.

The target authority split is 99% ordinary code and 1% constrained inference. Model size must never compensate for missing rails.

Proof applications are held-out workloads. Their behavior, source, tests, terminology, and repair instructions must not be encoded in Omnidex.

## Authority boundary

Code owns:

* transport, job identity, lifecycle, interruption, feedback continuity, and completion;
* model routing, concurrency, retry counts, byte budgets, and exact response schemas;
* opaque identities, requirements, capability IDs, skill IDs, versions, and lifecycle state;
* filenames, paths, imports, document order, signatures, dependency waves, and source membership;
* AST parsing, response validation, merge-patch application, structural diffs, and forbidden-state checks;
* package manifests, pinned toolchains, staging, commands, diagnostics, workspace writes, and reconciliation;
* PostgreSQL skill persistence and Redis-backed progress/realtime coordination.

A model may perform only a bounded task whose semantics cannot be reliably derived by code:

* classify one delivery surface;
* copy one exact product-context quote;
* select one code-registered requirement-analysis lens;
* critique one exact requirement-partition prompt as a bounded plain-text memo;
* extract or split exact requirement quotes after stable-model synthesis;
* classify one opaque artifact disposition;
* classify the direct live-state relation between two local needs;
* select one of at most five opaque active-skill summaries, or `none`;
* propose one short procedure for one local need;
* return one raw source declaration;
* correct one retained semantic leaf or one current source declaration.

No model receives a file name, path, workspace tree, project plan, queue, phase, block identity, prior application, memory transcript, or completion authority.

## Production flow

1. The unchanged request enters the typed coding transport.
2. Artifact names are replaced with code-owned opaque tokens before semantic calls.
3. Independent semantic stations derive the supported surface, one exact product-context quote, exact requirement quotes, and explicit artifact handling. One stable schema-bound requirement-partition station is used for every extraction and split decision; there is no production reasoning adviser or second split-model route.
4. Code repeatedly removes accepted exact spans, asks the same stable partition station about the residual, and recursively splits every feature envelope to a strict fixed point. Code assigns `requirement_NNN` identities and rejects ungrounded, overlapping, reordered, duplicate, empty, non-progressing, or excessive results.
5. The current browser assembly supports one through ten requirements. Other surfaces or larger graphs fail explicitly before construction work begins.
6. Each requirement receives a procedure binding:
   * an exact active learned skill is reused by code when its code-owned identity matches;
   * otherwise the configured embedding provider retrieves at most five active `code_procedure` candidates from PostgreSQL;
   * a tiny selector sees only local context, one need, and opaque purpose summaries;
   * if it selects `none`, a tiny procedure station proposes one bounded instruction;
   * code creates the opaque skill ID, fixed schemas, immutable version, embedding, provenance, and validation lifecycle.
7. Every unordered requirement pair becomes one tiny capability-relation job. It sees one bounded product context and exactly two local needs. Its four-value result can express independence, either read direction, or a bidirectional dependency.
8. Code converts those decisions into a direct capability graph. Each feature receives a TypeScript projection containing only selected direct capability fields. Unselected channels are absent from the type checked by the compiler; transitive or all-to-all context is not exposed.
9. The generic TypeScript browser adapter creates an in-memory blueprint with a task-neutral runtime, one feature document per requirement, isolated acceptance functions, a compositor, runtime tests, smoke tests, and pinned static toolchain files.
10. Dependency waves are calculated by code. A generated block receives only its exact signature, local product context, exact need, one validated procedure, direct symbol projection, and explicitly in-scope package symbols.
11. The model returns one raw function declaration. Tree-sitter proves that it is exactly the requested declaration and rejects extra nodes, altered signatures, forbidden calls, or undeclared symbols.
12. Accepted declarations remain in memory. Code stitches complete documents only after their declared dependencies exist.
13. The complete program is written to an isolated stage. Code runs the pinned install, generated tests, runtime tests, type check, and production build.
14. A mapped source failure opens a correction job for the smallest declared owner. It receives the current declaration, exact signature, direct capabilities, one code-owned repair imperative, and a bounded path-free diagnostic. Other accepted declarations survive.
15. Only an isolated assembly that passes is written to the authoritative workspace.
16. Code reconciles exact files and protected paths, repeats authoritative verification, then activates pending learned skills and declares completion.

## Final-partition promotion experiment

The checked-in complete-requirement gauntlet is not a second production path. It measures whether one advisory pass after the authoritative direct fixed-point partition is stronger than the direct stable-Qwen result.

For the experimental final-pass variant, code first obtains and validates a complete direct candidate `C0`. An immutable final-advisory subject binds the original source, `C0`, its SHA-256 digest, and the protocol version. The reasoning model receives that subject without a response schema and returns one bounded plain-text memo. It must reserve output for non-empty final content; native thinking alone is invalid. A separate synthesis job binds the advisory job ID and exact memo digest, and the stable station returns `C1` under the existing requirement-partition schema. Code then applies exact-source, residual, and requirement-graph validation.

The final advisory work kind is rejected by the production worker. Promotion requires a frozen corpus of at least 50 cases, repeated runs, improved paired correctness, zero direct-pass regressions, no reduction in stability, complete validity, and exact model digest and quantization evidence. Until all gates pass, the production flow above remains authoritative and unchanged.

## Semantic correction

A decoded semantic response is retained in code. A correction call never receives the original request or retained JSON. It receives only the exact validation failure and a response schema permitting one mutable top-level field.

Code applies the response as a JSON merge patch and proves that exactly one JSON leaf changed. Immutable fields, unsupported fields, multiple top-level fields, multiple changed leaves, no-ops, malformed JSON, repeated candidates, and full-response retries fail explicitly.

## Learned-skill registry

PostgreSQL is the runtime authority. The checked-in `skills/` directory is bootstrap input only and is synchronized into immutable active `bootstrap_specialist` versions during startup.

Learned domain procedures are database data, never Go branches or static skill files. A learned version moves through:

`candidate -> validating -> active`

or:

`candidate|validating -> rejected`

Active versions may be retired but never edited. Activation requires all of the following code-observed evidence:

* bounded contract validation;
* an immutable retrieval embedding tied to provider and model identity;
* complete isolated-stage verification;
* complete authoritative-workspace verification.

An active version is unavailable to retrieval until the transaction commits. Rejected content cannot be silently resurrected, and there is no filesystem or general-agent fallback.

## Context hard cuts

Every final model call has independent hard limits for payload, prompt, candidate, current declaration, correction, and capabilities.

The fragment station does not know:

* the document or path receiving its declaration;
* the number or identity of other workers;
* unrelated requirements or unselected capability channels;
* acceptance source, stack traces, neighboring failures, or prior attempts;
* whether it is creating or editing a file;
* whether the wider application is complete.

Progress telemetry may show paths and phases to the human. That presentation data never becomes model context.

## Proof discipline

Unit and integration tests establish primitives only. They are not autonomy proof.

A valid application proof requires:

1. the exact ordinary user request through the production boundary;
2. a fresh workspace;
3. a frozen Omnidex build and model configuration;
4. no Codex-authored decomposition, feature list, rubric, prompt, source, correction, or mid-run edit;
5. exact evidence for every model-visible envelope, response, rejection, accepted declaration, command, verification, elapsed duration, and human intervention;
6. evaluation only after Omnidex stops;
7. a new clean run after every framework change.

A partial or failed workspace is still measured. Adding workload-specific helpers after observing a failure invalidates the benchmark rather than fixing Omnidex.

## Current limits

The current generic compiler supports new TypeScript/React browser applications only. Command-line and service surfaces fail explicitly at compilation. Existing-project mutation, backend persistence for generated applications, arbitrary dependency installation, and cross-machine scheduling are not yet supported by this assembly adapter.

These limits are defects or future work to measure honestly. They are not permission to route through the removed whole-file agent or add a product adapter.
