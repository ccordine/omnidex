# Evidence of Omnidex work

Evidence records what code actually observed and executed. A hash, model activity,
or an approval label is not evidence that the user's objective was accomplished.

The current service uses its dedicated PostgreSQL schema. Service startup recreates
that schema from [database/setup.sql](../database/setup.sql); internal evidence is
not a cross-start archive or migration obligation.

## Current records

- Jobs retain the unchanged user request and code-owned execution state.
- `llm_call_evidence` records the actual prompt and provider response, model route,
  timing, and outcome. Root calls retain their initial input values; corrections
  refer to their persisted parent call and exact mutable span. Recovery uses the
  existing call, not a request hash or a model-returned identity.
- `verification_command_evidence` records the commands that actually ran and their
  observed output, exit status, timing, and workspace association.
- `database_evidence` records one completed database read: the inspected schema,
  typed relational plan, actual SQL and arguments, returned typed rows, PostgreSQL
  plan estimates, byte and row counts, acquisition time, and elapsed duration.
- `web_evidence` records the exact query, discovery and fetch reports, bounded
  fetched source text, observation times, and the code-owned selection and
  projection inputs. Report-local references are scoped to that recorded
  acquisition; they are not URL or content digests.
- Objective citations retain their source reference and exact excerpt with the
  requirement they support. Their completion record belongs to the same job and
  step that produced the response.

Raw internal records are not automatically public job-history payloads. Evidence
must be retrieved through the appropriate bounded, job-owned repository read.

## Model-input boundaries

Each work kind has one current five-answer semantic justification. These static
descriptions are admission metadata, not evidence that anything ran. They have no
separate versioned identity, historical constructor chain, or renderer-ID receipt.
The actual call input, response, and validation result establish what happened.

Source correction restores the native token IDs from its immediate parent's
complete captured response. The correction request records those IDs separately
from the new defect question and mutable span. PostgreSQL binds them to the same
parent, model route, and context limit. Missing or malformed context stops that
correction before dispatch; an independent accepted result needs no continuation
capability. Native IDs remain bounded by the retained context limit, and the
request byte ceiling includes their encoded representation.
[HTTP transport coverage](../internal/ollama/prepared_context_test.go) checks the
actual request body. PostgreSQL fixtures cover
[unrelated calls](../internal/worker/source_context_isolation_integration_test.go),
[expired attempts](../internal/worker/source_body_attempt_recovery_integration_test.go),
[successive defects](../internal/worker/source_context_chain_integration_test.go),
[missing capability and replay](../internal/worker/source_context_failure_integration_test.go),
and [forged context and request capacity](../internal/queue/llm_call_context_integration_test.go).
These fixed-provider tests establish transport and persistence behavior; they do
not establish live-model correction quality or autonomous building.

Shared objective stations return the semantic value and actual dispatch count;
there is no station-receipt wrapper or reuse flag. Code-resolved choices and
retained results both contribute zero provider calls. Call bounds still apply,
and code validates the returned text or relation before consuming it.
[Consumer regressions](../internal/worker/objective_station_usage_test.go) cover
conversation, grounded-answer limits, canon, ongoing actions, and database-read
failure accounting. [Boundary checks](../internal/worker/objective_station_calls_test.go)
reject invalid values and out-of-range counts and guard the removed receipt APIs.
[Real PostgreSQL fixtures](../internal/worker/station_usage_evidence_integration_test.go)
record one ordinary conversation-response call or two separate ongoing-action
relation/value calls under two unrelated inputs. Successful consumers return the
decoded value; oversized output records the exact rejection without accepting a
partial response. Rebuilding runtime-local state consumes either the accepted
result or rejection with zero further inference. Exact prompts, provider requests,
raw responses, and model routes are compared with the stored records. These are
fixed-provider execution tests, not live-language quality or autonomy evidence.

