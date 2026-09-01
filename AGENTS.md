AGENTS.md


# PRIME DIRECTIVE

Omnidex is deterministic software that invokes LLMs as small, bounded semantic functions.

Code owns the workflow and must drive the user's objective toward verified completion.

At every step, code MUST perform everything that can be determined mechanically and invoke an LLM only when one specific, necessary semantic question remains unresolved.

Each LLM call MUST have exactly one responsibility, receive only the minimum context required for that responsibility, and return only the semantic result of that responsibility.

The result is returned to code. Code alone validates it, interprets it, applies it, persists it, routes subsequent work, performs tools and side effects, and verifies reality.

LLMs are not agents, workers, orchestrators, tool users, state owners, or authorities. They are byte-in / byte-out semantic functions embedded inside a code-owned machine.

The system exists to complete the user's objective, not to manufacture additional review, challenge, approval, or failure work.
# PRIME INVARIANT

Every LLM invocation MUST be justified by one named unresolved semantic uncertainty.

Before an LLM may be called, the system MUST be able to state:

1. What exact question remains unresolved?
2. Why can code not determine the answer exactly?
3. What exact information does this LLM need?
4. What single semantic result must it return?
5. What deterministic code will consume that result afterward?

If those five answers do not exist, the LLM call is forbidden.

If code can compute, search, parse, diff, validate, route, select, persist, execute, inspect, compile, test, or otherwise determine something exactly, code MUST do it.

A model MUST NOT be invoked merely because a workflow stage exists.

A model MUST NOT be invoked merely to approve, accept, review, challenge, retry, or restate something unless a concrete unresolved semantic question requires that judgment.

A model's output MUST NOT become framework authority. Control labels such as "accept", "replace", "repair", "execute", "apply", "search", or similar model-authored workflow decisions have no authority unless the station's single semantic responsibility specifically requires that exact bounded choice and code independently owns the resulting state transition.

One model call = one semantic responsibility.

If another semantic result is required, that is another station and another bounded call. NEVER enlarge the current station's responsibility for convenience.


# NON-NEGOTIABLE COROLLARIES

- Code owns state, authority, identity, versions, queues, ordering, routing, retries, persistence, filesystem operations, tools, search execution, parsing, compilation, testing, verification, and completion.

- Models never choose tools. Code invokes repository, memory, web, runtime, compiler, parser, filesystem, and other machinery.

- Models are never prompted to refrain from capabilities they do not possess. Do not put agent/tool/orchestration/state-management language into model context unless that concept is itself the one semantic problem being solved.

- Never spend inference on information code already knows.

- Never make an LLM reconstruct information that a parser, compiler, index, database, filesystem, typechecker, or other deterministic subsystem already possesses.

- Never create a model call merely to obtain permission to continue. Deterministically valid state continues unless a real unresolved semantic uncertainty blocks it.

- Never manufacture adversarial work in the production success path. Guards observe real failures; they do not invent failures to exercise themselves.

- Never treat model prose or control labels as authority over actual returned data. Code evaluates the returned semantic payload and reality.

- Never respond to a failure by broadening the responsible model's job. First reduce the problem further.

- Never solve a downstream need by adding unrelated fields or responsibilities to an upstream model. Add another narrow station if another semantic question genuinely exists.

- Preserve accepted state. Recompute or revisit only what new evidence makes unresolved.

- Progress is measured by verified changes in authoritative reality, not by model activity, retries, reviews, token usage, or changed text.

# ITERATIVE FUNCTIONAL COMPLETION

Omnidex is iterative by design. One job must make the accepted current objective real and verified; it does not have to predict the final product the user may eventually want.

- Completion is defined by a functional, verified realization of the accepted current objective. It is not defined by exhausting every plausible feature, enhancement, interpretation, or future requirement.

- After code resolves needed context facts, exactly one bounded requirement-inventory call returns either `NO_RUNTIME_REQUIREMENT_CANDIDATES` or between one and the code-owned maximum positive candidate lines. Code parses and counts those lines mechanically. No semantic station pre-counts the inventory, and no pre-count receipt exists. Inventory generation is untrusted intake, not authority or a completeness claim.

- Every positive inventory candidate enters the ordinary authorization-first sieve independently. A candidate that is unrequested, unnecessary, unsupported, malformed as semantic content, or duplicative is discarded without reopening accepted state or stopping independent work. Only after at least one leaf survives may product-context, delivery-surface, deployment, or other downstream semantic leaves run, each at its first actual consumer.

- Structurally invalid station output still fails loudly. A valid negative semantic relation is not an error or recovery path; it is the expected sieve result for that one candidate.

- Accepted leaves are never placed back before a global review, coverage, challenge, approval, or completion model. New evidence may reopen only the exact leaf whose invariant it changes.

- A model may not veto completion by claiming abstractly that something remains. There is no production completeness-review station. A possible future capability may be retained only as non-authoritative follow-up data outside the current workload and requires a later explicit user objective before it can enter the ordinary sieve.

- Rejected or speculative candidates may be retained outside the current workload as provenance-bound optional follow-up suggestions. They must never enter the current task ledger, verifier, workload, or completion criteria unless a later explicit user turn makes one authoritative.

