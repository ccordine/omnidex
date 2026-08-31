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

* return one bounded source-ordered inventory of candidate repository-fact questions after code-owned context bootstrap;
* classify the necessity of one exact repository-fact candidate or compare it with one accepted question;
* return one bounded source-ordered inventory of untrusted atomic runtime-outcome candidates or the exact registered absence;
* classify or partition one exact authorized requirement candidate, or compare it with one accepted requirement;
* after at least one requirement survives, classify one delivery surface or derive one concise product/domain identity at its first consumer;
* classify one opaque artifact disposition;
* classify the direct live-state relation between two local needs;
* select one opaque compatible registered project-stack/version-profile candidate, or report
  that the request is unconstrained or unsupported;
* classify whether one service task requires state produced by one request or
  process to remain authoritative for a later request or process;
* select one of at most five opaque active-skill summaries, or `none`;
* return one bounded conversational response for one already-grounded objective;
* return one complete raw node tree for one frozen workload when structural naming remains unresolved;
* return one raw source declaration;
* diagnose one bounded path-free source failure into one self-contained imperative repair instruction without returning replacement source;
* execute one repair instruction against only one exact mutable source block;
* correct one exact defective candidate leaf after a separate positive grounding relation.

No semantic-review model exists. No coding, repair, or test-generation model receives
a file name, path, workspace tree, project plan, queue, phase, block identity, prior
application, memory transcript, or completion authority. The target-tree declaration station is a
narrow exception: it receives a bounded code-built current tree and returns one raw
hierarchy of directory and file basenames. Code parses those nodes, constructs every
normalized relative path, and derives parent-directory and file work from the validated
tree diff; see [TARGET_TREE_PLANNING.md](TARGET_TREE_PLANNING.md).

## Production flow

