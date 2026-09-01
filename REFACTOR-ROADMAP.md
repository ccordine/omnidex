The audit is not 67 independent bugs plus 10 more blockers. Treating it that way would preserve the architecture and consume another month.

It is roughly eight root-cause programs repeatedly expressing themselves in different packages:

1. Global work is performed before any consumer needs it.
2. The semantic sieve has redundant gates around it.
3. Filesystem mutation is treated as a privilege granted by Git, Docker, tests, clean baselines, and language-specific assumptions.
4. Repository work is secretly Go-only despite broader advertised support.
5. Multiple authorities and silent fallbacks make state impossible to reason about.
6. Product-specific and future-product behavior is embedded in the generic runtime.
7. Retired systems and compatibility layers are still active enough to interfere.
8. There is no small, surviving, authoritative boot path tying the system together.

The goal is not to make all 77 findings “pass.” The goal is to make most of them cease to exist.

Prime acceptance criterion

The production spine must become:

user request
→ accepted objective
→ code-owned work
→ bounded semantic calls where genuinely needed
→ desired workspace state
→ code-owned filesystem mutation
→ verification of resulting reality
→ completion

Nothing may block that path merely because an unrelated capability, provider, deployment route, repository property, test, Git state, Docker mapping, model setting, telemetry record, or future feature is unavailable.

Operating rules for the cleanup

These rules should govern the entire roadmap:

1. No Omnidex application runs until you explicitly authorize one.
2. Do not repair, recreate, or satisfy deleted tests.
3. Use compile-only checks while production code is changing.
4. Delete obsolete behavior instead of adding compatibility shims.
5. Do not replace one global validator with another differently named global validator.
6. Validation belongs to the first actual consumer of a value.
7. A mechanically satisfiable prerequisite must be satisfied by code, not requested from the caller.
8. A capability unused by the current operation cannot block that operation.
9. One state class gets one authoritative source.
10. Invalid or missing state fails explicitly; it does not silently fall back to a different authority.
11. Do not add model calls during this cleanup.
12. Do not work on documentation, file length, formatting, CI, benchmarks, model reduction, browser inference, roleplay, skills, or Charizard.
13. The only useful compile check is whether surviving production packages build.
14. An internal subsystem being “green” is irrelevant until the product writes real files.

⸻

Phase 0 — Rebase the audit against the destructive checkpoint

Some audit references may already be stale because you deleted tests, Docker definitions, environment files, agent files, and committed the result.

Do not perform another open-ended architecture investigation. Normalize the existing audit against the current HEAD.

For every cited symbol or file, assign exactly one state:

STILL EXISTS
ALREADY DELETED
UNREACHABLE / ORPHANED
NEEDS DESIGN DECISION

At the same time, identify the intended surviving production spine in terms of packages and functions:

input boundary
job/objective persistence
semantic/cognition runtime
generation/content path
workspace planning
workspace mutation
verification
completion

Do not infer the spine from README files or old diagrams. Use current imports and calls.

Deliverable

A compact live checklist grouped by the phases below, with stale findings removed.

Exit condition

You can point to the current source locations for:

request enters
work becomes authoritative
source/content is produced
workspace mutation should occur
completion is recorded

Even if some of those locations are currently broken or disconnected.

⸻

Phase 1 — Remove all pre-consumer gates

This addresses the largest family of violations: audit items 1–9, much of 11–17, and qualified blockers 1–4.

Delete the blanket runtime validation model

Remove global checks that validate all of these before startup or before any real consumer exists:

database
workers
provider routes
concurrency
timeouts
host mapping
realtime
UI sessions
web providers
deployment
service endpoints
artifact adapters
stack registry
cleanup commands

Configuration loading should perform only structural decoding:

can this value be parsed into the expected type?

Actual validity belongs at the consumer:

database operation
→ validate/open database config
station dispatch
→ resolve and validate that station’s provider/model
deployment task
→ validate deployment settings
workspace mutation
→ validate the one workspace root
web research
→ resolve the web provider

Change ordering so work determines context

The current system repeatedly acquires context before it knows whether that context is relevant.

Replace:

free-form request
→ recursive workspace scan
→ context retrieval
→ relevance inference
→ objective classification

with:

free-form request
→ minimal objective classification
→ determine required evidence domains
→ acquire only those domains

