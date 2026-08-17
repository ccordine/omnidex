The strongest version for Omnidex is not a collection of language modes, specialist agents, or model-owned toolchains. It is a capability-typed objective machine:

A project exposes a heterogeneous set of capabilities.
A user request becomes code-held objectives.
Each objective resolves the exact capabilities and evidence it needs.
Code drives the work deterministically.
Models fill only named semantic gaps or generate bounded source leaves.
Code proves completion.

That preserves everything that worked in Charmander while allowing a single project to freely cross PHP, Laravel, Blade, HTML, Stimulus, Tailwind, Java, Docker, NGINX, Go, TypeScript, browser testing, web research, databases, and whatever comes next.

The architecture

                    USER REQUEST
                         │
                         ▼
                  PROJECT CONTRACT
          observed facts + user authority
                         │
                         ▼
                 SEMANTIC INTAKE
             one bounded transformation
                         │
                         ▼
              CODE-HELD REQUIREMENTS
                         │
                         ▼
               OBJECTIVE COMPILER
                         │
                         ▼
              OBJECTIVE / WORK GRAPH
                         │
                         ▼
                CAPABILITY RESOLVER
                         │
       ┌─────────────────┼──────────────────┐
       ▼                 ▼                  ▼
 Repository facts   Specialist workflows   Charmander
 search/parsing      browser/web/DB/etc.   generation
       │                 │                  │
       └─────────────────┼──────────────────┘
                         ▼
              DETERMINISTIC CLOSURE
                         │
                genuine uncertainty?
                  ┌──────┴──────┐
                  │             │
                 no            yes
                  │             │
              code acts    tiny LLM station
                  │             │
                  └──────┬──────┘
                         ▼
                 CODE VALIDATES
                         ▼
                 STAGED CHANGE
                         ▼
           LOCAL + INTEGRATION PROOF
                         ▼
              SEMANTIC REVIEW GAP
                 only if required
                         ▼
                 CODE COMPLETES

The continuing actor is Omnidex. Models are replaceable semantic processors inside it.

1. Project Contract: establish reality once

Before serious work, Omnidex should inspect the project and create a typed Project Contract.

It first determines everything available mechanically:

go.mod                  → Go
composer.json           → PHP / Laravel dependencies
package.json            → JavaScript / TypeScript / Stimulus / Tailwind
pom.xml / build.gradle  → Java
Dockerfile              → Docker
compose.yaml            → Docker Compose service graph
nginx.conf              → NGINX
phpunit.xml             → PHPUnit
vite.config.*           → Vite
tailwind.config.*       → Tailwind

Then it interviews the human only about authority facts that cannot be inferred:

May the development database be destroyed?
Must existing data survive?
May dependencies be upgraded?
Is the framework/version pinned?
Which environments are production-critical?
What is the current top priority?
What actions require explicit approval?

These become durable authoritative facts:

project:
  languages:
    - php
    - javascript
    - java
  frameworks:
    - laravel
    - stimulus
    - tailwind
  infrastructure:
    - docker-compose
    - nginx
authority:
  development_database:
    disposable: true
    preserve_data: false
  dependencies:
    upgrades_allowed: true
  user_controls:
    - priority
    - unrelated_scope_expansion
    - destructive_production_changes
    - framework_replacement

The contract must distinguish:

* Observed fact: derived from the current repository.
* User authority: only the human can establish it.
* Accepted decision: made during an authorized workflow.
* Model proposal: never authoritative by itself.

That prevents another week of Codex preserving data you explicitly said could be deleted.

2. Organize around capabilities, not languages

“PHP project,” “Laravel mode,” or “NGINX agent” are the wrong abstractions.

The repository exposes composable capability packs:

Language capabilities
  php.syntax
  java.syntax
  javascript.syntax
  typescript.syntax
  go.syntax
Framework capabilities
  laravel.routes
  laravel.container
  laravel.models
  laravel.jobs
  stimulus.controller
  react.component
Document/presentation capabilities
  blade.template
  html.dom
  tailwind.utilities
  css.stylesheet
Infrastructure capabilities
  docker.image
  docker.compose
  nginx.proxy
  postgres.schema
Verification capabilities
  phpunit
  pest
  junit
  playwright/chromium
  nginx.test
  docker.compose.config
  http.probe

A project may contain all of them. A particular objective activates only the subset it actually needs.

Capability-pack contract