1. The unchanged request enters the typed coding transport.
2. Artifact names are replaced with code-owned opaque tokens before semantic calls.
3. Code bootstraps application context before requirement intake. It records the exact workspace state as a typed, hashed fact. Repository manifests, indexes, symbols, tests, runtime probes, and external evidence are code-owned acquisition results with source identity and digest; models never receive an operation catalog or choose an acquisition mechanism.
4. Code skips context inference for a fresh empty workspace: the workspace state and immutable request are already complete context. For an existing workspace only, one bounded raw call returns a source-ordered repository-fact-question candidate inventory or `NO_REPOSITORY_FACT_QUESTION_CANDIDATES`. The inventory is untrusted data. Code owns its queue and skips exact repeats. Each remaining candidate first enters one necessity/unresolvedness relation against the immutable request, current established facts, and that candidate alone. `NOT_NECESSARY_OR_ALREADY_RESOLVED` evaporates without a duplicate-relation call or evidence acquisition. Only a necessary unresolved candidate is compared with one already accepted question at a time through a separate `SAME_REPOSITORY_FACT_QUESTION`/`DISTINCT_REPOSITORY_FACT_QUESTIONS` relation; accepted state is immutable and is never collectively reviewed. A same-fact candidate evaporates. Code resolves a necessary distinct question exactly once through the registered deterministic provider, formalizes compact source-backed facts into the authoritative context, and exposes those facts to each later queued candidate. Queue exhaustion—not a model absence or completeness claim—returns the context without a completeness review, iterative question-generation call, or max-bound "another need" blocker. A question names one fact that matters; it cannot name a command, file, path, tool call, architecture, plan, implementation, or completion state. An authorized question whose registered resolver fails remains a loud machinery failure; code never falls back to guessed context.
5. Exactly one bounded requirement-inventory call returns either
   `NO_RUNTIME_REQUIREMENT_CANDIDATES` or between one and the code-owned maximum positive,
   source-ordered atomic runtime-outcome candidates. Code parses and counts the lines
   mechanically. No semantic station pre-counts the inventory, and no pre-count receipt
   exists. The inventory generator splits independent outcomes and
   may express only the literal core operation or governed result inherent in a
   purpose-denoting product or category name. A governed result must be independently
   verifiable, but that does not authorize an unstated presentation, delivery, storage,
   interface, or output-format choice. It omits construction constraints, customary
   features, and speculative enhancements. Inventory generation is untrusted intake, not
   authorization or a completeness claim. Code owns exact deduplication and the queue.
   Every remaining candidate first receives one request-entailment authorization relation;
   an unauthorized candidate evaporates before classification. Only then does code invoke
   kind and cardinality. An authorized candidate that still proves mixed or compound may
   return one bounded partition whose children re-enter the same queue and authorization
   boundary. A structurally invalid station response fails at that station. A structurally
   valid candidate whose semantic content remains malformed, cyclic, over-depth, or
   over-capacity dies locally without blocking an independent candidate or reopening
   accepted state. There is no later product-name generator or aggregate review.

   Product or category identity never licenses customary features, prerequisites, enabling
   behavior, or likely consequences. Its literal core action or governed result is proposed
   once at inventory intake and has no authority until its own candidate passes the ordinary
   sieve. Separately named controls, elements, states, persistence behavior, channels,
   formats, or other behavior still require their own authorized candidates. Candidate
   interpretation preserves semantic subjects: when the software produces a derived value
   from actor-selected or actor-supplied rule-bearing inputs, the application applies the
   rule and exposes the result; an actor or external source that supplies an already-derived
   value remains the source. Surface, technical or structural format, generic test or build,
   and deployment constraints are non-runtime and remain owned by their narrow code paths.
   A construction-workflow descriptor attached to the builder's act is non-runtime unless
   the request assigns that behavior or data flow to the completed application. A
   builder-directed test or verification clause adds no runtime outcome when it merely
   confirms that an accepted governed result is produced or conforms to the same determining
   rule. A genuinely different rule, external reference, scope, tolerance, event or
   observation time, retention boundary, time bound, delivery channel or recipient, output
   format, or state remains a distinct outcome when authorized.

   Only an authorized task-local one-outcome candidate is compared pairwise with one retained
   requirement at a time through the `SAME_RUNTIME_OUTCOME`/`DISTINCT_RUNTIME_OUTCOMES`
   relation. The model sees only those two exact statements; code binds the result to the
   candidate's complete kind/cardinality receipts and the retained requirement's
   result-relation receipt. An exact or semantic duplicate evaporates, and the retained
   requirement is never reviewed or reopened. Only a distinct candidate enters the separate
   three-way result-relation question. A derived result is an observable value whose
   correctness needs an independent value oracle. Selection, ordering, transformation,
   aggregation, measurement, or decision can establish that relation even when the value is
   the only rendered output. A named result-bearing operation applied to its governed object
   still asserts the resulting value when phrased grammatically as an action; action form does
   not turn a transform, read, extraction, decode, ordering, calculation, or selection into a
   result-free event. A named existing per-item grouping key completely determines
   group membership without requiring its origin or unasserted ordering. An expression,
   formula, predicate, or named operation supplied, configured, or selected by an actor is a
   rule-bearing input. A bare quality claim or output described only as calculated, computed,
   evaluated, generated, or selected remains missing. Actions, controls, unchanged rendering,
   state transitions, artifact availability, and event occurrences are `NO_DERIVED_RESULT`
   when they assert only that behavior.

   The result-relation job is bound to the candidate hash and complete kind/cardinality
   receipt hashes. An omitted relation opens one separate request-and-context grounding
   relation over only the immutable request, verified application facts, exact candidate,
   and complete missing-relation receipt. A negative result discards only that candidate
   before correction and cannot authorize a guessed policy. Only a positive
   `EXACTLY_ONE_DETERMINING_RELATION_ENTAILED` receipt permits one exact candidate
   correction. Code preserves all retained requirements and sends the corrected candidate
   back through ordinary exact deduplication, authorization, kind, cardinality, semantic
   duplicate, and result-relation checks. A second omission exhausts that candidate's one
   correction and discards only that candidate; there is no reviewer or correction retry.

   Queue exhaustion freezes the currently accepted functional objective for this iteration.
   It invokes no coverage, completeness, `REQUIREMENT_REMAINS`, or aggregate-review call;
   later objectives may continue iteratively from verified reality. Code alone assembles and
   freezes the typed intent, and semantic authority never depends on contiguous substrings,
   non-overlapping intervals, punctuation, or source allocation. If code retains a rejected,
   speculative, or over-capacity candidate as an optional follow-up suggestion, that record
   remains outside the current Task Ledger, workload, verifier, and completion criteria until
   a later explicit user objective returns it to the ordinary sieve. If at least one
   task-local requirement survives, code invokes product-context, delivery-surface,
   deployment, and other downstream semantic leaves only at their first actual consumer;
   if no leaf survives, those calls do not exist.
