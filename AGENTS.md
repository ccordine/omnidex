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
