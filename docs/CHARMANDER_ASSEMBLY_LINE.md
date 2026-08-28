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
3. Code bootstraps application context before product interpretation. It records the exact workspace state and bounded accepted durable-memory authorities as typed, hashed facts. Repository manifests, indexes, symbols, tests, runtime probes, and external evidence are code-owned acquisition results; models never receive an operation catalog or choose an acquisition mechanism.
4. Code skips context inference for a fresh empty workspace: the workspace state, request, and accepted memory are already complete context. For an existing workspace only, code runs a bounded fixed point. One raw coverage call returns the semantic relation `CONTEXT_NEED_REMAINS` or `NO_UNCOVERED_CONTEXT_NEED`. Only the former permits a separate call that returns one question. Code retains each decoded question, repeats coverage up to the code-owned three-question bound, resolves every retained question through its registered deterministic provider, formalizes the compact source-backed facts, and alone decides when the loop closes. A question names one fact that matters; it cannot name a command, file, path, tool call, architecture, plan, implementation, or completion state. Code never falls back to guessed context.
5. The surface station returns one registered delivery surface. A separate product-context station returns one raw product-context leaf from the immutable request and formalized context. Code then owns the bounded requirement fixed point: a raw coverage call returns `REQUIREMENT_REMAINS` or `NO_UNCOVERED_REQUIREMENT`; only the former permits a separate raw call for one requirement. Code decodes, validates, and retains each leaf, repeats coverage up to the ten-requirement bound, and assembles and freezes the typed application intent after it interprets the no-uncovered relation. It never asks a model to emit or review the aggregate intent. Semantic statements may faithfully paraphrase the request: substring, interval, source-order, disjointness, punctuation, and overlap tests are not authority boundaries.
6. Code assigns requirement identities and binds each statement to the immutable request digest. There is no one-shot quote gate, compatibility route, fallback requirements implementation, or mandatory semantic-review loop.
7. The semantic front door supports one through ten accepted requirements and keeps the typed browser, command-line, or service surface as frozen semantic authority. A surface is not a language or framework choice. A larger requirement graph fails explicitly; a surface without a complete registered project stack fails at stack selection.
8. For each accepted requirement, one raw call returns the concrete objective. Code then owns two independent bounded fixed points. Behavior coverage returns `BEHAVIOR_REMAINS` or `NO_UNCOVERED_BEHAVIOR`; criterion coverage returns `CRITERION_REMAINS` or `NO_UNCOVERED_CRITERION`. Only a remains relation permits the corresponding one-leaf call. These values describe semantic coverage and never direct workflow. Each call receives only the typed surface, product context, accepted requirements, focused requirement, and already accepted local leaves needed for its one question. Code decodes and validates each result, enforces the one-through-four bounds, and assembles the typed job specification itself. After one valid code-assembled specification per focused requirement, code assigns task identity and source order and freezes the complete workload under one content hash. No model emits or reviews the aggregate specification. Models cannot return task identity, dependencies, order, tools, paths, completion state, or a whole plan. Derived build decisions are labeled separately from exact user authority and may not invent capabilities, quantities, timing, defaults, or constraints absent from that authority.
9. Each requirement may receive one active procedure enrichment:
   * an exact active learned skill is reused by code when its code-owned identity matches;
   * otherwise the configured embedding provider retrieves at most five active `code_procedure` candidates from PostgreSQL;
   * a tiny selector sees only local context, one need, and opaque purpose summaries;
   * if the registry has no active match, code performs no embedding or skill-model call and the exact requirement remains sufficient for ordinary generation;
   * ordinary workload planning cannot synthesize, validate, reject, or promote a skill candidate. Candidate synthesis remains unavailable until a separate code-owned recurring-gap workflow and an exact held-out replay producer exist.
