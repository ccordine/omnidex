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
* model routing, concurrency, retry counts, byte budgets, and exact raw-response contracts;
* opaque identities, requirements, capability IDs, skill IDs, versions, and lifecycle state;
* filenames, paths, imports, document order, signatures, dependency waves, and source membership;
* AST parsing, raw-leaf decoding, typed-result assembly, structural diffs, and forbidden-state checks;
* package manifests, pinned toolchains, staging, commands, diagnostics, workspace writes, and reconciliation;
* PostgreSQL skill persistence and Redis-backed progress/realtime coordination.

Every current station uses one raw-text provider transport. Its durable projection
contains exactly the rendered prompt and renderer identity; there is no response-schema
field, structured-output option, or alternate replay contract. Code parses and validates
the station-specific raw semantic leaf after the provider returns it.

Models do not call tools and may not request deterministic machinery. Parsers,
formatters, compilers, indexes, repository reads, graph traversal, process execution,
tests, and workspace writes run whenever code-owned state requires them. Tool catalogs
and function-call schemas are never model context. Inference receives only the
semantic or source-code remainder left after deterministic closure.

A model may perform only a bounded task whose semantics cannot be reliably derived by code:

* classify one delivery surface;
* classify whether another semantic evidence question remains after code-owned context bootstrap;
* state one semantic evidence question in a separate call when coverage remains;
* derive one concise product context from one immutable request plus source-backed context facts;
* classify whether another requirement remains, or return one requirement in a separate call;
* classify one opaque artifact disposition;
* classify the direct live-state relation between two local needs;
* select one opaque compatible registered project-stack/version-profile candidate, or report
  that the request is unconstrained or unsupported;
* classify whether one service task requires state produced by one request or
  process to remain authoritative for a later request or process;
* select one of at most five opaque active-skill summaries, or `none`;
* propose one short procedure for one local need;
* return bounded plain-text, non-authoritative considerations for one already-grounded objective;
* return one complete raw node tree for one frozen workload when structural naming remains unresolved;
* return one raw source declaration;
* diagnose one bounded path-free source failure into one self-contained imperative repair instruction without returning replacement source;
* execute one repair instruction against only one exact mutable source block;
* correct one retained semantic leaf.

No coding, repair, test-generation, or semantic-review model receives a file name,
path, workspace tree, project plan, queue, phase, block identity, prior application,
memory transcript, or completion authority. The target-tree declaration station is a
narrow exception: it receives a bounded code-built current tree and returns one raw
hierarchy of directory and file basenames. Code parses those nodes, constructs every
normalized relative path, and derives parent-directory and file work from the validated
tree diff; see [TARGET_TREE_PLANNING.md](TARGET_TREE_PLANNING.md).

## Production flow