[Requirement-sieve regressions](../internal/worker/v3_application_authorization_sieve_test.go)
exercise two unrelated requests, with and without a requested candidate following a full retained-capacity batch of
unrequested candidates. Code discards each not-entailed candidate immediately:
no scope-annotation call, classification, partition, duplicate comparison, result
question, or retained-plan capacity is consumed after that negative relation.
The requested candidate still advances; an entirely rejected inventory returns
no proposals and invokes no downstream product or surface model.
[Removed-station checks](../internal/assemblyline/application_scope_station_absence_test.go)
reject the obsolete scope-annotation work kind at validation, rendering, response
framing, response bounds, and semantic-contract lookup.
[PostgreSQL lifecycle coverage](../internal/worker/v3_coding_plan_lifecycle_integration_test.go)
keeps the unrequested candidate out of both persisted plan generations and the
frozen execution scope. Exact provider requests, ordinary responses, model routes,
and accepted call records are compared with actual fixture observations. These
fixed-provider tests prove the sieve and persistence boundaries, not live
interpretation quality, empty-plan completion, or autonomous application building.

[Database parameter tests](../internal/assemblyline/database_parameter_prompt_boundary_test.go)
exercise nine narrow calls under two unrelated fixtures. Added accepted query
clauses do not change their model-visible bytes; changed focused meaning still
does. Selected enum values are excluded by code, and invalid retained state still
fails before rendering or result acceptance.
[Persisted-call coverage](../internal/worker/database_parameter_evidence_integration_test.go)
uses a fixed provider fixture and real PostgreSQL to verify the exact focused
inputs, ordinary text responses, decoded values, and accepted replay without a
second provider call. Full validation state remains in code. These checks do not
measure live-model interpretation quality.

[Database selection tests](../internal/assemblyline/database_selection_prompt_boundary_test.go)
exercise eight further calls under two unrelated fixtures. Changing prior accepted
clauses leaves the prompt unchanged when the applicable choices are unchanged.
Code retains those clauses for validation and excludes already selected relations
and ordering keys without rendering their history. Scoped field selection keeps
only its own relation, parent purpose, and eligible fields; sole choices remain
code-resolved, and exhausted choices fail before inference.
[Aggregate eligibility tests](../internal/assemblyline/database_query_aggregate_choices_test.go)
verify that operations without a compatible projected field are absent from both
rendering and decoding, including numeric-only aggregates on nonnumeric schemas
and extrema on closed-value domains. The available fields still supply necessary
semantic context when an aggregate operation is unresolved.
[Selection-call evidence](../internal/worker/database_selection_evidence_integration_test.go)
records actual fixed-provider requests and responses in PostgreSQL, including the
reduced aggregate menu and retained code-only validation state. Accepted replay
does not invoke the provider again. This remains framework-boundary evidence,
not a live interpretation or application-building claim.

[Query-purpose inventories](../internal/assemblyline/database_query_purpose_inventory_test.go)
return one bounded sequence of candidate lines or `NO_QUERY_PURPOSE_CANDIDATES`.
There is no separate presence call. Code ends an empty optional collection;
the existing projection, ranking, and set-membership consumers still reject
missing required results. Zero remaining capacity opens no inventory call.

[Finite filter-set regressions](../internal/worker/objective_database_filter_subset_test.go)
select enum and boolean membership values through remaining-value choice rounds.
They establish zero-call initial sole selection, removal of accepted values,
optional stopping with one candidate left, and deterministic domain exhaustion.
[Boundary checks](../internal/assemblyline/database_filter_subset_boundary_test.go)
reject the obsolete inventory and scalar-literal paths for closed membership,
keep unrelated query state out of the prompt, and admit the full 256-member enum
domain plus absence. The provider returns one opaque letter per round.
[Failure checks](../internal/worker/objective_database_filter_subset_failure_test.go)
reject an empty required set, invalid choices, list responses, and a selected
set beyond the predicate limit without another station or silent truncation.
[PostgreSQL workflow fixtures](../internal/worker/database_filter_subset_evidence_integration_test.go)
inspect real enum and boolean schemas, record every fixed-provider call, execute
parameterized `IN` and `NOT IN` queries, and compare their actual returned rows.
The fixtures use three and two subset calls respectively (13 and 12 total query
calls); reconstructing accepted intent requires zero additional provider calls.
[Persisted failure coverage](../internal/worker/database_filter_subset_failure_integration_test.go)
retains an accepted member and the subsequent invalid choice as separate call
outcomes; replay uses no inference and executes no SQL. These are framework
execution and boundary tests, not live-language quality evidence.

[Full query-intent fixtures](../internal/worker/database_purpose_evidence_integration_test.go)
exercise numeric and text filters using fixed provider text and actual PostgreSQL
call records. They remove exact and semantic duplicate purposes, discard an
unsupported candidate, bind the surviving projection and typed filter, and omit
sole-choice relation and field calls. Each fixture records 14 actual calls and
reconstructs the same query intent with zero calls after rebuilding runtime-local
state. These are intent-construction and call-boundary checks, not live-language
understanding or evidence that a data-source SQL query ran.