Examples:

roleplay turn
→ no repository scan
database research
→ no workspace scan
greenfield app
→ empty/current workspace structure only
existing-repository change
→ repository investigation

Remove global registry preflight

Do not validate every adapter, stack, version profile, deployment descriptor, and provider as one startup unit.

An adapter is validated when an artifact actually selects it.

A provider is validated when a station actually dispatches to it.

A deployment descriptor is validated when deployment is actually requested.

Remove early model-route resolution

Requirements and objectives must survive or fail based on their semantics—not on whether all future model routes have already been resolved.

Model routing occurs when a semantic station is actually ready to run.

Remove endpoint/service-route pre-resolution

Support-only, local-file, CLI, and browser tasks must not require every HTTP/service endpoint model to exist.

An endpoint is resolved when a task creates an actual endpoint obligation.

Telemetry cannot precede work

Telemetry records observations about work. It cannot be a transactionally mandatory prerequisite for beginning the work, and it cannot duplicate completion authority.

Persist authoritative job state first. Telemetry may follow and fail independently.

Exit condition

A minimal objective can be accepted and placed into code-owned work state without requiring:

repository acquisition
database
embeddings
deployment
service endpoints
all providers
all adapters
telemetry
Git
Docker
tests

unless that objective specifically needs one of them.

⸻

Phase 2 — Finish the semantic sieve and remove gates around the sieve

This addresses audit items 10–17, qualified blocker 3, and the extra task-local result-relation/review surfaces mentioned at the end of the audit.

The correct model is:

generator proposes candidates
→ code-owned queue
→ each candidate enters the sieve
→ accept, discard, or preserve as unresolved authority
→ queue exhausts
→ continue

Required changes

Exact quotations and byte spans are not semantic authority

Remove the requirement that repository requirements must be exact, contiguous, non-overlapping quotations.

Paraphrased, repeated, and overlapping valid requirements should be normalized semantically, not rejected because their byte spans are inconvenient.

Empty accepted sets are not globally fatal

An empty local result means:

this station produced no surviving candidate

It does not automatically mean the whole objective is invalid.

The immediate consumer decides whether an empty result is valid, requires another specific semantic question, or means no work is necessary.

Negative relations stay negative

Remove selection groups that reopen candidates after their individual relation was negative.

Code cannot say:

candidate B was rejected
but B belongs to group G
therefore force B back in

Duplicate context is deduplicated, not fatal

Identical content should collapse mechanically.

It must not terminate the entire context compilation or semantic sieve.

Delete duplicate model calls disguised as correction

fragment_generation_replacement and similar paths that feed the unchanged original prompt into the same semantic station are not correction. They are repeated inference with a renamed wrapper.

Delete them.

A source-body correction exists only when code has proved one specific defect and its
exact mutable byte span. It continues the same persisted job/model context, exposes
only that span and one necessary semantic question, and lets code splice the returned
ordinary text into its retained base.

An absence relation is a valid local semantic result

If a repository-owner choice resolves to the code-owned absence relation, that means
no current candidate owns this behavior. The model does not emit the internal label
`NONE`; for a genuine plurality it selects one opaque letter and code performs the
mapping. A sole applicable owner is used directly without a model call.

Code can investigate further, create a new artifact, or leave the task unresolved. It must not become a whole-job veto.

Multiple requirements may share an owner

Two requirements may legitimately map to one function, declaration, file, component, service, or query.

Shared ownership is ordinary software architecture, not a conflict.

Enforce minimum-context leaf envelopes

Database and repository semantic stations should receive one exact need and the smallest relevant current state.

Do not include accepted arrays, broad schemas, sibling state, or aggregate job context when the station’s responsibility is one relation.

Remove aggregate semantic vetoes

No model-produced token such as:

REQUIREMENT_REMAINS
COVERAGE_INCOMPLETE
CONTINUE
STOP
REVIEW_REJECT

may force another generation round.

A completeness probe may produce one concrete candidate. That candidate enters the same sieve. An abstract “something remains” statement has no authority.

Exit condition

The semantic phase is monotonic:

accepted leaves are never reopened
bad candidates evaporate locally
duplicates are boring
one invalid candidate cannot stop unrelated accepted work
queue exhaustion advances the workflow

⸻

Phase 3 — Replace the mutation gauntlet with a workspace reconciler

