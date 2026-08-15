AGENTS.md

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

When Omnidex needs a reusable ability that is not already available, it must be able to synthesize a narrowly scoped skill through bounded model jobs, validate that skill with code, and persist the accepted skill in PostgreSQL. Later jobs may retrieve the smallest relevant accepted skills from that registry. Natural-language skill matching is a narrow semantic-model job; it must not be replaced with keyword or phrase heuristics.

Code owns skill identity, schemas, version numbers, lifecycle state, tool permissions, dependency edges, validation, test evidence, activation, retrieval limits, and database writes. Models may propose only the small semantic or instructional fields that code cannot derive. A proposed skill is unavailable until its schema, boundaries, and executable checks pass. Rejected skill candidates remain rejected; there is no silent fallback to a checked-in domain skill or general-purpose agent.

The repository may contain only the task-neutral bootstrap machinery needed to create, validate, store, retrieve, and execute skills. A workload-specific skill must be data with provenance in the database, not a new Omnidex code path. Generated application source remains in the target workspace; reusable worker procedure belongs in the skill registry; neither belongs in Omnidex's static architecture.

Code must own:

* Generic language, runtime, and toolchain adapter selection.
* Document structure, paths, names, declarations, signatures, and block boundaries.
* Dependency graphs, scheduling, retries, diagnostic routing, and completion state.
* Parser and compiler context extraction.
* Imports, stitching, formatting, complete-graph dependency checks, isolated compiler/test staging, workspace writes, and final test execution.

A coding LLM may only fill one explicitly defined code block when deterministic code cannot provide that block. Initial generation receives only the language, exact signature, local behavioral contract, and strictly required declarations/symbols. Validation repair is split across two separate envelopes. A repair-guidance LLM receives the signature, required declarations/symbols, exact mutable block, compiler-proven lexical scope when available, and one exact path-free failure; it returns one self-contained imperative repair instruction and no replacement source. A repair-executor LLM then receives only that instruction and the exact mutable block; it returns exactly one parseable code node. The executor never receives the raw failure, capability inventory, scope inventory, or superseded initial behavioral narrative.

Dependency order does not grant model context. Every model-visible declaration must be named in a separate explicit capability allowlist, must be a direct dependency, and must be projected at symbol level rather than through an aggregate domain API. Transitive dependencies are invisible. Capability, current-declaration, repair-guidance, repair-execution, initial-envelope, and total-envelope budgets are hard failures at the final model-call boundary. A compiler or test failure contributes one bounded sanitized diagnostic only to the repair-guidance station; test source is never model context. Guidance has no mutation authority and becomes useful only when ordinary code validates the executor's returned block against the original signature and reruns the exact compiler/test stage.

No coding LLM may receive or choose:

* A file name, path, tree, workspace snapshot, project plan, queue, phase, or job graph.
* Whole-file or whole-project responsibility.
* Which block runs next, what depends on what, which failure to repair, or whether work is complete.
* Memories, prior projects, broad conversation history, or unrelated requirements.

Documents are constructed in memory from parser-validated blocks. Code waits until every required dependency block exists before complete-graph checks and isolated compiler/test staging. A failure is mapped by code to the smallest responsible block; accepted blocks remain accepted and only that block is corrected. The loop must never restart the project or ask a model to re-plan it.

If code can parse, derive, validate, route, format, or decide something, code must do it. AI is not the default solution.

Code owns the cognition loop. It restores typed state, evaluates completion, resolves prerequisites, acquires deterministically available evidence, grounds operation inputs, selects supporting evidence, executes transitions, and repeats. A model call is illegal unless code has exhausted registered deterministic work and persisted one precisely named semantic uncertainty it cannot resolve. That call may return only the station-specific typed leaf needed to cross that uncertainty. Models never call tools and may not request deterministic machinery. Deterministic machinery runs whenever code-owned authoritative state requires it; inference receives only the unresolved semantic remainder after deterministic closure. Tool and adapter selection, invocation, arguments, ordering, retries, and result validation are code control flow; tool catalogs and tool-call schemas are never model context. A model must never choose an environment operation, construct its arguments, cite its execution evidence, predict its effect, manage the Task Ledger or Working Set, invent an obligation graph while acting, or declare completion. There is no universal cognition-decision or tool-calling fallback. The normative boundary is [docs/CHARMELEON_COGNITION_RESOLUTION.md](docs/CHARMELEON_COGNITION_RESOLUTION.md).