Every capability pack should provide a small fixed contract:

Responsibility	Example
Detect	Identify whether the capability applies
Observe	Derive exact facts from artifacts/environment
Operations	Registered deterministic actions
Mutation ownership	Which artifact regions it may modify
Verification	How claims are proved
Failure reduction	Convert failures into typed facts
Context rendering	Material needed by a bounded semantic job
Dependencies	Other capability packs required

Conceptually:

type CapabilityDescriptor struct {
    ID           CapabilityID
    Version      string
    Detect       []Detector
    Provides     []FactKind
    Operations   []Operation
    ArtifactKinds []ArtifactKind
    Requires     []CapabilityID
    Verifiers    []VerifierID
    Reducers     []FailureReducerID
}

No model selects these capabilities. Code resolves them from project facts, the active objective, and the artifact being handled.

3. Layer capabilities onto artifacts

One file may require multiple capabilities.

For example:

resources/views/patients/index.blade.php
Primary artifact grammar:
    Blade template
Semantic overlays:
    PHP expressions
    HTML DOM
    Stimulus attributes/actions
    Tailwind utility classes

That does not mean four agents edit the file.

Use a strict ownership rule:

Multiple capability layers may observe and constrain an artifact, but one code-owned mutation plan owns each node or source region.

So:

* Blade/HTML machinery owns structural template mutation.
* Stimulus capability annotates data-controller, targets, values, and actions.
* Tailwind capability constrains class usage and build availability.
* PHP capability parses embedded PHP expressions.
* Code combines all constraints into one staged artifact.

That prevents competing specialists from independently rewriting the same file.

4. Compile vague requests into code-held objectives

The first model call may perform one coherent semantic transformation over the intact user request:

Build an appointment-search page using the existing Laravel application,
with dynamic filters and a modal for appointment details.

Output:

{
  "objective": "Add appointment search and detail viewing",
  "requirements": [
    "Users can search appointments",
    "Users can dynamically filter the visible results",
    "Users can open appointment details in a modal"
  ],
  "constraints": [
    "Use the existing Laravel application"
  ]
}

Code validates grounding, bounds, duplicates, and contradictions. It assigns identities and creates authoritative requirements.

Then the objective compiler creates a workload:

O1 Understand current appointment architecture
O2 Identify current route/controller/query ownership
O3 Establish exact filter behavior
O4 Implement server-side search
O5 Implement Blade result view
O6 Implement Stimulus filtering/modal behavior
O7 Apply existing Tailwind conventions
O8 Verify backend behavior
O9 Verify browser behavior
O10 Perform semantic consistency review

The initial planner does not need a perfect plan. Work may discover more work later.

Each objective is code-owned and has:

desired predicate
dependencies
acceptance criteria
required facts
status
applicable capability set

5. Resolve a capability closure per objective

Before an objective becomes executable, Omnidex calculates its capability closure.

For a Blade/Stimulus leaf:

Required:
  blade.template
  html.dom
  stimulus.controller
  tailwind.utilities
Verification:
  blade compilation/render
  JavaScript build
  browser interaction proof

For an NGINX change:

Required:
  nginx.proxy
  docker.compose   // only if service discovery depends on Compose
Verification:
  nginx -t
  HTTP probe
  expected upstream response

For a Java leaf:

Required:
  java.syntax
  gradle or maven
  junit
Verification:
  formatter/static checks
  compile
  focused test
  module test

An objective receives an execution certificate only when Omnidex has established:

artifact type recognized
parser/observer available
mutation owner available
staging available
formatter/linter available
focused verifier available
integration verifier available
acceptance criteria mapped to proof obligations

This gives you the strongest realistic capability guarantee:

Omnidex will not claim it can execute an objective unless the required capability closure exists and has passed its own conformance suite.

Unsupported work fails explicitly or creates an authorized “add capability” objective. It does not let an LLM improvise a toolchain.

6. Deterministic closure before inference

Once an objective is active, code runs everything it can derive:

Need source definition
→ index lookup
Need references
→ graph traversal
Need unread artifact
→ read it
Need route list
→ Laravel route inspection
Need syntax structure
→ parser
Need service port
→ Compose graph
Need NGINX owner
→ include/upstream graph
Need tests
→ test ownership lookup
Changed PHP
→ formatter + php -l
Changed NGINX
→ nginx -t
Changed Compose
→ docker compose config