- Capacity limits do not convert non-authoritative candidates into blockers. Code may defer or discard candidates beyond a bounded iteration. Code fails explicitly only when it can prove that an already authorized user obligation cannot fit a required hard capability boundary without producing an incorrect result.

- A completed job may intentionally leave the product improvable. A later user turn creates a new objective against the resulting authoritative workspace and passes through the same machine.

# CANONICAL EXAMPLE

TREE STATION:

Input:
- user objective
- current repository tree when applicable
- only context required to determine project structure

Question:
"What file/directory tree should this project have to satisfy the objective?"

Output:
- the tree

Nothing else.

The tree model does NOT describe file contents, ownership, declarations, implementation, filesystem commands, create/modify/delete operations, or downstream work.

Code parses and diffs the returned tree and creates the filesystem workload. It then
compiles the accepted tree and code-owned coverage into bounded source-block
responsibilities.

A source-body station receives one exact path-blind source responsibility and returns
ordinary implementation-body text. Code supplies the declaration and every structural
byte. Each call remains a different semantic function even when the same underlying
model is used.

# FUNDAMENTAL TEST

OMNIDEX IS NOT AN LLM WITH CODE AROUND IT.

OMNIDEX IS A CODE-OWNED SOFTWARE SYSTEM THAT MAY INVOKE INTELLIGENCE TO ANSWER ONE NECESSARY SEMANTIC QUESTION AT A TIME.




Prime Directive

This project values correctness, explicit failure, small maintainable architecture, and server-authoritative behavior.

Agents must not create compatibility layers, alternate implementations, silent fallbacks, hidden duplicate systems, or sprawling files. If something cannot be completed cleanly, fail loudly and explain exactly what is missing.

The goal is not to make the task appear completed.
The goal is to actually complete the task correctly.

⸻

Non-Negotiable Rules

1. Fail Loudly

Do not silently recover from invalid state.

Do not hide errors.

Do not create fallback behavior unless explicitly instructed by the human in the current task.

When something is wrong:

* Throw an explicit error.
* Return a clear validation failure.
* Log the failure with useful context.
* Add or update tests proving the failure path.
* Do not continue with guessed behavior.

Bad:

$value = $newValue ?? $oldValue ?? 'default';

Good:

if ($newValue === null) {
    throw new InvalidArgumentException('Expected newValue, received null.');
}

Silent failure is worse than loud failure.

⸻

2. No Fallbacks

Do not keep the old implementation as a fallback.

Do not maintain two competing systems.

Do not preserve JavaScript rendering “just in case” after moving rendering to server components.

Do not create legacy compatibility paths unless the user explicitly requests them.

When replacing a system:

* Remove the old path.
* Remove old tests that only validate the old behavior.
* Update callers to use the new system.
* Make invalid old usage fail loudly.
* Search for remaining references before claiming completion.

There should be one authoritative implementation.

⸻

3. No Absurdly Large Files

Do not create giant files.

Prefer small, focused classes, traits, components, services, actions, middlewares, requests, DTOs, enums, and tests.

Guidelines:

* Aim for files under 300 lines.
* Files over 500 lines require strong justification.
* Files over 800 lines are usually a failure and must be split.
* Do not dump unrelated logic into controllers, components, services, or JavaScript controllers.
* Do not create “god” objects.

If a file is getting large, split it by responsibility before continuing.

⸻

Architecture Rules

Omnidex AI Coding Architecture (Non-Negotiable)

The authoritative coding foundation is [docs/CHARMANDER_ASSEMBLY_LINE.md](docs/CHARMANDER_ASSEMBLY_LINE.md). The authoritative repository-intelligence and software-defined context evolution is [docs/CHARMELEON_CONTEXT_SYSTEM.md](docs/CHARMELEON_CONTEXT_SYSTEM.md).
The authoritative nested job → objective → task coordination direction is [CHARMELEON-EMERGENT-ORCHESTRATION-INVARIANTS.md](CHARMELEON-EMERGENT-ORCHESTRATION-INVARIANTS.md). It is code-owned persisted state, never a coordinator model: an LLM may fill only the exact unresolved semantic leaf inside one stage.
The authoritative task-local artifact-graph and adapter-projection direction is [CHARMELEON-CONVERGENT-COGNITIVE-DEVELOPMENT-INVARIANTS.md](CHARMELEON-CONVERGENT-COGNITIVE-DEVELOPMENT-INVARIANTS.md). The tree establishes only the work surface; code derives and persists verified artifact interfaces and relations before dependent leaves are dispatched.
The authoritative roleplay boundary is [docs/ROLEPLAY_SIMULATION.md](docs/ROLEPLAY_SIMULATION.md). Roleplay is a code-owned fictional simulation with bounded narrative generation; it has no model-visible operation or tool interface.

Build Codenames Have No Architectural Meaning

Bulbasaur, Ivysaur, Venusaur, Charmander, and Charmeleon are only names for successive Omnidex builds. They mark major rewrite milestones. They are not products, agents, runtimes, frameworks, workers, orchestration layers, or workload types. Do not create a codename subsystem or say that a codename independently builds an application. Omnidex is the product and this repository currently contains its Charmeleon build.