1. The unchanged request enters the typed coding transport.
2. Artifact names are replaced with code-owned opaque tokens before semantic calls.
3. Code bootstraps application context before product interpretation. It records the exact workspace state as a typed, hashed fact. Repository manifests, indexes, symbols, tests, runtime probes, and external evidence are code-owned acquisition results with source identity and digest; models never receive an operation catalog or choose an acquisition mechanism.
4. Code skips context inference for a fresh empty workspace: the workspace state and immutable request are already complete context. For an existing workspace only, code runs a bounded fixed point. One raw coverage call returns the semantic relation `CONTEXT_NEED_REMAINS` or `NO_UNCOVERED_CONTEXT_NEED`. Only the former permits a separate call that returns one question. Code retains each decoded question, repeats coverage up to the code-owned three-question bound, resolves every retained question through its registered deterministic provider, formalizes the compact source-backed facts, and alone decides when the loop closes. A question names one fact that matters; it cannot name a command, file, path, tool call, architecture, plan, implementation, or completion state. Code never falls back to guessed context.
5. The surface station returns one registered delivery surface. A separate product-context station returns one raw product-context leaf from the immutable request and formalized context. Code then owns the bounded requirement fixed point: a raw coverage call returns `REQUIREMENT_REMAINS` or `NO_UNCOVERED_REQUIREMENT`; only the former permits a separate raw call for one requirement. Code decodes, validates, and retains each leaf, repeats coverage up to the ten-requirement bound, and assembles and freezes the typed application intent after it interprets the no-uncovered relation. It never asks a model to emit or review the aggregate intent. Semantic statements may faithfully paraphrase the request: substring, interval, source-order, disjointness, punctuation, and overlap tests are not authority boundaries.
6. Code assigns requirement identities and binds each statement to the immutable request digest. There is no one-shot quote gate, compatibility route, fallback requirements implementation, or mandatory semantic-review loop.
7. The semantic front door supports one through ten accepted requirements and keeps the typed browser, command-line, or service surface as frozen semantic authority. A surface is not a language or framework choice. A larger requirement graph fails explicitly; a surface without a complete registered project stack fails at stack selection.
8. Code deterministically projects each accepted requirement into exactly one frozen task in accepted source order. Each task contains only its code-owned task identity, requirement identity, and exact accepted requirement. The workload hash covers that exact projection. There is no workload-planning model call and no model-authored objective, behavior list, acceptance contract, dependency, order, tool, path, completion state, or plan. Downstream construction and verification receive the same exact requirement contract.
9. Each requirement may receive one active procedure enrichment:
   * an exact active learned skill is reused by code when its code-owned identity matches;
   * otherwise the configured embedding provider retrieves at most five active `code_procedure` candidates from PostgreSQL;
   * a tiny selector sees only local context, one need, and opaque purpose summaries;
   * if the registry has no active match, code performs no embedding or skill-model call and the exact requirement remains sufficient for ordinary generation;
   * ordinary workload execution cannot synthesize, validate, reject, or promote a skill candidate. Candidate synthesis remains unavailable until a separate code-owned recurring-gap workflow and an exact held-out replay producer exist.