[Paragraph boundary tests](../internal/assemblyline/grounded_paragraph_boundary_test.go)
separate question relevance from complete factual support. Relevance sees only the
question, compact meaning context, and candidate paragraph; support sees only the
paragraph, bounded evidence, and factual claim scope. Neither judges character style.
[Production-pipeline fixtures](../internal/worker/grounded_paragraph_evidence_integration_test.go)
record actual provider requests, ordinary responses, and decoded outcomes in
PostgreSQL for both ordinary answers and roleplay. Negative candidates stop at their
own relation, exact duplicates create no further calls, and later candidates never
receive accepted paragraphs. Both pipelines replay the accepted result with zero
provider calls. A regression caught and fixed a receipt that incorrectly rejected
zero-call replay. These fixed-response tests do not measure semantic answer quality.

[Context-reduction tests](../internal/contextcompiler/reduction_completion_test.go)
reproduce and eliminate an extra minification call after the combined context
already fits. Code retains the remaining groups verbatim with all original source
references. Further reduction still runs when the combined text exceeds its bound,
and invalid required semantic output still fails.
[Persisted reduction fixtures](../internal/worker/context_reduction_evidence_integration_test.go)
use two unrelated contexts, fixed provider responses, and real PostgreSQL. Each
records exactly one necessary ordinary-text reduction call, excludes the untouched
tail and code-owned IDs from that prompt, consumes the result in compiled context,
and replays with zero further provider calls. These are execution-boundary checks,
not live summary-quality evidence.

[Context usage regressions](../internal/contextcompiler/semantic_usage_test.go)
remove the separate call receipt, reuse flag, and unused per-stage counters. Relevance and minification
consumers validate the returned value and actual 0..1 dispatch count directly.
Failures retain the calls already made without accepting partial compiled context.
[PostgreSQL usage fixtures](../internal/worker/context_usage_evidence_integration_test.go)
exercise two unrelated contexts through both relevance and minification. Each
successful compilation records three actual calls and consumes the resulting
text with both source references. A rejected relevance result records two calls;
an oversized minification result records three. Rebuilding runtime-local state
replays either the accepted result or the exact rejection with zero provider
calls. Actual prompts exclude candidate IDs and other candidates from each
relevance leaf. These are fixed-provider execution checks, not summary-quality
or autonomous-building evidence.

[Canon-inventory tests](../internal/assemblyline/roleplay_canon_inventory_test.go)
replace the separate pre-inventory presence call with one bounded plain-text
inventory, including an explicit no-candidates result. Code counts the lines and
ends an empty queue without candidate calls. Malformed or mixed absence output
fails explicitly, and the removed presence work kind cannot render or dispatch.
[Persisted canon fixtures](../internal/worker/roleplay_canon_evidence_integration_test.go)
exercise two unrelated contributions, including a user source and an assistant
source with its reference-only antecedent. Real PostgreSQL records the exact
fixed-provider prompts and responses. Exact duplicates require no further call;
unsupported and paraphrased duplicate candidates are removed without reopening
accepted facts. Pairwise duplicate checks receive only their two facts. Rebuilding
runtime-local state consumes the persisted accepted results with zero provider
calls. Mixed absence-and-fact output records the actual rejection and also replays
that failure without another provider call. These checks prove intake execution
and context boundaries, not live canon interpretation quality.

## Database evidence

Code compiles a relational plan inside the PostgreSQL executor. Models neither
supply SQL nor approve a compiled statement. The executor uses a read-only
transaction, bounded timeouts, an actual PostgreSQL plan, and bounded typed results.