10. Every unordered requirement pair becomes one tiny capability-relation job. It sees one bounded product context and exactly two local needs. Its three-value result can express independence or either direct read direction. These relations never choose task order. Code converts the results into a direct capability graph; unselected and transitive channels remain absent from each task projection.
11. Code selects one complete registered project stack compatible with the frozen surface. An authoritative existing manifest resolves a unique stack-local version profile mechanically; an unknown or ambiguous profile fails before inference. For a greenfield project, one bounded station sees only the surface, product context, and opaque code-enumerated stack/version-profile technical formats and returns one candidate ID, `UNCONSTRAINED`, or `UNSUPPORTED`; code validates and maps that leaf. It does not see registry IDs, parser identities, manifests, paths, or operational hooks. `UNCONSTRAINED` selects the code-owned surface default and its explicit default version profile. TypeScript/React remains the sole browser default; generic PHP and Laravel are additional server-rendered HTTP candidates for browser or service delivery; Go, JavaScript, Rust, and Java remain command-line candidates; generic PHP remains the service default. Each profile binds one parser-qualified source dialect, compatible manifests, exact dependency and lock authority, generated static values, bounded allowlisted runtime probes, and assembly validation. Code completes those probes before downstream semantic work. Adding another proven version profile extends the same bounded candidate registry; it creates neither a new station nor a central version switch. A stack that registers the HTTP compiler capability activates the task-local lifetime and endpoint stations regardless of which compatible delivery surface selected it. Its executable state hook validates the workload-hash-bound results before any tree or source work and again at compilation; code emits PostgreSQL authority only when a lifetime result requires cross-request state.
12. Code resolves one complete expected target tree for the whole frozen workload. When the selected stack leaves a genuine naming question, one target-tree call receives all accepted goals in frozen order, the stack's exact technical tree grammar and file-count constraints, and the bounded current managed and reserved trees. It returns only the raw `ROOT` node grammar: directory and file basenames arranged as one complete hierarchy. Code parses the nodes and constructs the sorted normalized relative paths; the model never emits paths, a per-task fragment, ownership, or filesystem operations. When code can derive the exact tree, inference is forbidden: the command-line stacks allocate neutral implementation/verification pairs in their registered package layouts, while the PHP HTTP stacks allocate the first free `src/FeatureNNN.php` and matching test pair. Those mechanical projections preserve current and reserved pairs and retain their code-known task provenance. Code binds the resulting `TargetTree` to the selected stack and version-profile identities and builds the workload-hash-bound coverage plan. For the current inferred TypeScript stack, the registered two-leaf rule mechanically binds every frozen task to both leaves; an inferred stack without an equally explicit coverage rule is unsupported. Omission from the complete tree has no destructive authority by itself: a delete transition exists only when code separately proves and supplies eligibility for that exact current managed file. The coding driver mechanically classifies adapter-recognized files from the current snapshot as the managed workload set and separately grants exactly that set as deletion eligibility; unmanaged and reserved paths never become eligible. Before any source call, code extends the exact artifact-identity provenance boundary with every accepted target path.
13. The selected stack compiler consumes the frozen workload, direct capability graph, target union, and coverage plan and emits a language-neutral `SourceBlueprint`. A `SourceDocument` owns one adapter-bound path, preamble fragments, and ordered blocks. Each `SourceBlock` has exactly one static or generated authority, a bounded API, explicit dependency and direct-capability edges, and optional code-owned task ownership with support, implementation, representation, or verification role. Code validates the graph and task ownership. TypeScript/TSX, Go, JavaScript, Rust, Java, and PHP register focused document composers; parse-only or structural-only leaf adapters cannot enter source construction without a composer. The stacks also add only their task-neutral static runtime, entrypoint, manifest, configuration, and test support. Code then rebuilds provenance with every compiled document and static-file path before dispatching the first bounded source leaf, so a model-returned path literal cannot evade the boundary merely because code selected that filename during the same run.
14. Code executes frozen tasks in source order. The current task projection retains the selected version-profile identity and contains the authoritative surface and product, its accepted requirement and derived objective, its required behaviors and acceptance criteria, direct capability declarations, and only the runtime/source evidence needed for that task. Other tasks, the application shell, workload hash, paths, identities, and unrelated criteria remain absent from source prompts.
15. A generated-source model receives one exact parser-qualified source-dialect label, one exact signature, that minimum task projection, an optional independently promoted active procedure, direct symbol declarations, and explicitly permitted symbols. It returns untrusted text from which code projects exactly one matching declaration. Each source language uses its registered parser and scope/authority policy before its composer constructs the complete file. Path-bearing literals and comments fail at the final provenance-aware source boundary; PHP additionally rejects heredoc and nowdoc authority. Raw provider responses remain evidence; no language grants a model whole-file authority.
16. Acceptance is stack-specific while task ownership remains neutral. Every stack constructs a separate owned verification declaration and deterministically proves that it invokes the owned implementation and contains failure-capable assertions grounded directly in the returned result. There is no acceptance-grounding model or reviewer. The TypeScript browser harness alone renders the public feature and performs registered interactions and assertions. Go uses `testing`; JavaScript uses strict Node assertions; Rust and Java use their focused registered test runners; PHP uses typed fixtures and `RuntimeAssertions`. HTML representation is a separate PHP source leaf, never part of the implementation question. No stack begins the next task before the current task passes isolated verification.
17. An initial parser, signature, scope, or authority rejection of one cleanly projected generated implementation declaration may enter the same separated repair boundary for TypeScript/TSX, Go, JavaScript, Rust, Java, and PHP. Initial-generation projection failures and generated verification declarations remain terminal and never become repair-model context. Guidance receives the exact language/dialect, signature, current declaration or TypeScript-local region, direct capabilities, compiler-proven lexical scope when available, and one bounded path-free diagnostic; it returns one imperative instruction and no source. The separate executor receives only that instruction and exact mutable source and returns one replacement node. Code projects every executor response through the same registered language decoder used for initial generation; a correction response outside that declaration grammar is code-owned invalid-source evidence for the next bounded guidance attempt, never source or model-visible diagnostic context. Code validates the original signature and capability boundary, changes only the diagnosed generated block, preserves every other accepted block, and reruns the exact failed stage. Staged compiler and acceptance failures enter this boundary only for TypeScript and registered PHP. PHP acceptance failures route mechanically from the immutable verification block to its single generated implementation owner. Correction stops on success, no-op, repeated instruction or source state, lost authority, provider failure, cancellation, or a code-owned correction limit. A staged Go, JavaScript, Rust, or Java failure remains terminal because those adapters register no stage-failure mapper.
18. After every task passes independently, code composes all documents and runs the selected stack's final isolated verification. TypeScript runs integrity-locked npm installation, tests, typechecking, browser smoke coverage, and a Vite production build with the pinned Tailwind CSS v4 plugin; the build must emit utilities used by assembled source. Go runs focused tests, full tests, vet, and build. JavaScript runs Node with exact filesystem permissions and syntax checks. Rust uses locked offline Cargo test/check/build. Java uses strict `javac`, its reflection-free test runner, and archive creation. PHP runs Composer validation, PHP lints and tests, digest-pinned Docker Compose build/config and NGINX checks, starts the isolated app and NGINX services, performs typed real HTTP requests for every route and media contract, and then tears the stack down.
19. The complete in-memory assembly retains the selected version-profile identity and must pass every leaf validator, profile invariant, and stack-wide invariant before the first authoritative workspace mutation. Code records the task-local artifact graph, derives filesystem transitions, and proves protected paths.
20. Only an isolated complete assembly that passes is written to the authoritative workspace. Code repeats leaf validation at the write gate, reconciles exact files and protected paths, reruns authoritative verification, and declares completion. The workload has no learned-skill mutation authority.