10. Every unordered requirement pair becomes one tiny capability-relation job. It sees one bounded product context and exactly two local needs. Its three-value result can express independence or either direct read direction. These relations never choose task order. Code converts the results into a direct capability graph; unselected and transitive channels remain absent from each task projection.
11. Code selects one complete registered project stack compatible with the frozen surface. An authoritative existing manifest resolves a unique stack-local version profile mechanically; an unknown or ambiguous profile fails before inference. For a greenfield project, one bounded station sees only the surface, product context, and opaque code-enumerated stack/version-profile technical formats and returns one candidate ID, `UNCONSTRAINED`, or `UNSUPPORTED`; code validates and maps that leaf. It does not see registry IDs, parser identities, manifests, paths, or operational hooks. `UNCONSTRAINED` selects the code-owned surface default and its explicit default version profile. TypeScript/React remains the sole browser default; generic PHP and Laravel are additional server-rendered HTTP candidates for browser or service delivery; Go, JavaScript, Rust, and Java remain command-line candidates; generic PHP remains the service default. Each profile binds one parser-qualified source dialect, compatible manifests, exact dependency and lock authority, generated static values, bounded allowlisted runtime probes, and assembly validation. Code completes those probes before downstream semantic work. Adding another proven version profile extends the same bounded candidate registry; it creates neither a new station nor a central version switch. A stack that registers the HTTP compiler capability activates the task-local lifetime and endpoint stations regardless of which compatible delivery surface selected it. Its executable state hook validates the workload-hash-bound results before any tree or source work and again at compilation; code emits PostgreSQL authority only when a lifetime result requires cross-request state.
12. Code resolves one complete expected target tree for the whole frozen workload. Every currently registered stack has an exact mechanical projection and makes zero target-tree model calls. TypeScript browser applications allocate one neutral numbered source/test pair for the complete workload and bind every frozen task to both leaves; a valid existing pair is reconciled, while partial or ambiguous managed state fails loudly. The command-line and PHP HTTP stacks allocate neutral per-task implementation/verification pairs in their registered package layouts. An occupied half advances the whole pair. Code separately tracks regular files and directories, rejects reserved/static or existing-file ancestry conflicts, retains exact task provenance, binds the resulting `TargetTree` to the selected stack and version-profile identities, and builds the workload-hash-bound coverage plan. Only a future stack with a genuine unresolved naming question may use the raw `ROOT` hierarchy station; the model can return basenames arranged as one complete hierarchy but never normalized paths, a per-task fragment, ownership, or filesystem operations. An inferred stack without an explicit code-owned coverage rule is unsupported. Omission from the complete tree has no destructive authority by itself: a delete transition exists only when code separately proves and supplies eligibility for that exact current managed file. The coding driver mechanically classifies adapter-recognized files from the current snapshot as the managed workload set and separately grants exactly that set as deletion eligibility; unmanaged and reserved paths never become eligible. Before any source call, code extends the exact artifact-identity provenance boundary with every accepted target path.
13. The selected stack compiler consumes the frozen workload, direct capability graph, target union, and coverage plan and emits a language-neutral `SourceBlueprint`. A `SourceDocument` owns one adapter-bound path, preamble fragments, and ordered blocks. Each `SourceBlock` has exactly one static or generated authority, a bounded API, explicit dependency and direct-capability edges, and optional code-owned task ownership with support, implementation, representation, or verification role. Code validates the graph and task ownership. TypeScript/TSX, Go, JavaScript, Rust, Java, and PHP register focused document composers; parse-only or structural-only leaf adapters cannot enter source construction without a composer. The stacks also add only their task-neutral static runtime, entrypoint, manifest, configuration, and test support. Code then rebuilds provenance with every compiled document and static-file path before dispatching the first bounded source leaf, so a model-returned path literal cannot evade the boundary merely because code selected that filename during the same run.
14. Code executes frozen tasks in source order. The current task projection retains the selected version-profile identity and contains the authoritative surface and product, its one exact accepted requirement, direct capability declarations, and only the runtime/source evidence needed for that task. It contains no workload-planner projection or second acceptance contract. Other tasks, the application shell, workload hash, paths, identities, and unrelated requirements remain absent from source prompts.
15. A generated-source model receives one exact parser-qualified source-dialect label, one exact signature, that minimum task projection, an optional independently promoted active procedure, direct symbol declarations, and explicitly permitted symbols. It returns untrusted text from which code projects exactly one matching declaration. Each source language uses its registered parser and scope/authority policy before its composer constructs the complete file. Path-bearing literals and comments fail at the final provenance-aware source boundary; PHP additionally rejects heredoc and nowdoc authority. Raw provider responses remain evidence; no language grants a model whole-file authority.
16. Acceptance is stack-specific while task ownership remains neutral. Every stack constructs a separate owned verification declaration and deterministically proves that it invokes the owned implementation and contains failure-capable assertions grounded directly in the returned result. There is no acceptance-grounding model or reviewer. The TypeScript browser harness alone renders the public feature and performs registered interactions and assertions. Go uses `testing`; JavaScript uses strict Node assertions; Rust and Java use their focused registered test runners; PHP uses typed fixtures and `RuntimeAssertions`. HTML representation is a separate PHP source leaf, never part of the implementation question. No stack begins the next task before the current task passes isolated verification.
17. An initial parser, signature, scope, or authority rejection of one cleanly projected generated implementation declaration may enter the same separated repair boundary for TypeScript/TSX, Go, JavaScript, Rust, Java, and PHP. Initial-generation projection failures and generated verification declarations remain terminal and never become repair-model context. One exact compiler or parser diagnostic permits exactly one guidance call and one executor call. Guidance receives the exact language/dialect, signature, current declaration or TypeScript-local region, direct capabilities, compiler-proven lexical scope when available, and that one bounded path-free diagnostic; it returns one imperative instruction and no source. The separate executor receives only that instruction and exact mutable source and returns one replacement node. Code projects the executor response through the same registered language decoder used for initial generation. Invalid source fails immediately. Byte-identical source is a zero delta: code performs no mutation and creates no rejection-history or replacement-guidance call. Only a validated source transition is accepted, after which code changes the diagnosed generated block, preserves every other accepted block, and reruns the exact failed stage. A later guidance call requires a newly compiler-proven diagnostic after that accepted transition; the code-owned transition limit remains bounded. Staged compiler and acceptance failures enter this boundary only for TypeScript and registered PHP. PHP acceptance failures route mechanically from the immutable verification block to its single generated implementation owner. A staged Go, JavaScript, Rust, or Java failure remains terminal because those adapters register no stage-failure mapper.
18. After every task passes independently, code composes all documents and runs the selected stack's final isolated verification. TypeScript runs integrity-locked npm installation, tests, typechecking, browser smoke coverage, and a Vite production build with the pinned Tailwind CSS v4 plugin; the build must emit utilities used by assembled source. Go runs focused tests, full tests, vet, and build. JavaScript runs Node with exact filesystem permissions and syntax checks. Rust uses locked offline Cargo test/check/build. Java uses strict `javac`, its reflection-free test runner, and archive creation. PHP runs Composer validation, PHP lints and tests, digest-pinned Docker Compose build/config and NGINX checks, starts the isolated app and NGINX services, performs typed real HTTP requests for every route and media contract, and then tears the stack down.
19. The complete in-memory assembly retains the selected version-profile identity and must pass every leaf validator, profile invariant, and stack-wide invariant before the first authoritative workspace mutation. Code records the task-local artifact graph, derives filesystem transitions, and proves protected paths.
20. Only an isolated complete assembly that passes is written to the authoritative workspace. Code repeats leaf validation at the write gate, reconciles exact files and protected paths, reruns authoritative verification, and declares completion. The workload has no learned-skill mutation authority.