Database model-call totals are usage measurements, not a second proof of the
query or its answer. There is no separate raw-leaf, reduction, or acquisition
call-proof ledger. Code retains its dispatch bounds and validates the semantic
values and executed results directly; a deterministic zero-call result does not
need a model receipt to become usable.
[Regression tests](../internal/worker/objective_database_usage_test.go) reproduce
the removed zero-call rejection and duplicate completion-ledger gate.
[Workflow fixtures](../internal/worker/database_workflow_evidence_integration_test.go)
use fixed model text and real PostgreSQL with unrelated numeric and text filters.
Each records 11 actual semantic calls, executes parameterized SQL, and checks the
returned value. Replaying the retained interpretation uses zero model calls but
performs a second actual read with a distinct execution record. Wrong job/query
results, invalid row counts, missing queried columns, and invented citation rows
still fail. Exact completion replay neither reruns inference nor invents another
query execution.
[Failure fixtures](../internal/worker/database_workflow_failure_integration_test.go)
retain both the successful shape call and rejected inventory call, including the
actual request, response, and diagnostic. Replaying that rejection invokes no
provider and never executes SQL. These fixtures exercise framework execution and
source validation; their fixed responses are not live interpretation-quality or
autonomous-building evidence.

There is no schema, intent, query, or result hash and no compiled-query seal.
Unrelated catalog changes do not invalidate an accepted query. PostgreSQL reports
invalid statements, and code checks the returned column and value types, counts,
and execution bounds.

Each actual read receives a database record ID. Two executions that return the same
rows remain two observations. A database citation identifies that recorded read and
an exact row range; completion loads the record for the owning job and compares the
actual source, acquisition time, column labels, and row excerpt. It does not ask a
model whether the citation or read should be accepted.

Implementation: [database execution](../internal/datasource/postgres_evidence.go),
[recording](../internal/queue/database_evidence.go),
[retrieval](../internal/queue/database_evidence_read.go), and
[citation checking](../internal/queue/database_evidence_citation.go).

## Web and research evidence

Discovery and fetch run directly in their fixed code-owned sequence. The generic
specialist registry, versioned acquisition contracts, attempt reservations,
verification receipts, and unused stage counters are removed. They supplied no
execution evidence and could discard actual reports on error or cancellation.

[Acquisition regressions](../internal/webresearch/acquisition_observation_test.go)
check returned observations, provider ownership boundaries, and zero dispatch for
pre-cancelled work. [Validation tests](../internal/webresearch/acquisition_validation_test.go)
reject mismatched, malformed, and oversized reports without accepting evidence or
invoking relevance. [Real HTTP failure fixtures](../internal/webresearch/acquisition_http_test.go)
use two unrelated subjects to verify completed source observations and the actual
failure diagnostic survive a later HTTP cancellation or invalid-text response.
Parser-bound failures and refused document redirects retain the same observations
without following an unrequested destination.
Those bounded reports return to the caller; they are not accepted `web_evidence`
records. [PostgreSQL failure checks](../internal/worker/research_acquisition_failure_integration_test.go)
confirm failed discovery or a failed later fetch produces no model call, accepted
source record, assistant message, completion, or simulation advance. These are
controlled execution fixtures, not live-public-web quality evidence.

The bounded fetched document stays unchanged when code reduces model context.
Fetch truncation, projection truncation, and citation-excerpt truncation are
distinct observations. Completion retrieves the recorded acquisition for the
owning job, reconstructs the same deterministic projection, and compares the
exact source URL, observation time, excerpt, and truncation flag.

Two acquisitions remain two records even when their content is equal. Neither
recording nor citation checking invokes a model. There is no generic evidence
hash field; old JSON that supplies one fails decoding.

Roleplay research uses these same records. Its prepared question is compared as
actual text, and completion points to the persisted assistant message. It does
not store question, answer, or source hashes, nor duplicate the citation's source
fields in another receipt. Real-world evidence remains separate from fictional
canon and character knowledge.

Web and research model-call totals measure actual dispatch, not a second proof
of the acquired sources or answer. The duplicate semantic-call ledger and
receipt type are removed. A validated retained result can contribute zero calls;
missing, repeated, or ignored decoding still fails at the portable boundary.
[Usage regressions](../internal/webresearch/semantic_usage_test.go) cover that
boundary, failed-call accounting, and rejection of nonexistent selected sources.