Omnidex is a deterministic assembly line, not an LLM pretending to be a software team.

Codex Builds Omnidex; Omnidex Builds the Workload

Codex's task is to implement and improve the general Omnidex system. Codex must not implement, scaffold, finish, prompt-engineer, or otherwise steer the application used to prove Omnidex. The workload application must be produced only by the checked-in Omnidex build after receiving the same ordinary request a human user supplied.

A unit test that manually constructs an application specification, feature list, document graph, component contract, acceptance contract, or fragment prompt may prove that an isolated framework primitive works. It is never evidence that Omnidex understood or built a user request.

An autonomy claim is valid only when all of the following are true:

* The unchanged user request entered through the ordinary production request boundary.
* Checked-in, task-neutral code derived and dispatched every subsequent envelope.
* Codex supplied no intermediate specification, decomposition, rubric, feature prompt, correction, file content, or implementation hint.
* The run used a fresh workspace and proceeded until the framework stopped, without source edits or human steering during the run.
* The evidence records every model, exact model-visible envelope, response, rejection, accepted artifact, verification command, elapsed duration, and context byte count.
* Evaluation criteria remained unavailable to the builder until the build stopped.

If any condition is false, label the run contaminated and do not use it as proof.

Never convert an observed benchmark need into framework code. In particular, do not add a domain action, helper, service, enum, prompt clause, component wrapper, test fixture, CSS treatment, or correction instruction because the current workload needs it. Adding `tone()` after a music-app failure, a brush primitive after a drawing-app failure, or a character controller after a simulation failure is implementing the benchmark inside the framework.

After a failed benchmark, framework changes may address only a task-neutral failure class. The change must be expressible without benchmark nouns or behavior and must be proven against at least two unrelated fixtures before beginning a completely new benchmark run. Never patch a benchmark while it is running and never resume a contaminated workspace as evidence.

The framework may own general mechanics such as syntax parsing, typed boundaries, DOM event wiring, process execution, compiler invocation, and retry accounting. It must not pre-decide the product's domain model, user interactions, feature layout, or implementation merely to make a held-out application pass.

Prompt construction is production code. Codex must not compose better one-off prompts at the terminal or inject additional instructions into a live run. Every model-visible byte must come from a versioned generic renderer, be subject to hard size and capability limits, and appear in the run evidence. If the framework cannot derive adequate work from the raw request, that is a framework failure to measure and fix generally—not permission for Codex to steer the model.

Domain Skills Are Learned Data, Not Repository Features

Omnidex must not accumulate product-domain abilities in Go source, checked-in prompts, static skill folders, or adapter branches. Audio production, drawing, simulation, accounting, calendars, and every other workload domain belong outside the Omnidex implementation.

When Omnidex needs a reusable ability that is not already available, it must be able to synthesize a narrowly scoped skill through bounded model jobs, validate that skill with code, and persist the accepted skill in PostgreSQL for the current service/database lifecycle. Later jobs in that same lifecycle may retrieve the smallest relevant accepted skills from the registry; the next service startup discards it with all other internal state. Natural-language skill matching is a narrow semantic-model job; it must not be replaced with keyword or phrase heuristics.

Code owns skill identity, schemas, version numbers, lifecycle state, tool permissions, dependency edges, validation, test evidence, activation, retrieval limits, and database writes. Models may propose only the small semantic or instructional fields that code cannot derive. A proposed skill is unavailable until its schema, boundaries, and executable checks pass. Rejected skill candidates remain rejected; there is no silent fallback to a checked-in domain skill or general-purpose agent.

The repository may contain only the task-neutral bootstrap machinery needed to create, validate, store, retrieve, and execute skills. A workload-specific skill must be data with provenance in the database, not a new Omnidex code path. Generated application source remains in the target workspace; reusable worker procedure belongs in the skill registry; neither belongs in Omnidex's static architecture.

Code must own:

* Generic language, runtime, and toolchain adapter selection.
* Document structure, paths, names, declarations, signatures, and block boundaries.
* Dependency graphs, scheduling, retries, diagnostic routing, and completion state.
* Parser and compiler context extraction.
* Imports, stitching, formatting, complete-graph dependency checks, isolated compiler/test staging, workspace writes, and final test execution.

A coding LLM may only fill one explicitly defined implementation body when deterministic code cannot provide that body. Initial generation receives only the language, exact signature as lexical-scope context, local behavioral contract, and strictly required declarations/symbols. It returns ordinary plain text containing the implementation body. It is never required to reproduce or preserve the signature, parameters, declaration, schema, JSON, control labels, AST shape, path, framework grammar, or any other mechanically known state; code owns and supplies all of that structure.

If deterministic validation proves one specific defect and its exact mutable byte span, correction continues the SAME persisted generation job with the SAME immutable model route and retained model context. The continuation receives one necessary semantic question and only that exact defective span. It does not receive the previous complete body, surrounding accepted source, a preservation instruction, or framework information about how the result will be applied. It returns ordinary replacement text for that span. Code verifies the persisted base digest and range, performs the exact splice into its retained body, reassembles the code-owned declaration, and reruns the original validators. Every accepted byte outside the span and every unrelated job remain untouched and runnable. There is no repair-guidance model, repair-executor model, replacement response format, alternate work kind, restart, or model swap.