No model decides whether any of that should happen.

A model call is legal only when Omnidex can name the unresolved semantic question:

Which of these two services owns the scheduling policy?
Which interpretation of “dynamic filters” matches the requirement?
Which DOM candidate corresponds to the requested appointment modal?
What source declaration satisfies this exact executable job?

Then the model gets one sufficient packet and returns one bounded result.

7. Specialists are code workflows, not agents

A specialist is a machine for one class of evidence or execution.

For example, a browser specialist owns:

Chromium launch
profile isolation
navigation
timeouts
DOM/accessibility inspection
console/network capture
interactions
screenshots/video
cleanup
acceptance evaluation

The model never uses Chromium.

If the workflow reaches an unresolved configuration question, it may ask:

Goal:
Observe the populated chart.
Observed:
- loading indicator remains after network idle
- websocket remains active
- chart container exists
Candidates:
C1 wait for loading indicator to disappear
C2 wait for chart canvas to appear
C3 fixed 15-second delay
Return one candidate ID.

Then code continues the browser workflow.

The same specialist kernel can support:

Browser
Web retrieval
Database diagnostics
Performance measurement
Network inspection
Container verification
Visual comparison

Each specialist has:

configuration
deterministic executor
observation/evidence output
acceptance evaluator
failure reducer
optional named semantic gaps

No specialist persona and no model-owned tools.

8. Hand source generation to Charmander

Once cognition has established:

target artifact and declaration
specific objective
required behavior
acceptance criteria
applicable constraints
direct dependencies
relevant evidence

it creates an executable Charmander job.

The coding model gets:

WHY
Parent objective and relevant project intent
WHAT
One specific declaration or bounded artifact region
MUST DO
Accepted behavior
DONE WHEN
Code-owned acceptance criteria
MUST RESPECT
Applicable constraints and invariants
KNOW THIS
Only direct source/evidence/dependencies

That packet may be 3K, 8K, or 20K tokens. The goal is minimum sufficient context, not minimum bytes.

A leaf is ready only when a competent developer receiving that packet could independently understand what to implement and how success will be judged.

Then:

model generates one source unit
→ parser validates structure
→ code stages it
→ formatter/linter
→ focused verification
→ broader verification

9. Cross-stack changes use atomic change groups

A real feature may require:

Laravel route/controller
PHP service/query
Blade template
Stimulus controller
Tailwind styling
Java service
Docker Compose
NGINX proxy

Omnidex compiles that into leaves with explicit dependencies:

Java API contract
        ↓
Docker service registration
        ↓
NGINX proxy path
        ↓
Laravel API client
        ↓
Blade/Stimulus UI
        ↓
integration/browser verification

Each leaf uses its own capability closure and tiny coding job.

All leaves belong to one change group:

local verification per leaf
        ↓
stage complete change group
        ↓
cross-stack integration verification
        ↓
semantic acceptance
        ↓
promote atomically

If integration fails, code maps the failure to the smallest owning artifact/capability and opens a bounded correction there. It does not ask one model to reconsider the whole stack.

10. Model routing is station-specific

No single “project model.”

Each station has a measured capability requirement:

semantic intake            → model proven at grounded extraction
job specification          → model proven at bounded elaboration
search terms               → cheap small model
candidate relevance        → cheap small model
source generation          → coder model
strategic architecture     → strong model/frontier pilot
protocol normalization     → disciplined small model

A station manifest can record:

allowed models
minimum benchmark score
context budget
temperature
latency expectation
failure rate

That lets Omnidex use:

27B for difficult intake/specification
9B for bounded generation
4B/9B for tiny classification
ChatGPT/another frontier model for rare strategic review

without changing authority or workflow.

The eventual long-context pilot should see a compiled strategic state, not a filesystem or toolbelt:

top-level intent
objective graph
important architecture facts
accepted decisions
major failures
candidate strategies

It proposes strategy or decomposition. The mech still operates itself.

11. Capability conformance is how you guarantee support

Every capability pack needs an executable conformance suite before it becomes active.

For example, php.syntax must prove:

detect PHP artifact
parse declarations
locate syntax error
apply bounded replacement
format/lint
run focused test
localize failure

laravel.routes must prove:

discover route
resolve controller
understand middleware
add/update route safely
run feature test

blade + stimulus + tailwind must prove:

render template
bind controller/actions/targets
compile frontend
interact through browser
prove visible behavior