Do not fake natural-language understanding with keyword lists, regex phrase routing, or checks for one expected wording. Human phrasing is variable and semantic interpretation is one of the narrow jobs that legitimately requires a model. Split interpretation into fixed tiny stations: surface classification, exact product-context extraction, exact requirement extraction and fixed-point splitting, opaque artifact handling, pairwise direct capability relation, bounded learned-skill selection, and one-need procedure synthesis. No station may emit an expanded software contract. Every station remains blind to documents, paths, workers, and orchestration. A capability-relation station sees exactly two local needs and returns only one registered direction; code owns the resulting graph and compiler-enforced per-feature projection. Code validates every small output and deterministically maps the result to one registered technical adapter. Invalid, contradictory, or unsupported semantic output fails loudly.

Semantic correction must preserve the decoded candidate in code. A correction model receives the exact validation failure and a schema permitting exactly one top-level field; its one-field merge patch must alter exactly one JSON leaf in retained state. Repeating the full response, changing an unrelated label, changing multiple nested values, or returning a no-op is rejected. Never ask a model to reconstruct already accepted semantic fields during correction.

Execution transport is explicit state, never inferred from wording. The coding pipeline and Scrum Play enter the coding assembly line directly. Chat, assistant, and story transports always enter semantic intent interpretation; code must not inspect greetings, verbs, nouns, token overlap, or English sentence prefixes to reroute or rewrite them. Semantic outputs may be structurally validated, but code must not reconstruct meaning from phrases in the user request or model response.

An actionable free-form turn has one cohesive objective. If the semantic interpreter invents multiple action/advice objectives, reject that hierarchy at the boundary and correct the small intent artifact. Once the typed objective requires workspace mutation and command verification, code owns the coordinator plan, post-build analysis, response summary, evidence verification, and memory suppression. Do not call planner, analysis, response, verifier, or memory models to restate a deterministic coding result.

There is one free-form front door. Ordinary CLI chat text is passed unchanged to semantic interpretation. Shell, browser, screen, media, and audio operations require explicit typed commands. Do not add a keyword-routed research or local-automation sidecar, a freshness/version ledger, or a failure fallback around the authoritative runtime.

Free-form text must never select a UI or API mode. Project Planning and Scrum Coach modes are explicit transport fields. Memory categories come only from hard-typed memory kind and explicit structured tags; code must never scan memory content for technology names, preferences, errors, or other semantic phrases. Typed message roles own presentation, so assistant text must not be reclassified or hidden because it resembles a tool payload.

User feedback, interruption, and replanning update the same authoritative job. A repository result with a different job ID is an invariant failure, never a signal to create or follow a successor job.

Per-job model routing is immutable. Concurrent workers must resolve routing into job-local state and must never mutate shared service routing before attempting to restore it.

Every exposed CLI or API control must have one authoritative runtime consumer and a test proving its effect. Write-only metadata is forbidden. The removed profile, planning-pass, persistent-execution, review, missing-tool, generic reasoning, autonomy, approval, verification, web, workspace, and external-agent toggles must not return under new names; old top-level metadata using them fails explicitly. Keep only typed settings that actually alter execution, such as model routing and the consumed explicit research query.

Whole-file generation, model-owned planning, model-owned repair routing, and path-bearing prompts are forbidden regressions and require source-level absence tests.

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

* PostgreSQL for durable state.
* Redis for temporary/cache/session/realtime coordination state.
* Server-side singletons where appropriate for application-level orchestration.
* Middleware for request lifecycle behavior.
* Migrations for schema changes.
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

12. Use Migrations

Any database schema change must use a migration.

Do not assume columns exist.

Do not manually patch schema.

Do not hide schema requirements inside runtime code.

Migrations should be reversible when practical.

⸻

State and Data Rules

13. PostgreSQL for Durable State

Use PostgreSQL for durable application state.

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

Redis should not become the durable source of truth unless explicitly designed that way.

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
4. Server updates durable/ephemeral state.
5. Server returns updated component or emits Recyclr event.
6. UI reflects server-confirmed state.
7. UI clears loading state.
8. Failures show explicit error state.

Do not optimistically fake durable state unless specifically approved.

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
* Migrations.
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
* Durable state and ephemeral state.
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
* PostgreSQL is used for durable state.
* Redis is used only for appropriate ephemeral/realtime state.
* Migrations exist for schema changes.
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