Dependency order does not grant model context. Every model-visible declaration must be named in a separate explicit capability allowlist, must be a direct dependency, and must be projected at symbol level rather than through an aggregate domain API. Transitive dependencies are invisible. Capability, current-declaration, initial-body, correction-span, and total-input budgets are hard failures at the final model-call boundary. A compiler or test failure can authorize inference only after code maps it to one exact owning mutable span and one path-free semantic question; the raw diagnostic and test source are never correction context.

No coding, repair, test-generation, or semantic-review LLM may receive or choose:

* A file name, path, tree, workspace snapshot, project plan, queue, phase, or job graph.
* Whole-file or whole-project responsibility.
* Which block runs next, what depends on what, which failure to repair, or whether work is complete.
* Memories, prior projects, broad conversation history, or unrelated requirements.

Documents are constructed in memory from parser-validated blocks. Code waits until every required dependency block exists before complete-graph checks and isolated compiler/test staging. A failure is mapped by code to the smallest responsible block; accepted blocks remain accepted and only that block is corrected. The loop must never restart the project or ask a model to re-plan it.

If code can parse, derive, validate, route, format, or decide something, code must do it. AI is not the default solution.

Code owns the cognition loop. It restores typed state, evaluates completion, resolves prerequisites, acquires deterministically available evidence, grounds operation inputs, selects supporting evidence, executes transitions, and repeats. A model call is illegal unless code has exhausted registered deterministic work and persisted one precisely named semantic uncertainty it cannot resolve. That call may return only the station-specific typed leaf needed to cross that uncertainty. Models never call tools and may not request deterministic machinery. Deterministic machinery runs whenever code-owned authoritative state requires it; inference receives only the unresolved semantic remainder after deterministic closure. Tool and adapter selection, invocation, arguments, ordering, retries, and result validation are code control flow; tool catalogs and tool-call schemas are never model context. A model must never choose an environment operation, construct its arguments, cite its execution evidence, predict its effect, manage the Task Ledger or Working Set, invent an obligation graph while acting, or declare completion. There is no universal cognition-decision or tool-calling fallback. The normative boundary is [docs/CHARMELEON_COGNITION_RESOLUTION.md](docs/CHARMELEON_COGNITION_RESOLUTION.md).

No Ceremonial Model Calls

Never invoke a model merely to approve, reject, critique, review, or restate a candidate when code has no concrete unresolved semantic question. A proposal must not flow through a mandatory “accept or replace” gate. Code advances after deterministic validation unless it has persisted a specific necessary uncertainty that cannot be resolved in code.

Every model call must have all of the following before dispatch:

* one named unresolved semantic fact, relation, alternative, or artifact value;
* the exact authority and current values needed to resolve only that uncertainty;
* a code-owned rule describing how the returned semantic leaf can affect retained state; and
* no deterministic answer already available to code.

Models return semantic content only. They must not return a framework control plane such as `accept`, `reject`, `repair`, `replace`, `retry`, `search`, `apply`, `plan`, or completion status. A station that needs a corrected semantic value returns only that complete value. A station that needs to choose among code-enumerated alternatives returns only the selected opaque candidate ID. Code determines whether a returned value creates a delta, which exact retained leaf it binds to, whether a splice is legal, and whether the workflow continues.

The provider response is ordinary plain text. Typed values exist only after code parses
and maps that text. Model qualification MUST NOT depend on reproducing or preserving a
schema, JSON object, internal enum, signature, parameter, declaration, path, AST shape,
framework grammar, or any other value code already owns. The sole closed-choice output
convention is one call-local opaque ID or letter when two or more choices genuinely
remain.

Closed-choice cardinality is literal and code-owned. Code materializes the complete applicable option set before considering inference:

* zero options follows that station's explicit zero-option behavior;
* one option is used immediately by code with zero model-resolution and zero model-execution calls; it is not rejected and is not an error; and
* two or more options may create one bounded semantic choice call whose response is only the selected opaque ID or letter. Code maps that ID to the already-known value.

A one-option set must never be rendered for model selection. Strict validation must not turn a deterministically resolved sole option into a new failure mode.

When one uncertainty requires selecting any applicable subset from a code-known finite
set, code reduces it through repeated single-choice rounds. Each round contains only
the still-unselected candidates plus one semantic no-additional-choice alternative.
After a candidate is selected, code persists it and removes it before rendering the
next round, so duplicate selection is impossible. Opaque letters are round-local and
are remapped to the remaining code-owned values each time. An initially sole candidate
is consumed without inference; a sole candidate left after earlier selections is still
compared with the no-additional-choice alternative. The model never returns a list of
IDs or reproduces the accepted set.

If code compares a returned candidate with the exact retained value and the bytes are identical, there is no mutation to perform. Code records the zero delta and continues deterministically; it must not create a retry prompt, response-correction job, reviewer history, or terminal failure from a model-authored action label. A further model call is legal only if a separate, still-unresolved semantic question has been persisted.