docker.compose must prove:

parse service graph
derive networks/ports/dependencies
stage change
docker compose config
start/health-check

nginx.proxy must prove:

resolve includes/server/location/upstream
stage bounded change
nginx -t
HTTP routing probe

java must prove:

parse symbols
compile
format/static check
JUnit
localized correction

A capability version moves through:

candidate
→ validating
→ active

An objective may use only active versions.

12. The first real heterogeneous proof

Do not implement all adapters before use.

After the React vertical finishes, use one real Laravel workload:

Add an appointments page with server-side search, dynamic Stimulus filters, a detail modal, existing Tailwind conventions, feature tests, and browser verification.

Expected capabilities:

PHP
Laravel
Blade
HTML
Stimulus
Tailwind
PostgreSQL
browser verification

Docker and NGINX are detected and available, but should remain untouched unless the objective actually requires them.

Then a second proof:

Add a Java reporting service, register it in Compose, proxy it through NGINX, call it from Laravel, and prove the complete flow.

That exercises:

Java
Gradle/Maven
Docker
NGINX
PHP/Laravel
integration HTTP
browser verification

If both complete without changing the objective/cognition core, the architecture is genuinely heterogeneous.

13. Implementation order

I would use this sequence:

Phase 1 — freeze the current React/TypeScript vertical

Get one ordinary omni chat application all the way to working source and verification. Do not generalize before that.

Phase 2 — wrap current support in capability contracts

Describe the existing Go and TypeScript/React behavior as capability packs without changing behavior.

Phase 3 — Project Contract

Add mechanical stack detection and the minimal authority interview. This prevents assumed constraints from driving work.

Phase 4 — PHP core

PHP parsing, Composer facts, formatting/linting, PHPUnit/Pest, source mutation, localized repair.

Phase 5 — Laravel

Routes, controllers, middleware, services, models, migrations, config, jobs, container bindings, feature tests.

Phase 6 — Blade/HTML/Stimulus/Tailwind

Layered artifact understanding plus Chromium verification.

Phase 7 — Docker and NGINX

Infrastructure facts, bounded config mutation, exact validation and probes.

Phase 8 — Java

Maven/Gradle, symbols, JUnit, formatting, service/API patterns.

Phase 9 — heterogeneous change groups

One objective spanning several capability packs and one integration proof.

Phase 10 — strategic pilot

Give a strong long-context model rare architecture-level semantic gaps while keeping all execution code-owned.

14. Anti-drift invariants

These should be architecture tests, not suggestions:

1. No project-wide language or framework mode.
2. No model-selected tools or capability packs.
3. No model call without one named semantic gap or one bounded source job.
4. No inference while deterministic progress remains.
5. No model-owned filesystem, browser, database, scheduler, or completion loop.
6. No model sees every capability merely because the project contains them.
7. Every leaf receives minimum sufficient context, not minimum possible text.
8. Every mutation has one artifact/region owner.
9. Every claimed completion maps to explicit proof obligations.
10. Unsupported capability fails before mutation.
11. Discovered unrelated work cannot become active without user authority.
12. Behavior is proved vertically before persistence or generalization.
13. Repeated semantic work becomes a candidate deterministic skill in Charizard.

What “guarantee” can honestly mean

No system can guarantee that an arbitrary model will solve every arbitrary software request.

Omnidex can guarantee much stronger and more useful properties:

It knows exactly which capabilities a task requires.
It refuses tasks whose capability closure is missing.
Models cannot gain undeclared capabilities.
Every side effect is code-owned.
Every model call has one bounded responsibility.
Every task has explicit completion criteria.
Every completion claim has accepted evidence.
Every failure is localized to a capability, artifact, or semantic station.
A larger project does not automatically create a larger model context.

That is the right capability guarantee:

Omnidex never pretends that “the model seems smart enough” is support for a stack. It supports a stack only when the required deterministic capabilities, bounded semantic stations, and proof mechanisms are present and independently validated.

The organizing abstraction should therefore be:

Project Contract → Objective Graph → Capability Closure → Deterministic Workflow → Semantic Gap → Charmander Leaf → Proof

That lets Omnidex switch freely across PHP, Laravel, Blade, HTML, Stimulus, Tailwind, Java, Docker, NGINX, Go, and TypeScript without ever turning into a zoo of agents or handing the machine back to an LLM.