This is the highest-value production work. It addresses audit items 18, 22–32, 34, and qualified blocker 6.

The mutation subsystem should accept:

authoritative workspace root
current filesystem state
desired filesystem state or explicit bounded mutations

and derive real operations.

The mutation engine must support

ensure directory
create file
replace file
modify file
delete file
delete directory when appropriate
move/rename
chmod/executable creation
empty files
binary files
CRLF or no-final-newline content
mixed create/modify/delete in one objective

Do not create separate architectural paths for .txt, .gitignore, .dockerignore, source files, and configuration files unless their verification genuinely differs. They are all artifacts entering the same filesystem reconciler.

Zero delta is success

If current reality already equals desired reality:

desired == actual
→ complete

No mutation is required to prove that work succeeded.

Git becomes optional evidence

Remove these prerequisites:

.git directory
nonempty HEAD
Git CLI
commit identity
snapshot implementation
clean baseline
Git-backed rollback

Git may later provide:

history
diff evidence
rollback convenience
source provenance

It is not the authority that permits Omnidex to write a file.

Docker becomes optional execution infrastructure

Remove:

same host/runtime inode requirement
same path requirement
rootful /var/run/docker.sock requirement
Docker mapping as filesystem authority

A workspace path is one authoritative path. Docker may mount it when a container executor is selected.

Tests cannot authorize mutation

Delete requirements that an existing file must already have directly bound tests before it can be modified.

Tests verify behavior after work. They do not grant permission to perform work.

Broken repositories are valid inputs

A failing repository is often exactly what the user wants repaired.

Analysis must be capable of returning:

partial parse
known symbols
unknown regions
compiler diagnostics
broken imports
available ownership evidence

It must not reject the repository merely because it does not already compile.

Remove arbitrary deletion cardinalities

Delete restrictions such as:

must have 2–8 candidates
must have one ambiguity group
each file must expose <=4 declarations
cannot delete last production file
only one pure deletion

Code should use actual authority, dependencies, and user intent—not arbitrary list sizes.

Exit condition

There is one direct production function capable of applying a valid mutation plan to a normal host directory without Git, Docker, tests, or a clean baseline.

Do not run it yet unless explicitly authorized. At this phase, establish it through source tracing and compilation.

⸻

Phase 4 — Replace Go-shaped repository handling with artifact adapters

This addresses audit items 19–23, 29, 31–35, and qualified blockers 4–5.

The existing-repository workflow currently advertises multiple stacks but routes them through Go assumptions. That must stop.

Normalize language-specific reality

Each adapter should expose a common code-owned contract conceptually like:

recognize artifact
parse as much as possible
extract declarations/interfaces/references
return diagnostics
project a bounded repair region
validate resulting artifact
provide available verification commands

Initial adapters can be:

Go
TypeScript/JavaScript
plain text/configuration
PHP
Java
Rust
Python

Support does not need to be equally deep. Capability must be explicit.

Example:

TypeScript:
parse=yes
types=yes
compile=yes
tests=optional
plain text:
parse=basic
types=no
compile=no
content validation=available

Dispatch per artifact, not per repository

Mixed-language repositories should naturally select different adapters per file.

Do not classify an entire repository as “Go” and route every artifact through the Go analyzer.

Relevance before breadth limits

Scope repository evidence to the active task first.

Then apply budgets.

Do not traverse every analyzer and make omissions fatal before determining which artifacts are relevant.

Source-body correction only after a specific defect exists

Continue the same persisted source-body job with its immutable model route. Do not
create guidance, executor, replacement, or fallback routes. The current hard resource
ceiling is three total body attempts; only validated reality determines acceptance.

Remove Go-specific verification assumptions

Delete hard-coded dependencies on:

Linux
Bubblewrap
specific host directory topology
preexisting module cache
offline/read-only toolchain

An adapter declares what verification is available in the actual environment.

Exit condition

No source path routes a non-Go artifact through Go analysis merely because Go is the most mature adapter.

⸻

Phase 5 — Collapse competing authorities and remove silent fallbacks

This addresses audit items 45–59 and qualified blockers 1, 7–10.

This is where the system becomes understandable again.

Establish one authority per state class

A recommended authority map:

State class	Authority
User objective and accepted decisions	Task/job ledger
Job/task lifecycle	PostgreSQL
Workspace root	One immutable job/workspace record
Model route	Immutable station route bound at dispatch
Process boot configuration	One explicit decoded config object
Mutable runtime settings	One database-backed settings authority, if needed
UI/session state	Server-authoritative store; Redis may cache but not override
Secrets	One explicit secret resolver configured at boot
Schema version	Embedded migration manifest/database ledger
Provider result	Exact persisted station receipt

Do not merge:

request
.env
process environment
database
project config
hidden defaults

into one ambiguous value.

Invalid configuration must remain invalid

Remove automatic replacement of malformed values with defaults.

A blank or invalid timeout, URL, TTL, concurrency setting, or provider value should produce an explicit error at its consumer.

Defaults are allowed only when the value is genuinely optional and the default is part of the declared contract—not as error masking.

Remove provider aliases and guessed provider modes

Delete compatibility aliases, misspelling recovery, URL-based API inference, first-nonempty provider chains, and silent fallback to another provider.

A station route names one provider/model/runtime profile.

If it fails, that route fails. Changing the route is an explicit new decision.

Unify API construction

There should be one server/runtime constructor, not several optional paths that inject or derive competing stores and clients.

PostgreSQL and Redis must have explicit roles. For example:

PostgreSQL = authority
Redis = optional cache/transport

A Redis failure must not silently produce a second authoritative implementation.

Unify UI/session storage

Choose one authoritative session representation.

Do not read from Redis, then PostgreSQL, then process memory, then invent a fresh object.

Do not write through all three stores.

No time-derived ID fallback when ID generation fails.

Simplify secrets

Remove process-global mutable secret state, environment fallback after store failures, and nil-authority conversion into no-op/background behavior.

A missing secret needed by the current consumer fails explicitly.

A secret unused by the current operation does not matter.

Simplify host/terminal routing

Choose one typed host transport per operation. Remove “first reachable” probing across configured hosts, environment hosts, Docker aliases, gateway, loopback, and localhost.

Remove raw absolute-path fallback.

Replace keyword routing with typed state

Delete workflow decisions inferred from words such as:

done
success
ok
diff
error prose
HTTP status prose
shell output suffixes

Use typed internal events and results.

LLMs may return semantic text or one bounded token. Code turns that into typed state at the exact station boundary.

Remove raw SQL execution

Keep the typed relational-intent path.

Delete the regex-based “read-only” SQL classifier and direct raw execution endpoint.

Database transactions used for research must actually be read-only.

Pagination must be real

Lists must return:

items
cursor or next offset
has_more

Do not silently truncate without declaring truncation. Invalid pagination fails instead of resetting itself.

Errors must remain errors

Delete fallbacks such as:

invalid payload → {}
query failure → zero metrics
telemetry failure → empty map
unknown constraint → invented key

Artifact persistence must be typed

Replace arbitrary string kind/version plus unvalidated JSON with registered artifact schemas or typed records.

Exit condition

For every major state class, you can name one authority and prove no silent fallback changes its meaning.

⸻

Phase 6 — Remove product hard-coding and dormant future systems

This addresses audit items 36–44, 60, 65, and qualified blocker 9.

Most of this should be deletion or quarantine, not repair.

Remove skill machinery from Charmeleon

The current skill subsystem:

retrieves skills
cannot create them
cannot validate them
cannot store them
cannot activate them
requires embeddings globally
has schema triggers rejecting learning

is dead weight until Charizard.

Remove it from the active runtime and schema. Preserve it in Git history, not production execution.

When Charizard begins, rebuild it around explicit learned-skill contracts.

Remove global embeddings prerequisites

A task that does not need similarity search must not require an embeddings client or active skill model.

Resolve embeddings only when a retrieval operation specifically requests them.

Remove TypeScript/React skill identity hard-coding

No generic coding identity may be hashed under a literal TypeScript/React worldview.

Artifact type comes from the actual adapter.

Remove audio from framework core

PCM rates, audio contexts, playback assumptions, and buffering belong in an optional media adapter or product feature—not the generic coding runtime.

Remove browser product layout and interaction rules

The browser adapter may understand browser artifacts.

It must not impose:

one component per requirement
Live Workspace header
capability-card grid
one intrinsic root
unconditional controls
no forms
no links
no dialogs
no custom components
hard-coded visual layout