Do not fake natural-language understanding with keyword lists, regex phrase routing, or checks for one expected wording. Human phrasing is variable and semantic interpretation is one of the narrow jobs that legitimately requires a model. Split interpretation into fixed tiny stations: bounded repository-fact questions when context is needed, one bounded untrusted requirement inventory, one-candidate authorization and classification, bounded compound-candidate partitioning, exact product-context extraction and surface or deployment semantics only at their first downstream consumer, opaque artifact handling, pairwise direct capability relation, bounded learned-skill selection, and one-need procedure synthesis. No semantic station pre-counts the inventory, and no pre-count receipt exists. Candidate-level cardinality remains one local candidate question and may only permit bounded partitioning. No station may emit an expanded software contract. Every station remains blind to documents, paths, workers, and orchestration. A capability-relation station sees exactly two local needs and, when inference is required, receives one opaque letter that code maps to one registered direction; code owns the resulting graph and compiler-enforced per-feature projection. Code validates every small output and deterministically maps the result to one registered technical adapter. Invalid, contradictory, or unsupported semantic output fails loudly.

Semantic correction must preserve the decoded candidate in code. It is legal only after code has established one exact, grounded semantic defect that deterministic machinery cannot correct. The model receives that defect, the exact current leaf, and the minimum authority required to return only one complete replacement value. Code performs the exact one-leaf splice and preserves all accepted state. Never ask a model to reconstruct already accepted semantic fields, emit a repair plan, or decide whether the workflow advances.

Execution transport is explicit state, never inferred from wording. The coding pipeline and Scrum Play enter the coding assembly line directly. Chat, assistant, and story transports always enter semantic intent interpretation; code must not inspect greetings, verbs, nouns, token overlap, or English sentence prefixes to reroute or rewrite them. Semantic outputs may be structurally validated, but code must not reconstruct meaning from phrases in the user request or model response.

An actionable free-form turn has one cohesive objective. If the semantic interpreter invents multiple action/advice objectives, reject that hierarchy at the boundary and correct the small intent artifact. Once the typed objective requires workspace mutation and command verification, code owns the coordinator plan, post-build analysis, response summary, evidence verification, and memory suppression. Do not call planner, analysis, response, verifier, or memory models to restate a deterministic coding result.

There is one free-form front door. Ordinary CLI chat text is passed unchanged to semantic interpretation. Shell, browser, screen, media, and audio operations require explicit typed commands. Do not add a keyword-routed research or local-automation sidecar, a freshness/version ledger, or a failure fallback around the authoritative runtime.

Free-form text must never select a UI or API mode. Project Planning and Scrum Coach modes are explicit transport fields. Memory categories come only from hard-typed memory kind and explicit structured tags; code must never scan memory content for technology names, preferences, errors, or other semantic phrases. Typed message roles own presentation, so assistant text must not be reclassified or hidden because it resembles a tool payload.

User feedback, interruption, and replanning update the same authoritative job. A repository result with a different job ID is an invariant failure, never a signal to create or follow a successor job.

Per-job model routing is immutable. Concurrent workers must resolve routing into job-local state and must never mutate shared service routing before attempting to restore it.

Every exposed CLI or API control must have one authoritative runtime consumer and a test proving its effect. Write-only metadata is forbidden. The removed profile, planning-pass, persistent-execution, review, missing-tool, generic reasoning, autonomy, approval, verification, web, workspace, and external-agent toggles must not return under new names; old top-level metadata using them fails explicitly. Keep only typed settings that actually alter execution, such as model routing and the consumed explicit research query.

Whole-file generation, model-owned execution planning, model-owned repair routing, and
path-bearing coding/repair/test prompts are forbidden regressions and require
source-level absence tests. The sole exception is the typed target-tree declaration
station defined in [docs/TARGET_TREE_PLANNING.md](docs/TARGET_TREE_PLANNING.md): it may
see the code-built current managed tree and return only one complete raw hierarchy of
directory and file basenames. Code parses that hierarchy and constructs the normalized
relative file paths; the model never emits a path or a flat path list.
It cannot return artifact metadata, filesystem actions, commands, source, declaration
contracts, a work queue, or completion. Code derives parent-directory work and every
create/reconcile/delete transition. Omission has no deletion authority unless code
separately proves that exact current managed file is eligible. There is no file-content
station. The selected stack
compiler turns the accepted tree and code-owned coverage into bounded source-block
responsibilities. Each source call remains path-blind and returns only one exact
ordinary implementation body; code supplies the declaration, parses and validates the
assembled node, stitches it, and verifies the complete documents.

Artifact support is adapter-based, never language-hard-coded into the tree or
model prompt. Code selects the registered stack from authoritative project
facts and supplies that exact stack only as technical tree context. A code-constructed
path is then recognized, parsed, scoped, validated, repaired, and verified by
its code-owned artifact adapter. New PHP, Java, NGINX, Dockerfile, Blade, CSS,
JSON, YAML, or other support is added as a focused adapter with explicit
capability/verification limits; do not create a universal coder prompt.
The normative capability contract is
[docs/ARTIFACT_ADAPTERS.md](docs/ARTIFACT_ADAPTERS.md).

Product-Specific Workloads Are Not Framework Code