6. Code assigns requirement identities and binds each statement to the raw production request digest. A private authority carries the raw digest beside the path-redacted semantic request; only the redacted request is model-visible. After semantic resolution, code rebinds every accepted receipt to the raw digest and revalidates that authority when constructing and projecting the task result-relation plan, so jointly fabricated or merely self-consistent hashes fail. There is no one-shot quote gate, compatibility route, fallback requirements implementation, or mandatory semantic-review loop.
7. The semantic front door freezes the accepted requirement set, including an empty set, for one iteration. Only after at least one requirement survives does the first stack consumer resolve and freeze the typed browser or command-line surface; the first product consumer likewise resolves product identity only then. The bounded surface station may return `unspecified` only when the immutable request imposes no observable delivery constraint. Code resolves that leaf to `browser_application`, the one explicit default; the model does not choose the default. `unsupported` is reserved for an explicitly required unregistered surface or incompatible multiple explicit surfaces and fails before stack selection instead of falling through to the default. A surface is not a language or framework choice, and the default supplies no feature, requirement, or technical-format authority. A candidate beyond that bounded iteration is deferred or discarded as non-authoritative intake rather than becoming a completion blocker. Code fails only when it can prove that a requirement already admitted to the frozen objective cannot fit a required hard capability boundary without producing an incorrect result. A surface without a complete registered project stack fails at stack selection.
8. Code deterministically projects each accepted task-local runtime implementation requirement into exactly one frozen task in accepted source order. Each task contains only its code-owned task identity, requirement identity, and exact accepted requirement. The workload hash covers that exact projection. Separately, code constructs a workload-SHA-bound result-relation validation plan with exactly one task/requirement/receipt binding per frozen task and projects only the current task's one binding into its stage. This private plan is not part of the frozen workload or task context, is never rendered into any model envelope, and is revalidated against the workload and accepted requirement before it can constrain browser verification. There is no workload-planning model call and no model-authored objective, behavior list, acceptance contract, dependency, order, tool, path, completion state, or plan. Downstream construction and verification receive the same exact requirement contract.
9. Each requirement may receive one active procedure enrichment:
   * an exact active learned skill is reused by code when its code-owned identity matches;
   * otherwise the configured embedding provider retrieves at most five active `code_procedure` candidates from PostgreSQL;
   * a tiny selector sees only local context, one need, and opaque purpose summaries;
   * if the registry has no active match, code performs no embedding or skill-model call and the exact requirement remains sufficient for ordinary generation;
   * ordinary workload execution cannot synthesize, validate, reject, or promote a skill candidate. Candidate synthesis remains unavailable until a separate code-owned recurring-gap workflow and an exact held-out replay producer exist.