## Semantic correction

A decoded semantic leaf is retained by code and never becomes workflow authority. Code advances after a valid leaf without a ceremonial review. Only one exact deterministic validation defect may create a response-correction call. That call receives the original single semantic question, the complete rejected raw leaf, and the bounded grounded defect, and returns one complete replacement in the original raw format. It returns no JSON wrapper, schema object, merge patch, diff, control label, or aggregate candidate. Code passes the replacement through the original station decoder and splices only the resulting typed leaf into code-owned state. The already accepted product context, requirements, objective, behaviors, criteria, and other leaves are not reconstructed. Empty, malformed, repeated, still-invalid, or no-progress replacements fail explicitly. Target-tree correction likewise replaces the same complete raw tree rather than patching it. Repository requirement extraction remains a separate existing-repository boundary and must not be used as an application-intent fallback.

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

The current generic greenfield compiler supports TypeScript/React browser applications; Go, JavaScript, Rust, and Java command-line applications; and request-local PHP/NGINX/Docker HTTP services with JSON, XML, text, binary, form, multipart, or server-rendered HTML boundaries. Cross-request or durable service authority fails before structural or source inference because no PostgreSQL or Redis workload-state adapter is registered. The existing-repository path additionally supports one explicitly named, previously absent, standalone unstructured plain-text document through a code-selected adapter, mechanically projected target, task coverage, focused source-node compiler, durable host mutation, and Docker-isolated exact-byte verification. It is not a general existing-project application compiler. The path does not support arbitrary existing-project mutation, arbitrary dependency installation, or cross-machine scheduling.

These limits are defects or future work to measure honestly. They are not permission to route through the removed whole-file agent or add a product adapter.