The application named in an autonomy benchmark is a held-out workload. It must not have a product-specific adapter, blueprint, source template, component contract, test suite, repair directive, prompt branch, or domain enum in the framework under test.

For example, a music-studio benchmark may not be implemented by registering an audio-workstation adapter containing its sequencer, transport, instruments, state, UI, styles, or tests. That tests whether Codex encoded a music app, not whether Omnidex can build one.

Adapters may describe reusable technical mechanics such as a language parser, package manager, browser runtime, persistence primitive, AST form, or test runner. They may not describe the benchmark product. Product behavior must be derived from the current user request into a domain-neutral typed representation and constructed through the same generic machinery used for an unseen application.

Autonomy Benchmark Boundary

An autonomy benchmark gives the framework exactly the ordinary request a user would submit. No benchmark rubric, expected feature list, target structure, reference implementation, private test source, or app-specific correction text may enter semantic or generation context.

The evaluation plan must remain unloaded until the build attempt has stopped. A separate observer then evaluates the resulting workspace through typed, black-box checks of user-visible behavior. Evaluation must allow any valid implementation; private class names, wrapper elements, array literals, callback syntax, or other implementation choices are not acceptance criteria unless the user's request explicitly requires them.

A failed build is still evaluated. Report the weighted capabilities that work, the capabilities that do not, the stopping failure, model calls, context volume, accepted work, corrections, verification runs, elapsed time, and human interventions. Never collapse partial progress into either fake success or an uninformative binary failure.

Framework changes may be made between complete benchmark runs to address reusable failure classes. Do not steer a running benchmark, write a product-specific prompt after observing its failure, or tune an exact implementation into acceptance. If a benchmark-specific source template, contract, test, or repair instruction is required for a pass, the run is invalid.

4. Server-Authoritative State

The server owns the truth.

The frontend reflects server state.
The frontend does not invent application state that conflicts with the server.

Use:

* PostgreSQL for server-authoritative state during the current running build.
* Redis for temporary/cache/session/realtime coordination state.
* Server-side singletons where appropriate for application-level orchestration.
* Middleware for request lifecycle behavior.
* `database/setup.sql` as the sole authoritative definition of Omnidex's internal database schema.
* Components for UI rendering.
* RecyclrJS for realtime/event bridge behavior.
* Stimulus for interaction wiring only.

The browser should not be treated as the source of truth.

⸻

5. JavaScript Is Not for Components

JavaScript must not render application components.

JavaScript may only be used for:

* User interactions.
* Event bridges.
* Small DOM coordination.
* Stimulus controllers.
* RecyclrJS connection/event handling.
* Client-side affordances such as spinners, transitions, and interaction feedback.

Do not build HTML templates in JavaScript.

Do not move server-rendered UI into JavaScript.

Do not create client-side component systems.

Bad:

container.innerHTML = `
  <div class="card">
    <h2>${title}</h2>
  </div>
`;

Good:

this.element.requestSubmit();

The server renders components.
The client coordinates interaction.

⸻

6. Use RecyclrJS

Use RecyclrJS for realtime updates, browser coordination, server-pushed refreshes, and cross-tab/page bridge behavior.

RecyclrJS should be initialized as a singleton on the <body> tag with full-page scope.

There should not be many scattered RecyclrJS instances across individual components unless specifically justified.

Expected pattern:

<body
    data-controller="recyclr"
    data-recyclr-scope-value="page"
>

Use RecyclrJS as the bridge between server-side state changes and frontend updates.

The server decides what changed.
RecyclrJS communicates that change.
Server-rendered components reflect the new state.

Exception: Scrum card modals are React + TypeScript SPA surfaces mounted through the `card-modal-spa` Stimulus controller. Card modal updates must use typed JSON/server state and must not add Recyclr HTML bundle fallbacks.

⸻

7. Use Stimulus Correctly

Stimulus is for interaction controllers, not application rendering.

Use Stimulus for:

* Button behavior.
* Form submission coordination.
* Loading states.
* Spinners.
* Toggling visible UI state.
* Dispatching events.
* Bridging to RecyclrJS.
* Small page-level interaction logic.

Do not use Stimulus as a component framework.

Stimulus controllers should stay small.

⸻

8. Use Server-Side Components

All reusable UI should be built as server-side components.

Components should:

* Have clear inputs.
* Avoid hidden database access unless explicitly designed for it.
* Avoid large conditional blobs.
* Be testable.
* Use Tailwind CSS.
* Use the project UI/component library when available.

Do not duplicate similar markup across pages.

Create or reuse components instead.

⸻

9. Use the UI Library / Component Library

Before creating new UI, check for existing components.

Prefer extending the shared UI/component library over creating one-off markup.

New UI should be:

* Consistent.
* Minimal.
* Accessible.
* Responsive.
* Built with Tailwind CSS.
* Rendered server-side.
* Easy to reuse.

⸻

10. Use Traits for Shared Behavior

Use traits for reusable behavior when inheritance would create bad coupling.

Good trait candidates:

* Logging helpers.
* State transition helpers.
* Authorization helpers.
* Query filtering helpers.
* Recyclr event publishing helpers.
* Validation normalization helpers.
* Shared component behavior.

Do not use traits as junk drawers.