10. Every unordered requirement pair becomes one tiny capability-relation job. It sees one bounded product context and exactly two local needs. Its three-value result can express independence or either direct read direction. These relations never choose task order. Code converts the results into a direct capability graph; unselected and transitive channels remain absent from each task projection.
11. Code selects one complete registered project stack compatible with the frozen surface. An authoritative existing manifest mechanically narrows selection to one compatible stack-local version profile; an unknown or ambiguous profile fails before inference. One bounded station then sees only the already-redacted immutable request and opaque code-enumerated technical-format candidates, each including its packaging shape, and returns one candidate ID, `UNCONSTRAINED`, or `UNSUPPORTED`; an existing compatible manifest supplies only its one candidate so semantic request conflicts cannot bypass validation. Code validates and maps that leaf. The station does not see product-context or requirement projections, registry IDs, parser identities, manifests, paths, or operational hooks. `UNCONSTRAINED` selects the compatible manifest profile when one exists, otherwise the code-owned surface default and its explicit default version profile. TypeScript/React remains the sole browser default; generic PHP and Laravel are additional server-rendered HTTP candidates for browser or service delivery; Go, JavaScript, Rust, and Java remain command-line candidates; generic PHP remains the service default. Each profile binds one parser-qualified source dialect, compatible manifests, exact dependency and lock authority, generated static values, bounded allowlisted runtime probes, and assembly validation. Code completes those probes before downstream semantic work. Adding another proven version profile extends the same bounded candidate registry; it creates neither a new station nor a central version switch. A stack that registers the HTTP compiler capability activates the task-local lifetime and endpoint stations regardless of which compatible delivery surface selected it. Its executable state hook validates the workload-hash-bound results before any tree or source work and again at compilation; code emits PostgreSQL authority only when a lifetime result requires cross-request state.
12. Code resolves one complete expected target tree for the whole frozen workload. Every currently registered stack has an exact mechanical projection and makes zero target-tree model calls. TypeScript browser applications allocate one neutral numbered source/test pair for the complete workload and bind every frozen task to both leaves. Existing filenames never imply managed ownership: an occupied half or pair remains ordinary user state and advances the whole allocation to a free pair. Because the current greenfield browser compiler has no persisted managed-file receipt, any collision with its fixed source, static, manifest, configuration, or generated-tool-output paths fails loudly before a source call or mutation. The command-line and PHP HTTP stacks allocate neutral per-task implementation/verification pairs in their registered package layouts. Code separately tracks regular files and directories, rejects reserved/static or existing-file ancestry conflicts, retains exact task provenance, binds the resulting `TargetTree` to the selected stack and version-profile identities, and builds the workload-hash-bound coverage plan. Only a future stack with a genuine unresolved naming question may use the raw `ROOT` hierarchy station; it receives the exact immutable request and separately code-selected technical tree context, and the model can return basenames arranged as one complete hierarchy but never normalized paths, a per-task fragment, ownership, or filesystem operations. The raw request stops at that tree boundary and cannot enter source prompts. An inferred stack without an explicit code-owned coverage rule is unsupported. Omission from the complete tree has no destructive authority by itself: a delete transition exists only when code separately proves and supplies eligibility for that exact current managed file. No adapter or filename heuristic creates that eligibility. Before any source call, code extends the exact artifact-identity provenance boundary with every accepted target path.
13. The selected stack compiler consumes the frozen workload, direct capability graph, target union, and coverage plan and emits a language-neutral `SourceBlueprint`. A `SourceDocument` owns one adapter-bound path, preamble fragments, and ordered blocks. Each `SourceBlock` has exactly one static or generated authority, a bounded API, explicit dependency and direct-capability edges, and optional code-owned task ownership with support, implementation, representation, or verification role. Code validates the graph and task ownership. TypeScript/TSX, Go, JavaScript, Rust, Java, and PHP register focused document composers; parse-only or structural-only leaf adapters cannot enter source construction without a composer. The stacks also add only their task-neutral static runtime, entrypoint, manifest, configuration, and test support. Code then rebuilds provenance with every compiled document and static-file path before dispatching the first bounded source leaf, so a model-returned path literal cannot evade the boundary merely because code selected that filename during the same run.
14. Code executes frozen tasks in source order. The current task projection retains the selected version-profile identity and contains the authoritative surface and product, its one exact accepted requirement, direct capability declarations, and only the runtime/source evidence needed for that task. It contains no workload-planner projection or second acceptance contract. Other tasks, the application shell, workload hash, paths, identities, and unrelated requirements remain absent from source prompts.
15. A generated-source model receives one exact parser-qualified source-dialect label, one exact signature, that minimum task projection, an optional independently promoted active procedure, direct symbol declarations, and explicitly permitted symbols. It returns untrusted text from which code projects exactly one matching declaration. Each source language uses its registered parser and scope/authority policy before its composer constructs the complete file. Path-bearing literals and comments fail at the final provenance-aware source boundary; PHP additionally rejects heredoc and nowdoc authority. Raw provider responses remain evidence; no language grants a model whole-file authority.
16. Acceptance is stack-specific while task ownership remains neutral. Every stack constructs a separate owned verification declaration and deterministically proves that it invokes the owned implementation and contains failure-capable assertions grounded directly in the returned result. There is no acceptance-grounding model or reviewer. The upstream requirement sieve cannot retain a derived-result leaf until its candidate-hash- and receipt-hash-bound relation establishes that the leaf either needs no derived oracle or names the semantic relation and observable operands, conditions, and result meaning required to compute one. An underdetermined leaf first receives a separate request-and-validated-context-bound recoverability relation: a negative result discards only that leaf, while only a positive receipt permits correction of that exact leaf and ordinary revalidation before it can become task authority.

   Before TypeScript browser verification is generated, code projects the accepted implementation without its unresolved verification declaration, closes that implementation-only assembly through the real typechecker, exhausts any registered deterministic compiler correction, and revalidates the accepted declaration and public-surface shape. Only then does code extract one bounded path-blind public-interaction receipt. The receipt contains allowlisted intrinsic control roles, canonical role counts and ordinals, literal accessible names and placeholder hints, value kinds, explicit public action claims, and named dynamic `<output>` selector facts. Every visible output must have one unique exact literal `aria-label`, direct dynamic-only nonmixed content, and the intrinsic `status` role. Its expression must resolve through code-proven dataflow to declared state/capabilities or to local state whose setter value depends on an event value, prior accepted state, or another such derived local value; literals, constant aliases, static memoization, and constant setter calls fail. The receipt exposes only the accessible name. Static text, current values, JSX expressions, handlers, source, expected results, and dynamic text outside a registered output are never projected.

   Public-surface extraction is fail-closed. It accepts only registered intrinsic elements, an exact per-attribute grammar, and bounded JSX shape; every unknown, effectful, namespaced, duplicate, spread, or wrongly typed attribute fails. Forms are unavailable and every button requires exact `type="button"`, eliminating implicit submission/navigation authority. It rejects custom or unsupported intrinsics, dynamic visibility-bearing attributes, unsupported role-bearing attributes, inaccessible or unavailable control ancestry, and unregistered interaction or reference authority. `hidden`, `inert`, `aria-hidden`, disabled/read-only states and ancestry, and non-allowlisted Tailwind classes cannot conceal or disable a claimed public surface. Tailwind is an explicit safe utility/variant allowlist, so arbitrary values and visibility, opacity, pointer-event, clipping, transform, zero-size, screen-reader-only, or other unproved concealment classes fail instead of being guessed visible.

   The implementation cannot escape that declared surface through runtime host authority. All unbound runtime identifiers fail unless present in the exact deterministic ECMAScript/React allowlist or supplied as one requirement-bound registered host-capability call. Raw browser-global, DOM-selection, navigation, network, storage, audio, scheduler, dynamic-evaluation, and reflection authority remains denied. For each registered technical host capability, the candidate-bound runtime-necessity station sees only that capability's semantic purpose; code projects only a selected wrapper declaration into that one implementation block and requires one direct call inside a statically bound public event handler. The wrapper cannot be aliased or escaped and never exposes the underlying host API. It publishes one validated owned request to the code-owned application host bridge; the assembled application mounts that bridge and the selected static driver performs the real host operation. The code-owned isolated harness requires a dispatch receipt for every selected host capability, so a DOM-only surrogate cannot pass. Generated verification cannot invoke the raw host API, wrapper, or receipt observer and cannot install a fake host seam. Dynamic import and host metadata fail. Computed properties are limited to literals or lexically resolved immutable numeric indexes, while unresolved computed access and reflective destructuring fail. Code statically resolves inline, named, and `useCallback` JSX event handlers; computed event access, aliases, mutation, and event-object escape also fail. The only event-root data path is a direct, read-only `value` or `checked` leaf through `event.target` or `event.currentTarget`, represented by the canonical `event.target.value` and `event.currentTarget.checked` forms. Ordinary numerically indexed domain data remains available without opening dynamic property or DOM authority.

   Browser `state` and `capabilities` are immutable authority: extraction rejects direct or mechanically aliased rebinding, property writes, and registered mutator calls, while every `SharedValue` fallback and publication is boundedly validated as dense arrays or plain/null-prototype records, rejects cycles, accessors, hidden or symbol properties, custom prototypes, and unsupported values atomically, and deep-freezes the accepted graph before exposure.

   The owned verification declaration receives the portable receipt as its sole implementation-derived direct-dependency projection. Code freezes the exact receipt, the result-relation receipt, and the implementation's internal element-ID sequence before verification generation; it re-extracts and compares them after each staged attempt and before final execution. Element IDs remain code-only, must be unique within a task and globally across all assembled task surfaces, and cannot collide with reserved code-owned mount IDs. Any drift or collision fails.

   Browser verification has one exhaustive execution grammar: exactly one executable verification function whose non-empty body is a flat sequence of direct `fireEvent.click`/`change`/`input` calls, direct `expect(screen...)` assertions using a registered matcher, or an explicitly awaited `waitFor` whose parameterless callback contains only direct expectations. Declarations, assignments, branches, loops, returns, helpers, nested or dead closures, optional chains, aliased authorities, non-throwing queries, and any other statement or expression shape fail. A supported `screen` query must be called directly and consumed immediately as the asserted value or first event target. Singular role queries use one exact role and optional receipt name; plural role queries use the role and a literal in-range index; asynchronous queries and `waitFor` are explicitly awaited. Events carry only the exact registered argument count and compatible static target value.

   An exact derived text result is selectable only as the singular implicit `status` role with its receipt-proven accessible name and qualifies only through a non-negated `toHaveTextContent` assertion using an anchored literal regular expression. Generic text queries and mere presence cannot stand in for that owner or result. An explicit derived-result relation requires this exact named status-output assertion after at least one receipt-grounded public interaction, and the assertion must occur after the final `fireEvent`. A future no-interaction exception would require a separate exact semantic receipt proving that no public operand or operation is needed; the current result-relation receipt does not carry that authority, so zero-interaction explicit-result verification fails loudly. Every other interacting verification still needs a qualifying outcome after its final event; when no named output owns the outcome, the bounded alternatives are compatible checked, validity, value, or display-value control-state assertions with static scalar expectations. Action labels remain public claims that can select only among alternatives already established by the requirement; they never supply the relation or expected value. The static harness alone renders the feature and runs the accepted verification. A generated verifier's receipt-grounding or grammar rejection, and any staged execution failure originating in that generated verification block, is terminal evidence and cannot become implementation-repair model context. Go uses `testing`; JavaScript uses strict Node assertions; Rust and Java use their focused registered test runners; PHP uses typed fixtures and `RuntimeAssertions`. HTML representation is a separate PHP source leaf, never part of the implementation question. No stack begins the next task before the current task passes isolated verification.