Those are product design choices, not framework invariants.

Remove silent browser state substitution

Missing published capability state must not silently become empty feature state.

If a current consumer requires state, the absence is explicit.

Delete retired schema and routes

Remove retired tables, triggers, routes, telemetry duplicates, write-only histories, and compatibility surfaces that no current production path consumes.

Delete orphan packages

Packages with no surviving production importer should be deleted unless they are intentionally retained libraries with a near-term consumer.

Do not “fix” unreachable packages merely to keep them around.

Exit condition

The surviving core no longer contains product-specific behavior for audio, React layouts, future skill learning, or retired pipelines.

⸻

Phase 7 — Restore one boot path and one schema installation path

This addresses audit items 61–64 and parts of 45–50.

After deletion, the project needs one boring way to exist.

Add one executable entrypoint

Conceptually:

cmd/omnidex/main.go

It should:

decode structural boot configuration
construct authoritative stores
apply or verify migrations
construct one runtime/server
start requested surface

It must not globally validate every future capability.

Embed or directly own migrations

The executable must have one production path that applies the authoritative schema.

Use an embedded migration manifest or one explicit migration package.

Do not rely on Docker initialization scripts, missing shell scripts, or README instructions.

One runtime constructor

Remove alternate server constructors, stale binaries, passthrough compatibility bridges, and environment-driven binary discovery.

The CLI should invoke the actual current runtime, not search for agent-cli, cli, bin/agent-core, bin/omni, or retired binaries.

Docker is packaging, not architecture

Do not restore Docker yet.

Once the direct binary works, Docker can later wrap the exact same entrypoint.

Exit condition

go build ./...

produces a real Omnidex executable from current source, and the executable owns schema initialization without deleted external scripts.

Do not run it yet.

⸻

Phase 8 — Static no-run release gate

This is the point where Codex has to earn another application run.

Perform only:

go build ./...
go mod tidy
git diff --check
static call-path inspection
targeted source searches

No tests. No Docker. No full application run.

Required static assertions

The direct request-to-write path contains no mandatory dependency on:

global Validate(cfg)
global registry preflight
all model routes
all endpoint routes
deployment
embeddings
telemetry
Git
nonempty HEAD
Git CLI
Docker socket
same-inode mappings
clean baseline
preexisting tests
Go-only repository analysis
exact quotation byte spans
aggregate completeness review
model-authored workflow tokens
hidden config fallbacks

The mutation engine must have one authoritative workspace root and support the required file operations.

The source identity, when needed, is code-derived. It cannot be a caller-supplied prerequisite for writing files.

Exit condition

Codex presents:

1. The exact production call path from request to workspace mutation.
2. Every remaining gate on that path.
3. Why each remaining gate represents a real current requirement.
4. A successful production build.
5. No known artificial blocker in front of workspace mutation.

Then you decide whether it gets another run.

⸻

Phase 9 — User-authorized proof sequence

Only after Phase 8.

Proof A: filesystem primitive

Use the real production workspace mutation path to create one known file in an empty host directory.

Verify externally:

file exists
bytes match
path is under authoritative root
no Git required
no Docker required

This is not yet an Omnidex coding success. It proves the body can move.

Proof B: one ordinary minimal request

Submit one unchanged ordinary request.

No steering.

Persist the job ledger somewhere durable.

After terminal state, inspect externally:

find workspace -type f

Required:

file count > 0
expected files exist
files contain nonempty or intentionally empty valid content

Only then inspect internal receipts.

Proof C: functional result

Run the relevant real build or executable verification for the generated project.

Completion requires the user-visible result to function—not merely model success, staged source, or internal green state.

Failure rule

On failure, inspect only the first causal boundary that prevented progress.

Do not fix speculative downstream problems.

⸻

Phase 10 — Reintroduce minimal verification after files exist

This addresses audit items 66–67.

Do not restore the old test corpus.

Add small regression tests only for root causes that actually mattered:

1. Unused capability cannot block startup/work.
2. Validation occurs at first consumer.
3. Empty local sieve results are not global fatal errors.
4. Duplicate candidates are discarded locally.
5. NONE ownership is a valid local result.
6. Zero-delta desired state succeeds.
7. Workspace mutation works without Git/Docker/tests.
8. Create/modify/delete/move/empty/binary/executable operations work.
9. Broken repository diagnostics remain usable.
10. Non-Go artifacts do not route through Go.
11. One authority per state class; invalid values do not silently fall back.
12. Ordinary request produces a host-visible file.