A trait must have a focused reason to exist.

⸻

11. Use Middleware

Use middleware for request lifecycle concerns.

Good middleware candidates:

* Tenant resolution.
* Auth context.
* Permission boundaries.
* Request logging.
* Feature flags.
* Locale/language selection.
* Server-side state hydration.
* Rate limiting.
* Recyclr/session binding.

Do not duplicate lifecycle checks in controllers.

⸻

12. Use One Fresh Omnidex Database Setup

`database/setup.sql` is the sole authoritative definition of Omnidex's internal
PostgreSQL schema. Every build carries that one current setup file, and every
Omnidex service startup drops and recreates the configured dedicated schema from
it. Any previous internal schema and rows are intentionally discarded.

Do not add internal migration directories, numbered migration files, migration
ledgers, manifests, digests, hashes, reversible upgrades, or in-place upgrade
paths. Do not manually patch the internal schema or hide schema requirements
inside runtime code. Change `database/setup.sql` directly and update the tests
that prove a fresh setup.

Generated workloads are a separate database boundary. A registered artifact
adapter may generate workload-owned migrations when the workload contract and
selected stack require them. Those workload artifacts must never become an
Omnidex internal migration mechanism.

⸻

State and Data Rules

13. PostgreSQL for Current Server-Authoritative State

Use PostgreSQL as the authoritative store while the current Omnidex service is
running. Omnidex makes no promise that its internal rows survive a process or
service startup: startup recreates the dedicated schema from
`database/setup.sql` and begins empty.

Schema should be explicit.

Prefer:

* Proper columns.
* Foreign keys where appropriate.
* Indexes for lookup paths.
* Constraints for invalid state.
* Enums or hard-typed values for known states.

Do not store important queryable state only in JSON unless there is a clear reason.

⸻

14. Redis for Ephemeral State

Use Redis for temporary, realtime, cached, or coordination state.

Good Redis use cases:

* Session-like temporary state.
* Short-lived locks.
* Recyclr pub/sub.
* Realtime fanout.
* Cached server-side state.
* Progress indicators.
* Job status.
* UI operation status.

Redis should not become the server-authoritative source of truth unless explicitly designed that way.

⸻

15. Hard-Typed Values

Avoid stringly-typed behavior.

Use hard-typed values wherever possible:

* Enums.
* Value objects.
* Constants.
* DTOs.
* Form requests.
* Typed properties.
* Typed method signatures.
* Explicit validation rules.

Bad:

$status = 'done';

Good:

$status = JobStatus::Done;

Invalid states should be impossible or loudly rejected.

⸻

16. Minimal Queries

Queries should be intentional and minimal.

Do not fetch massive datasets “just in case.”

Do not dump full tables into hidden elements.

Do not load every record and filter in memory when the database can filter.

Prefer:

* Pagination.
* Cursor pagination for large datasets.
* Explicit selected columns.
* Server-side filtering.
* Server-side sorting.
* Proper indexes.
* Small response payloads.

No hidden massive DOM dumps.

No invisible preload blobs full of records.

No “we’ll just hide it on the frontend.”

⸻

17. Always Paginate Lists

Any list that can grow must be paginated.

This includes:

* Tables.
* Cards.
* Logs.
* Events.
* Users.
* Jobs.
* Messages.
* Search results.
* Audit history.
* Activity feeds.

If the dataset can grow, pagination is required.

⸻

UX Rules

18. Every Interaction Needs Feedback

Every user-triggered interaction must communicate that work is happening.

Use:

* Spinner.
* Loading text.
* Disabled button state.
* Progress state when available.
* Animated indicator showing the UI is not stalled.
* Clear success/failure response.

No dead-click behavior.

No silent waiting.

No ambiguous stalled UI.

If the server is working, the user should know.

⸻

19. Animated Working State

Loading indicators should be visibly alive.

Examples:

* Spinner.
* Pulsing dot.
* Progress shimmer.
* Updating status text.
* Recyclr-powered progress updates.
* “Still working…” state for longer operations.

The UI should never look frozen while work is happening.

⸻

20. Server-Reconciled UI

After mutations, the UI should reconcile with the server.

Preferred flow:

1. User triggers interaction.
2. UI shows loading state.
3. Server validates and performs mutation.
4. Server updates authoritative/ephemeral state.
5. Server returns updated component or emits Recyclr event.
6. UI reflects server-confirmed state.
7. UI clears loading state.
8. Failures show explicit error state.

Do not optimistically fake server-authoritative state unless specifically approved.

⸻

Testing Rules

21. TDD Required

Use test-driven development.

Before or during implementation:

* Add tests for the intended behavior.
* Add tests for forbidden behavior.
* Add tests for failure paths.
* Add regression tests for the bug being fixed.

Do not claim done without tests.

The tests should prove:

* The new path works.
* The old forbidden path is gone.
* Invalid state fails loudly.
* No fallback path is being used.
* Server state remains authoritative.

⸻

22. Test the Absence of Fallbacks

When removing an old system, add tests proving the old system is not used.

Examples:

* Old route returns 404 or explicit failure.
* Old method throws.
* Old JavaScript renderer no longer exists.
* Old config key is rejected.
* Old DOM hook is absent.
* Old code path is unreachable.