[Research workflow fixtures](../internal/worker/research_workflow_evidence_integration_test.go)
use two unrelated subjects, real HTTP acquisition, real PostgreSQL, and fixed
provider responses. Each accepts four paragraphs from two selected sources and
records 19 actual calls: two source relevance calls, one paragraph inventory,
and four paragraph-local relevance/support/attribution calls per paragraph.
The response limit includes both separate paragraph relevance and factual-support
relations; the former limit incorrectly rejected this valid execution afterward.
A second acquisition records a distinct actual fetch with zero model calls.
Publication checks the recorded source text, URL, observation time, and truncation;
changed values fail without partial publication. Exact publication replay adds
no HTTP request, model call, message, or turn advance, and creates no fictional
canon. [Failure fixtures](../internal/worker/research_failure_evidence_integration_test.go)
record exact successful and failed provider requests, raw responses, routes, and
rejection diagnostics without publishing an answer. Replaying their retained
semantic failure uses no further inference. These are framework execution and
source-validation tests, not live public-web or live-model quality claims.

Implementation: [acquired values and projection](../internal/webresearch/acquired_evidence.go),
[recording and retrieval](../internal/queue/web_evidence.go), and
[citation checking](../internal/queue/web_evidence_citation.go).

## Simulation execution

Roleplay preparation retains the exact user contribution and selected persona.
Replay compares those values and the requested channel, message, and input kind.
Applied transitions record the exact action, computed effects, and before/after
state. Turn advancement compares its actual preparation, job, message, channel,
and revision. These paths have no request hash or second request-receipt format.

Narrative freshness compares the actual scene, cast, persona, actions, meters,
inventory, visible canon, memories, events, and their contributing record IDs.
It identifies the changed category directly. No narrative or context fingerprint
is computed or retained; restoring exact persona content keeps the accepted
preparation usable even when the persona's write revision has advanced.

The normal terminal completion publishes the prepared transition and response
atomically. Replaying it verifies the stored result without applying the effect
or advancing the turn again. A transition cannot commit on its own without the
matching terminal response.

The prepared-turn identity is also the key of its single advancement record.
Inventory additions reuse the issuing transition's nonce, so preview rollback
and publication retain the same item identity without hashing item content.

## Compiler and workspace execution

[Real compiler fixtures](../internal/worker/v3_compiled_language_evidence_integration_test.go)
execute JavaScript, Rust, and Java through the production task lifecycle, then its
authoritative workspace commands, and retrieve their actual PostgreSQL records.
The workload is constructed explicitly and source generation returns fixed
declarations; verification and cleanup use production code. They check command
arguments, working directories, complete output streams, durations, and exit status.
Missing JavaScript imports and Rust/Java type defects fail at real execution before
any authoritative write. Successful fixtures also execute their built CLI and check
its exact output. Cleanup verifies that owned staging, compiler output, and caches
are removed on success and failure.

Those runs caught a Java helper missing from task projection and a reserved
JavaScript entrypoint parameter. The fixed fixtures now compile and execute. The
registered source-executor contract requires task/final checks and host verification;
the lifecycle also rejects missing hooks before generating or accepting source.
There is no optional verifier assertion that silently skips these stacks.

[JavaScript behavioral fixtures](../internal/worker/v3_coding_javascript_behavior_integration_test.go)
exercise arithmetic and text-transformation requirements through the production
task lifecycle. Correct results pass the owned Node tests; syntactically valid
wrong results produce actual assertion failures, do not become accepted source,
and never reach authoritative writes. PostgreSQL retains the test output and
nonzero exit. Successful staged and authoritative JavaScript runs also execute
their exact generated test files.
[Boundary tests](../internal/worker/v3_coding_javascript_acceptance_test.go) verify
that implementation bytes and filenames cannot enter the test-body prompt;
[body tests](../internal/worker/v3_coding_javascript_acceptance_body_test.go) prove
ordinary-text generation and reject tests which do not observe the behavior.
Code-owned input and result interfaces remain declaration context, not behavior
instructions or a model-response schema. An unchanged observation may use `const`,
`let`, or `var`; validation rejects actual rebinding, not the binding keyword.

[Task-isolation fixtures](../internal/worker/v3_coding_javascript_task_isolation_test.go)
execute three JavaScript tasks, including one direct capability dependency. Each
task runs only its owned test file. A wrong middle result leaves the earlier
accepted declarations unchanged, still verifies the unrelated later task, and
prevents complete-stage verification. When all three pass, the complete stage runs
all three test files. Dependency-bearing test context retains only the required
capability meaning and observation interface, not another implementation.