Then add CI around those.

The line-count issue is last. A 500-line file is ugly; it is not currently preventing file output.

⸻

Issue coverage matrix

Roadmap phase	Audit coverage
Phase 1: pre-consumer gates	1–9, Q1–Q4
Phase 2: semantic sieve	10–17, Q3, uncounted task-local review surfaces
Phase 3: mutation engine	18, 22, 24–32, 34, Q6
Phase 4: adapters/repository	19–23, 29, 31–35, Q4–Q5
Phase 5: authorities/fallbacks	45–59, Q1, Q7–Q8, Q10
Phase 6: hardcoding/dead systems	36–44, 60, 65, Q9
Phase 7: boot/schema	61–64
Phase 10: verification/quality	66–67

There is overlap because many findings are symptoms of two root causes. That is expected. Fix the root cause once; do not make two local patches.

Commit sequence

Use commits that correspond to architectural demolition, not individual audit findings:

1. remove global pre-consumer validation
2. make semantic sieve local and monotonic
3. replace mutation gates with workspace reconciliation
4. generalize repository work through artifact adapters
5. collapse duplicate authorities and silent fallbacks
6. remove dormant/product-hardcoded subsystems
7. remove retired schemas/routes/orphan packages
8. restore one entrypoint and migration path
9. compile-only audit and direct-path proof
10. user-authorized runtime proof

Each commit should:

change production code only
avoid test repair
avoid compatibility shims
avoid new abstractions unless strictly required
compile before proceeding

Directive to hand Codex

Execute the Omnidex demolition roadmap by root cause, not by individual audit finding.
The active objective is to remove artificial blockers and leave one understandable production path from an accepted objective to authoritative workspace mutation. Do not run Omnidex, Docker, broad tests, or integration environments until explicitly authorized.
Do not repair, recreate, or satisfy deleted tests. Do not work on documentation, CI, formatting, line length, benchmarks, model reduction, browser inference, roleplay, learned skills, or Charizard.
Rules:
1. Delete obsolete architecture instead of adding compatibility shims.
2. Validation belongs to the first actual consumer of a value.
3. A capability unused by the current operation cannot block it.
4. A mechanically derivable prerequisite must be derived by code.
5. Semantic candidates are filtered locally; accepted state is monotonic.
6. Bad or duplicate model output is discarded locally and cannot become a global blocker.
7. Git, Docker, tests, clean baselines, and nonempty HEAD are optional evidence/execution facilities, not mutation authority.
8. Workspace mutation must operate directly from one authoritative root and support create, modify, delete, move, rename, empty files, binary files, permissions, and zero-delta success.
9. Existing repositories may be broken and mixed-language.
10. One state class has one authority. Remove hidden fallbacks and silent default repair.
11. Remove dormant skills, global embeddings prerequisites, audio hardcoding, browser product layout rules, retired routes/tables, stale binaries, and orphan packages from the active core.
12. Restore one executable entrypoint, one runtime constructor, and one authoritative schema migration path.
13. Use compile-only checks while cleaning production code.
14. Do not declare completion based on internal tests or semantic receipts. The eventual acceptance condition is real host-visible files and functional output, but no runtime proof may begin until explicitly authorized.
Work through these phases in order:
A. Rebase the audit against current HEAD and identify the surviving request-to-workspace call path.
B. Remove global/pre-consumer gates.
C. Simplify the semantic sieve and remove aggregate review/veto paths.
D. Replace workspace/repository mutation gates with a generic reconciler.
E. Generalize artifact handling and remove Go-only dispatch assumptions.
F. Collapse duplicate authorities, fallback chains, keyword routing, raw SQL, and silent error masking.
G. Delete product-hardcoded, dormant, retired, and unreachable systems.
H. Restore one boot/schema path.
I. Compile all surviving production packages and statically audit the request-to-write path.
Do not stop to fix tests. Do not run the application. Do not create a replacement framework for anything deleted. Remove the root causes and continue until the compile-only static gate is complete.

The core priority is straightforward:

First make Omnidex capable of reaching the filesystem without asking permission from unrelated architecture. Then prove it writes files. Everything else comes afterward.