## Semantic leaf failure and narrow replacement boundaries

A decoded semantic leaf is retained by code and never becomes workflow authority. Code advances after a valid leaf without a ceremonial review. An invalid product-context, requirement-coverage, requirement, surface, relation, selection, or service-semantic leaf fails at its owning station; there is no generic response-correction work kind, correction model, retry budget, rejected-response history, or aggregate reconstruction path.

The target-tree station has one explicit tree-local exception because its single semantic responsibility is the complete raw basename hierarchy. After one exact code-proven grammar or structural defect, code may issue one replacement call to the same station responsibility. That call receives only the bounded defect and, when it parsed safely, the complete canonical current hierarchy, and must return one complete hierarchy. It cannot patch paths, return actions, or correct any other semantic state. An invalid or byte-identical replacement fails explicitly.

Source repair is a different two-station boundary described in production-flow step 17: one code-routed diagnostic permits one guidance instruction and one executor replacement node. It is not semantic response correction. Repository requirement extraction remains a separate existing-repository boundary and must not be used as an application-intent fallback.

## Learned-skill registry

PostgreSQL is the sole learned-skill authority. Omnidex does not load or synchronize checked-in skill files at startup.

Learned domain procedures are database data, never Go branches or static skill files.
Production currently exposes active-skill retrieval only. Candidate creation,
embedding writes, validation, rejection, and promotion are deliberately unavailable:
the repository has no code-owned recurring-gap producer or held-out replay executor,
so accepting caller-supplied success claims would manufacture authority.

A future promotion cutover must introduce one separate code-owned transaction whose
exact replay job differs from the creating job and which loads, rather than accepts
from its caller, all of the following immutable evidence:

* bounded contract validation;
* an immutable retrieval embedding tied to provider and model identity;
* at least two held-out replay cases bound to one immutable fixture-set digest;
* complete isolated-stage replay evidence;
* complete independent-workspace verification evidence.

The candidate, its one frozen provider/model embedding identity, replay receipt, fixed
validation checks, and activation must be bound by database constraints and commit
atomically. Until that producer exists, database mutation guards reject every learned
skill and embedding write. Same-job promotion, raw activation, caller-asserted hashes,
rejected-content resurrection, filesystem skills, and general-agent fallbacks are
forbidden.

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

The current generic greenfield compiler supports TypeScript/React browser applications; Go, JavaScript, Rust, and Java command-line applications; and request-local PHP/NGINX/Docker HTTP services with JSON, XML, text, binary, form, multipart, or server-rendered HTML boundaries. Cross-request or durable service authority fails before structural or source inference because no PostgreSQL or Redis workload-state adapter is registered. The existing-repository path additionally supports one explicitly named, previously absent, standalone unstructured plain-text document through a code-selected adapter, mechanically projected target, task coverage, focused source-node compiler, durable host mutation, and Docker-isolated exact-byte verification. Its one-new-complete-plain-text relation is a separate semantic station from the repository-artifact-absence relation; neither station may answer the other's question. It is not a general existing-project application compiler. The path does not support arbitrary existing-project mutation, arbitrary dependency installation, or cross-machine scheduling.

These limits are defects or future work to measure honestly. They are not permission to route through the removed whole-file agent or add a product adapter.