[Rust behavioral fixtures](../internal/worker/v3_coding_rust_behavior_integration_test.go)
run separate arithmetic and text-transformation requirements through native Cargo
tests and retrieve the actual PostgreSQL command records. Correct results pass;
wrong results compile but produce assertion failures and exit 101 before accepted
source or authoritative writes. The existing authoritative compiler fixtures also
run the task-owned Rust tests against the written workspace.
[Boundary tests](../internal/worker/v3_coding_rust_acceptance_test.go) and
[body checks](../internal/worker/v3_coding_rust_acceptance_body_test.go)
verify direct declarations, one ordinary-text body call, independent expected values,
and failure before inference when the registered acceptance validator is absent.
These tests exposed and fixed a parser error that treated associated-item names
inside macro arguments as free values. Owned temporary source and Cargo output
are removed on successful and failed execution.

[Java behavioral fixtures](../internal/worker/v3_coding_java_behavior_integration_test.go)
exercise arithmetic and text transformation through actual `java -ea` test classes.
Correct results pass; wrong results compile but record `AssertionError` and exit 1
before accepted source or authoritative writes. The authoritative compiler fixtures
also execute these native tests against the written workspace.
[Prompt boundaries](../internal/worker/v3_coding_java_acceptance_test.go) and
[body checks](../internal/worker/v3_coding_java_acceptance_body_test.go) establish one
ordinary-text test body, independent expectations, rejection of non-executed
assertions, and enforcement at the write gate.
[Native boundary fixtures](../internal/worker/v3_coding_java_runtime_integration_test.go)
execute a direct capability dependency through task-local and complete verification,
prove the runner fails when assertions are disabled, and prove missing input raises
an explicit error instead of becoming an empty value. Parser regressions cover
ordinary map equality, inferred local types, and comments between arguments.
All owned staging and compiler output are removed after successful and failed runs.

These are explicitly constructed framework fixtures, not autonomous builds or
live-model test-quality evidence. Executed assertions establish their measured
observations, not functional completion of arbitrary accepted requirements.

## Verification scope

[Database integration coverage](../internal/queue/database_evidence_integration_test.go)
runs two unrelated fixtures against a real isolated PostgreSQL database. It checks
catalog save/load, query arguments and returned rows, retrieval, invalid citations, completion replay,
unrelated schema changes, and failure when a queried column disappears. The
temporary fixture databases are removed by test cleanup.

[Web evidence integration coverage](../internal/queue/web_evidence_integration_test.go)
uses actual HTTP requests to a local fixture server and a real isolated PostgreSQL
database. It covers two unrelated source fixtures, stored response text, invalid
citations, and exact replay without refetching.
[Roleplay research coverage](../internal/queue/roleplay_research_evidence_integration_test.go)
exercises explicit research commands, literal question binding, atomic message
and citation publication, one turn advance, and real-world/fictional isolation.
These are controlled framework fixtures, not live public-web research or model
generation tests.

[Simulation execution coverage](../internal/queue/roleplay_simulation_evidence_integration_test.go)
uses two unrelated configured meters and commands through the same production
enqueue and completion paths. It verifies preview rollback, clamped meter effects,
atomic response publication, exact preparation and advancement replay, and
rejection of changed replay inputs without additional effects or messages.
It also checks persona-content changes and restoration, and verifies actual
canon visibility for granted and ungranted characters.
[Inventory identity coverage](../internal/queue/roleplay_inventory_identity_integration_test.go)
gives, takes, and gives an item again under finite-use and infinite-use
configurations. It checks the stored identity, remaining uses, and one advancement
per turn despite exact completion replay.

[Package-name coverage](../internal/worker/v3_coding_package_identity_test.go)
compiles two unrelated browser fixtures under Unicode and long directory names.
The manifest and lock retain the same local technical name while the HTML title
retains the product's actual text. Go module generation uses the same technical
name. Work-state source checks guard against reintroducing content-hash imports;
they do not substitute for the behavioral tests above.

[Registry documentation coverage](../internal/worker/v3_registered_adapter_documentation_test.go)
compares the documented adapters, stacks, and dialects with the actual executable
registries. Unsupported historical paths are not reported as current capabilities.

These tests establish framework behavior, not autonomous understanding or
construction of an application. That requires the unchanged ordinary request,
an unsteered run, and inspection of the actual resulting behavior as described in
[the autonomy proof ledger](CHARMANDER_PROOF.md).

The retired `omni ledger export`, `omni run:trace`, and local session-hash ledger
are not current interfaces. The supported CLI entrypoints are defined in
[cmd/omni/main.go](../cmd/omni/main.go).