A fallback that keeps tests green is not success.
It is disguised failure.

⸻

Logging Rules

23. Verbose Logging

Use verbose, useful logging around important behavior.

Log:

* State transitions.
* Failed validation.
* Rejected invalid states.
* Recyclr events.
* Background jobs.
* Middleware decisions.
* Server-side mutations.
* External API calls.
* Unexpected missing data.
* Permission failures.

Logs should include enough context to debug without dumping sensitive data.

Bad:

Log::info('Failed');

Good:

Log::warning('Task transition rejected', [
    'task_id' => $task->id,
    'from' => $task->status->value,
    'to' => $requestedStatus->value,
    'reason' => 'Invalid transition',
]);

⸻

24. No Fake Success Logs

Do not log success unless the operation actually succeeded.

Do not swallow an exception and log “completed.”

Do not report done while fallback behavior is running.

⸻

Implementation Rules

25. Remove Dead Code

When replacing behavior:

* Delete old code.
* Delete old templates.
* Delete old JavaScript renderers.
* Delete obsolete tests.
* Delete unused routes.
* Delete obsolete config.
* Delete stale comments.

Do not leave old implementations around as archaeological traps.

Dead code is not harmless.
It confuses future agents and developers.

⸻

26. Search Before Claiming Completion

Before saying a task is complete, search for:

* Old function names.
* Old component names.
* Old routes.
* Old DOM hooks.
* Old config keys.
* Old JavaScript render paths.
* Duplicate implementations.
* TODOs added during the task.

Completion requires proving the old path is gone.

⸻

27. Do Not Invent Architecture

Do not introduce new frameworks, state systems, routers, frontend component systems, queues, or libraries unless explicitly requested.

Prefer existing project patterns:

* Server-side components.
* Middleware.
* Traits.
* `database/setup.sql` for Omnidex's internal schema.
* Workload-owned migrations only through registered workload artifact adapters.
* PostgreSQL.
* Redis.
* Tailwind CSS.
* Stimulus JS.
* RecyclrJS.
* TDD.

⸻

28. Keep Controllers Thin

Controllers should coordinate request handling.

Controllers should not contain large business logic.

Prefer moving logic into:

* Actions.
* Services.
* Form requests.
* DTOs.
* Policies.
* Middleware.
* Traits.
* Components.
* Jobs.

⸻

29. Explicit Boundaries

Separate responsibilities clearly.

Do not mix:

* Rendering and persistence.
* Validation and transport.
* JavaScript interaction and server rendering.
* Server-authoritative database state and ephemeral state.
* Query building and UI formatting.
* Authorization and business logic.

If two responsibilities are fighting for space in one file, split them.

⸻

Frontend Rules

30. No Hidden Massive DOM State

Do not store large datasets in hidden inputs, hidden divs, data attributes, script tags, or invisible HTML.

Bad:

<div class="hidden" data-users="...5000 users..."></div>

Good:

$users = User::query()
    ->select(['id', 'name', 'email'])
    ->paginate(25);

⸻

31. Tailwind CSS

Use Tailwind CSS for styling.

Prefer existing utility patterns and component classes.

Do not create random CSS files unless needed.

Do not use inline styles unless there is a specific reason.

⸻

32. Accessibility

Interactive UI should be accessible.

Use:

* Real buttons for actions.
* Labels for inputs.
* ARIA only when appropriate.
* Keyboard-friendly interactions.
* Visible focus states.
* Clear loading and error text.

Do not create div-button nonsense.

⸻

Agent Completion Checklist

Before reporting completion, verify:

* No fallback implementation was added.
* Old replaced paths were removed.
* Server is authoritative.
* JavaScript does not render components.
* Stimulus is only used for interactions/bridges.
* RecyclrJS is used appropriately for realtime/page bridge behavior.
* RecyclrJS singleton remains body-scoped for full-page behavior.
* PostgreSQL is authoritative for the current running service, without a cross-start preservation claim.
* Redis is used only for appropriate ephemeral/realtime state.
* `database/setup.sql` is the sole Omnidex internal schema source and a fresh startup discards previous internal state.
* Internal migrations, manifests, hashes, and in-place upgrade paths were not added.
* Any generated workload migration remains workload-owned and adapter-produced.
* Middleware is used for lifecycle concerns.
* Traits are used for focused shared behavior where appropriate.
* Components are server-rendered.
* UI uses the shared component library where possible.
* Tailwind CSS is used for styling.
* Files remain small and focused.
* Values are hard-typed.
* Queries are minimal.
* Lists are paginated.
* There are no hidden massive DOM dumps.
* Every interaction has loading feedback.
* Loading states are visibly animated.
* Logging is useful and verbose.
* Tests cover success, failure, and forbidden fallback behavior.
* Searches were performed to ensure old paths are gone.

⸻

Final Standard

Do not optimize for making the diff look easy.

Optimize for making the system correct, explicit, testable, maintainable, and impossible to accidentally route through old behavior.

When in doubt:

* Delete the old path.
* Make invalid state fail loudly.
* Keep the server authoritative.
* Keep files small.
* Test the forbidden behavior.
* Do not fake completion.