17. An initial parser, signature, scope, or authority rejection of one cleanly projected generated implementation declaration may enter the same separated repair boundary for TypeScript/TSX, Go, JavaScript, Rust, Java, and PHP. Initial-generation projection failures and generated verification declarations remain terminal and never become repair-model context. For a registered TypeScript diagnostic, code first exhausts exact compiler-proven deterministic normalizations; only the mechanically unresolved remainder may authorize guidance. One exact residual compiler or parser diagnostic permits exactly one guidance call and one executor call. Guidance receives the exact language/dialect, signature, current declaration or TypeScript-local region, direct capabilities, compiler-proven lexical scope when available, and that one bounded path-free diagnostic; it returns one imperative instruction and no source. The separate executor receives only that instruction and exact mutable source and returns one replacement node. Code projects the executor response through the same registered language decoder used for initial generation. Invalid source fails immediately. Byte-identical source is a zero delta: code performs no mutation and creates no rejection-history or replacement-guidance call. Only a validated source transition is accepted, after which code changes the diagnosed generated block, preserves every other accepted block, and reruns the exact failed stage. A later guidance call requires a newly compiler-proven diagnostic after that accepted transition; the code-owned transition limit remains bounded. Staged compiler and acceptance failures can enter this boundary only for TypeScript and registered PHP and only through their exact code-owned source mapper. A TypeScript failure originating in a generated verification block is terminal; dependency edges cannot reassign it to implementation authority. PHP acceptance failures route mechanically from the immutable verification block to its single generated implementation owner. A staged Go, JavaScript, Rust, or Java failure remains terminal because those adapters register no stage-failure mapper.
18. After every task passes independently, code composes all documents and runs the selected stack's final isolated verification. TypeScript runs integrity-locked npm installation, tests, typechecking, browser smoke coverage, and a Vite production build with the pinned Tailwind CSS v4 plugin; the build must emit utilities used by assembled source. Go runs focused tests, full tests, vet, and build. JavaScript runs Node with exact filesystem permissions and syntax checks. Rust uses locked offline Cargo test/check/build. Java uses strict `javac`, its reflection-free test runner, and archive creation. PHP runs Composer validation, PHP lints and tests, digest-pinned Docker Compose build/config and NGINX checks, starts the isolated app and NGINX services, performs typed real HTTP requests for every route and media contract, and then tears the stack down.
19. The complete in-memory assembly retains the selected version-profile identity and must pass every leaf validator, profile invariant, and stack-wide invariant before the first authoritative workspace mutation. Code records the task-local artifact graph, derives filesystem transitions, and proves protected paths.
20. Only an isolated complete assembly that passes is written to the authoritative workspace. Code repeats leaf validation at the write gate, reconciles exact files and protected paths, reruns authoritative verification, and declares completion. The workload has no learned-skill mutation authority.

## Semantic leaf failure and narrow replacement boundaries

A decoded semantic leaf is retained by code and never becomes workflow authority. Code advances after a valid leaf without a ceremonial review. An invalid product-context, requirement-inventory, candidate-relation, surface, selection, or service-semantic leaf fails at its owning station; there is no generic response-correction work kind, correction model, retry budget, rejected-response history, or aggregate reconstruction path.

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
