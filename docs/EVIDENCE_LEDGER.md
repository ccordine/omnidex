# Evidence of Omnidex work


## Task-local independent review execution grant — 2026-10-19

Root: `/home/gryph/Projects/tools/omnidex`; branch `main`, base `3b252b68`.
Scope: reuse the frozen task's execution grant during independent review, at
thread and turn startup. Preserve the queue, fresh task-local sessions, essential
questions, authority boundaries, owner edits and the user-intent implementation.
No cordine.site workload/job, production database, native service, installation,
source copy, worktree or deployment was changed.

Implemented: `agentPermissions` gives execution and review `danger-full-access`,
`never`, and turn policy `dangerFullAccess`. Planning remains read-only. Role
instructions require actual verification, preserve scope/authority checks and
essential questions, and prohibit reviewer implementation changes. Protocol
assertions cover both startup messages and a fresh ephemeral review thread.

Verification resources: read-only source; existing builder image
`omnidex-r6-snapshot-debug-builder:local`; one runner, 2 CPUs/2 GiB, 256 PIDs,
768 MiB executable `/tmp` and 768 MiB `/cache` tmpfs; `GOMAXPROCS=2` and `-p 1`.
Private internal network `omnidex-review-grant-fixture`; disposable
`omnidex-review-grant-db` (`postgres:16-alpine`), 1 CPU/384 MiB and 256 MiB
database tmpfs. No production data. Browser fixtures use existing
`mcr.microsoft.com/playwright:v1.61.0-noble`, no network/mounts, read-only root,
1 CPU/512 MiB, 128 PIDs, 128 MiB `/tmp` and shared memory, auto-removal plus
bounded failure cleanup. The runner alone receives the Docker socket to create
these fixtures. A static Docker CLI extracted from existing `docker:27-cli`
occupies 39 MiB in ignored `.agent-review-grant-cache`; retire it after checks.
Existing module cache is mounted read-only for queue compilation, `GOPROXY=off`.
All task resources are removed at completion; shared images/caches are preserved.

Commands and observations:
- `go test -p 1 ./internal/codex ./internal/codingagent -count=1 -timeout=180s`
  inside the bounded runner passed both packages. First attempt could not execute
  test binaries on Docker's default noexec tmpfs; rerun with `/tmp:rw,exec` passed.
- Queue fixture command (runner cwd `/src/internal/queue`):
  `go test -p 1 $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .) user_intent_fixture_test.go coding_review_runtime_integration_test.go -run TestFrozenTaskExecutionIndependentReviewRuntime -v -count=1 -timeout=150s`.
  Environment: `OMNI_TEST_REVIEW_RUNTIME=1`,
  `OMNI_TEST_DATABASE_URL=postgres://postgres:fixture@omnidex-review-grant-db:5432/fixture?sslmode=disable`.
  Final result: PASS, both subtests; 5.970s. Four real isolated Chromium
  commands (execute/review for text and JSON fixtures) exited 0 and returned
  `<html><head></head><body>42</body></html>`. Exact implementation bytes were
  unchanged by verification. Both task acceptance writes succeeded and the next
  task query returned nil. An initial invocation mixed absolute/relative file paths and
  failed before tests; the command above corrects that fixture invocation.
  The first runtime attempt mounted the system Docker socket, inaccessible from
  the rootless runner. Final runner mounts active context socket
  `/run/user/1000/docker.sock:/var/run/docker.sock`. Chrome's full binary failed
  with `chrome_crashpad_handler: --database is required` on the read-only image;
  a writable temporary home allowed startup but the full binary did not promptly
  finish the offline probe. That owned probe was stopped. The installed Chromium
  headless-shell binary completed the same JavaScript check with exit 0 in 0.6s.
  Final fixtures use that binary and temporary HOME/XDG directories.
  A subsequent run executed both execution browser checks successfully, then
  correctly rejected scripted parser replies missing their required frame receipts
  (`agent returned unbound or incomplete provider evidence`). The fixture now emits
  explicitly labeled scripted response receipts for parser roles as well.
- Final protocol/coding-agent rerun passed all enabled tests, including exact
  native owner-choice/question handling and durable task-local reconstruction.
  Installed live-model tests were explicitly skipped (not enabled).
- `go test -p 1 ./internal/queue -run "^$" -count=1 -timeout=180s` fails to compile
  unchanged obsolete tests: `coding_agent_carry_integration_test.go:84` expects
  three completion-count return values instead of two;
  `coding_agent_completion_test.go` references removed Task.Acceptance/Checks,
  codingagent.Check/CheckResult and Entry.Checks;
  `coding_captured_freeze_integration_test.go` references removed
  EngineeringLedger.Objectives/AcceptanceCriteria and Controller.Requirements.
  These unrelated tests were neither weakened nor edited. The scoped command
  compiles every production queue file with the current-contract fixture helpers.

The new queue fixture uses ordinary admission, approval/freeze, step claim,
`NextIncompleteCodingLedgerTask`, controller execution then independent review,
PostgreSQL evidence, acceptance and next-task query. It supplies scripted semantic
responses; its actual Docker/Chromium operations and exact-byte observations are
runtime evidence, not live-model evidence. Protocol tests use scripted RPC replies.
No live-model review quality or autonomous understanding is claimed.

The exact browser command is constructed and logged in
`internal/queue/coding_review_runtime_integration_test.go`. It executes
`/ms-playwright/chromium_headless_shell-1228/chrome-headless-shell-linux64/chrome-headless-shell`
with `--disable-background-networking --no-sandbox --disable-gpu
--disable-dev-shm-usage --no-first-run --user-data-dir=/tmp/profile --dump-dom`
and quoted URL `data:text/html,<body><script>document.body.textContent=6*7</script>`.
Observed successful container names:
`omnidex-review-grant-browser-execute-1792388441195278576`,
`omnidex-review-grant-browser-review-1792388442781373776`,
`omnidex-review-grant-browser-execute-1792388445140798866`,
`omnidex-review-grant-browser-review-1792388445681640442`.
Protocol and task-local unit suites passed on the final production changes;
`git diff --check` passed after the runtime fixture and evidence changes.

Implemented and verified as scoped above. Delivered: local uncommitted changes
only; no commit/push requested. Cleaned: all owned runner/browser/probe containers
were auto-removed; `docker stop -t 5 omnidex-review-grant-db` removed the disposable
database, `docker network rm omnidex-review-grant-fixture` removed its network,
and the extracted CLI plus `.agent-review-grant-cache` directory were removed.
Final scoped `docker ps -a` and network listing returned no task resources.
Tmpfs test workspaces, database records and caches disappeared with their containers.
Owner source edits, shared images/caches and the cordine.site job/workload remain
untouched.

Independent review recheck, same workspace, 2026-10-19:
- Inspected both startup call sites, task-context/authority validation, the queue
  task loop, fresh-session construction and the execution/review instructions.
  No implementation changes were made during review.
- `docker exec omnidex-grant-audit-runner go test -p 1 ./internal/codex ./internal/codingagent -v -count=1 -timeout=180s`
  passed both packages, including plan/execute/review protocol assertions, durable
  task reconstruction and exact owner-choice/question handling. Installed
  live-model tests were skipped because their opt-in flags were unset.
- `docker exec -w /src/internal/queue omnidex-grant-audit-runner /bin/sh -c 'go test -p 1 $(go list -f "{{range .GoFiles}}{{.}} {{end}}" .) user_intent_fixture_test.go coding_review_runtime_integration_test.go -run TestFrozenTaskExecutionIndependentReviewRuntime -v -count=1 -timeout=150s'`
  passed both cases in 4.678s. All four real Chromium commands (execution and
  independent review for each fixture) exited 0, produced `<body>42</body>`, and
  preserved exact implementation bytes. Queue acceptance and the empty next-task
  query passed. These are scripted semantic fixtures with actual Docker/browser
  runtime checks, not live-model execution or review evidence.
- `docker exec omnidex-grant-audit-runner go test -p 1 ./internal/queue -run '^$' -count=1 -timeout=180s`
  reproduced the separately reported obsolete-test compile failures above.
  `git diff --exit-code -- internal/queue/coding_agent_carry_integration_test.go internal/queue/coding_agent_completion_test.go internal/queue/coding_captured_freeze_integration_test.go`
  confirmed those test files are unchanged.
- Resources: one read-only-source runner (2 CPUs/2 GiB, 256 PIDs), disposable
  PostgreSQL (1 CPU/384 MiB, 256 MiB data tmpfs, no network), and the fixture's
  capped Chromium containers. Runner and database shared only their isolated
  network namespace; no published ports or production fixtures. Compilation
  used read-only existing modules, 768 MiB cache/tmp tmpfs each and a 48 MiB tools
  tmpfs; observed cache/tools use was 187.6/38.5 MiB. Runner/database lifetimes
  were capped at 540/600 seconds.
- `docker stop -t 5 omnidex-grant-audit-runner omnidex-grant-audit-db` succeeded;
  both auto-removed. Filtered `docker ps -a` checks returned no audit resources
  or review browser containers. No host scratch/cache was created; existing
  `bin` remained 96 MiB. Only this existing evidence record was edited by review.

## Reusable user intent boundary — 2026-10-19

Root: `/home/gryph/Projects/tools/omnidex`; branch `main`, base `3b252b68`.
The owner explicitly allowed direct implementation for this request. Duplicate
queued job #13 was canceled and its temporary CLI session exited. Website job #12
was not changed by this work.
Objective: reusable semantic user interpretation with caller-provided options and
callbacks, dedicated context of scoped persisted facts, two recent exchanges and
pending proposals, and deterministic routing into existing services.
Implemented: `internal/userintent` separates context building, semantic option
selection and deterministic callback dispatch. Callers supply options and callback
functions; callbacks return reply, handoff or resume outcomes. The session adapter
routes ordinary CLI text, plan discussion/approval and native approval choices
through that boundary before planner/executor delivery. Bare exit/quit phrase
routing and unconditional conversational replanning were removed from that front
door. Ambiguous references produce a focused question. Removal/control pitches
are persisted; approving one executes the stored action without another approval.
Long recent messages are minified for their current consumer. No full conversation
or global memory dump enters the interpreter. PostgreSQL retains exact owner text,
workspace actor, state/revision bindings, model envelopes/responses/timing, and
committed receipts. Execution uses a separate transaction and the existing ledger
mutation/freeze and workflow implementations. Pending pitches survive reconnect.
Physical request interruption revokes the message without restarting inference;
committed effects are reconciled before interrupting their exact active job.

Verified in disposable Docker fixtures:
- `go test -race ./internal/userintent ./internal/api ./cmd/omni -count=1 -timeout=180s` passed.
- The task's PostgreSQL tests passed with race detection against all production
  queue files plus `user_intent_fixture_test.go` and `user_intent_integration_test.go`.
  They verify exact-revision approval/start, idempotent interpretation/execution,
  stale/other-actor rejection, removal pitch and one confirmation, scoped context,
  shared typed controls, persisted parser failure and request revocation.
- Real HTTP provider-protocol and workspace-WebSocket fixtures exercised two
  unrelated tasks through interpretation, workflow admission, a removal pitch,
  reconnect, confirmation and committed removal; replay caused no duplicate work.
- Production builds of `cmd/omni` and `cmd/omnidex` passed. `git diff --check` passed.

These use scripted semantic/model fixtures; live-model interpretation accuracy
was not measured. Complete client/queue package tests remain blocked by existing
references to removed ledger types in unchanged tests, including
`internal/client/coding_plan_test.go`; those tests were not weakened or changed.
The scoped queue tests use self-contained current-contract fixtures rather than
those obsolete helpers.
Delivered: local uncommitted source and documentation. No installation, native
service restart, production database change, commit, push or deployment. The new
intent table is defined only in the authoritative `database/setup.sql`.
Cleaned: task-owned Go runner, PostgreSQL fixture, their private network, tmpfs
caches and build outputs were removed. No host test caches were created. Existing
source edits, shared images/caches, application data and native services remain.

## Installed CLI workspace change — 2026-10-17

Root: `/home/gryph/Projects/tools/omnidex`; branch: `main`, base `3b252b68`.
Objective: a communicative Codex × LunarVim × tmux `omni chat` interface.
Scope: terminal presentation and keyboard navigation only; preserve the existing
server ledger, input provenance, approval, cancellation, and execution flow.
Acceptance: split conversation/activity panes, discoverable focus/zoom/scroll keys,
readable themed transcript and live observed status, responsive terminal sizing,
intact draft/paste/plan approval, and terminal restoration on exit. No invented
progress, additional model calls, shell multiplexing, or alternate runtime.
Implemented: fullscreen themed conversation/activity panes, docked composer,
project/branch header, observed role/model/command/file/elapsed status, keyboard
focus/zoom/scroll/key guide, responsive layout and `NO_COLOR`. The existing x/term
editor and review router retain input/approval authority. No backend or model
workflow changed. Official Codex CLI documentation informed keyboard discoverability;
the statusline and prefix-driven panes follow the requested LunarVim/tmux direction.
Verified: `GOMAXPROCS=2 GOFLAGS=-p=2 go test -race ./cmd/omni -count=1` passed.
Coverage includes exact draft/paste preservation during output, Unicode bounds,
approval selection/confirmation after resize, approval visibility over pane/help
state, exact review-key passthrough, split-width oldest scrolling, bounded display
scrollback, untrusted-escape display, observed activity, and unchanged idle frames.
Actual isolated tmux terminals exercised the installed `omni chat` at 140×40 and
70×20: truecolor and inherited `NO_COLOR`, pane focus/zoom/help, scroll and return,
Unicode draft/cursor, live resize, `/exit` status 0, caller-screen restoration and
identical before/after `stty -g`. No user turn, model call or coding job was submitted.
Delivered locally: rebuilt `bin/omni` using `scripts/build-core.sh --package ./cmd/omni`;
the installed `~/.omnidex/bin/omni` launcher selects it. Source remains uncommitted
on `main`; no push was requested for this interface task.
Cleanup: private terminal servers stopped and their six socket files removed;
disposable `bin/omni-ui-preview` removed.
Shared build caches, owner terminal sessions, native services and data are preserved.

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
  observed output, exit status, timing, and workspace association. Docker executions
  retain the exact daemon-observed container, image, and exec identities and
  network state. Successful native-process records and networking outside the
  two dependency-acquisition phases are rejected by code and PostgreSQL.
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

## CLI build boundary

[Dependency checks](../cmd/omni/portability_dependencies_test.go) resolve the
production CLI with C dependencies disabled and reject imports of the queue,
database, worker, experiment, language-parser, or testing packages. Shared session
state, history, control types, and operation identities have one definition in
`internal/model`; the API, repository, and client consume those definitions.
Test-only console helpers are excluded from the production executable.

The CLI cross-builds with `CGO_ENABLED=0` for Linux amd64, macOS arm64, and Windows
amd64. The Linux artifact executes `version --json`. Native directory identity
adapters use Unix device/inode values or Windows volume/file identifiers. Actual
macOS/Windows filesystem and terminal execution remain unverified.

[Workspace transport integration](../internal/api/cli_workspace_transport_integration_test.go)
opens an actual WebSocket from the CLI client while the service's configured
host mount is unavailable. Two unrelated workspace fixtures bootstrap, retrieve
history, submit unchanged ordinary turns to persisted jobs, and reconnect to the
same channel. Directory replacement and disconnection reject subsequent access.
[Filesystem boundary checks](../internal/api/cli_workspace_boundary_test.go)
prevent session authority from consulting a matching server filesystem path.
The persisted client installation identifier disambiguates local filesystem
numbers; it contains no workspace index, source, job ledger, or history.
[Path persistence checks](../internal/queue/cli_workspace_path_integration_test.go)
exercise Windows drive/UNC and Unix paths against PostgreSQL and prove that
different client installations cannot share a channel by presenting equal local
filesystem numbers. These are protocol/storage checks, not native Windows runs.

[Retained-directory checks](../internal/projectroot/directory_handle_test.go)
open the actual invoking directory before resolving its display path or starting
configuration/network setup. [Handshake replacement coverage](../internal/client/workspace_retained_directory_test.go)
rejects a substituted directory before the connection can gain authority.
The connection and CLI tests pass with race detection.
[Bootstrap lifetime coverage](../cmd/omni/chat_bootstrap_lifetime_test.go)
proves completing the bootstrap HTTP request does not close the realtime stream.

[Client file transport checks](../internal/workspacetransport/files_test.go)
read exact file bytes and ordered directory pages, prepare the existing workspace
reconciler, and receive each verified change before the final operation result.
Content uses binary chunks of at most 1 MiB, with the reconciler's existing
32 MiB file and 256 MiB aggregate content bounds. Directory pages contain at most
256 entries. Code rejects malformed, unrelated, or out-of-sequence operation
fields and validates change events against the exact prepared files, move
sources, and parent transitions. Repeated or unrequested changes fail explicitly.
[Connection lifetime checks](../internal/workspacetransport/lifetime_test.go)
keep attestation available while another acquisition waits and prove that
disconnection releases the native directory lock.
[Canceled acquisition coverage](../internal/workspacetransport/acquisition_cancellation_test.go)
delays a grant until after cancellation and requires release of that exact grant;
canceling a lock waiter does not close another owner's connection.
[In-flight cancellation coverage](../internal/workspacetransport/inflight_cancellation_test.go)
blocks a constructed prepared operation while holding a real native fence, then
disconnects its caller and observes both cancellation and lock release. The socket
reader remains active during filesystem work so connection loss cancels its
operation context.
Already decoded change evidence is retained before reporting transport failure.

[The worker boundary](../internal/worker/client_workspace_boundary_test.go)
rejects substituting a same-named server directory for a disconnected client job.
[Worker publication checks](../internal/worker/client_workspace_publication_test.go)
exercise the connected client while the configured server mount is unavailable;
they preserve earlier verified files across a later conflict, retain user edits,
and continue zero-delta publication without rewriting accepted files. Source
checks forbid direct server file reads in the coding observation and publication
consumers. The runtime supplies the worker with the API's actual connection hub;
the persisted job transport selects client access or explicit server-local access.

[Constructed client/Docker checks](../internal/worker/client_workspace_docker_integration_test.go)
create an actual CLI connection, submit a turn through the ordinary API, acquire
the resulting persisted job's exact client workspace, and run two supplied Go
declaration fixtures through Docker verification, progressive publication, and
verification of the published bytes. PostgreSQL retains the actual container
command evidence. The fixture supplies the program and enters the task lifecycle
directly; it does not prove intent interpretation or an autonomous build.
Requirement intake uses lexical request context without acquiring filesystem
authority. A fresh unsteered ordinary-request build remains unproven. Native
mutation currently has a Linux adapter; macOS/Windows mutation adapters and
native terminal/filesystem execution remain unfinished.

## Workspace publication

[Publication lifecycle checks](../internal/worker/v3_application_publication_lifecycle_test.go)
require verified publication before the next task starts and retain accepted source
after generation, verification, publication, or final-stage failure.
[Document projection checks](../internal/worker/v3_application_publication_projection_test.go)
defer incomplete shared documents and their dependents, then release complete
documents after their remaining owners are accepted.

[Real execution fixtures](../internal/worker/v3_application_publication_integration_test.go)
record Node compiler/test commands in PostgreSQL and inspect the actual workspace
before later generation starts. Failures in argument counting and text conversion
leave their own files unpublished while independent verified tasks publish. Combined accepted source runs
its tests before additional publication; incomplete jobs never reach final verification.
[Registered compiler coverage](../internal/worker/v3_compiled_language_evidence_integration_test.go)
also exercises publication followed by final host verification for JavaScript, Rust,
and Java.

[Reconciliation regressions](../internal/worker/v3_application_publication_workspace_test.go)
cover zero-delta publication, edits after preparation, a partial write failure,
unowned-file conflicts, protected move sources, and continuation without rewriting
earlier accepted files. [TypeScript stage checks](../internal/worker/v3_coding_typescript_stage_source_integration_test.go)
run successful commands that change source or package bytes and require explicit
rejection before publication. These fixtures establish framework mechanics; they
do not establish live-model quality, autonomous application building, or general
existing-project support.

## Docker experiments

[Docker execution fixtures](../internal/experiment/docker_integration_test.go)
run two unrelated byte-transfer and arithmetic examples in actual containers from
one locally available immutable image. Code transfers exact input files, invokes
the declared command, observes Docker's exec state, and collects only declared
regular files. The fixtures check binary stdin and output, file permissions,
absence of host mounts and ambient host environment values, nonzero exits,
timeouts, stream bounds, invalid artifact links, and removal of each owned
container after success or failure.

[Archive checks](../internal/experiment/archive_test.go) reject malformed paths,
duplicate or missing files, invalid permissions, and exceeded byte bounds before
returning artifacts. [Ownership checks](../internal/experiment/ownership_test.go)
prevent an unbound create response from authorizing removal of another container.
[Stream checks](../internal/experiment/cli_test.go) exercise Go's optimized copy
path so it cannot bypass the output limit.

[Workspace fixtures](../internal/experiment/workspace_integration_test.go) retain
files across separate commands, preserve them after an ordinary nonzero exit,
and prevent further dispatch after a timeout destroys the experiment. These
package tests pass with race detection.

All five registered stacks now use this runner through the production task and
publication lifecycle. Code acquires the registered technical image, executes
inside one retained container, and stores each command's actual Docker identity
and result in PostgreSQL. Their final workspace verification checks authoritative
source bytes before and after verifying an exact copy inside Docker. The compiler
fixtures install failing host-toolchain shims to prove those native executables
are not used.

[Acquisition fixtures](../internal/experiment/acquisition_integration_test.go)
retain text and arithmetic data while disconnecting the container's network.
Ordinary execution and additional file transfer are unavailable during acquisition;
after sealing, code observes no remaining network attachment and acquisition
cannot reopen. The registered browser stack installs its exact manifest/lockfile
with scripts disabled, seals the network, and then accepts application source.

[Go publication fixtures](../internal/worker/v3_coding_go_docker_publication_test.go)
exercise text output and argument selection through focused tests, progressive
publication, final compilation, and Docker verification of the written source.
[Browser publication fixtures](../internal/worker/v3_browser_assembled_project_integration_test.go)
exercise text and numeric state through real npm installation, TypeScript checking,
DOM tests, production builds, and publication. Existing host `node_modules`, `dist`,
and `.vite` directories retain their exact marker files. Incorrect runtime results
fail before publication. The browser fixtures supply both implementation and
expected assertions; they do not prove production oracle generation.

[Artifact tree checks](../internal/experiment/archive_tree_test.go) cover bounded
subtree collection and reject missing roots, links, and excess files. Browser
builds collect their production artifacts as data and compare CSS with the exact
assembled source. [Source absence checks](../internal/worker/v3_coding_docker_boundary_test.go)
prevent the removed native verification runner from returning. General dependency
acquisition, learned skills, and an autonomous application build remain unproven.

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
execute JavaScript, Rust, and Java through the production task lifecycle, then
verify an exact copy of the authoritative workspace in Docker and retrieve the
actual PostgreSQL records.
The workload is constructed explicitly and source generation returns fixed
declarations; verification and cleanup use production code. They check command
arguments, working directories, container/image/exec identities, complete output
streams, durations, and exit status.
Missing JavaScript imports and Rust/Java type defects fail at real execution before
any authoritative write. Successful fixtures also execute their built CLI and check
its exact output. Cleanup verifies that owned containers are removed on success
and failure and that no native staging or compiler output remains.
[Source observation fixtures](../internal/worker/v3_compiled_language_source_observation_test.go)
require rejection when an otherwise successful container command changes checked
source or manifest bytes.

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
run separate arithmetic and text-transformation requirements through Cargo tests
inside Docker and retrieve the actual PostgreSQL command records. Correct results pass;
wrong results compile but produce assertion failures and exit 101 before accepted
source or authoritative writes. The existing authoritative compiler fixtures also
run the task-owned Rust tests against an exact copy of the written workspace.
[Boundary tests](../internal/worker/v3_coding_rust_acceptance_test.go) and
[body checks](../internal/worker/v3_coding_rust_acceptance_body_test.go)
verify direct declarations, one ordinary-text body call, independent expected values,
and failure before inference when the registered acceptance validator is absent.
These tests exposed and fixed a parser error that treated associated-item names
inside macro arguments as free values. Owned container source and Cargo output
are removed on successful and failed execution.

[Java behavioral fixtures](../internal/worker/v3_coding_java_behavior_integration_test.go)
exercise arithmetic and text transformation through actual `java -ea` test classes.
Correct results pass; wrong results compile but record `AssertionError` and exit 1
before accepted source or authoritative writes. The authoritative compiler fixtures
also execute these assertions in Docker against an exact copy of the written workspace.
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

## Frozen task review evaluation — job 15, task p_1816da2e4c9a_1_task

Root `/home/gryph/Projects/tools/omnidex`, existing `main` workspace, base
`3b252b68`. Scope: evaluate review completion and findings against the exact
frozen task, preserve original review/evidence, retain required failures and
questions. Existing queue correction/input routing is unchanged.

Read-only PostgreSQL capture selected job 14's latest `coding_agent`
`review_response` and corresponding `call_opened` review assignment. No database
writes or schema changes. The original response and frozen task are preserved in
`testdata/parser-sieve/job14-review.json`, also included in the native parser
corpus. Historical parser output had classified its unrelated obsolete-test
limitation as both an error and a finding despite "no task-specific defects".
No recap or historical response was edited.

Parser leaves now receive frozen task instructions and adjacent original blocks.
Scan distinguishes required failed/incomplete verification from unrelated
limitations; finding extraction binds only reported defects to that task.
Unknown review meaning fails validation instead of becoming ignored text.
Original response remains the exact verification payload; concrete findings
retain exact source quotes and code-owned task identity.

Verification budget: existing builder image, one auto-removed container named
`omnidex-review-scope-tests`, no network, read-only source/root, 2 CPUs, 2 GiB RAM
and memory/swap ceiling, 256 PIDs, 768 MiB executable `/tmp` tmpfs and 768 MiB
`/cache` tmpfs. `GOMAXPROCS=2`, `GOCACHE=/cache`, `GOPROXY=off`. No source copies,
host build cache or persistent fixture artifacts. Before work: available RAM
23,274 MiB; swap used 16,862 MiB; workspace disk available 45 GiB. Shared images,
existing caches, owner changes and unrelated tests are preserved.

Exact terminal check:
```sh
docker run --rm --name omnidex-review-scope-tests --network none --memory 2g --memory-swap 2g --cpus 2 --pids-limit 256 --read-only --tmpfs /tmp:rw,exec,size=768m --tmpfs /cache:rw,size=768m -v "$PWD:/src:ro" -w /src -e GOMAXPROCS=2 -e GOCACHE=/cache -e GOPROXY=off omnidex-r6-snapshot-debug-builder:local go test -p 1 ./internal/codingagent ./internal/codex -count=1 -timeout=180s
```
Initial attempts exposed two test-harness mistakes: decoding code-owned assembled
findings through the external parser wire DTO (`unknown field "task_id"`), then
redeclaration of `err`. Both were corrected in the new test only.

Final result: exit 0; `internal/codingagent` PASS (0.003s),
`internal/codex` PASS (0.002s). New scripted semantic fixtures exercise the saved
job 14 response (done, zero findings, all original evidence), required failure
(error), concrete finding (done with exact finding for correction), essential
question (question), and unresolved meaning (validation failure). Existing enabled
controller/protocol tests also pass. This is actual Go runtime execution with
scripted semantic responses, not live-model interpretation evidence. The native
corpus includes job 14 for opt-in inference qualification; it was not enabled.
Full queue compilation was not repeated: its unrelated obsolete-test limitation
is preserved in the earlier entries, and no queue code/tests changed in this task.

`git diff --check` passed. JSON validation confirmed the saved response/task and
corpus entry match exactly. Cleanup: `docker ps -a --filter
name=omnidex-review-scope-tests --format '{{.Names}}'` returned no containers;
container-local build/test tmpfs and processes were retired by `--rm`. No task
images, networks or volumes created. Implemented and verified locally; no commit,
push, deployment, installation or service restart requested or performed.

## Task-scoped execution and review interpretation — 2026-10-19

Authoritative root `/home/gryph/Projects/tools/omnidex`, existing `main` tree;
owner changes present at start are preserved. Frozen scope: supply exact task
instructions to execution interpretation, preserve existing review scope and
original #14/#15 reports, retain required failure/input/finding/unknown routes,
and verify affected runtime at configured high parser reasoning. No workload,
queue architecture, historical entry, deployment or schema changes authorized.
Acceptance: saved #14 review and #15 execution complete under their assignments;
required checks and essential questions still block; defects retain exact quotes
and correction routing; unresolved meaning fails validation; execution then fresh
independent review remains the code-owned controller path.

Read-only SQL inspected `information_schema.tables` and selected only assignment,
response and parser settings in `omnidex_runtime_4ffc6561`. An initial query against
`omnidex_runtime` returned an unrelated historical job-15 assignment and null
response; it was not saved or used. The correct unmodified execution assignment
and response are saved in `testdata/parser-sieve/job15-execution.json`. Existing
`job14-review.json` remains untouched. Configured parser:
`codex / gpt-6.1-sol / high / default`. No database mutations.

Resource check: `free -m` showed available RAM 23,356 MiB, swap used 16,871 MiB;
`df -h .` showed 45 GiB available. Existing `bin/agent-runtime` was empty;
unknown owner `bin/pilot-gauntlet.JqLg1u` (2.7 MiB) is preserved. Test-owned
`bin/task-scope-checks` holds only disposable binaries/scratch, budget 768 MiB,
removed after verification. Disposable Docker tests use existing builder image,
read-only source/root, no network, 2 CPUs, 2 GiB memory/swap ceiling, 256 PIDs,
768 MiB `/tmp` and `/cache` tmpfs, one heavy operation at a time. Necessary live
parser receipts will be retained under existing ignored `runs/`; source is never
copied. Supervising terminal invokes tests and parser controls, never `omni chat`.

First isolated command used the bounded Docker invocation below with
`go test -p 1 ./internal/codingagent ./internal/codex -count=1 -timeout=180s`.
It exposed task-activity validation still treating execution_response as global;
controller tests failed before parsing. Codex tests passed. Fixed the affected
activity role boundary; unresolved execution meaning now fails rather than
silently becoming ignore. Existing review implementation remains intact.
Verification in progress; results and cleanup appended below.

Verification checkpoint:
- `go test -p 1 ./internal/codingagent ./internal/codex -count=1 -timeout=180s`
  passes after the fixes, including controller correction/input/failure routes.
- Full `./internal/model` compilation fails in unchanged
  `coding_ledger_test.go` (removed Priority/Objectives/Requirements/Constraints/
  Groups types). No unrelated tests changed. An initial explicit activity-file
  invocation omitted `agent_activity.go` and failed on `activityText`; corrected
  `go test internal/model/agent_call_activity.go internal/model/agent_activity.go internal/model/agent_call_activity_test.go`
  passed. The CLI lifecycle fixture now supplies the task identity for this role.
- Host launch of the first Alpine-built test binary failed because its musl
  loader is unavailable on the host. Recompiled with `CGO_ENABLED=0` in Docker;
  this is test binary output only, no source copy or installation.
- First live parser capture failed on metadata-only events with no JSON frame
  (`unexpected end of JSON input`). Interrupted that test attempt, fixed only
  the capture sink to skip empty frames, and kept its original partial receipts
  at `runs/task-scope-parser-20261019.jsonl`. Production interpretation was not
  changed to accommodate a test failure. Second run uses a new evidence file.
- Queue fixture compilation with the builder's built-in module cache lacked
  current dependencies and failed read-only cache writes. Reused the already
  installed `/home/gryph/go/pkg/mod` read-only, `GOPROXY=off`; no dependencies
  downloaded or installed. Queue source/tests remain in the authoritative tree.
- Disposable PostgreSQL's first setup exited because entrypoint `psql` expected
  the default socket while the test uses `/fixture`; corrected setup with
  `PGHOST=/fixture`. Both failed instances auto-removed. Fixture has no network,
  published port, production credentials or production data; data is 256 MiB
  tmpfs, socket alone is in owned scratch. It will be stopped after the checks.

Final verification commands and observations:

```sh
# Bounded isolated package checks; /out holds disposable binaries only.
docker run --rm --name omnidex-task-scope-tests --network none --memory 2g --memory-swap 2g --cpus 2 --pids-limit 256 --read-only --tmpfs /tmp:rw,exec,size=768m --tmpfs /cache:rw,size=768m -v "$PWD:/src:ro" -v "$PWD/bin/task-scope-checks:/out:rw" -w /src -e GOMAXPROCS=2 -e GOCACHE=/cache -e GOPROXY=off omnidex-r6-snapshot-debug-builder:local sh -c 'go test -p 1 ./internal/codingagent ./internal/codex -count=1 -timeout=180s && go test internal/model/agent_call_activity.go internal/model/agent_activity.go internal/model/agent_call_activity_test.go && go test -p 1 -c -o /out/codex.test ./internal/codex'
# Static host-runnable parser binary; source remains mounted read-only.
docker run -d --rm --name omnidex-task-scope-builder --network none --memory 2g --memory-swap 2g --cpus 2 --pids-limit 256 --read-only --tmpfs /tmp:rw,exec,size=768m --tmpfs /cache:rw,size=768m -v "$PWD:/src:ro" -v "$PWD/bin/task-scope-checks:/out:rw" -w /src -e GOMAXPROCS=2 -e GOCACHE=/cache -e GOPROXY=off -e CGO_ENABLED=0 omnidex-r6-snapshot-debug-builder:local sleep 600
docker exec omnidex-task-scope-builder go test -p 1 -c -o /out/codex.test ./internal/codex
# Run from internal/codex, through the actual configured provider/production sieve.
OMNI_TEST_PARSER_SIEVE=1 OMNI_TEST_CODEX_COMMAND=/home/gryph/.local/share/mise/shims/codex OMNI_TEST_CODEX_MODEL=gpt-6.1-sol OMNI_TEST_CODEX_EFFORT=high OMNI_PARSER_SIEVE_EVIDENCE=/home/gryph/Projects/tools/omnidex/runs/task-scope-parser-20261019-attempt2.jsonl TMPDIR=/home/gryph/Projects/tools/omnidex/bin/task-scope-checks GOMAXPROCS=2 ../../bin/task-scope-checks/codex.test -test.run '^TestInstalledParserSieveCurrentCorpus/(saved-job14-task-scoped-review|saved-job15-task-scoped-execution|task-scoped-)' -test.v -test.timeout=8m
```

Final live result: PASS, nine subtests, 145.93s. Saved #14 review: done,
zero findings, entire original verification retained (39.46s). Saved #15
execution: done (25.54s). Required failure: error in both roles. Essential
question: question in both roles. The same live-model limitation, when required
by the frozen assignment, remains error in both roles. Concrete defect: done
with exact source quote and task identity. These are live semantic interpretation
results, not a live execution/review of newly implemented source, and not universal
natural-language correctness. Unresolved meaning rejection, permission routing,
correction execution with exact findings, failure stopping before review, and
fresh independent sessions are additionally verified by scripted controller
fixtures (`TestFrozenExecutionScopeAndRouting`,
`TestFrozenReviewOutcomesRetainCorrectionAndInputRoutes`). No recap was rewritten.

The successful live capture has 1,493 events / 863,458 bytes, including original
provider frames and separately labeled code-rendered envelopes:
`runs/task-scope-parser-20261019-attempt2.jsonl`, SHA-256
`34623dc478c2e3cc086d82889bfb09dc57869e77a9e0b1c65fd4dd7ac50ca03e`.
Read-only requery compared task/response values from the original PostgreSQL
entries with both saved JSON fixtures and corpus inputs: exact matches.
`job14-review.json` SHA-256
`de5725dfc7f661b8ec62ed48465c4c6dfd9741cc29c96bd31cdf11df546b2509`;
`job15-execution.json` SHA-256
`fa42520aecd54ca92811373910c8d4b48bdb9d3781d5a23cfc61f99b6d6bfb10`.

Disposable queue runtime setup/check:
```sh
docker run --rm --name omnidex-task-scope-tools --network none --memory 128m --memory-swap 128m --cpus 1 --pids-limit 32 --read-only -v "$PWD/bin/task-scope-checks:/out:rw" --entrypoint /bin/sh docker:27-cli -c 'cp /usr/local/bin/docker /out/docker'
docker run -d --rm --name omnidex-task-scope-db --network none --memory 384m --memory-swap 384m --cpus 1 --pids-limit 128 --read-only --tmpfs /var/lib/postgresql/data:rw,size=256m --tmpfs /var/run/postgresql:rw,size=16m --tmpfs /tmp:rw,size=16m -v "$PWD/bin/task-scope-checks/socket:/fixture:rw" -e POSTGRES_HOST_AUTH_METHOD=trust postgres:16-alpine postgres -k /fixture,/var/run/postgresql -h ''
docker run -d --rm --name omnidex-task-scope-runner --network none --memory 2g --memory-swap 2g --cpus 2 --pids-limit 256 --read-only --tmpfs /tmp:rw,exec,size=768m --tmpfs /cache:rw,size=768m -v "$PWD:/src:ro" -v /home/gryph/go/pkg/mod:/go/pkg/mod:ro -v "$PWD/bin/task-scope-checks/socket:/fixture:ro" -v "$PWD/bin/task-scope-checks/docker:/usr/local/bin/docker:ro" -v /run/user/1000/docker.sock:/var/run/docker.sock -w /src -e GOMAXPROCS=2 -e GOCACHE=/cache -e GOPROXY=off -e OMNI_TEST_REVIEW_RUNTIME=1 -e 'OMNI_TEST_DATABASE_URL=postgres://postgres@/postgres?host=/fixture&sslmode=disable' omnidex-r6-snapshot-debug-builder:local sleep 600
docker exec -w /src/internal/queue omnidex-task-scope-runner sh -c 'go test -p 1 $(go list -f "{{range .GoFiles}}{{.}} {{end}}" .) user_intent_fixture_test.go coding_review_runtime_integration_test.go -run TestFrozenTaskExecutionIndependentReviewRuntime -v -count=1 -timeout=180s'
docker exec omnidex-task-scope-runner sh -c 'go test -p 1 ./internal/codingagent ./internal/codex -count=1 -timeout=180s && go test internal/model/agent_call_activity.go internal/model/agent_activity.go internal/model/agent_call_activity_test.go && go test -p 1 ./cmd/omni -run "TestAgentCallLifecycleShowsTypedActualRoleAndStatus|TestQuietAgentCallLifecycleKeepsFailuresWithoutRoutineHandoffs" -count=1 -timeout=180s'
```

`PGHOST=/fixture` alone did not fix entrypoint initialization, which explicitly
uses its default socket. That failed disposable instance was inspected and
removed. The final command exposes both sockets only inside the isolated fixture;
`pg_isready -h /fixture` succeeded. The first queue attempt had failed with
connection refused during this setup issue. Final queue result: PASS, two fixtures,
4.141s. Each ordinary approval/freeze/claim dispatched `[execute review]` with
fresh independent identities; acceptance and empty next-task query passed.
Four actual bounded Chromium commands exited 0 with `<body>42</body>` and unchanged
implementation bytes. These use scripted semantic responses plus actual queue,
PostgreSQL and browser execution; they are not live-model execution/review proof.
Final codingagent, codex, explicit activity validation and affected CLI lifecycle
checks all passed (0.004s / 0.002s / 0.001s / 0.006s).
Unchanged obsolete tests remain preserved; full model/queue suites are not claimed.
`git diff --exit-code` for the observed obsolete model/queue tests passed;
`git diff --check` passed. No source/DB/schema/deployment changes were made to
cordine.site, and no live service was restarted or workspace connection closed.

Cleanup and completion:
`docker stop -t 5 omnidex-task-scope-builder` retired the static-binary builder
before queue compilation; `docker stop -t 5 omnidex-task-scope-runner
omnidex-task-scope-db` retired the final runner/database. All were disposable;
filtered final `docker ps -a` returned no task containers or fixture browser
containers. Runner cache peaked at an observed 201.2 MiB; host scratch was 49 MiB.
Removed exact owned `bin/task-scope-checks` binaries/socket directory and the
interrupted attempt's `bin/agent-runtime/call-3629422190` (only a verified empty
instruction mask). `/proc` cwd inspection found no remaining task-scratch
processes; `bin/agent-runtime` returned to its initial empty state. No task images,
networks or volumes were created; shared images, module caches and unknown owner
artifacts were preserved. Original partial/successful live receipts are necessary
retained evidence, not disposable build outputs. Final whitespace check passed.

Implemented: exact frozen execution instructions and task identity now reach
interpretation and activity validation; review scope is preserved. Verified:
live high-reasoning parser regressions, scripted failure/input/unknown/correction
routes, protocol checks and actual isolated queue/browser runtime all pass as
qualified above. Delivered: local uncommitted changes only; no commit/push or
installation/deployment requested. Cleaned: task-owned disposable resources
retired. No further task or architecture work added.


## Recovery job 19 — activation packet, 2026-10-19

Authority: the one existing `/home/gryph/Projects/tools/omnidex` tree, `main`,
HEAD `3b252b684449447fd84bc122bace2ae494b11bf1`, including all existing
uncommitted changes. One frozen recovery task `p_c4c5910e4a75_1_task`, job 19,
generation 2, coding step 108, execution `call_opened` evidence 132781.
This packet is preparation, not activation. No source implementation was changed.
Job 18 remains failed after installed-runtime result interpretation stopped its
execution before independent review; the supported replan API rejects terminal
jobs. Job 17 remains `waiting_input`. Neither job was changed or used as fixture.
No nested `omni chat`, installer/reset, source copy/worktree, installation,
deployment, active-binary replacement, service restart or live-schema write occurred.

Execution evidence is retained in `runs/activation-recovery-20261019/`.
`frozen-assignments.json` is a read-only capture of original execution assignments,
including panel job 18 generation 3 step 104 evidence 111764 and this recovery
assignment (SHA-256 `f415c09443c47bd1f183c55812968ef7acecfeae60d16975a9ee7e97a8420c2e`).
The former contains the original full panel scope, including controls, viewport,
long goals, preserved drafts/navigation and necessary confirmation rules.
Historical `runs/ledger-panel-task18/` evidence is preserved.

### Source, assets, dependencies and candidates

`source-manifest.json` records 1,843 tracked/nonignored source paths and individual
SHA-256 values (excluding this append-only ledger). Manifest SHA-256:
`0404d1bfe9b1f40f3f5b0613605178ceb0b526718e1656c5eb9d9d8d9245e186`.
Every entry was rehashed after verification with zero discrepancies.
`asset-manifest.json` separately includes the actual ignored embedded dist assets,
panels and locales: 26 files, SHA-256
`ac925ced01665f986a4e2c4058bc0b9bc0c8f32100b7e1dae052a52e060ccc9a`.
Existing assets were used directly; `scripts/build-core.sh` was inspected and
bypassed because it calls `build-ui.sh`. No UI/dependency rebuild or installation.
`identities.txt` records HEAD, branch, starting changes, go.mod/go.sum/setup.sql
hashes, active/candidate binary identities and service identity.
`toolchain-dependencies.txt` records actual linked dependency versions and sums
from both `go version -m` outputs, the offline module graph, compiler/linker/libc
identities and hashes. Existing `/home/gryph/go/pkg/mod` was mounted read-only;
GOPROXY=off, no dependency downloads. Shared cache measured 6,242 MiB, preserved.

Existing builder image: `omnidex-r6-snapshot-debug-builder:local`,
`sha256:4c1b27b8fdbcdd877fef43fef587451d4862384f0509c8c464c18e81b0fe98e5`.
Actual compiler: Go 1.24.1 linux/amd64, CGO_ENABLED=1, GCC Alpine 14.2.0,
GNU ld 2.43.1, Alpine 3.21.3/musl. Go executable SHA-256
`f6e39435e78f3320d4cc14fccf895ddc9dbf355b2d2b5786c83d7be46bcccea6`;
GCC `0d42ad3750de6cce7431a0b46df0cc697c6f1c06d06c2124c09e831196518a55`;
ld `4dec5e3c46e950fafcbb5a55d37315053030650ffebc2db1da5781628aa82ade`;
static libc archive `23d87be15eb015ff229312547790126b559b42df4bdd995b9a0077c6a1fa94df`.
CGO is required by the core's tree-sitter dependencies. Static external linking
retains CGO and produces host-runnable ELF x86-64 binaries without a missing
musl interpreter. Both binaries actually loaded on the host. Host Go 1.26.3
was inspected but was not the candidate compiler.

| Candidate (relative to authoritative root) | Bytes | SHA-256 |
| --- | ---: | --- |
| `bin/activation-ready-20261019/omnidex` | 33,851,120 | `1d3e97017dcb2aa2501b4c1dc6bff90fcd42b28859f3994f72f49c4927679099` |
| `bin/activation-ready-20261019/omni` | 11,262,104 | `f87c359926871bc727d136f6df84041b62aad4e92a1eff41c1897016a140606a` |

Total 45,113,224 bytes (43.0 MiB), below 400 MiB. These ignored review outputs
remain pending the owner's activation decision. No active binary was overwritten.

### Exact isolated execution and verification

Commands ran from the authoritative root unless `-w` specifies otherwise.
The runner was disposable, network-none, read-only root/source/module cache,
2 CPUs/2 GiB memory+swap ceiling, 256 PIDs, separate 768 MiB cache/tmp tmpfs.
Only candidate and owned scratch output mounts were writable. No production
socket, credentials, database or published ports were mounted into fixtures.
The sole socket shared by fixtures was their newly created PostgreSQL socket.

```sh
mkdir -p bin/activation-ready-20261019 bin/activation-checks-20261019 runs/activation-recovery-20261019
docker run -d --rm --name omnidex-activation-runner --network none --memory 2g --memory-swap 2g --cpus 2 --pids-limit 256 --read-only --tmpfs /tmp:rw,exec,size=768m --tmpfs /cache:rw,size=768m -v "$PWD:/src:ro" -v /home/gryph/go/pkg/mod:/go/pkg/mod:ro -v "$PWD/bin/activation-ready-20261019:/candidates:rw" -v "$PWD/bin/activation-checks-20261019:/out:rw" -w /src -e GOMAXPROCS=2 -e GOCACHE=/cache -e GOPROXY=off omnidex-r6-snapshot-debug-builder:local sleep 1800
docker exec omnidex-activation-runner sh -c 'go version; gcc --version; ld --version; go env GOOS GOARCH CGO_ENABLED CC; go build -p 1 -ldflags "-linkmode external -extldflags -static" -o /candidates/omnidex ./cmd/omnidex && go build -p 1 -ldflags "-linkmode external -extldflags -static" -o /candidates/omni ./cmd/omni'
mkdir -p bin/activation-checks-20261019/socket
chmod 777 bin/activation-checks-20261019/socket
docker run -d --rm --name omnidex-activation-db --network none --memory 384m --memory-swap 384m --cpus 1 --pids-limit 128 --read-only --tmpfs /var/lib/postgresql/data:rw,size=256m --tmpfs /var/run/postgresql:rw,size=16m --tmpfs /tmp:rw,size=16m -v "$PWD/bin/activation-checks-20261019/socket:/fixture:rw" -e POSTGRES_HOST_AUTH_METHOD=trust postgres:16-alpine postgres -k /fixture,/var/run/postgresql -h ''
docker exec omnidex-activation-runner go run -p 1 /src/runs/activation-recovery-20261019/candidate-check.go
docker exec omnidex-activation-runner sh -c 'go test -v -p 1 ./cmd/omni -run "TestWorkspace|TestLedgerPanel|TestChat|TestPlanReview" -count=1 -timeout=90s'
docker exec -w /src/internal/client omnidex-activation-runner sh -c 'go test -v -p 1 $(go list -f "{{range .GoFiles}}{{.}} {{end}}" .) chat_transport_test.go session_recovery_test.go session_message_reply_test.go session_turn_mode_test.go mutation_failure_test.go -count=1 -timeout=90s'
docker exec omnidex-activation-runner sh -c 'go test -p 1 ./internal/client -run "^$" -count=1 -timeout=90s'
file bin/activation-ready-20261019/omnidex bin/activation-ready-20261019/omni
bin/activation-ready-20261019/omni version --json
sha256sum bin/activation-ready-20261019/omnidex bin/activation-ready-20261019/omni
stat -c '%n %s' bin/activation-ready-20261019/omnidex bin/activation-ready-20261019/omni
git diff --check
```

Build exit 0 (`build.txt`). `candidate-check.go` is retained reproducible test
harness evidence, not a production source change. It starts the **actual candidate**
with synthetic `activation_fixture` schema and a fresh disposable PG16.14 database,
LISTEN_ADDR `127.0.0.1:18091` inside the network-none runner, an empty fixture HOME,
and no model invocation. It uses the actual production client workspace transport
and session DTO validation, with synthetic client identity
`1234567890abcdef1234567890abcdef` and owned scratch directory `/out/client`.
Results (`candidate-check.txt`, `core.log`, `postgres.log`): exit 0; startup,
`/healthz` and `/readyz` HTTP 200/status ok; server-issued channel bootstrap and
snapshot with exact workspace authority; reconnect reuses the durable channel;
snapshots pass again after graceful candidate stop/start against the same fixture.
The `channel_user_intents` table, index, function and enabled trigger exist.
An exact synthetic provenance row permits its initial receipt completion, rejects
provenance mutation and deletion, and survives candidate-process restart with the
same request and receipt. Existing channel identity also survives. This is actual
candidate-process/DB integration, not a mocked server or a live-schema test.

Terminal/panel/chat/approval result: PASS, 101 top-level tests, 0.947s
(`terminal-tests.txt`). Affected production client files plus the exact five
requested test files: PASS, 10 top-level tests, 0.003s (`client-tests.txt`).
These are isolated synthetic terminal rendering/HTTP/session tests, not interactive
production chat. CLI host identity/load: exit 0, v0.5.0 Charmeleon
(`cli-identity.json`). Core release identity is read from actual `/healthz` and
build info; an attempted unsupported `omnidex version --json` exited 1 with its
`omnidex health [--stdin]` usage, not a candidate startup defect.

The documented full-client-package limitation remains: compilation fails in
unchanged `coding_plan_test.go` on removed Constraints, LedgerConstraint, Priority
and LedgerPriorityNormal fields/types (`obsolete-client-limitation.txt`). It was
reproduced, not repaired; no full-suite pass is claimed. Historical model/queue
obsolete-test limitations remain documented in earlier entries; no unrelated
suites or source repairs were added. Initial PostgreSQL fixture attempts failed
because the owned socket directory denied PostgreSQL writes; disposable attempts
auto-removed, permissions were corrected only on that task directory. Initial
read-only host psql attempts used the default socket; explicit `--dbname` used the
observed native URI successfully. Compiler-helper path lookup was corrected to
GCC's reported libexec paths. Final relevant checks all passed.

Reuse, without rerunning inference: nine-case live high-reasoning parser result
from `runs/task-scope-parser-20261019-attempt2.jsonl`, SHA-256
`34623dc478c2e3cc086d82889bfb09dc57869e77a9e0b1c65fd4dd7ac50ca03e`
(rehashed unchanged). Its scope/limitations remain those of the earlier entry;
it proves those interpretation cases, **not independent panel review**.

### Observed live identities and preservation boundary

Read-only inspection: `omnidex.service`, fragment
`/home/gryph/.config/systemd/user/omnidex.service`, MainPID 146151,
ExecStart `/home/gryph/Projects/tools/omnidex/bin/omnidex` (also `/proc/146151/exe`).
Listen/core URL `127.0.0.1:18090` / `http://127.0.0.1:18090`.
Active core SHA-256 `413a00c336e92a2705a0dd1da651afb92f1cbe9c55df45d5d63c7f0678f1c284`;
active CLI `028f8e6e9e5357ef66e5f1410116900f943c9f73923db2296d4e4d505fa48d8e`.
The launcher found by PATH is `/home/gryph/.omnidex/bin/omni`, resolving to
`/home/gryph/.local/bin/omnidex`. Its managed script defaults to chat only with
no arguments, obtains CORE_URL from `/home/gryph/.config/omnidex/runtime.env`
only if unset, and execs this tree's `bin/omni`. It requires no replacement.
Service/launcher content hashes are recorded in `identities.txt`; no environment
secrets or credentials were printed or retained.

Native DB identity: database `omnidex`, role/schema owner `gryph`, schema
`omnidex_runtime_4ffc6561`; native socket
`/home/gryph/.local/state/omnidex/socket`, port 15432. The observed
`omnidex-postgres.service` points to this tree's `bin/native/usr/bin/postgres`
and retained data `/home/gryph/.local/state/omnidex/postgres`.
Read-only `to_regclass('omnidex_runtime_4ffc6561.channel_user_intents')` is NULL
(`live-identities.txt`). `internal/db/postgres.go` preserves an existing schema
rather than applying setup additions. **Restart alone cannot supply the table.**
No identity, setting, historical ledger or existing data was changed. Live queries
used default_transaction_read_only=on and selected only identities/job status or
frozen assignments, never used production rows as test fixtures.

### Existing additive SQL, verbatim

Authoritative `database/setup.sql` SHA-256
`6c94bd45e607050347ac3198068c32be46ff7fe6b8e8dd51981018d0731c5489`.
The following existing block is the only proposed schema addition; it is not the
whole setup/reset script. It has **not** been applied to the live schema.

```sql
CREATE TABLE channel_user_intents (
    operation_id text PRIMARY KEY CHECK (operation_id ~ '^lifecycle_operation_[a-z0-9_]{1,128}$'),
    channel_id text NOT NULL REFERENCES ai_channels(id),
    workspace_identity text NOT NULL,
    request_json jsonb NOT NULL CHECK (jsonb_typeof(request_json)='object'),
    context_json jsonb NOT NULL CHECK (jsonb_typeof(context_json)='object'),
    result_json jsonb NOT NULL CHECK (jsonb_typeof(result_json)='object'),
    evidence_json jsonb NOT NULL CHECK (jsonb_typeof(evidence_json)='array'),
    receipt_json jsonb CHECK (jsonb_typeof(receipt_json)='object'),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK (request_json->>'operation_id'=operation_id),
    CHECK (request_json->'state'->>'channel_id'=channel_id),
    CHECK (request_json->'state'->>'workspace_identity'=workspace_identity)
);
CREATE INDEX channel_user_intents_channel_created ON channel_user_intents(channel_id,created_at DESC,operation_id);
CREATE FUNCTION preserve_channel_user_intent() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' THEN RAISE EXCEPTION 'User intention provenance is immutable'; END IF;
    IF ROW(NEW.operation_id,NEW.channel_id,NEW.workspace_identity,NEW.request_json,NEW.context_json,NEW.result_json,NEW.evidence_json,NEW.created_at)
       IS DISTINCT FROM ROW(OLD.operation_id,OLD.channel_id,OLD.workspace_identity,OLD.request_json,OLD.context_json,OLD.result_json,OLD.evidence_json,OLD.created_at)
       OR OLD.receipt_json IS NOT NULL THEN
        RAISE EXCEPTION 'User intention provenance and completed receipts are immutable';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER channel_user_intents_immutable BEFORE UPDATE OR DELETE ON channel_user_intents
 FOR EACH ROW EXECUTE FUNCTION preserve_channel_user_intent();
```

### Later owner exceptions — proposed commands only

Only these three activation actions await later owner exceptions:
(1) additive maintenance of the existing live schema, (2) active-binary
replacement, (3) core service restart. No additional approval is required for
this task's candidate preparation, verification, evidence or controller review.
The commands below are a concrete proposal, not executed commands. They preserve
the observed database and binaries before the corresponding action. No launcher,
configuration, identity, job, ledger, data directory or unrelated service is reset.
The retained copy is one rollback set, not another source tree. The three proposed
exceptions should be considered against this exact packet/candidate hash set.

```sh
cd /home/gryph/Projects/tools/omnidex
set -euo pipefail
# After independent review and explicit later activation exceptions only.
# Refuse changed candidates/active inputs before creating preservation outputs.
printf '%s
' \
 '1d3e97017dcb2aa2501b4c1dc6bff90fcd42b28859f3994f72f49c4927679099  bin/activation-ready-20261019/omnidex' \
 'f87c359926871bc727d136f6df84041b62aad4e92a1eff41c1897016a140606a  bin/activation-ready-20261019/omni' \
 '413a00c336e92a2705a0dd1da651afb92f1cbe9c55df45d5d63c7f0678f1c284  bin/omnidex' \
 '028f8e6e9e5357ef66e5f1410116900f943c9f73923db2296d4e4d505fa48d8e  bin/omni' \
 '6c94bd45e607050347ac3198068c32be46ff7fe6b8e8dd51981018d0731c5489  database/setup.sql' | sha256sum -c -
umask 077
mkdir -m 700 bin/activation-preserved-20261019
cp -p -- bin/omnidex bin/activation-preserved-20261019/omnidex
cp -p -- bin/omni bin/activation-preserved-20261019/omni
cmp -- bin/omnidex bin/activation-preserved-20261019/omnidex
cmp -- bin/omni bin/activation-preserved-20261019/omni
# Preserve all schemas/data in the existing database with native local identity.
pg_dump --dbname='postgresql:///omnidex?host=/home/gryph/.local/state/omnidex/socket&port=15432' --format=custom --file=bin/activation-preserved-20261019/omnidex-before-additive.dump
pg_restore --list bin/activation-preserved-20261019/omnidex-before-additive.dump > bin/activation-preserved-20261019/omnidex-before-additive.list
sha256sum bin/activation-preserved-20261019/omnidex bin/activation-preserved-20261019/omni bin/activation-preserved-20261019/omnidex-before-additive.dump > bin/activation-preserved-20261019/SHA256SUMS
# Exception 1: additive table/index/function/trigger, one transaction, existing schema.
# Run with bash; the extracted block is exactly the verbatim block above.
psql --dbname='postgresql:///omnidex?host=/home/gryph/.local/state/omnidex/socket&port=15432' -X -v ON_ERROR_STOP=1 --single-transaction -c 'SET search_path TO omnidex_runtime_4ffc6561' -f <(sed -n '/^CREATE TABLE channel_user_intents (/,$p' database/setup.sql)
# Exception 2: stage and atomically replace the two exact observed active paths.
install -m 755 bin/activation-ready-20261019/omnidex bin/omnidex.activation-next
install -m 755 bin/activation-ready-20261019/omni bin/omni.activation-next
cmp -- bin/activation-ready-20261019/omnidex bin/omnidex.activation-next
cmp -- bin/activation-ready-20261019/omni bin/omni.activation-next
mv -f -- bin/omnidex.activation-next bin/omnidex
mv -f -- bin/omni.activation-next bin/omni
# Exception 3: restart only the observed core service.
systemctl --user restart omnidex.service
# Later activation verification; never an installer/reset or a replan of #17/#18.
systemctl --user show omnidex.service -p MainPID -p ExecStart -p ActiveState
curl --fail --max-time 10 http://127.0.0.1:18090/healthz
curl --fail --max-time 10 http://127.0.0.1:18090/readyz
/home/gryph/.omnidex/bin/omni version --json
sha256sum bin/omnidex bin/omni
psql --dbname='postgresql:///omnidex?host=/home/gryph/.local/state/omnidex/socket&port=15432' -X -v ON_ERROR_STOP=1 -c "BEGIN READ ONLY; SELECT to_regclass('omnidex_runtime_4ffc6561.channel_user_intents'),to_regclass('omnidex_runtime_4ffc6561.channel_user_intents_channel_created'),to_regprocedure('omnidex_runtime_4ffc6561.preserve_channel_user_intent()'); SELECT id,status,current_generation FROM omnidex_runtime_4ffc6561.jobs WHERE id IN(17,18) ORDER BY id; COMMIT;"
```

### Resource accounting, cleanup and controller review handoff

All constraints belong to this single recovery task: measure RAM/swap/disk/scoped
build usage before heavy work; one heavy build/test at a time; disposable Docker
only; runner 2 CPUs/2 GiB and cache/tmp 768 MiB each; PostgreSQL 1 CPU/384 MiB and
256 MiB data; ignored scratch `bin/activation-checks-20261019` <=512 MiB;
candidates <=400 MiB; each command <=10 minutes, each execution/review resource
session <=30 minutes; read-only source/module cache, synthetic fixtures, no
production credentials/data/sockets or published ports. Review must retain the
same limits in its own fresh resource session.
Initial host measurement: RAM available 23,646 MiB; swap 17,129/21,969 MiB used;
disk available 45,307 MiB; existing bin 96 MiB. Owned candidate/scratch directories
and new container cache/tmp started empty. Existing unknown/shared artifacts were
preserved. Observed runner cache <=236 MiB, sampled build tmp <=170 MiB, owned
scratch <=1 MiB; candidate allocation 44 MiB and retained evidence <1 MiB.
The execution runner session lasted under five minutes; all commands finished
within ten minutes, tests have explicit 90s timeouts and harness 150s timeout.
Final available RAM 23,392 MiB, swap used 17,153 MiB, disk available 45,277 MiB;
these are observations, not claims that disk cleanup repaired RAM pressure.

Cleanup actually executed: `docker stop -t 5 omnidex-activation-runner
omnidex-activation-db`; both auto-removed. Exact owned scratch
`bin/activation-checks-20261019` was removed with Python shutil.rmtree.
A subsequent filtered `docker ps -a --filter name=omnidex-activation-` returned
no containers. No owned images, networks or volumes were created. Harness core
children were gracefully stopped; final runner process inspection showed only
its bounded sleep and inspection command. Container cache/tmp/PG data retired
with the containers. Shared caches, unknown bin artifacts, original evidence,
settings and existing services remain intact. Final whitespace check passed.

Independent-review provenance boundary: this is the execution receipt for the
existing job 19 task-local controller, not an independent-review certificate.
Execution evidence 132781 binds this session to task `p_c4c5910e4a75_1_task`.
Return this result through that controller's ordinary execution-response path;
code owns dispatch of its required fresh independent review. The reviewer must
assess the existing panel against the exact original frozen task in evidence
111764 **and** this candidate verification, inspect the source/asset manifests
and candidate hashes above, and append its own call/session/evidence identities,
findings and conclusion to this ledger. Bind every finding/conclusion to these
reviewed bytes; do not treat parser evidence or this executor's checks as panel
review. No substitute reviewer, manually injected review receipt or self-approval
was created. The independent review's result is recorded by that controller and
its dispatched review session after this execution response.

Execution stage complete: candidates built, requested execution checks verified,
activation proposal recorded, disposables cleaned. Local workspace delivery only;
no commit/push requested. Fresh independent review is the next code-owned stage,
not a later owner exception; activation remains unperformed.


## Installed Ledger panel — verification findings, 2026-10-19

This entry completes the requested verification and records concrete findings; it
does not create an acceptance receipt or require a future controller lifecycle
receipt. The controller-dispatched review returns its own findings through its
existing response path. No additional reviewer was invoked.

### Installed identity and retained evidence

The [activation receipt](../runs/activation-recovery-20261019/activation.txt)
records the actual activation and installed CLI smoke check, superseding the older
proposal-only activation status above. Its SHA-256 is
`c2122c41e23e757715fb2290c1746c5437da1154c74ab2f4fe668de7ba115d87`.
Current direct hashing of `bin/omni` returned
`f87c359926871bc727d136f6df84041b62aad4e92a1eff41c1897016a140606a`;
`bin/omnidex` returned
`1d3e97017dcb2aa2501b4c1dc6bff90fcd42b28859f3994f72f49c4927679099`.
Both match that receipt. No installation or process restart was performed here.

Reviewed workspace HEAD is `3b252b684449447fd84bc122bace2ae494b11bf1`
with pre-existing owner modifications/untracked source preserved. HEAD alone does
not identify these bytes: a Python SHA-256 comparison of **all 1,843 entries** in
[source-manifest.json](../runs/activation-recovery-20261019/source-manifest.json)
found zero changed or missing files. Manifest SHA-256:
`0404d1bfe9b1f40f3f5b0613605178ceb0b526718e1656c5eb9d9d8d9245e186`.
That manifest identifies the exact reviewed production source and tests, including
`cmd/omni/chat_workspace{,_keys,_layout,_ledger}.go`,
`cmd/omni/chat_ledger_panel.go`, `cmd/omni/chat_plan_review_mutation.go`,
`internal/client/coding_ledger.go`, `internal/api/coding_plan.go`, and
`internal/queue/coding_plan_mutation.go`.
For direct panel source anchors, SHA-256:

| Source | SHA-256 |
| --- | --- |
| `cmd/omni/chat_workspace_ledger.go` | `6b490b8d7df69c2c013d774c76cc1baf4a157667a0eed6b6c5f0ac4520da1b34` |
| `cmd/omni/chat_ledger_panel.go` | `a5775a2121ab7ed9d3261d36a5e33373a8fb9b830d5c442f2f7563fff1845169` |
| `cmd/omni/chat_workspace_keys.go` | `091f76e50a20be0536482aef71997343255cabcbbd38ca7cb2565e52f4ed0f3e` |
| `cmd/omni/chat_workspace_layout.go` | `5b5c94b5417b790afd5953b25eb01a73a51606922955abdc8bbecd7471d122f7` |

Retained [terminal results](../runs/activation-recovery-20261019/terminal-tests.txt)
SHA-256 `5f2b5cd2f7a6276200c29bef7147fdf1ac31e294f714ed2a11779f5ce9c44257`
show 101 top-level tests PASS, package time 0.947s, using
`go test -v -p 1 ./cmd/omni -run 'TestWorkspace|TestLedgerPanel|TestChat|TestPlanReview' -count=1 -timeout=90s`.
The [client results](../runs/activation-recovery-20261019/client-tests.txt)
retain 10 passing selected transport/session tests (0.003s).
No changed source or unresolved failed panel observation invalidated these runs;
therefore no existing test was repeated and no Docker fixture was needed.

### Concrete findings

- **Bounded labels and complete details: PASS.** `ledgerLinesLocked` collapses
  paragraph whitespace only for ordered labels, truncates each label to content
  width with an ellipsis, and separately renders the selected stable identity,
  original goal, review state, disposition, rationale, provenance kind/reference/
  revision, rank, and every read/write/delete path. Empty file groups explicitly
  show `(none)`. Selected details wrap rather than using the label truncation.
  `TestWorkspaceLedgerDetailsSelectionRefreshAndNarrowResize` and
  `TestWorkspaceLedgerLongGoalsRenderedNavigation` passed. The latter compares
  the exact wrapped multiline goal and checks every nonempty selected detail row
  is reachable in actual rendered frames, not merely present in backing data.
- **Visible keyboard selection and long-goal scrolling: PASS.** The retained
  viewport fixture traverses 45 multiline goals in both directions at 32x12 and
  120x24, checks the visible `>` selection and recognizable bounded following
  labels, and resizes at first/middle/last selections. `j/k` and decoded arrow
  keys select; PgUp/PgDn scroll the detail viewport. Navigation reveals its row
  without preventing manual detail scrolling. Resize preserves manual scrolling
  and follows a previously visible selection. The installed read-only smoke
  check in the activation receipt additionally observed job17 revision2,
  recognizable long rows and `j` selection through the normal CLI.
- **Switching, draft, cursor and navigation: PASS.** The retained real-editor
  fixture starts with `ab`, moves the cursor left, inserts `X`, switches Ledger/
  Activity/Compose, zooms, opens/closes help, receives streamed output and refreshes
  the ledger across 38x12 and 120x28 sizes. Inserting `Y` after returning produces
  exactly `aXYb`; selected `panel-second` and the activity event survive.
  `TestWorkspaceLedgerKeepsActualEditorDraftCursorStreamAndNavigation` passed.
  Additional resize frames cover 120x30 down to 1x1 and back to 80x24 without
  width overflow; 1x1 is bounded rendering evidence, not readable-detail evidence.
  The activation smoke independently retained an unsent draft across pane changes.
- **Direct controls without duplicate approval: PASS.** `a`, `d`, `x`, `s`, `[`,
  `]` dispatch agree/discuss/remove/restore/reorder directly through the existing
  decisions endpoint. The retained HTTP integration observes exactly one initial
  read and six mutations for six controls, with no confirmation request or second
  approval. Display changes use the returned server ledger, including a synthetic
  server-changed rationale, rather than optimistic edits. Reorder uses rank then
  stable identity; first/last, zero-rank ties and maximum-rank boundaries passed.
  The existing separate freeze/signoff flow remains separate from item controls.
- **Authority, state and revision: PASS within the reviewed boundaries.** Panel
  requests carry displayed job/generation/revision and selected stable ID; the
  session rejects changed active jobs/generations and unresolved competing work.
  The client carries workspace root/identity, the API requires that workspace
  identity for the job, and repository mutation locks the ledger and checks exact
  base revision and allowed state before applying a new mutation. The panel is
  read-only outside `proposed` (the repository also supports `revising` for its
  existing callers); tombstoned items permit restore only. Returned job,
  generation, operation, base and next revision and ledger validity are checked.
  Same-authority snapshots cannot regress or change content without revision.
  Retained tests reject foreign/malformed responses and frozen controls. Conflict
  refreshes current state without rebasing the attempted mutation. An arbitrary
  403 remains unresolved under the existing shared-client classification; its
  permission error is visible and pending retry authority is retained. Failed
  reads clear loading and expose refresh failure. These are isolated HTTP/client
  and source-review findings, not a new live database mutation test.
- **Ambiguous retry identity and payload: PASS.** The retained test drops the
  first control connection, observes revision3 still displayed and pending state,
  blocks a competing remove, then uses `t` to retry. It compares the complete
  decoded bodies with `reflect.DeepEqual`: operation ID, owner-turn ID/text,
  revision, workspace root/identity and full action all remain identical. Exactly
  two mutation attempts resolve to revision4 and clear pending state. Source
  review confirms the same pending object is reused, response mismatch retains
  unresolved authority, and repository replay checks operation/base/owner/action/
  text/payload before returning the persisted revision with `Applied=false`.
  This establishes fixture transport behavior and existing repository mechanics;
  no ambiguous retry was induced against an owner job.

### Result, limitations and cleanup

No discrepancy requiring a panel source change was found in the requested scope.
The installed binary identity, retained isolated checks and concrete source
findings cover the requested behavior. Installed interactive evidence is limited
to the activation receipt's read-only smoke; mutation, exhaustive resize and
cursor evidence comes from the retained isolated fixtures. The previously recorded
full-client-package compile limitation in obsolete `coding_plan_test.go` remains;
this entry does not claim a full repository suite pass. Parser inference evidence
is not used as panel verification.

This pass changed only this ledger by appending findings. Jobs17/18/19, owner
source/settings/identities, prior evidence and the private preservation set were
not written or used as mutation fixtures. No nested chat, schema change, source
copy, new test artifact, service restart or binary replacement occurred. Scoped
artifact inspection found the prior activation scratch absent, retained activation
evidence 404 KiB, and no `omnidex-ledger-panel*` or `omnidex-activation*` containers.
No disposable resource was created. Existing shared artifacts were preserved.
`git diff --check` passed before this append; final content/whitespace validation
is recorded by the verification command for this entry.

Stages: existing implementation verified; findings delivered locally in this
ledger; no commit/push requested; task resources clean. No further receipt is a
prerequisite for these findings.

### Independent recheck of these findings, 2026-10-19

Recomputed the [activation receipt](../runs/activation-recovery-20261019/activation.txt)
and both installed binary hashes: all match the identities recorded above.
Executed SHA-256 comparisons against every entry in the retained
[source manifest](../runs/activation-recovery-20261019/source-manifest.json)
and [asset manifest](../runs/activation-recovery-20261019/asset-manifest.json):
1,843 source and 26 asset entries match, with zero missing or changed files.
The reviewed source remains HEAD `3b252b684449447fd84bc122bace2ae494b11bf1`
plus the exact manifest-bound owner changes; no implementation was edited.

Independently inspected the assertions in
[workspace fixtures](../cmd/omni/chat_workspace_ledger_test.go) and
[HTTP integration fixtures](../cmd/omni/chat_ledger_panel_test.go), their retained
PASS output, and the corresponding production paths:

- [Ledger rendering and navigation](../cmd/omni/chat_workspace_ledger.go) keep
  ordered labels bounded and render the complete selected identity, goal, review,
  disposition, rationale, provenance, rank and file groups separately. Retained
  viewport checks cover all 45 long-goal selections in both directions and every
  selected detail row. Switching/resize checks retain selection and scrolling;
  the real editor check proves retained draft and middle cursor with `aXYb`.
- [Panel dispatch and reconciliation](../cmd/omni/chat_ledger_panel.go) use the
  existing direct endpoint. Six fixture controls produce six mutations without
  another approval; returned server content supplies the displayed state.
  Displayed job/generation/revision, frozen/disposition guards, pending-operation
  exclusion, workspace identity, response validation and repository revision/state
  checks agree with the findings above.
- [Pending operation handling](../cmd/omni/chat_plan_review_mutation.go) reuses
  the unresolved operation. The ambiguous-transport fixture compares the complete
  decoded retry body, blocks a competing control, and resolves two attempts to
  one returned revision. Repository replay compares the stored owner/action/text
  and mutation payload. No owner job was a mutation fixture.

Parsed retained output: 101 top-level terminal tests and 10 selected client tests
PASS, with no FAIL entries; both output hashes match the preceding entry.
No changed source or specific unresolved observation invalidated those checks,
so none was repeated. This recheck found no discrepancy in the recorded requested
behavior. Installed interactive evidence remains the activation smoke; exhaustive
mutation/editor evidence remains isolated-fixture evidence. The retained obsolete
client-test compile failure is unchanged, and no full-suite pass is claimed.

Evidence-link resolution and `git diff --check -- docs/EVIDENCE_LEDGER.md` passed.
Only this append was written; prior ledger bytes, owner changes, jobs17/18/19,
settings, identities, evidence and the private preservation set were preserved.
Prior activation scratch is absent and filtered Docker inspection found no panel
or activation containers. No disposable resources or nested chat were created.
No installation, binary replacement, schema change or service restart occurred.
Existing implementation verified; findings delivered locally; cleanup complete.
These are concrete review findings, with no additional acceptance receipt or
future controller receipt prerequisite.

## Same-job recovery — job 21 execution, 2026-10-19 (in progress)

Authority: existing tree `/home/gryph/Projects/tools/omnidex`, HEAD
`3b252b684449447fd84bc122bace2ae494b11bf1` plus preserved owner changes;
job21 generation1 step112 task `p_2cc9345103a0_1_task`, dispatched execution
call146911 (read directly from PostgreSQL). No nested session is launched.
Scope: repair retained-authority admission and reconciliation, isolated verification,
and this append. Installed runtime, historical jobs, private rollback set untouched.
Execution session owns disposable `omnidex-recovery-runner` and
`omnidex-recovery-db`, scratch `bin/recovery-checks-20261019` (<=512 MiB,
remove before finish). Runner: 2 CPUs/2 GiB, 256 PIDs, cache/tmp tmpfs 768 MiB
each, lifetime 1800s; PG: 1 CPU/384 MiB, 128 PIDs, data tmpfs 256 MiB,
socket/tmp 16 MiB each. Both --rm, --network none, read-only root; source and
module cache read-only, only synthetic socket/scratch writable. Commands <=10min,
Go -p1/GOMAXPROCS2, one heavy operation at a time. Retire both containers and
scratch on completion. No fixture uses production sockets, credentials, ports,
data or volumes. Initial available RAM24195 MiB, swap18048/21969 MiB used,
disk45090 MiB available, existing bin353 MiB/runs458 MiB; shared artifacts preserved.

### Implemented recovery and final verification

Frozen execution interruption now pauses the same step/generation and terminalizes
its worker attempt as waiting, keeping the frozen ledger and task authority.
The explicit `execution.resume` action is offered through the existing generic
intent registry only for the latest recoverable failed/paused job with its
unchanged frozen revision. Execution rechecks actor, state and revision under the
existing locks and persists the existing idempotent intent receipt. Failed steps
and attempts remain historical; a matching same-generation recovery step is
appended. Cancellation, completed ledgers and newer jobs cannot be revived.
Unrelated new instructions still use the existing new-job boundary.

Controller projection retains execution **and independent review** session IDs.
Completed native results are parsed without redispatching their execution/review;
a parsed completed review is reconciled without repeating either parser or
native calls. Native reconnect uses `thread/resume` + `thread/turns/list`, never
`turn/start` for a completed or still indeterminate original turn. A confirmed
terminal interrupted/failed turn records its idle identity; explicit owner resume
then continues partial work in that same thread with an instruction to inspect
retained results and avoid repeating completed effects. Unknown/in-progress
turns remain indeterminate and never gain replay authority. Review starts its own
fresh thread initially and retains that review identity solely for recovery.
Original task-local permission grants remain unchanged.

Necessary schema guards were changed only in `database/setup.sql`, and exercised
only by fresh disposable schemas. They bind recovery/pause exceptions to frozen
ledger, exact job/generation, owner intent/revision/workspace or interrupt
operation and terminalized attempt. No live schema was changed. The installed
runtime therefore remains the preserved installed version, not these source bytes.

Commands executed (runner definition/resource limits above; all source mounts ro):

```sh
docker exec -w /src/internal/queue omnidex-recovery-runner sh -c 'go test -v -p 1 $(go list -f "{{range .GoFiles}}{{.}} {{end}}" .) user_intent_fixture_test.go coding_recovery_integration_test.go -run "TestFrozenCodingRecovery" -count=1 -timeout=150s'
docker exec omnidex-recovery-runner go test -p 1 ./internal/codingagent ./internal/codex -count=1 -timeout=90s
docker exec -w /src/internal/client omnidex-recovery-runner sh -c 'go test -v -p 1 $(go list -f "{{range .GoFiles}}{{.}} {{end}}" .) chat_transport_test.go session_recovery_test.go session_message_reply_test.go session_turn_mode_test.go mutation_failure_test.go -count=1 -timeout=90s'
docker exec omnidex-recovery-runner go test -p 1 ./cmd/omni -run 'TestWorkspace|TestLedgerPanel|TestChat|TestPlanReview' -count=1 -timeout=90s
docker exec omnidex-recovery-runner go build -p 1 -o /out/omnidex-check ./cmd/omnidex
git diff --check
```

Final PostgreSQL recovery suite: PASS, 9.979s. Each of six independent fresh
schemas retains job1/generation1: pause keeps step2; execution-parser,
review-parser, execution disconnect, review disconnect and interrupted partial
continuation append recovery step3 while step2 stays failed. Native identities
are `original-execute/turn-execute` and `original-review/turn-review`; partial
continuation preserves its thread and advances to `continued-execute` once.
Every fixture observes exactly `execute:1 review:1` completed effects, unchanged
retained file bytes, identical duplicate resume receipts, no calls for accepted
retry, accepted task projection, and **completed** final job1/generation1.
Execution-parser calls are 2 only for execution-parser failure; review-parser
calls are 2 only for review-parser failure; all other parser counts are 1.
Five boundary cases PASS: cancelled, completed, foreign workspace, newer job,
and immutable saved context. Direct revival of completed/cancelled jobs fails.
Paused server snapshot reconnect retains the original active job and generation1
interrupt control. Client validation now accepts that same-generation pause.

Controller/native packages PASS (0.006s/0.003s). New reconciliation interruption
test observes exactly four initial provider calls (execution, parser, review,
parser); both reconciliation retry and accepted duplicate leave that count four.
Protocol fixture checks execute/review × completed/inProgress/interrupted with
exact original thread/turn and no new dispatch. Completed responses are retained;
interrupted responses retain an idle continuation identity; in-progress responses
remain incomplete. Native permission and independent initial-review protocol tests
PASS. These are scripted protocol/runtime and real PostgreSQL/file-effect
fixtures, not a claim of live-model recovery judgment.
Selected client transport/recovery tests PASS (0.004s); selected terminal/editor/
ledger-panel tests PASS (0.948s). Core build PASS. No panel rendering, reusable
interpreter implementation, permission configuration or installed asset changed.

Additional existing intent integration checks ran with the same selected-file
command and `user_intent_integration_test.go`: approval/replay, removal approval,
stale/foreign actor and recent-context cases PASS before a fixture loss. The
remaining `TestUserIntentSharedTypedLedgerControlsStillCommit`,
`TestUserIntentParserFailurePersistsAndNeverExecutes`, and
`TestUserIntentInterruptionNeverRestartsInferenceOrEffects` were rerun with
`-run` selecting those exact names: all PASS, 2.482s.

Limitations/failures investigated: full queue test-package compilation fails in
pre-existing obsolete tests (`coding_agent_carry_integration_test.go` expects three
completion-count results; other tests reference removed `Task.Acceptance`,
`Task.Checks`, `codingagent.Check`, `ledger.Objectives`, `Controller.Requirements`).
They were preserved, and verification compiled **all production queue files**
with only the relevant fixtures. Full repository suite is not claimed. During the
broader intent run the first disposable PG connection returned unexpected EOF,
then recovery-mode errors; that --rm container was subsequently absent. Its exit
cause was not retained, so OOM/disk causation is not asserted. Recreated only the
owned fixture with the same 384 MiB/256 MiB limits and explicit
`shared_buffers=16MB max_connections=20 max_wal_size=32MB min_wal_size=32MB
checkpoint_timeout=30s`; affected cases and final recovery suite passed.
The final PG sample was 130.7 MiB resident/container usage with data later sampled
104/256 MiB. Runner cache sampled337.4 MiB, tmp0, scratch32 MiB, all within limits.
A protocol assertion was corrected to count frame evidence separately from the
new idle-session event; final protocol suite passes both exact counts.

Changed-byte provenance: 24 entries differ from the preserved 1,843-entry
activation source manifest, with no missing entries; all differences are the
recovery paths listed below. Four task-owned source/test files were added.
SHA-256 of sorted `path + NUL + file_sha256 + newline` across these 28 files:
`2a4e964edfb05f40a2b9f75e59e8c03cb4ece412b7b7f986068bbe566848cfa3`.

```text
database/setup.sql
internal/client/session.go
internal/client/session_recovery_test.go
internal/codex/agent.go
internal/codex/agent_prompt.go
internal/codex/agent_recovery.go
internal/codex/agent_recovery_protocol_test.go
internal/codex/agent_resume.go
internal/codex/manifest_protocol_test.go
internal/codingagent/assignment_context.go
internal/codingagent/controller.go
internal/codingagent/dispatch.go
internal/codingagent/execution_session.go
internal/codingagent/reconciliation_test.go
internal/codingagent/review.go
internal/codingagent/review_state.go
internal/codingagent/task.go
internal/codingagent/task_interruption.go
internal/queue/channel_session_controls.go
internal/queue/coding_recovery.go
internal/queue/coding_recovery_integration_test.go
internal/queue/lifecycle_replan_replay.go
internal/queue/repository_interrupt.go
internal/queue/repository_job_reads.go
internal/queue/user_intent.go
internal/queue/user_intent_actions.go
internal/queue/user_intent_execute.go
internal/sessionintent/contracts.go
```

Installed binary recheck (unchanged): `bin/omni`
`f87c359926871bc727d136f6df84041b62aad4e92a1eff41c1897016a140606a`;
`bin/omnidex`
`1d3e97017dcb2aa2501b4c1dc6bff90fcd42b28859f3994f72f49c4927679099`.

### Cleanup and dispatched review handoff

Execution runner identity
`3695a857d1e81e99c29dedfd7142a1fb85bbbd38e3132fb0625446d0462e6081`,
started09:41:19 UTC; replacement fixture DB identity
`b7c42bec4a973454e95d1437cca381ee194db18cb51431acb4ff366cc9d6a3b8`,
started09:46:52 UTC. Each command completed within its ten-minute bound, the
execution resource session within30 minutes. `docker stop -t 5
omnidex-recovery-runner omnidex-recovery-db` stopped both auto-removing containers.
The first immediate listing briefly showed the runner Dead during automatic
removal; the subsequent `docker ps -a --filter name=omnidex-recovery-` returned
no containers. Python `shutil.rmtree('bin/recovery-checks-20261019')` removed only
the exact task-owned scratch and candidate; subsequent existence check false.
No images, networks or volumes were created. Container caches, PG data, test
roots and processes retired with the containers; shared artifacts were preserved.
Final available RAM24285 MiB, swap18177/21969 MiB used, disk45061 MiB available.
These are measured observations, not a claim of repairing host memory pressure.
Final `git diff --check` passed, including the appended evidence.

Stages: implementation exists; isolated execution checks pass; delivered locally
in the authoritative tree and this existing evidence ledger; task resources clean.
No commit/push, installation, live schema mutation, runtime restart, historical
owner-job mutation, private rollback-set change or nested Omnidex session occurred.
This is job21's dispatched execution result, not its independent review. The
existing controller owns dispatch of the required fresh independent review after
this response, using the same frozen task and authoritative tree. That reviewer
must run its own bounded verification and append its findings through the
existing evidence paths; no substitute reviewer or self-approval was created.


## Frozen authority and role-correct outcomes — job22 task1, 2026-10-19

Authority: `/home/gryph/Projects/tools/omnidex`, HEAD
`3b252b684449447fd84bc122bace2ae494b11bf1` plus preserved owner changes;
PostgreSQL ledger20 frozen revision7, job22 generation2 dispatched execution
call154248, task `p_821e1757350d_3_task`. This is that execution assignment,
not the pilot or independent review; no additional Omnidex session was launched.
Global constraint retained: “Preserve failed job21 generation1, ledger19 frozen
revision6, and evidence149667–149669 unchanged.” Only task1 was implemented.

Native role instructions now describe the current execution/review result and
its subsequent code-owned transition explicitly. The existing parser's bounded
scan instructions evaluate execution or independent review against the same
frozen task. Later independent review/acceptance cannot be prerequisites for
execution's enabling response; later correction routing/acceptance cannot be
prerequisites for review's response. Actual required-check failures, essential
questions, permission requests, unresolved meaning and concrete findings retain
their existing routes. No text filtering, phrase heuristic, recap rewriting,
self-approval or alternate acceptance path was added. Existing permission code
is unchanged: protocol checks verify execution AND review receive full task
grants at thread/start and turn/start, while planning remains read-only.

Read-only capture of evidence149321 supplied job21's exact task and response;
the response matches evidence149668 notes[0]. The new
`testdata/parser-sieve/job21-execution.json` retains both unchanged strings and
records the rejected classification `error` / “Required independent review
remains pending” as regression provenance, never as model input. Corpus cases
include this report, execution before review, review before acceptance, and an
actual required review-protocol check failure. Retained job14/job15 fixtures
were not edited. Expected outcomes are test oracles outside model context.

Changed files for this task (pre-existing changes in these files preserved):
`internal/codex/agent_prompt.go`, `internal/codex/manifest_protocol_test.go`,
`internal/codingagent/parser_sieve_contract.go`,
`internal/codingagent/parser_execution_scope_test.go`,
`internal/codingagent/parser_review_scope_test.go`,
`internal/codingagent/parser_review_routing_test.go`, new
`internal/codingagent/parser_authority_retry_test.go`, new job21 fixture,
`testdata/parser-sieve/cases.json`, and `docs/CODING_RESPONSE_PARSER.md`.
SHA256 of sorted `path + NUL + file_sha256 + newline` across those ten files:
`516d7710458ba3ef670bad1ec8d576491459200f67fe7f4557c8a5df2a0efa3b`.
This append and the linked original live-model receipts are the evidence outputs.

### Current verification

Execution-owned scratch `bin/authority-checks-20261019` budget <=512 MiB,
cleanup before completion. Runner `omnidex-authority-runner`, image
`omnidex-test-builder:local`, ID
`5c8df861cde24fcaa9123284f8780b4eef4c3878e4a0ba0f0be77ba584e4c932`,
started10:21:03 UTC: --rm, --network none, read-only root/source/module cache,
2 CPUs/2 GiB, 256 PIDs, /tmp512 MiB and /cache768 MiB tmpfs, lifetime1200s.
Fixture DB `omnidex-authority-db`, image `postgres:16-alpine`, ID
`8a1b631139c18e2b704481b57bd5bd68b0d85fea2f893e4799a407252da73760`,
started10:22:05 UTC: --rm, --network none, read-only root, 1 CPU/384 MiB,
128 PIDs, data256 MiB/socket16 MiB/tmp16 MiB tmpfs. Its only host mount was
scratch/socket; trust auth was synthetic and socket-only. PostgreSQL arguments:
`-k /fixture,/var/run/postgresql -h '' -c shared_buffers=16MB
-c max_connections=20 -c max_wal_size=32MB -c min_wal_size=32MB
-c checkpoint_timeout=30s`. No production credentials/data/volumes were mounted.
Source mounted at /src:ro; host module cache at /go/pkg/mod:ro;
GOMAXPROCS2, GOCACHE=/cache, GOPROXY=off. One heavy command at a time.

Commands (all exited0 unless the bounded export attempts described below):

```sh
docker --context default exec omnidex-authority-runner go test -p 1 ./internal/codingagent ./internal/codex -count=1 -timeout=120s
docker --context default exec omnidex-authority-runner go test -v -p 1 ./internal/codingagent ./internal/codex -run 'TestFrozenExecution|TestFrozenReview|TestRoleScopedParserRetry|TestManifestRoleSettings|TestReconcileParsed|TestReviewUnknown' -count=1 -timeout=120s
docker --context default exec -w /src/internal/queue -e 'OMNI_TEST_DATABASE_URL=postgres://postgres@/postgres?host=/src/bin/authority-checks-20261019/socket&sslmode=disable' omnidex-authority-runner sh -c 'go test -v -p 1 $(go list -f "{{range .GoFiles}}{{.}} {{end}}" .) user_intent_fixture_test.go coding_recovery_integration_test.go -run TestFrozenCodingRecoverySameJob -count=1 -timeout=150s'
docker --context default exec omnidex-authority-runner go test -v -p 1 ./internal/codingagent ./internal/codex -count=1 -timeout=120s
docker --context default exec -e CGO_ENABLED=0 omnidex-authority-runner go test -c -o /tmp/authority-codex.test ./internal/codex
docker --context default exec omnidex-authority-runner cat /tmp/authority-codex.test > bin/authority-checks-20261019/codex.test
```

Final two-package scripted suite: 24 top-level PASS, 5 opt-in installed/live
checks SKIP, zero FAIL. This includes the new job21 and pending-review cases,
retained job14/job15 cases, genuine failure, essential input, permission,
unknown meaning, exact finding-to-correction routing, fresh review startup,
thread/turn grants, and reconciliation. The new parser-retry test fails either
parser once, then retries and repeats accepted reconciliation: execute1/review1,
unchanged file bytes and original execution response. Real PostgreSQL fixture:
PASS5.546s, six cases (pause, execution-parser, review-parser, disconnect,
review-disconnect, interrupted); every case retains job1/generation1 and exact
file bytes, executes each native effect once, and preserves original identities.
Only the failing parser has two invocations; duplicate resume and accepted
replay repeat no effects. These are scripted semantic/native-runtime fixtures,
not live-model recovery evidence. All production queue files were compiled with
the named fixtures; no full queue-test-package or repository-wide pass is claimed.
Final scripted output SHA256
`aa15e3ee1d17b9f1d8738017cc5dcc8f88bf8a1e1b6b0deb0bf0bf3c6e88e547`;
PostgreSQL output SHA256
`b78d6f49ea2141eab4c7a12f94f7d0541fb77946b449e115c8b1411b7a24b5ca`.

Focused **live-model** qualification used that current source-built static test
binary on the host through the existing production parser adapter and normal
Codex sign-in, without copying credentials or installing a binary. Working
directory `internal/codex`; TMPDIR and all temporary test roots inside the
above scratch. `OMNI_TEST_PARSER_SIEVE=1`,
`OMNI_TEST_CODEX_COMMAND=/home/gryph/.local/share/mise/shims/codex`,
`OMNI_TEST_CODEX_MODEL=gpt-6.1-sol`, default effort low;
`OMNI_PARSER_SIEVE_EVIDENCE` pointed to scratch/live.jsonl. Command:

```sh
timeout 180s /home/gryph/Projects/tools/omnidex/bin/authority-checks-20261019/codex.test -test.v -test.run '^TestInstalledParserSieveCurrentCorpus$/(job21-execution-before-independent-review|review-before-code-owned-acceptance|execution-later-review-pending|execution-required-review-protocol-check-failed)$' -test.timeout=170s
```

All four PASS, total40.91s: job21 done12.34s, review before acceptance done16.84s,
execution before review done6.42s, actual required protocol failure error5.31s.
No prompt/source changes or retries during this live run. Exact original frames,
source dispositions and separately marked code-rendered envelopes retained in
[raw live-parser evidence](evidence/frozen-authority-parser-2026-10-19.raw-model-evidence.jsonl.gz):
250019 raw bytes, 19251 compressed bytes, artifact SHA256
`0eee28a6b6c0fd194133549ba082102b1cd6e1a435e88fbee429fca67410eb8b`.
Live stdout SHA256
`9ae31352bb3334c61f5d85d95123aa0dd85b7956e6fbc69c57652d2267096146`.
This is finite parser-inference evidence, not a claim of universal semantic
accuracy or a substitute independent implementation review.

Bounded export failures: Docker cp could not read the test binary from container
tmpfs; an initial relative host output path also failed. No inference ran in
those attempts. Streaming the binary with docker exec cat resolved the export;
the resulting live test above passed. No policy or assertion was relaxed.

### Preservation and cleanup

Before/after read-only PostgreSQL `md5(row_to_json(...))` comparisons are equal:
job21 `dc1a83062d9d099021c75ffa2490ec21`, ledger19
`9e8a0752ff32cbbc6a07c322ec535f2a`; ordered evidence149667–149669 aggregate
`39099ef97b715f4818149932a2c10102`. Job21 remains failed/generation1 and ledger19
frozen/revision6. No SQL mutation was issued against the authoritative database.
Installed bin/omni SHA256 remains
`f87c359926871bc727d136f6df84041b62aad4e92a1eff41c1897016a140606a`;
bin/omnidex remains
`1d3e97017dcb2aa2501b4c1dc6bff90fcd42b28859f3994f72f49c4927679099`.

Initial available RAM24383 MiB, swap18306/21969 MiB used, disk45053 MiB free;
final available RAM24571 MiB, swap18475/21969 MiB used, disk45042 MiB free.
Sampled runner305.8 MiB/DB127.2 MiB; cache192 MiB/tmp0, host scratch10.46 MiB.
`docker --context default stop -t 5 omnidex-authority-runner omnidex-authority-db`
completed; filtered subsequent container listing empty. No task native process
remained in the scoped process check. Exact scratch removed with shutil.rmtree,
absence verified. No images/networks/volumes, persistent services, source copies,
worktrees or extra source trees created. Existing shared caches, owner changes,
settings, identities, historical evidence and private rollback set preserved.
No live-schema change, installation, restart, reset, commit or push performed.

Stages: implemented; verified by current scripted/protocol/PostgreSQL checks and
four focused live-parser cases; delivered locally in the authoritative tree;
task resources cleaned. Omnidex owns the subsequent fresh independent review
and acceptance transitions after this execution response.

### Independent review — job22 task1, 2026-10-19

Fresh Omnidex-dispatched independent review call155916, native thread
`01a153b3-8515-7f80-a239-134e62637b9d`, ledger20 frozen revision7,
task `p_821e1757350d_3_task`. Read-only PostgreSQL inspection confirmed that
assignment and the exact supplied instructions. Review was limited to task1.
Global constraint retained: “Preserve failed job21 generation1, ledger19 frozen
revision6, and evidence149667–149669 unchanged.” No implementation was modified.
The ten-file source digest remains
`516d7710458ba3ef670bad1ec8d576491459200f67fe7f4557c8a5df2a0efa3b`.

Independent inspection confirmed role-specific native instructions and parser
leaves use the frozen task, retain original response bytes, distinguish later
code-owned transitions from actual required checks, and leave findings,
essential input, permission, unknown meaning and parser retry on their existing
routes. No recap rewriting, evidence filtering, source self-approval or alternate
acceptance route was found. The job21 corpus task/response equal the saved
fixture, which exactly equals evidence149321's frozen assignment and
evidence149668 notes[0]. Its historical rejected classification is preserved.

Review-owned scratch `bin/authority-review-checks-20261019`, budget512 MiB,
held only disposable socket, test binary and outputs; observed footprint12 MiB.
Runner `omnidex-authority-review-runner`, ID
`19aa46d89097976b5fc71a218f7c9bb5337f7e740258d70f2e217d3dd9cef1a8`,
started10:28:24 UTC, used existing `omnidex-test-builder:local`: --rm,
--network none, read-only root/source/module cache, 2 CPUs/2 GiB with no swap,
256 PIDs, /tmp512 MiB and /cache768 MiB tmpfs, sleep1200s lifetime.
Fixture `omnidex-authority-review-db`, ID
`b4a949b48f107d551e1958d6aa556a27f86d7071249d7f1b9a7666de0ee371e6`,
started10:29:14 UTC, used existing `postgres:16-alpine`: --rm, --network none,
read-only root, 1 CPU/384 MiB with no swap, 128 PIDs, PG data256 MiB and
socket/tmp16 MiB tmpfs. Its only host mount was the review-owned socket;
synthetic trust authentication and socket-only PostgreSQL used the same
`-k /fixture,/var/run/postgresql -h ''` and bounded PG settings as execution.
No production data, credentials, socket, published ports or Docker socket were
mounted into fixtures. Source was /src:ro; module cache /go/pkg/mod:ro;
GOMAXPROCS2, GOCACHE=/cache, GOPROXY=off; one heavy command at a time.

Current commands, all exit0:

```sh
docker --context default exec omnidex-authority-review-runner go test -v -p 1 ./internal/codingagent ./internal/codex -count=1 -timeout=120s
docker --context default exec -w /src/internal/queue -e 'OMNI_TEST_DATABASE_URL=postgres://postgres@/postgres?host=/src/bin/authority-review-checks-20261019/socket&sslmode=disable' omnidex-authority-review-runner sh -c 'go test -v -p 1 $(go list -f "{{range .GoFiles}}{{.}} {{end}}" .) user_intent_fixture_test.go coding_recovery_integration_test.go -run TestFrozenCodingRecoverySameJob -count=1 -timeout=150s'
docker --context default exec -e CGO_ENABLED=0 omnidex-authority-review-runner go test -c -o /tmp/authority-review-codex.test ./internal/codex
docker --context default exec omnidex-authority-review-runner cat /tmp/authority-review-codex.test > bin/authority-review-checks-20261019/codex.test
```

Scripted/protocol suite: 24 top-level PASS, 5 opt-in SKIP, zero FAIL. Startup
protocol assertions cover execution AND review grants at thread/start and
turn/start, fresh review identity and planning read-only permissions. Routing
checks cover job21/job15 execution, job14 review, later transitions, genuine
required failures, essential questions, permission, concrete finding/correction,
unknown interpretation, original responses and retry without repeating effects.
Real PostgreSQL fixture: six cases PASS, total5.470s. Both parser-failure cases
retain job1/generation1, original native identities, original output bytes and
execute1/review1; only the failing parser runs twice. Accepted reconciliation
and duplicate retry repeat no completed effect. The named existing fixture also
passed pause, disconnect, review-disconnect and interruption cases. These are
scripted semantic/native-runtime fixtures, not live-model recovery evidence.
No repository-wide or full queue-test-package pass is claimed. Scripted stdout
SHA256 `aa15e3ee1d17b9f1d8738017cc5dcc8f88bf8a1e1b6b0deb0bf3c6e88e547`;
PostgreSQL stdout SHA256
`c486ec7b47667973d1f2e5b5ff85f0bc0df6cb4c9551ff52f30d89db1a91d21e`.

Separate **live-model** checks used the source-built static test binary through
the existing production parser adapter, ordinary Codex sign-in, one case at a
time, model gpt-6.1-sol/effort low. Working directory internal/codex; no source
or prompt changes, retries, credential copies, installation or nested Omnidex.
Exact command:

```sh
OMNI_TEST_PARSER_SIEVE=1 OMNI_TEST_CODEX_COMMAND=/home/gryph/.local/share/mise/shims/codex OMNI_TEST_CODEX_MODEL=gpt-6.1-sol OMNI_PARSER_SIEVE_EVIDENCE=/home/gryph/Projects/tools/omnidex/bin/authority-review-checks-20261019/live.jsonl TMPDIR=/home/gryph/Projects/tools/omnidex/bin/authority-review-checks-20261019 GOMAXPROCS=2 timeout 360s /home/gryph/Projects/tools/omnidex/bin/authority-review-checks-20261019/codex.test -test.v -test.run '^TestInstalledParserSieveCurrentCorpus$/(job21-execution-before-independent-review|review-before-code-owned-acceptance|execution-later-review-pending|execution-required-review-protocol-check-failed|saved-job14-task-scoped-review|saved-job15-task-scoped-execution|task-scoped-.*|execution-mixed-question|execution-permission)$' -test.timeout=350s
```

All15 PASS, 196.80s. Verbatim job21 returns done; execution before independent
review and review before code-owned acceptance return done; actual required
review-protocol verification failure returns error. Retained job14/job15 pass;
required failures and required missing live evidence remain error, essential
questions remain question, permission remains permission, and the concrete
review finding remains done with exactly one finding. This finite semantic
qualification does not claim universal interpretation accuracy. Original1781
frames and separately identified code-rendered envelopes are retained in
[independent live-parser receipts](evidence/frozen-authority-independent-review-2026-10-19.raw-model-evidence.jsonl.gz):
1107774 raw bytes/75297 compressed bytes; artifact SHA256
`8c4e02ddf4d1f116e446557335dcb28409203db7a738176d10f2ef89afd71aa3`;
raw SHA256 `c188dc71ad9266efc5105a89d94856b8acb774e7593fa449f4dc7a0d25d8f228`;
stdout SHA256 `07b9b0abffc7cf690d599a02786c57321b2ff27183db7683ffeaa4d72509264c`.
Compression was checked to reproduce the original receipt bytes exactly.

Read-only inspection attempts initially assumed a local .env, used psql's
default socket, and used an absent fixture provenance key. They failed before
inference or mutation. Inherited configuration, explicit --dbname and the
existing rejected_classification key resolved those inspection details; no
implementation, fixture, assertion or policy was changed.

Before/after read-only database hashes are equal and match execution's record:
job21 `dc1a83062d9d099021c75ffa2490ec21`, ledger19
`9e8a0752ff32cbbc6a07c322ec535f2a`, ordered evidence149667–149669 aggregate
`39099ef97b715f4818149932a2c10102`. Job21 remains failed/generation1;
ledger19 remains frozen/revision6. Retained job14/job15, prior live receipts
and installed bin/omni/bin/omnidex hashes also remain unchanged. No live
database write, schema change, historical owner-job edit, installation/restart,
reset, source copy/worktree, private rollback-set change, commit or push occurred.

Cleanup: `docker --context default stop -t 5 omnidex-authority-review-runner
omnidex-authority-review-db` completed and a scoped subsequent listing was
empty. Sampled runner228.8 MiB/DB87.88 MiB, cache210.6 MiB, tmp10.2 MiB,
PG data59 MiB. Their tmpfs/cache/data retired with auto-removal. After host
inference completed, scoped codex.test process check was empty, exact owned
scratch was removed with shutil.rmtree and absence verified; shared
bin/agent-runtime was empty. No images, networks or volumes were created.
Initial available RAM24557 MiB, swap18475/21969 MiB used, disk approximately
45040 MiB free; final pre-scratch-removal RAM24523 MiB, swap18483/21969 MiB
used, disk45039 MiB free. Existing owner/shared artifacts were preserved.

Independent review complete: no task-specific defects found. Implemented
behavior was independently verified; results delivered in this existing ledger
and original receipts; review resources cleaned. Omnidex owns the subsequent
acceptance transition after this response.


## Retained same-job recovery — job22 task2 execution, 2026-10-19

Authority: authoritative tree `/home/gryph/Projects/tools/omnidex`, job22
generation2 frozen ledger20 revision7, dispatched execution call157532, task
`p_821e1757350d_4_task`; assignment read directly from PostgreSQL.
Global constraint: “Preserve failed job21 generation1, ledger19 frozen
revision6, and evidence149667–149669 unchanged.” Reuse job21 implementation;
correct only demonstrated defects. This execution is not the independent review.

Execution-owned disposable resources declared before creation: scratch
`bin/recovery-task2-checks-20261019` (<=512 MiB); runner
`omnidex-recovery-task2-runner` (2 CPUs/2 GiB, no swap, 256 PIDs,
/cache768 MiB and /tmp512 MiB tmpfs); DB `omnidex-recovery-task2-db`
(1 CPU/384 MiB, no swap, 128 PIDs, PG data256 MiB, socket/tmp16 MiB
each). Existing builder and postgres16 images only; --rm, --network none,
read-only roots/source/module cache, synthetic socket only, no production
credentials/data/ports/volumes. Lifetime1800s, command timeout<=10min,
GOMAXPROCS2/-p1, one heavy command at a time. Scratch holds only fixture
socket and bounded command outputs; retire both containers and scratch before
completion. Initial available RAM24523 MiB, swap18483/21969 MiB used,
disk45042 MiB available; shared bin/runs and private rollback set preserved.
No stale task2 resources found.

Session discovery briefly opened installed `bin/omni chat` and submitted only
`/exit`; it resumed the existing CLI channel/job17 and exited0. No job or
mutation was submitted. This was unnecessary; read-only PostgreSQL inspection
then identified this already-dispatched native execution, and no further
Omnidex session will be opened by this execution.

### Current execution verification and findings

No demonstrated implementation defect was found; retained recovery source and
tests were reused without edits. Inspection confirms same-generation evidence
reads include completed execution/review responses from prior recovery steps;
controller reconciliation precedes new effects, parsed reviews are reused by
original CallID, and execution/review session projections remain separate.
Explicit owner resume consumes the existing frozen authority and idempotent
intent receipt; failed steps remain historical. Native reconnect inspects the
original turn through thread/resume and thread/turns/list. Unknown/in-progress
turns do not gain replay authority. Initial reviews still receive fresh identities.

Current commands (Docker context default, runner/source/resource limits above):

```sh
docker --context default exec -w /src/internal/queue -e 'OMNI_TEST_DATABASE_URL=postgres://postgres@/postgres?host=/src/bin/recovery-task2-checks-20261019/socket&sslmode=disable' omnidex-recovery-task2-runner sh -c 'go test -v -p 1 $(go list -f "{{range .GoFiles}}{{.}} {{end}}" .) user_intent_fixture_test.go coding_recovery_integration_test.go -run TestFrozenCodingRecovery -count=1 -timeout=150s'
docker --context default exec omnidex-recovery-task2-runner go test -v -p 1 ./internal/codingagent ./internal/codex -count=1 -timeout=120s
docker --context default exec -w /src/internal/client omnidex-recovery-task2-runner sh -c 'go test -v -p 1 $(go list -f "{{range .GoFiles}}{{.}} {{end}}" .) chat_transport_test.go session_recovery_test.go session_message_reply_test.go session_turn_mode_test.go mutation_failure_test.go -count=1 -timeout=90s'
git diff --check
```

Final commands exit0. Real PostgreSQL suite PASS9.958s: six recovery cases
(pause, execution-parser failure, review-parser failure, execution disconnect,
review disconnect, interrupted partial continuation), plus five canceled,
completed, foreign-workspace, newer-job and immutable-context boundaries. Each
recovery retains job1/generation1, exact frozen task and file bytes, original
execution/review thread/turn identities, execute1/review1 completed effects,
identical duplicate resume receipts and accepted retry without new effects.
Pause retains step2; other failures preserve failed step2 and append step3.
Partial continuation reuses its thread and advances its turn exactly once.
Only the failed parser runs twice; all other parser counts remain one.
Every recovered job completes; canceled/completed direct revival is rejected.

Controller/native packages: 24 top-level PASS, five opt-in SKIP, zero FAIL.
Parsed-review reconciliation interruption and duplicate retry leave the initial
four provider calls unchanged. Protocol checks cover both execution/review ×
completed/inProgress/interrupted: retained original identity/result, no new
thread/start or turn/start, and idle continuation only for confirmed interruption.
Selected client/session checks: 10 top-level PASS,0.004s, including exact matching
recovery step and same-generation paused snapshot/control validation. These are
scripted provider/protocol/runtime and real PostgreSQL/file-effect checks, not
live-model recovery evidence. No new live-model call was needed for execution
verification. Existing task1 live interpretation evidence is retained above.
Full repository/queue/client suites are not claimed; the pre-existing obsolete
test compilation limitations documented in the job21 entry remain unchanged.

Failure retained: the first runner compiled but could not execute its test
binary (`fork/exec /tmp/go-build.../queue.test: permission denied`); Docker
tmpfs defaulted to noexec. Zero tests ran in that attempt. Stopped only that
owned runner, then recreated it with `/tmp:rw,exec,size=512m`. An immediate
recreation collided with automatic removal; no replacement was created by that
failed command, and the subsequent exec reported unavailable container. After
filtered inspection showed removal complete, recreation succeeded and all final
commands above passed. No test/assertion or product permission was weakened.
First runner ID `e85e8a1b941d6aae169da84d571eba2b1a4161a5b82ed176b5eb89c5e184a5a2`;
final runner ID `94da44db2b13353a398e1ad54c2911506f40b634a4fddd3360dcd52e21dde532`,
started10:39:28 UTC. DB ID
`75c7b4201e9118175ee7a11b6a7970beaae2520a1ac3c79f494e8cde9d431812`,
started10:37:47 UTC. Sampled runner cache192 MiB/tmp0; DB data63 MiB,
usage109.1/384 MiB; scratch1 MiB. All within declared bounds.

SHA256 current stdout: PostgreSQL
`e4b071c9eeb4c37a88104b0e4e9b330482558a44bb167732dbb9c03d02c754cf`;
controller/native `bb04a6504db1c6c9660be80b1c5d1b3b1c1e26f2e7f1ce8210bbc9f7a57b89ee`;
client `71728848869276ee8d7abbf5bb7a099140caeff1d738f14a996d930e59a03b29`.
The exact 28-path recovery list in job21's entry, with later accepted task1
changes preserved, hashes to
`33b24d1bac285b729501e503fa85fd169b67ddb5ae34bbb1fca8fb779a7fcbcc`
using its sorted path/NUL/file_sha256/newline method. Only this evidence append
was written by task2 execution. Installed binary hashes remain the unchanged
values documented above. Read-only before/after row-to-JSON MD5 comparisons
match for job21, ledger19 and evidence149667–149669: respectively
`dc1a83062d9d099021c75ffa2490ec21`, `9e8a0752ff32cbbc6a07c322ec535f2a`,
`489bd5ec9d81eac0eab9a1d5cc4e70ab`, `e782a7ef4c1710bd07c432f647887f40`,
`9639729950751b4db0f2bab74b82c753`. No historical owner rows were mutated.

Cleanup executed: `docker --context default stop -t 5
omnidex-recovery-task2-runner omnidex-recovery-task2-db`; both --rm containers
stopped, and subsequent filtered `docker ps -a` returned no task2 containers.
Removed only declared scratch with Python shutil.rmtree; existence check false.
All fixture PG data, socket, native scripts, test roots and container caches
retired. No task-owned image/network/volume was created. Final available
RAM24620 MiB, swap18610/21969 MiB used, disk45022 MiB available. Shared
artifacts and the private rollback set were preserved. Final diff whitespace
check passed.

Stages: retained implementation exists; current execution checks verified;
findings delivered locally in this ledger; owned resources cleaned. No commit,
push, installation, runtime restart or live schema change was performed. This
is the dispatched execution result for task2; its controller-dispatched reviewer
performs the independent review with its own checks and concrete findings. No
preliminary review, substitute reviewer or future receipt is an execution
completion prerequisite.

## Same-job recovery and activation candidates — 2026-10-19

Completed assignment in the sole authoritative tree
`/home/gryph/Projects/tools/omnidex`, HEAD
`3b252b684449447fd84bc122bace2ae494b11bf1` plus preserved owner changes.
This is implementation/verification/candidate preparation, not activation or
independent review. Task2's original execution report, checks and history above
remain intact; its independent review was not reached. Accepted job22 task
`p_821e1757350d_3_task` remains accepted/state_version2 and was not reopened.

Recovery now enters through `POST /v1/jobs/{id}/resume`, the existing API
lifecycle gate, typed `ResumeJobCommand`, repository lifecycle transaction,
ordinary queue claim, controller reconciliation and original native adapter.
The request carries operation_id, feedback, expected_generation,
observed_ledger_revision and exact workspace_root/workspace_identity. The client
exposes the same typed control. Channel `execution.resume` uses that same
transaction; no second pipeline or phrase routing exists. Operation replay
returns its immutable receipt; changed payloads, generations, frozen revisions,
workspaces, terminal jobs and newer scope authority cannot gain recovery.
The existing native-updater admission check also consults the existing recovery
predicate for failed jobs, protecting recoverable native identities without
blocking on unrelated stale failures.

Recovery retains the original job/generation/frozen tasks and appends a recovery
step after failure. Failed steps/attempts/evidence remain historical. A retained
completed response with an interrupted interpretation receives append-only
`response_recovery` evidence and is parsed again using the accepted parser.
A corrected semantic `done` proceeds to remaining review/acceptance without
native redispatch. Only a newly confirmed genuine incomplete result consumes
the owner's resume as continuation on its original native thread; review
continuation retains its original response context. A later interruption records
its own continuation rather than reusing an older failure. Accepted tasks are
excluded from continuation selection. Questions/permissions still require their
ordinary exact input reply; cancellation/completion are not recovery authority.

Changed ownership is confined to existing recovery/lifecycle/controller/native
updater admission and their tests: new repository/API resume files, lifecycle
command/route/client registration, channel intent dispatch, coding_recovery,
coding_agent_settings, controller/task/review_state/task_interruption, and the
recovery/API/client fixtures. Existing native-session, reconciliation and parser
implementations are reused. No historical job was edited to demonstrate this.

### Concrete activation packet (pending owner decision)

Only two candidate executables are retained in
`bin/recovery-activation-20261019`:

| Candidate | Bytes | SHA-256 |
| --- | ---: | --- |
| `omnidex` | 33,903,664 | `b236304c73c9a4beb6f002470d9c4d6e2b973a8397cb89b2202f168d49dd82dd` |
| `omni` | 11,262,208 | `8a4f58176de410d8d6ad4a00b34742d9a8031c24265dfbaba3a4385f515947a9` |

[Source manifest](../bin/recovery-activation-20261019/source-manifest.json)
SHA-256 `96b2b6b3c41c6039c47a6f9008787e6564f4003d4d407073b14de15765808ab6`
binds 1,869 existing source/test/embedded-asset paths to exact bytes, root and
HEAD. The ledger is an evidence output excluded from this binary-input manifest.
Post-build comparison found zero changed manifested paths. This is a hash
manifest, not a source copy. The accepted parser's original ten-path aggregate
still equals `516d7710458ba3ef670bad1ec8d576491459200f67fe7f4557c8a5df2a0efa3b`.
Both candidates contain those accepted parser bytes and the repaired recovery.
No release version or acceptance state was invented. Build/module details are in
[build-info](../bin/recovery-activation-20261019/build-info.txt), compiler details
in [toolchain](../bin/recovery-activation-20261019/toolchain.txt), and preserved
owner modifications in [source status](../bin/recovery-activation-20261019/source-status.txt).

Build used existing `omnidex-test-builder:local`, Go1.24.1 linux/amd64,
CGO_ENABLED1, Alpine GCC14.2.0 and GNU ld2.43.1, read-only authoritative source
and existing module cache, GOPROXY=off, GOMAXPROCS2. Exact final commands,
sequentially inside the isolated runner:

```sh
timeout 600s go build -p 1 -ldflags '-linkmode external -extldflags -static' -o /out/omnidex ./cmd/omnidex
timeout 600s go build -p 1 -ldflags '-linkmode external -extldflags -static' -o /out/omni ./cmd/omni
```

Both exit0; static x86-64 ELF inspection passed. Candidate CLI loaded on the host
with `version --json`. Actual candidate core ran through the unchanged existing
`runs/activation-recovery-20261019/candidate-check.go` in a network-none runner:
fresh synthetic schema, /healthz and /readyz200, workspace bootstrap/reconnect,
immutable provenance/receipt checks, graceful stop and restart, original channel
identity and exact receipt retained. This is candidate runtime evidence, not a
live-service or live-model recovery claim.

**Required schema differences, compared read-only against live definitions:**
[Exact catalog comparison](../bin/recovery-activation-20261019/schema-differences.json),
SHA-256 `dd7df0704d4f7fe26ea5c8287eb9509deccba113e0435571855b91a8ecd67675`,
contains full live/source definitions with the actual schema identifier
normalized to `RUNTIME_SCHEMA`. Exactly eight entries are marked
`required_for_recovery=true`:

- Add `frozen_coding_resume_authorized(bigint,bigint)`: uncommitted ordinary
  resume lifecycle operation, exact generation/frozen revision and retained
  failed/paused step replace the channel-intent-only assumption.
- Replace `enforce_jobs_state_authority()`: retain terminal protection, allowing
  only authorized failed-to-running recovery in the same generation.
- Replace `prevent_job_step_generation_identity_mutation()`: preserve step
  identity, admit frozen same-generation pause and its exact resume, retain
  ordinary question/native-input reply requirements. The zero-attempt paused
  step may return to pending only under this recovery authority.
- Replace `require_lifecycle_generation_transition()`: frozen execution pause
  has a same-generation waiting step and no active attempt, instead of requiring
  the planning interruption's new generation.
- Replace `job_lifecycle_operations_check3`: retain all existing alternatives;
  add same-generation pause with a waiting step and resume with running job,
  equal observed/result generations and null step/result_step_status.
- Replace `job_lifecycle_operations_kind_check` and
  `lifecycle_operation_registry_kind_check`: add only `resume_job`.
- Replace `job_lifecycle_operations_roleplay_payload_check`: retain its existing
  NOT VALID status and alternatives; add the exact resume payload with required
  operation_id/job_id/feedback/expected_generation/observed_ledger_revision,
  optional paired workspace fields, existing workspace validation and no extras.

The other19 catalog differences predate this assignment: task-contract and task
transition definitions, removed source-only dependency/edge functions, and
source task/item/check/edge constraints. They are explicitly marked false and
are not instructions to remove live history, tables, columns, triggers or
constraints. Live PostgreSQL18 reports590 NOT NULL catalog objects differently
from fixture PostgreSQL16; comparison of all661 source columns found zero
nullability differences. The17 live-only columns are listed in
[nullability comparison](../bin/recovery-activation-20261019/not-null-comparison.json).
No live-shaped schema conversion or historical-row transformation is claimed.
Fresh-schema installation and preserving restart were verified separately.

Existing controls remain: user service `omnidex.service`, fragment
`/home/gryph/.config/systemd/user/omnidex.service`, ExecStart
`/home/gryph/Projects/tools/omnidex/bin/omnidex`, active MainPID1211502.
Before/after service properties match. Installed core SHA-256 remains
`1d3e97017dcb2aa2501b4c1dc6bff90fcd42b28859f3994f72f49c4927679099`;
installed CLI remains
`f87c359926871bc727d136f6df84041b62aad4e92a1eff41c1897016a140606a`.
Candidate paths are not service targets. No install, live DDL, service restart,
commit or push occurred.

Preservation for any separately authorized activation: recheck candidate hashes
and source manifest, exact current schema definitions and retained state before
changing controls; use the existing service/binary targets and preserved
`bin/activation-preserved-20261019` set without creating another rollback copy.
Apply only owner-authorized preserving schema differences, never replay fresh
setup.sql over live state or manually rewrite jobs/ledgers/evidence/receipts.
Retain frozen revisions, accepted tasks/native identities and append-only history;
then verify actual activated hashes and affected ordinary boundaries. These are
activation handoff details, not actions performed or extra completion gates.

### Verification, preservation and cleanup

All final checks exit0. Full `internal/db`, `internal/codingagent`,
`internal/codex` tests passed (five opt-in native/live-model tests skipped).
Fresh schema and preserving startup tests passed. Selected tests compile all
production queue/API/client files; full repository/queue/client suites are not
claimed because the pre-existing obsolete test compilation limits above remain.
Exact selected-file pattern is `go test -v -p 1 $(go list -f
'{{range .GoFiles}}{{.}} {{end}}' .) <fixture files> -count=1 -timeout=150s`:

- Queue fixtures: user_intent_fixture_test.go,
  coding_recovery_integration_test.go, coding_recovery_input_integration_test.go;
  `-run 'TestFrozenCodingRecovery|TestFrozenRecovery'`: eight recovery modes,
  five terminal/stale boundaries, unanswered question and permission; PASS13.599s.
- API fixtures: cli_chat_session_identity_integration_test.go and
  job_resume_integration_test.go; `-run TestCodingEnqueuePublicResumeRecovery`:
  nine public coding-enqueue/handler/service/repository/controller scenarios;
  PASS8.127s. Structural and semantic parser failure for execution/review,
  execution/review disconnect, partial continuation and genuine incomplete
  execution/review interpretation. Each retains an already accepted task,
  original job1/generation1/frozen3, original native identities and exact files;
  execute/review effects for both tasks each remain1. Continuations0 for the
  six completed-response/disconnect cases and1 only for partial/genuine work.
  Duplicate resume receipts, stale revision and stale worker rejection pass.
- Selected client transport/session/input/recovery tests, including
  resume_transport_test.go, PASS0.004s; affected CLI workspace/chat/ledger/plan
  tests PASS0.948s. Final whitespace check passed.

[Runtime](../bin/recovery-activation-20261019/runtime.txt),
[public API](../bin/recovery-activation-20261019/public-api.txt),
[channel and input](../bin/recovery-activation-20261019/recovery.txt),
[client](../bin/recovery-activation-20261019/client.txt),
[CLI](../bin/recovery-activation-20261019/cli.txt) and
[actual candidate](../bin/recovery-activation-20261019/candidate-runtime.txt)
retain current outputs. These use isolated synthetic PostgreSQL and scripted
semantic/native providers, plus existing native-protocol fixtures. No new real
model invocation or recovery of a live owner job was performed. Accepted parser
live qualification and its original receipts remain the earlier evidence.

Read-only [preservation SQL](../bin/recovery-activation-20261019/preservation.sql)
and identical before/after outputs cover complete jobs21/22, ledgers19/20,
all their evidence, tasks and channel-intent receipts. Job21 MD5
`dc1a83062d9d099021c75ffa2490ec21`, ledger19
`9e8a0752ff32cbbc6a07c322ec535f2a`; evidence149667–149669 are unchanged as part
of the complete job21 evidence comparison. Job22 MD5
`dbbc002a2ae1a93640244884d0db6a5d`; accepted task state_version2 row MD5
`4e11238fbe8ed33f7186592199084592`. Installed binary hashes, service properties
and every file hash in `bin/activation-preserved-20261019` also match before/after.

Owned resource budget was declared before creation: ignored
`bin/recovery-activation-20261019` <=512MiB including candidates/evidence;
runner2 CPUs/2GiB/no swap/256 PIDs, tmp512MiB/cache768MiB tmpfs, lifetime3600s;
PG1 CPU/384MiB/no swap/128 PIDs, data256MiB/tmp16MiB and synthetic socket only.
Both --rm/network none/read-only root and source, existing images/module cache,
no production credentials/data/ports/volumes. One heavy operation at a time,
commands <=600s. Candidate-only replacement runner used cache256MiB/lifetime600s
and the established read-only candidate mount. Initial available RAM24612MiB,
swap18615/21969MiB used, disk45023MiB available. Sampled original runner414MiB,
DB175MiB, PG data102MiB; candidate cache149MiB. Within declared bounds.

Failures retained here: initial read-only preservation queries omitted schema
selection, then used item_id instead of task_item_id; corrected without writes.
API fixture initially used wrong typed constants/rank type, expected202 rather
than actual201, then omitted required native frame evidence; corrected harness,
never product assertions. First disposable DB disappeared during fixture churn;
its cause is unproven. Recreated only that fixture with established
shared_buffers16MB/max_connections20/max_wal_size32MB/min_wal_size32MB/
checkpoint_timeout30s; all final affected tests passed. A raw synthetic catalog
load omitted startup's check_function_bodies=off; using the established bootstrap
settings resolved it. The genuine-review case exposed and verified the actual
retained-response-context fix. No policy, authority or assertion was weakened.

Cleanup completed: original runner, replacement candidate runner and DB stopped
and auto-removed; final filtered Docker listing empty. Fixture data, schemas,
sockets, client workspace, empty fixture HOME, build caches and processes retired.
No task-owned image/network/volume was created. Redundant catalog/build scratch
was removed; only the two candidates and necessary manifest/comparison/check
outputs remain (approximately45MiB), pending owner activation decision. No source
copy/worktree, nested Omnidex or new rollback copy was created. Final available
RAM24766MiB, swap18868/21969MiB used, disk44980MiB available; shared/owner
artifacts preserved.

Stages: implemented; verified; candidates and activation packet delivered locally;
owned disposable resources cleaned. Current recovery changes have not received
independent code-owned review/acceptance. That later review and the owner's
activation decision do not block completion of this preparation assignment.

### Recovery activation packet completion — 2026-10-19 11:31 UTC

This completes the same preparation assignment and updates the concrete packet
above; its earlier execution reports remain historical evidence. A bounded
reproduction identified one remaining defect: after reconnecting a review that
returned findings, successful correction retained the consumed review session
in memory. The next review could retrieve that obsolete turn and repeat the
correction. Existing task authority now clears that session after successful
execution, matching its existing durable projection on task_executing. No new
pipeline or state authority was added.

The public production-boundary fixture now includes
review-disconnect-findings. It retains the accepted task, recovers the original
review identity, performs exactly one required correction and a fresh review,
and accepts without native continuation. It bounds unexpected extra executions
at two. Original accepted execute/review effects remain one each; remaining
execute/review effects are exactly two each (original plus required correction).
All other recovery modes still have one effect each.

**Current candidates replace the earlier candidate hashes in this packet:**

| Path under bin/recovery-activation-20261019 | Bytes | SHA-256 |
| --- | ---: | --- |
| omnidex | 33,903,672 | 0f3f52d08ee868bbdbe093f283235a1665d99b903fde4948905b048273b0f469 |
| omni | 11,262,208 | 1198dc1f47e583f1fb5c6dcfc6e8a3b547cb611bc88b332378059a734d4e7752 |

Updated source-manifest.json SHA-256:
9112bfe5f86b24200873b11762d80586d52b813f8fff374e6fb8fab0829223b2.
All 1,869 manifested paths matched after build. Relative to the prior packet's
manifest only internal/codingagent/task.go and
internal/api/job_resume_integration_test.go changed. HEAD remains
3b252b684449447fd84bc122bace2ae494b11bf1.
Accepted parser bytes, setup.sql and all other owner source/settings remain
unchanged. Both static linux/amd64 candidates were rebuilt sequentially with
the packet's exact existing Go1.24.1 external/static commands, read-only source
and module cache, GOPROXY=off. Updated build-info.txt records their module data.
No source or rollback copy was made.

Current checks (all exit0), using the same selected-file commands recorded above:
[public-api-final.txt](../bin/recovery-activation-20261019/public-api-final.txt)
passes all ten ordinary enqueue/API/service/repository/controller recovery modes
in9.053s, including duplicate receipt and stale revision/attempt rejection;
[recovery-final.txt](../bin/recovery-activation-20261019/recovery-final.txt)
passes channel recovery, terminal/stale boundaries and unanswered question/
permission in13.691s;
[runtime-final.txt](../bin/recovery-activation-20261019/runtime-final.txt)
passes full db/codingagent/codex suites, including fresh-schema installation and
preserving startup. Five opt-in native/live-model checks remain skipped.
Existing selected-file limitations and independent-review limitations above still
apply. Synthetic providers are not live-model acceptance evidence.

[candidate-runtime-final.txt](../bin/recovery-activation-20261019/candidate-runtime-final.txt)
records the actual rebuilt core passing fresh startup, health/readiness,
workspace reconnect, immutable receipt/provenance checks, graceful stop/restart
and original channel identity retention through the unchanged existing candidate
checker. Actual rebuilt CLI version --json also passes. No owner job was run.

The precise eight required setup.sql function/constraint differences, the19
pre-existing differences, live/source full definitions, NOT VALID qualification,
and nullability caveat remain exactly those in this packet's unchanged
schema-differences.json and not-null-comparison.json. No new schema difference
was introduced by this correction. Existing service/binary controls and
preservation/activation steps above remain the handoff: activation requires a
separate owner decision; do not replay setup.sql or rewrite historical data.
Installed hashes and every activation-preserved-20261019 file hash were
rechecked and match the original before files (installed-final.txt and
preserved-final.txt). Job21 generation1/ledger19 frozen6/evidence149667–149669,
job22 history/receipts and accepted p_821e1757350d_3_task state_version2 were
never written by this completion; prior read-only preservation evidence remains
above. Task2's original report/checks/history remain intact; its independent
review was not reached.

Resources reused the declared <=512MiB existing artifact directory, with no new
source tree. Capacity before work:24GiB available RAM,18GiB used swap,44GiB
available disk. Runner omnidex-recovery-final-runner used2 CPUs/2GiB/no swap,
256 PIDs, tmp512MiB/cache768MiB tmpfs,3600s lifetime; synthetic socket-only PG
omnidex-recovery-final-db used1 CPU/384MiB/no swap,128 PIDs, data256MiB and
tmp/socket16MiB tmpfs with the established bounded PG settings. All containers
were --rm/network none/read-only root; source/module cache read-only.
One heavy operation at a time. Sampled runner387MiB/DB151MiB.
The first disposable PG exited before readiness; it was recreated after making
the task-owned synthetic socket directory writable. Its exact exit cause was
not retained. Final checks use only the replacement synthetic database.
Hash comparisons initially differed solely in file ordering; sorting by path
proved byte-identical hashes.

The compiled candidate checker was disposable and removed. Original runner,
replacement PG and candidate-check container were stopped/auto-removed;
subsequent scoped Docker listing is empty. Socket directory, empty synthetic
client directory, tmpfs database/caches/HOME/workspace and processes retired.
Only two necessary candidates plus packet evidence remain,44MiB. Final sampled
available RAM24,831MiB, used swap18,864MiB, available disk44,964MiB.
No install, live-schema write, live service restart, historical state edit,
commit or push occurred.

Stages: implemented; verified; updated activation candidates/packet delivered
locally; task-owned disposable resources cleaned. Independent code-owned
review/acceptance and owner activation remain pending separately.

### Semantic user-intent routing completion — 2026-10-19

Scope: complete the existing caller-owned options/callbacks, session registry,
bounded context, reference/recipient/native-choice interpretation and exact
authority/receipt replay. No second pipeline, live activation, schema changes,
historical edits or reopening accepted work. Prior task2 report and its unreached
independent review remain intact. Job21 generation1/ledger19 frozen6/evidence
149667–149669 and job22 history/receipts, accepted p_821e1757350d_3_task
state_version2 are excluded from writes.

Acceptance: route before dispatch; ambiguity has no effects; distinguish
recipients and current native choices; confirm pending proposals directly;
reject stale actor/workspace/job/generation/revision/operation authority; replay
committed receipts without effects through API/service/repository/client.

Task resources: bin/intent-checks-20261019 holds bounded check evidence (<=16MiB)
and disposable synthetic socket; remove socket on completion, retain check
evidence. Disposable containers omnidex-intent-runner (2 CPUs, 2GiB RAM/no swap,
256 PIDs, tmp768MiB/cache768MiB tmpfs, 1800s lifetime) and omnidex-intent-db
(1 CPU,384MiB/no swap,128 PIDs,data256MiB/socket16MiB tmpfs) use network none,
read-only roots, auto-removal and synthetic data only. Source and existing module
cache mount read-only. One heavy operation at a time; retire both on completion.
Initial capacity: available RAM24800MiB, used swap18860MiB, available disk44GiB.
Existing owner/shared containers and caches are outside cleanup scope.

In progress: inspecting and completing exact execution authority, native-choice
validation and focused clarification; then isolated checks.

Completion: the existing generic userintent interpreter supplies bounded opaque
semantic choices; callers retain options, callbacks and continuation ownership.
The existing CLI routes ordinary text through interpretation before planner/
executor dispatch. Dedicated facts, four recent messages and a pending pitch
feed the bounded context builder; only long recent messages are minified.
No phrase router, second pipeline or model-owned effects was introduced.

Execution now additionally checks the captured job/generation/ledger/plan state,
native prompt and choices against current state under existing job/channel
locks, and validates job/channel/workspace authority through the existing
validator. Native choices require a current input, and selecting multiple
answers is rejected. Incompatible resolved intentions now yield one bounded
focused clarification with zero executable actions. Pending confirmation still
dispatches the original exact proposal directly in the existing transaction,
without recursive approval. Stored interpretation and committed receipt reuse
remain bound to their original operation/channel/actor workspace record.

Checks (all final commands exit0; source manifest records current hashes and
HEAD in bin/intent-checks-20261019/source-manifest.json):

- semantic.txt: go test -p 1 ./internal/userintent ./internal/sessionintent
  -count=1 -timeout=90s. PASS: caller options/callbacks, cancellation, ambiguity
  without callbacks, invalid/duplicate choices, bounded dedicated context and
  minification, single current native answer. No ceremonial extra inference.
- queue-final.txt: from internal/queue, go test -v -p 1
  $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .)
  user_intent_fixture_test.go user_intent_integration_test.go
  user_intent_native_integration_test.go -run TestUserIntent -count=1
  -timeout=150s. PASS10.476s: exact revision approval/start, direct proposal
  confirmation/removal, stale execution and other actor rejection, recent/
  pending context, shared typed controls, parser failure/cancel replay, changed
  job/generation/ledger/revision/plan/workspace/input/prompt/choices, changed
  operation payload and stale intake rejected before inference. Actual native
  allow-once and deny responses each commit one execution_reply; exact receipt
  replay creates no second reply. Synthetic state is created through repository
  APIs; no authoritative historical records are patched.
- api-final.txt: from internal/api, same selected-file production compilation
  pattern with cli_chat_session_identity_integration_test.go,
  user_intent_integration_test.go, user_intent_interpret_test.go;
  -run 'TestUserIntent|TestSessionUserIntent' -count=1 -timeout=150s.
  PASS1.912s: two unrelated synthetic goals traverse the real HTTP provider/
  API/service/repository/client path. Ambiguous references ask one question and
  cannot execute or change session authority. Pending removal survives workspace
  reconnect, and ordinary confirmation directly commits it. The fixture discards
  the committed execution response (truncated JSON); reconnect and client replay
  recover the identical stored receipt without a second ledger revision.
  Stale API state and changed operation payload return409 without inference.
  Recipient distinctions, plan approval/start, exact native choices and one
  focused question for incompatible native answers pass; caller-authored
  callbacks are rejected at the API boundary.
- cli.txt: go test -v -p 1 ./cmd/omni
  -run 'TestWorkspace|TestLedgerPanel|TestChat|TestPlanReview' -count=1
  -timeout=90s. PASS0.946s, including interpretation before plan dispatch.
- Final git diff --check passes.

These are synthetic PostgreSQL, scripted semantic-provider and CLI/component
checks, not live-model semantic accuracy or owner-job execution evidence.
Selected queue/API tests compile every production file; full suites are not
claimed because the previously documented obsolete test compilation limitations
remain. No installation, live-schema change, restart or activation was needed.

Preservation: preservation.txt confirms all initial recorded hashes match,
including every activation-preserved-20261019 file, installed binaries,
database/setup.sql and codingagent parser sources. No live database was accessed;
job21 generation1/ledger19 frozen6/evidence149667–149669, job22 history/receipts
and accepted p_821e1757350d_3_task state_version2 were not touched. Accepted parser
changes remain source-only. This ledger was appended only; task2's original
execution report, checks, history and unreached independent review remain intact.
Existing recovery activation candidates were not rebuilt or activated.

Resource accounting: both disposable containers stopped and auto-removed;
filtered Docker listing is empty. Synthetic schemas/data/socket, tmpfs build
cache, temporary workspaces and processes retired. Only64KiB of necessary
evidence remains in the declared ignored output directory. No source copy,
worktree, nested Omnidex, task image/network/volume or persistent host service
was created. Two initial compilation commands briefly overlapped; the second
was terminated, then all final checks ran sequentially. Sampled peak observed
runner524MiB, DB169MiB, within limits; final available RAM24832MiB,
swap18859MiB used, disk44941MiB available. Owner/shared resources were preserved.

Stages: implemented; verified with the bounded synthetic checks above; delivered
locally as source and evidence (no commit/push/install); cleaned. Later
independent code-owned review/acceptance and owner activation remain separate
and do not block this assignment's completion.

### Semantic user-intent authority completion — 2026-10-19

Resumed the same source-only task and acceptance above. Existing reports remain
unchanged. Findings: admission checked only intent rows; lifecycle callers could
reuse a committed status receipt ID, and intent admission could reuse a committed
channel-turn ID. Fix uses the existing shared operation lock and registry: reject
cross-kind IDs before inference or lifecycle effects. Dispatched effects receive
deterministic action-bound child IDs; the original receipt remains replay authority.
No schema changes, alternate pipeline, live records or activation involved.

Reusing the declared bin/intent-checks-20261019 evidence destination (16MiB ceiling)
and disposable socket, removed at finish. Task-owned containers
omnidex-intent-finish-db and omnidex-intent-finish-runner use the same bounded
isolated synthetic settings above, 1800s runner lifetime; source/module cache
read-only. Capacity before compilation: RAM available24GiB, swap18GiB used,
disk44GiB available; existing evidence64KiB. One compiler operation at a time.

Completion evidence for the authority fix (all final commands exit0):

- `bin/intent-checks-20261019/authority-queue.txt`: same selected-file queue
  command documented above, including every production file and all three intent
  fixture/integration files; TestUserIntent passes11.305s. New regression rejects
  a committed channel-turn ID at interpretation before inference and at intent
  cancellation, with unchanged session state. Existing exact-context, stale
  authority, pending confirmation and native receipt replay checks pass.
- `authority-api.txt`: same selected-file API command documented above;
  TestUserIntent/TestSessionUserIntent pass1.791s. Both unrelated synthetic
  goals now also commit a status receipt through client/HTTP/service/repository,
  attempt lifecycle cancellation using that ID, receive409, replay the identical
  status receipt, and verify unchanged session state. Existing direct proposal
  confirmation, lost committed response and reconnect tests still pass without
  a second ledger revision. Scripted provider calls remain exactly bounded.
- `authority-components.txt`: go test -p1 ./internal/userintent
  ./internal/sessionintent ./cmd/omni -run
  'TestWorkspace|TestLedgerPanel|TestChat|TestPlanReview|Test' -count=1 -timeout=90s
  passes all matched tests (the final Test alternative matches all test names).
  Generic semantics0.003s, session contracts0.002s, CLI1.176s.
- `authority-source-manifest.json` records HEAD and exact current source hashes.
  `authority-preservation.txt` and the final repeat verify all25 paths in the
  original preservation manifest match, including activation-preserved files,
  installed binaries, schema source and parser source. Final diff whitespace
  check passes. These are synthetic database/scripted provider/component checks;
  no live-model accuracy, full queue/API suite or independent review is claimed.

Cleanup: runner and synthetic DB stopped and auto-removed; filtered task Docker
listing empty, socket directory removed, tmpfs caches/database/workspaces and
processes retired. Initial DB startup failed because the task socket directory
was not writable by its isolated PostgreSQL user; only that disposable directory
permission was corrected before tests. No owner resource was changed. Sampled
runner362MiB and DB180MiB within bounds. Necessary retained evidence88KiB;
final RAM available24GiB, swap18GiB used, disk44GiB available. No source copies,
installation, live-schema writes, historical job edits or service restarts.
Job21/job22 and accepted job22 task remain untouched; parser remains source-only.
Task2's original execution report/checks/history and unreached independent review
remain unchanged.

Stages: implemented; verified; delivered locally as source/evidence (no commit,
push or activation requested); cleaned. Later code-owned independent acceptance
and owner activation are separate and do not block this assignment's completion.

### Semantic user-intent forwarding identity serialization — 2026-10-19

Same task/authority and source-only exclusions as above. The remaining finding
was forwarding's child operation reservation bypassing the shared identity lock.
Forward/discuss/revise now take that existing lock before admission/effects.
A synthetic concurrency regression holds the derived ID during ordinary status
interpretation, requires forwarding to wait, commits the status receipt, rejects
forwarding's collision and verifies receipt replay and unchanged session state.

Resources declared before creation: reuse bin/intent-checks-20261019 (16MiB
retained evidence ceiling); disposable socket directory there, removed at finish.
Containers omnidex-intent-lock-db and omnidex-intent-lock-runner: auto-remove,
network none, read-only roots, synthetic data only, 1200s lifetime; DB 1CPU/384MiB
no swap/128 PIDs/data256MiB tmpfs; runner 2CPU/2GiB no swap/256 PIDs,
cache768MiB/tmp768MiB tmpfs. Source/module cache read-only. One heavy operation
at a time. Initial RAM available24786MiB, swap18853MiB used, disk44958MiB
available; retained check evidence88KiB. No live data or service access.

Final verification (all commands exit0, sequential; same selected-file commands
as recorded above, compiling every production file in each affected package):
- lock-queue.txt: TestUserIntent PASS4.672s, including new
  TestUserIntentForwardSerializesChildIdentity PASS0.58s. In-flight admission
  blocks forwarding until its deadline; after status commit, forwarding returns
  ErrLifecycleOperationConflict, no child lifecycle entry exists, status receipt
  replays identically, and session state is unchanged. Existing stale context,
  actor, operation, direct confirmation and native-choice replay checks pass.
- lock-api.txt: TestUserIntent/TestSessionUserIntent PASS0.683s. Existing real
  client/HTTP/service/repository paths prove ambiguous references without effects,
  recipient distinctions, exact native choices, proposal confirmation, reconnect,
  lost committed response and identical receipt reuse without repeated effects.
- lock-components.txt: go test -p1 ./internal/userintent ./internal/sessionintent
  ./cmd/omni -count=1 -timeout=90s PASS (0.003s,0.002s,1.117s), covering bounded
  context/minification, caller callbacks and routing before planner dispatch.
- lock-preservation.txt: all25 original preservation hashes match; source state
  recorded in lock-source-manifest.json. Final git diff --check passes.

These are isolated synthetic PostgreSQL, scripted provider and component checks;
no live-model accuracy, independent review or full queue/API suite is claimed.
Initial disposable DB startup attempts failed; retained lock-db.txt shows the
initialization client attempting the default socket while the server used the
custom socket path. Mounting the synthetic socket at PostgreSQL's default path
resolved initialization; all final database checks ran against that isolated DB.

Cleanup: both containers stopped and auto-removed, filtered listing empty;
synthetic socket directory removed, database/workspaces/cache/tmpfs/processes
retired. Sampled runner375MiB and DB215MiB, within limits. Retained evidence
under128KiB. Final available RAM24GiB, swap18GiB used, disk44GiB available.
No installed binaries, live schemas, owner services, protected history, accepted
job22 state, parser sources or activation-preserved files changed. No source
copy/worktree or nested Omnidex. Prior task2 evidence/report/history remains
unchanged; its independent review was not reached.

Stages: implemented; verified with current checks above; delivered locally as
source/evidence (no commit/push/install/activation requested); cleaned. Later
code-owned review/acceptance and owner activation are separate.


### Readable terminal and combined flow — 2026-10-19

Scope: terminal command/code/output/native-choice presentation, preserved editor,
panels and navigation, and isolated combined approval/execution/review/interruption/
recovery/retry/reconnect verification. Existing owner changes and all protected
job21/job22 state, receipts, task2 original report and activation-preserved files
are excluded from writes. No installation, live schema writes or service restart.

Resources: bin/terminal-checks-20261019, retained evidence <=16MiB; disposable
compiled terminal harness <=64MiB removed at finish. Containers
omnidex-terminal-runner (2CPU/2GiB/no swap/256PIDs, cache768MiB/tmp768MiB tmpfs)
and omnidex-terminal-db (1CPU/384MiB/no swap/128PIDs, data256MiB tmpfs),
network none/read-only/auto-remove/1800s lifetime; synthetic socket directory
removed at finish. Source/module cache read-only; one heavy operation at a time.
Private tmux server/socket under that evidence path, <=2 panes, lifetime <=600s,
removed at finish. Initial capacity: RAM available24628MiB, swap used18858MiB,
disk available44GiB. Unknown/shared builds/caches preserved.

Native qualification resources: reuse the synthetic DB with a socket mounted at
bin/terminal-checks-20261019/socket; no live database connection. Host test runner
and native app-server children bounded to600s,2CPU/2GiB/no swap/256tasks via a
transient task scope. Synthetic workspaces under bin/terminal-checks-20261019/tmp
and exported static test binary <=64MiB are disposable and removed at finish.
Existing signed-in native transport is used without installing or changing its
configuration/identity; native evidence/history is preserved. This is not nested
Omnidex. One heavy operation at a time. No source copies or worktrees.

Completion: streamed message/code/output blocks now label the item once and align
continuations to that label; indentation, empty lines, trailing spaces and quotes
are retained. Multiline command outcomes use the same block renderer, with exact
exit status and a quoted working directory at start. Native choices are one exact
label per line; their typed response bytes are unchanged. Normal reasoning/output
verbosity remains bounded as before. Receipt presentation includes its job target;
control outcomes display the returned status. Pause help names same-job continuation
and redirection rather than requiring redirection. No stored receipt was rewritten.

The existing workspace/editor remains the only presenter/input owner. A scrolled
conversation now compensates for newly appended rows and anchors its top source
character across resize/reflow. Draft/cursor, focus, functional ledger selection
and scroll controls remain intact. No workflow authority or pipeline was added.

Current verification, all final commands exit0:
- bin/terminal-checks-20261019/final-components.txt:
  `go test -p 1 ./cmd/omni ./internal/codex ./internal/codingagent
  ./internal/sessionintent ./internal/userintent -count=1 -timeout=90s`
  PASS(1.012s,0.004s,0.009s,0.002s,0.003s). Includes multiline formatting,
  terminal-safe text, native-choice details/unchanged replies, editor and ledger
  interaction, streamed scroll anchoring and normal/narrow resize regression.
- queue-final.txt: in bounded isolated runner, with synthetic PostgreSQL URL,
  `cd /src/internal/queue; go test -v -p 1 $(go list -f
  '{{range .GoFiles}}{{.}} {{end}}' .) user_intent_fixture_test.go
  coding_recovery_integration_test.go coding_combined_flow_integration_test.go
  user_intent_native_integration_test.go -run
  'TestCodingCombined|TestFrozenCodingRecovery|TestUserIntentCurrentNativeChoice'
  -count=1 -timeout=150s` PASS5.898s; 4 top-level tests,15 subtests. These use
  scripted provider/semantic fixtures and real repository/controller/DB paths.
- api.txt: same runner/isolated DB, every production API file plus
  cli_chat_session_identity_integration_test.go,user_intent_integration_test.go,
  job_resume_integration_test.go; `-run
  'TestUserIntent|TestSessionUserIntent|TestCodingEnqueuePublicResumeRecovery'
  -count=1 -timeout=150s` PASS4.476s. Real client/HTTP/service/repository checks
  include exact receipt replay, reconnect, stale authority and public resume.
- Actual tmux PTY, production newChatConsole/x/term editor/input router/resize
  watcher/renderers, test TestTerminalPresentationPTY: PASS3.34s. Normal140x40
  and narrow54x18 captures show multiline commands, quoted target, exit0, output
  indentation/blank lines and separate native-choice labels. Refresh returned the
  synthetic authoritative ledger; panel-second remained selected during streaming
  and resize. A middle-cursor edit and multiline bracketed paste returned exactly
  `aXb\n  quoted 'value'\n\nend`. Narrow scrolled body rows2–9 remained identical
  across streamed output. pty-*.txt retains captures/comparison and restoration.
  This is a real terminal presentation fixture; it does not connect the PTY to
  the native qualification job below. An initial pilot assertion failed because
  paste was intentionally inserted before the final b; sending Right before the
  paste corrected the pilot sequence. The final exact-input assertion passed.

One combined installed-native qualification, through the existing production
repository/controller/codex.Client (not nested Omnidex):
`combined.test -test.run TestCodingCombined -test.v -test.timeout=590s`, host cwd
internal/queue; explicit installed command /home/gryph/.local/share/mise/shims/codex,
OMNI_TEST_COMBINED_NATIVE=1, existing manifest routes/settings, task-local TMPDIR,
synthetic Unix-socket DB, and OMNI_TEST_COMBINED_EVIDENCE at the retained path.
The binary was compiled from all current production queue files plus the three
fixture/recovery/combined test files using static CGO linking in the bounded
runner. An attempted CGO-disabled build failed because tree-sitter requires CGO;
static CGO linking succeeded. No installation or existing binary replacement.

Final combined-native.txt PASS48.43s (an earlier successful48.93s run preceded the
complete evidence-export hook). Approval used ordinary InterpretUserIntent /
ExecuteUserIntent boundaries with scripted semantic choices. Actual execution and
independent review used dispatched installed native sessions; the pilot supplied
only approval, interrupt and resume controls. Execution wrote the requested11
bytes `alpha\nbeta\n` and verified hex61 6c 70 68 61 0a 62 65 74 61 0a, exit0.
Independent review separately read those bytes and ran its own Python equality
check, exit0. The pilot interrupted through InterruptJob after native review's
effect, invalidating the old attempt before the controller could commit its result.
Fresh repository reconnect and ordinary resume recovered that original review
turn. One synthetic malformed review-parser response exercised failure and a
second ordinary resume/retry. The final reconnect observed the same completed
job/generation; duplicate resume receipts and file bytes were identical. Native
effect counts execute1/review1; parser counts execution_response1/review_response2.
No native work was redispatched and no SQL state patch was used.

Full413-row synthetic native evidence is retained losslessly in
bin/terminal-checks-20261019/combined-native-evidence.jsonl.gz (35,865bytes;
compression round-trip checked). Execution thread01a15421-2ee7-7a10-84f6-fb5a8cb4ac03,
turn01a15421-3511-7051-b31e-99a0e6b1d042; independent review
thread01a15421-7bf8-7972-aac4-0c5c15e5e94c,
turn01a15421-8191-7783-adeb-7919d1a77c5b. Synthetic evidence413 is task_accepted.
This proves the combined native dispatch/recovery protocol for this synthetic
fixture, with scripted approval meaning and an injected parser fault; it is not
an autonomous workload benchmark or historical job21/job22 acceptance/review.

Preservation: all25 original manifest hashes match, including installed binaries,
activation-preserved artifacts, schema source and protected parser source. No live
DB was connected. Job21 generation1/ledger19 frozen6/evidence149667–149669 and
job22 history/receipts/accepted p_821e1757350d_3_task state_version2 were not edited
or reopened. Task2's original report/checks/history and unreached independent
review remain intact. Parser changes remain source-only. Source manifest records
HEAD and current changed-file hashes. Final git diff --check passes.

Cleanup: native task scope inactive; both auto-remove containers stopped and the
filtered task listing is empty; private tmux server/socket, exported test binary,
synthetic DB/workspaces/socket, tmpfs build caches and task processes retired.
Existing native session history/evidence preserved. bin/agent-runtime empty.
Sampled native scope110MiB, runner383MiB, DB135MiB, within limits. Necessary
retained evidence124KiB; final available RAM24637MiB, swap used18894MiB, disk44GiB.
Owner/shared caches/builds/resources were preserved. No installation, service
restart, live-schema write, source copy/worktree or destructive reset.

Stages: implemented; verified as scoped above; delivered locally as source and
persisted evidence (no commit/push/install/activation requested); cleaned. Later
code-owned review/acceptance and owner activation remain separate stages.
### Paused-planning intent — documentation assignment boundary, 2026-10-19

Frozen assignment supplied to this execution: `{"task":{"write_files":["docs/EVIDENCE_LEDGER.md"]}}`.
Authoritative root: `/home/gryph/Projects/tools/omnidex`; inspected HEAD
`3b252b684449447fd84bc122bace2ae494b11bf1`, with existing owner changes.
This documentation-only write set does not authorize implementation source,
test files, or candidate outputs. No additional task was created and no alternate
execution/review pipeline was invoked. Code-owned independent review and acceptance
remain later workflow transitions, not evidence supplied by this executor.

Read-only source inspection confirms the reported boundary remains present:
`reconcilePlanReview` clears review on `currentJobInterrupted()`;
`acceptUserMessage` supplies `LedgerRevision` only with nonnil `planReview`;
`userIntentContextTx` derives `Planning` from a waiting current-generation
`v3_coding_plan`; `InterpretUserIntent` rejects differing revisions when either
the request supplies a positive revision or that derived `Planning` is true.
Consequently the specified revision-zero request against proposed revision 2
is rejected before the semantic callback by the current condition. This is static
inspection, not an executed synthetic or model result. Execution still rechecks
state, revision, recoverability, job, generation, ledger, plan state, planning,
native prompt, choices and input identity in `lockUserIntentContextTx`; inspection
does not establish runtime preservation of those checks after a future repair.

Commands: `pwd`; scoped `rg --files` and `rg -n`; `git status --short`;
`cat`/`sed` of AGENTS.md, README workflow guidance, the intent admission/context,
execution and CLI paths; `git rev-parse HEAD`; `git diff --check --
docs/EVIDENCE_LEDGER.md`; `free -m`; `df -m .`; `du -sm bin`.
Capacity before any heavy operation: available RAM 24,516 MiB, swap used
18,903 MiB of 21,969 MiB, filesystem available 44,685 MiB, existing bin 663 MiB.
No heavy operation ran. One attempted read of nonexistent
`internal/sessionintent/registry.go` failed; scoped discovery located the registry
in `internal/sessionintent/contracts.go` and its planning capability condition
was inspected. No test/build success is inferred from these reads.

Limitations: no implementation, synthetic CLI→HTTP→repository verification,
authority/replay/control/native regression run, model run, static-CGO build,
candidate identity or SHA-256 is claimed by this documentation assignment.
`bin/paused-intent-activation-20261019` was not created. The broader requested
repair and candidate qualification require their ordinary frozen implementation
assignment; this entry must not be treated as their completion or acceptance.

Preservation and cleanup: no live database, job17, ledger15, owner conversation,
job22/23 history, installed binaries, active release, private preservation set,
credentials or settings was accessed for fixture use or mutated. No installation,
restart, backup, source copy, container, test process, cache or disposable artifact
was created; existing shared resources were retained. Only this ledger was edited.

Stages for this assignment: implemented documentation of the concrete authority
boundary; verified by scoped source/content inspection and ledger whitespace
check; delivered locally in this ledger, without commit/push; cleaned, with no
task-owned disposable resources. Product repair remains unimplemented/unverified
by this assignment.

### Paused-planning intent — continued execution completed, 2026-10-19

Owner continuation clarified that the evidence-delivery file-list fragment is
not an exclusive write allowlist. This supersedes the authority interpretation
in the preceding entry; that entry remains historical. Same authoritative tree,
HEAD `3b252b684449447fd84bc122bace2ae494b11bf1`, existing owner changes preserved.
No new task, schema change, source copy, installation or live restart.

Implemented the existing context/admission boundary in
`internal/queue/user_intent.go`, `user_intent_context.go`,
`user_intent_execute.go` and `user_intent_actions.go`. A zero displayed revision
now admits ordinary text when session authority matches. A positive supplied
revision still must exactly match the current authoritative revision. The
semantic/action context withholds planning capabilities and displayed items when
the caller supplies no displayed revision; a selected item without that authority
is rejected. Persisted planning state remains intact for the existing exact
execution-time comparison. Neither the request nor its observed revision is
filled from server state. Execution validates actions against the same displayed
binding, including the original binding when directly confirming a proposal.
Existing job/generation/actor/workspace/operation/revision/native-input checks,
registry, dispatcher and real plan-review path remain the authorities. No phrase
matching, fallback or second interpretation pipeline was introduced.

New `cmd/omni/chat_paused_intent_integration_test.go` uses ordinary
`reconcilePlanReview` → `acceptUserMessage` → client → HTTP → semantic service →
repository/PostgreSQL. Synthetic public enqueue/replan/ledger-mutation/interrupt
operations establish waiting_input, generation4, proposed ledger revision2,
no frozen revision, waiting current planning step and persisted interruption.
The CLI clears its review and sends exactly
`What is the current Omnidex job status?`, unchanged, with displayed revision0
and no selected item. Two scripted semantic calls select status and no additional
intention; the receipt is `Job 1: waiting_input.`, generation4/revision2. Snapshot
job/session and complete current ledger comparisons remain identical after status
and receipt replay. The test rejects approve/remove/discuss-item/start/revise
without display authority, explicit stale revision1, and execution after an
ordinary state-changing replan. The stale request causes no provider call.
A separate fresh interpretation before that replan adds two scripted calls.
Exact scripted prompts/responses and final receipt are retained in
`bin/paused-intent-activation-20261019/components-final.txt`.

Current verification, sequential in the isolated runner, all indicated checks
passed unless explicitly stated otherwise:

- `queue.txt`: every production queue file plus
  `user_intent_fixture_test.go user_intent_integration_test.go
  user_intent_native_integration_test.go coding_recovery_integration_test.go`;
  `go test -v -p 1 ... -run 'TestUserIntent|TestFrozenCodingRecovery'
  -count=1 -timeout=150s`, PASS24.068s. Covers exact context/actor/revision,
  operation collisions and in-flight serialization, interpretation and receipt
  replay, direct nonrecursive confirmation, shared typed plan controls,
  interruption/cancellation, current native choices and frozen paused recovery.
- `api.txt`: every production API file plus
  `cli_chat_session_identity_integration_test.go user_intent_integration_test.go
  user_intent_interpret_test.go job_resume_integration_test.go`;
  `go test -v -p 1 ... -run
  'TestUserIntent|TestSessionUserIntent|TestCodingEnqueuePublicResumeRecovery'
  -count=1 -timeout=150s`, PASS10.995s. Includes real client/HTTP/service/repository,
  stale admission, lost committed response/reconnect and exact receipt reuse,
  direct confirmation, native input and ten synthetic public recovery modes.
- `components-final.txt`: `go test -v -p 1 ./cmd/omni ./internal/client
  ./internal/sessionintent ./internal/userintent ./internal/codex
  ./internal/codingagent -count=1 -timeout=90s`. CLI PASS2.052s, including the
  final new paused fixture PASS0.92s and existing real plan-review interaction,
  concurrent review changes and native-input presentation/routing checks.
  Sessionintent/userintent/codex/codingagent PASS0.001/0.003/0.005/0.008s.
  Aggregate command exit1: full client tests fail compilation on existing
  `coding_plan_test.go` references to removed Constraints/LedgerConstraint/
  Priority/LedgerPriorityNormal. No unrelated test or model was changed.
- `client.txt`: every production client file plus `chat_transport_test.go
  lifecycle_transport_test.go resume_transport_test.go session_recovery_test.go
  session_planning_reply_test.go session_message_reply_test.go`;
  `go test -v -p 1 ... -count=1 -timeout=90s`, PASS0.005s. Exact paused-control,
  planning/native reply and recovery transport bindings pass.

For selected-file commands above, production arguments were mechanically supplied
with `$(go list -f '{{range .GoFiles}}{{.}} {{end}}' .)` inside the affected
package directory. Components and transport commands compile all production
files in their packages. Full queue/API/client suites are not claimed. PTY and
five installed/native-model opt-in tests were skipped. These checks use scripted
semantic/native providers and disposable synthetic data; no real-model accuracy,
live-job recovery or independent acceptance is claimed. Preliminary fixture
attempts exposed schema-name length, missing scripted final token counts, missing
console, comparing in-process versus JSON timestamp representations, and reuse
of the captured stale envelope; harness corrections retained the actual product
assertions. Earlier `paused-cli.txt` is a successful pre-export run; final source
proof is `components-final.txt`.

Capacity/resources: measured before heavy work, available RAM24,502MiB,
swap18,901/21,969MiB used, disk44,701MiB available. Purpose/budget/cleanup declared
in `resource-plan.txt` before the runner/DB were created. Task-owned containers
`omnidex-paused-intent-runner` and `omnidex-paused-intent-db` used --rm,
--network none, read-only roots, source/module cache read-only; runner2CPU/2GiB,
no swap/256PIDs/cache768MiB/tmp768MiB tmpfs, lifetime1800s; DB1CPU/384MiB,
no swap/128PIDs/data256MiB/tmp16MiB tmpfs, synthetic Unix socket only. DB settings:
shared_buffers16MB, max_connections20, max_wal_size32MB, min_wal_size32MB,
checkpoint_timeout30s. Only the synthetic URL
`postgres://postgres@/postgres?host=/fixture&sslmode=disable` was supplied.
One heavy operation at a time; test limits90/150s, builds600s each. No production
credentials, ports, volumes, data or live schemas were used.

Static-CGO candidates were built sequentially with Go1.24.1 linux/amd64,
CGO_ENABLED1, Alpine GCC14.2.0, GNU ld2.43.1, GOMAXPROCS2, GOPROXY=off:

```sh
timeout 600s go build -p 1 -ldflags '-linkmode external -extldflags -static' -o /out/omnidex ./cmd/omnidex
timeout 600s go build -p 1 -ldflags '-linkmode external -extldflags -static' -o /out/omni ./cmd/omni
```

Both exit0. Existing builder image
`omnidex-r6-snapshot-debug-builder:local` identity
`sha256:4c1b27b8fdbcdd877fef43fef587451d4862384f0509c8c464c18e81b0fe98e5`;
PostgreSQL image identity is retained in `images.txt`. Compiler, module/build
settings, static ELF inspection and candidate CLI `version --json` success are
in `toolchain.txt`, `build-info.txt`, `static-check.txt` and
`candidate-cli-version.json`. No version or acceptance label was invented.
Candidates remain only under ignored `bin/paused-intent-activation-20261019`:

| Candidate | Bytes | SHA-256 |
| --- | ---: | --- |
| omni | 11,268,328 | 883db3306508ca65b217e7321fc592aea1ff4d3f847cc0e3ef44f8ed74e64c76 |
| omnidex | 33,912,944 | 80481f86cf1eb334dc72b238846ebaa20832eb3cfd1c98b36dec600c9a1c01fa |

`source-manifest.json` SHA-256
`8f0275ef893c829d69d627f4da7c306395dfe91ef4dbe0e6785e88602aa2d45c`
binds all1,864 current source/test/asset paths, root and HEAD, excluding this
append-only build-evidence output. Post-build comparison: all manifested paths
unchanged. `source-status.txt` preserves the dirty-tree identity; SHA256SUMS binds
both candidates and the manifest. These are hash identities, not source copies.
No candidate core server startup is claimed; candidates are built and inspected,
while affected production boundaries were exercised by the tests above.

Preservation: `before.json` was taken after the initial admission/context/
execution edit, before gofmt. Comparison identified only subsequent scoped
confirmation validation and context-helper formatting; the new test appears in
the final manifest. An initially too-narrow comparison rejected that formatting
change; inspection corrected the expected scoped set, not any protected hash.
All17 checkpointed protected binary/private-preservation/schema paths and all
other checkpointed source/settings hashes match (`preservation.txt`). Neither
private preservation set was copied or changed. There was no live DB connection,
so live job17/ledger15/owner conversation, accepted jobs22/23 and every other live
job/history were preserved by no access; no live-row equality claim is made.
Installed/active binaries remained unchanged. No service restart or installation.

Cleanup: both task containers stopped and auto-removed; the immediate listing
briefly showed the runner Dead during removal, and the final filtered listing is
empty. Socket directory and formatting scratch removed; synthetic schemas,
workspaces, data, processes and tmpfs caches retired with the containers.
Existing shared caches/builds and native history retained. Sampled runner319MiB,
DB130MiB, within limits. Retained packet approximately45.8MB, below128MiB; final
available RAM24,667MiB, swap19,061MiB used, disk44,673MiB available.
Final source-manifest/preservation and `git diff --check` checks pass.

Stages for this continued execution assignment: implemented the requested repair;
verified affected acceptance/authority checks as qualified above; delivered locally
as source/test/evidence and source-bound static candidates, no commit/push/install;
cleaned task-owned disposables. Omnidex's separate frozen-task independent review
and recorded acceptance follow this execution report. Pilot installation/restart
remains deferred until independent acceptance; this session did not self-restart
or dispatch an alternate reviewer.

### Paused-planning intent — independent frozen-task review, 2026-10-19

Independent review of this same paused-planning repair, completed 13:46 UTC.
The supplied `write_files` fragment is context, not an exclusive access allowlist.
Reviewed the existing implementation without modifying implementation or tests;
this review adds only evidence and ignored verification outputs. Same root and
HEAD `3b252b684449447fd84bc122bace2ae494b11bf1`. No additional task,
alternate workflow, acceptance recording, installation or restart was performed.
Omnidex's later correction/acceptance transition is outside this review report.

Inspection confirms the repair is confined to admission and displayed-context
projection in the existing pipeline. Revision zero does not manufacture an
observed revision. It withholds planning capabilities/items; a selected item
without displayed authority is rejected. Positive revisions retain exact
comparison. The unprojected authoritative context is persisted and compared at
execution, with displayed-context validation before dispatch and direct proposal
confirmation. Actor/workspace/job/generation/ledger/operation/native-input checks
remain present. No phrase classifier, schema change, fallback or second pipeline
was introduced by this repair. All 1,864 source-manifest paths matched before and
after this independent run, including schema, CLI and the four repaired queue
files. No task-specific defect was found.

Executed the new `TestPausedPlanningOrdinaryUserIntent` through ordinary
`reconcilePlanReview` / `acceptUserMessage`, client, HTTP, semantic service and
repository against disposable PostgreSQL. The synthetic fixture asserts
waiting_input, generation 4, proposed revision 2, no frozen revision, a waiting
planning step and persisted interruption clearing CLI review. Exactly
`What is the current Omnidex job status?` reached the provider unchanged, with
request revision zero and no selected item. The first two scripted semantic
responses select status and no additional intention. Receipt: `Job 1:
waiting_input.`, generation 4, authoritative revision 2. Complete current ledger,
job and session comparisons remained equal after status and receipt replay.
Approve/remove/discuss-item/start/revise without displayed authority were
rejected. Explicit revision 1 was rejected before another provider call. A fresh
interpretation used two further scripted calls; a subsequent ordinary replan
caused execution to reject changed state. Exact prompts/responses and receipt
are in `bin/paused-intent-activation-20261019/independent-review/components.txt`.
This is scripted semantic/provider evidence, not real-model accuracy evidence.

Commands and observed results, run sequentially in the established isolated
builder with `GOMAXPROCS=2`, `GOCACHE=/cache`, `GOPROXY=off`, `CGO_ENABLED=1`:

```sh
go test -v -p 1 ./cmd/omni ./internal/sessionintent ./internal/userintent ./internal/codex ./internal/codingagent -count=1 -timeout=90s
# In internal/queue, production files plus the existing affected fixtures:
go test -v -p 1 $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .) user_intent_fixture_test.go user_intent_integration_test.go user_intent_native_integration_test.go coding_recovery_integration_test.go -run 'TestUserIntent|TestFrozenCodingRecovery' -count=1 -timeout=150s
# In internal/api:
go test -v -p 1 $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .) cli_chat_session_identity_integration_test.go user_intent_integration_test.go user_intent_interpret_test.go job_resume_integration_test.go -run 'TestUserIntent|TestSessionUserIntent|TestCodingEnqueuePublicResumeRecovery' -count=1 -timeout=150s
# In internal/client:
go test -v -p 1 $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .) chat_transport_test.go lifecycle_transport_test.go resume_transport_test.go session_recovery_test.go session_planning_reply_test.go session_message_reply_test.go -count=1 -timeout=90s
# Recheck the reported full-client compilation limitation:
go test -p 1 ./internal/client -run '^$' -count=1 -timeout=90s
```

The first four commands exited 0. Component package times were
2.093/0.001/0.003/0.003/0.007s; queue 24.925s, API 10.837s and selected client
0.005s. Logs are `components.txt`, `queue.txt`, `api.txt` and `client.txt` in the
review directory. These executed exact authority and stale-context rejection,
operation collision/serialization, interpretation/receipt replay, direct
nonrecursive confirmation, paused interrupt/cancel/recovery controls, current
native choices, typed ledger controls, real plan-review input/mutation and
concurrent review changes. The full-client compile command exited 1, reproducing
the supplied existing `coding_plan_test.go` references to removed Constraints,
LedgerConstraint, Priority and LedgerPriorityNormal (`full-client-compile.txt`).
Those tests were not changed. Full queue/API/client suites are not claimed.
The existing PTY test and five installed/model opt-in tests skipped; no installed
CLI session, real model or candidate core startup was exercised.

Independently rebuilt both candidates sequentially in runner tmpfs using the
recorded static-CGO procedure, each bounded by `timeout 600s`:

```sh
go build -p 1 -ldflags '-linkmode external -extldflags -static' -o /tmp/review-omnidex ./cmd/omnidex
go build -p 1 -ldflags '-linkmode external -extldflags -static' -o /tmp/review-omni ./cmd/omni
```

Both builds exited 0 and `cmp` matched the retained candidates byte-for-byte;
comparison outputs were removed immediately. Existing builder image
`sha256:4c1b27b8fdbcdd877fef43fef587451d4862384f0509c8c464c18e81b0fe98e5`,
Go 1.24.1 linux/amd64, Alpine GCC 14.2.0 and GNU ld 2.43.1 were independently
observed (`rebuild.txt`). `sha256sum -c .../SHA256SUMS`, static ELF inspection,
candidate CLI `version --json` and `git check-ignore` passed. Retained identities:

- `omni`, 11,268,328 bytes: `883db3306508ca65b217e7321fc592aea1ff4d3f847cc0e3ef44f8ed74e64c76`.
- `omnidex`, 33,912,944 bytes: `80481f86cf1eb334dc72b238846ebaa20832eb3cfd1c98b36dec600c9a1c01fa`.
- `source-manifest.json`: `8f0275ef893c829d69d627f4da7c306395dfe91ef4dbe0e6785e88602aa2d45c`;
  root/HEAD and all 1,864 current source/test/asset hashes matched throughout.

Resource/preservation/cleanup commands included `free -m`, `df -h .`,
`df -m .`, `du -sm` of this packet, Python SHA-256 manifest/checkpoint comparisons,
`docker image inspect`, scoped `docker inspect`/`stats`, read-only
`systemctl --user show omnidex.service`, active `/proc/<pid>/exe` hashing,
`docker stop -t 5 omnidex-paused-review-runner omnidex-paused-review-db`,
filtered `docker ps -a`, socket removal and `git diff --check`.
The review's resource plan was recorded before creating resources. Initial
available RAM 24,671 MiB, swap used 19,107/21,969 MiB, disk 44 GiB available.
Before rebuild: available RAM 24,134 MiB, swap used 19,215 MiB, disk 44,672 MiB.
Runner used 2 CPUs/2 GiB/no swap/256 PIDs, cache/tmp 768 MiB each;
PostgreSQL used 1 CPU/384 MiB/no swap/128 PIDs, data 256 MiB/run/tmp 16 MiB each.
Both had read-only roots, `--network none`, `--rm` and 1,200-second lifetime;
source and existing module cache were read-only, with only review output/socket
mounts writable. PostgreSQL used the same bounded settings as execution and only
`postgres://postgres@/postgres?host=/fixture&sslmode=disable`. No Docker socket,
live data, production credentials or production port was mounted. Actual limits
and mounts are retained in `containers.txt`; sampled runner memory was 314 MiB,
DB 118.2 MiB. Only one heavy operation ran at a time.

All 17 checkpointed preservation/binary/schema hashes matched independently
before and after review. Both private preservation sets were hashed without
copying or changing them. The active core PID 2722811, start time, active/running
state and executable SHA-256
`ff15b2410ef2af0422aadab65fea89c309386b92a413d6e187e22d32a8cd912d`
remained identical. Source/settings were unchanged. No live DB connection or
access to job17, ledger15, the owner conversation, jobs22/23, other live jobs or
historical revisions occurred; their preservation follows from no access, not a
live-row comparison. No installation, restart, backup or source copy occurred.

Both task-owned containers were stopped and auto-removed; a bounded final listing
confirmed neither remained. The socket directory was empty and removed.
Synthetic database/workspaces, comparison binaries and tmpfs caches retired.
Shared images/caches and every pre-existing resource were retained. Final packet
45,881,396 bytes (about 43.8 MiB), below 128 MiB, including review outputs.
Final available RAM 24,512 MiB, swap used 19,214 MiB, disk 44,670 MiB available.
`final-checks.json` records unchanged source/preservation/service identities and
cleanup; candidate checksums remained valid.

Stages for this independent-review assignment: implemented the review evidence
without implementation edits; verified the repair and affected checks above,
with the explicit full-client/PTY/model limitations; delivered this local review
and retained source-bound candidate identities, without commit/push/install;
cleaned all review-owned disposable resources. Independent review found no
task-specific defect. Acceptance recording and pilot installation/restart remain
later external transitions, not prerequisites or actions of this review.

### Job 25 / ledger 23 / revision 6 — task p_f1a58327a78d_1_task

Execution assignment: replace the user parser contract with a single structured
state-patch interpretation against the actual server snapshot, unchanged message,
and bounded relevant context. Existing execution admission and authority remain
in force. No schema, installation, production restart, AGENTS.md, duplicate prompt
display or unrelated historical tests are in scope. Root remains
`/home/gryph/Projects/tools/omnidex`; all pre-existing dirty work is retained.
This entry records execution evidence, not acceptance or independent review.

Implementation in progress: generic RFC6902 patch envelope, session snapshot and
state-transition mapping, shared context minification, targeted tests. Verification
plan: isolated Docker component/API tests, read-only source and existing module
cache; one container, 2 CPUs / 2 GiB RAM / no swap / 256 PIDs, cache/tmp each
768 MiB tmpfs, no network, 600-second lifetime. Owner: this task. Cleanup condition:
tests complete or fail; auto-remove container and tmpfs; retain only this evidence.
No source copy, persistent test services or build output. Initial RAM available
24,694 MiB, swap used 19,463 MiB, disk free 44 GiB. Existing `bin` 665 MiB and
`.agents` are pre-existing and retained; no owned stale artifacts found.

Owner continuation corrected the partial implementation: the transition list was
an action menu disguised as patches and was rejected. Removed it from the reusable
parser; no `permitted_changes`, `content_path`, option or callback protocol remains
in that package. The previous container auto-removed after a compile failure
(`validateDelta` source was added while its compile was already reading files).
That run is not current verification. No retained task container was present on
resume. RAM available 24,673 MiB; swap used 19,506 MiB; disk free 44 GiB.

Current task boundary: the reusable parser infers deltas directly from snapshot,
message and context. Caller-supplied writable-field and typed-state validators
are code-only and never rendered. The existing action caller/dispatch protocol
is isolated in `sessionintent` to preserve existing caller behavior until its
separate task-2 replacement; it is not a foundation for the state parser. The
partial API transition-menu adapter was removed. Shared context minification
now covers all relevant persisted facts/history/pending context, not just long
individual messages. No current message or snapshot reduction occurs.

Verification resource plan remains unchanged. Runner now lives at most 600s to
retain only its private tmpfs compiler cache across any necessary correction;
stop and auto-remove on completion. No persistent cache/output is created.

Task-1 execution completed (2026-10-19):

- Implemented `userintent.Input` with the actual code-built JSON snapshot,
  unchanged message, dedicated context, and code-only writable/typed-state
  validators. One semantic response contains RFC6902 state deltas or a focused
  clarification. No model-visible transition catalog, patch matching, action
  identities or callback selection remains in this parser. It neither applies
  effects to the supplied state nor calls execution machinery.
- Structural validation rejects unchanged values, duplicate/overlapping paths,
  absent parents, invalid pointers/indexes, unsupported operations and malformed
  responses. The consumer's writable fields and typed-state invariants validate
  the proposed result before any patch is returned. JSON integer precision is
  retained. Clarification cannot contain effects and needs no second call.
- Dedicated relevant context accepts at most two user/assistant exchanges and
  32 caller-scoped persisted facts. Shared `contextcompiler.ReduceRelevant`
  uses the existing grouping/minification prompt/decoder. Short input is kept
  verbatim. Large input is bounded at 128 KiB before reduction, with at most
  24 KiB returned context; no repeated squeeze/retry loop. Current message and
  actual snapshot stay separate and unminified.
- Task-2 boundary preserved: existing session option interpretation and dispatch
  now reside in `sessionintent`, outside the reusable parser. Existing API and
  queue still consume that caller protocol until task 2 removes it. The rejected
  partial API patch-to-action adapter is gone. No new integration architecture
  or action menu was substituted for the requested direct state interpretation.

Concrete final checks, existing builder image
`omnidex-r6-snapshot-debug-builder:local`, read-only `/src` and module cache,
`GOMAXPROCS=2`, `GOCACHE=/cache`, `GOPROXY=off`:

1. `go test -v -p 1 ./internal/userintent ./internal/sessionintent ./internal/contextcompiler -count=1 -timeout=90s`
   passed all three packages (0.008s / 0.003s / 0.004s test runtime). This includes
   direct ordinary/compound patches in one interpretation call, exact actual
   `sessionintent.Context` snapshot and whitespace-preserved message, established
   job/item references, recipient/negation/quotation fixtures, clarification with
   no effects, minimal deltas, typed/writable state checks, sequential array edits,
   precision, context bounds, shared minification and failure/cancellation.
2. From `internal/api`:
   `go test -v -p 1 $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .) user_intent_interpret_test.go -run 'TestSessionUserIntent|TestUserIntentAPI' -count=1 -timeout=90s`
   passed all six preserved caller checks, 0.003s.
3. From `internal/queue`:
   `go test -p 1 $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .)`
   compiled all production queue source successfully (`[no test files]`).
4. `git diff --check` passed. Search found no old `userintent` option/selection/
   dispatch references and no transition-menu implementation; forbidden menu
   names occur only in a negative test assertion.

An intermediate component run caught a `Resume` identifier collision when moving
retained dispatch into the existing caller package. Renamed that disposition to
`Continue`; final component/API/queue checks above include the correction.
These are scripted contract/component tests and production-source compilation,
not live-model language-quality evidence, DB integration, activation or independent
review. No production model, credentials, database or service was accessed.

Source/test revision binding: SHA-256
`60fa2aea6fcbf70b101c2c75795ee50bddbe3451ad14866b3297c398f69f0d11`
over concatenated sorted `sha256  path\n` entries for the seven Go files under
`internal/userintent`, `internal/sessionintent/{interpret,dispatch,dispatch_test}.go`,
`internal/api/user_intent_interpret{,_test}.go`,
`internal/queue/user_intent_execute.go`, and `internal/contextcompiler/reduction.go`
(14 files). No source changes followed final verification.

Cleanup: stopped only `omnidex-parser-patch-tests`; its --rm cleanup briefly
reported Dead, then a final filtered listing was empty. Private compiler cache
and test binaries/tmpfs retired with that container. Peak sampled runner memory
462.7 MiB in the earlier attempt; final runner sampled 312.2/244.7 MiB, within
2 GiB. No host output/cache was created. Existing `bin`, `.agents`, shared images
and caches retained. Final available RAM 24,749 MiB; swap used 19,615 MiB;
44 GiB disk free. No changes to AGENTS.md, duplicate prompt display, settings,
accepted/dirty work outside the listed task files, live schemas, installed
binaries, other jobs or production services were made by this execution.

Stages: implemented task 1; verified the concrete checks above; delivered local
reviewable source/tests and this existing evidence record, without commit, push,
installation or restart; cleaned task-owned resources. Caller integration remains
in its separate frozen task. Activation has no schema requirement from this task;
normal candidate rebuild and later caller integration are external to this
execution assignment. Independent review and acceptance remain Omnidex's later
code-owned transitions.

Task-1 supplied-finding correction (same job 25 / generation 1 / ledger 23 /
revision 6 / p_f1a58327a78d_1_task, 2026-10-19):

- Corrected repeated RFC6902 `add /items/-`: each append addresses a new
  element; ancestor/descendant and other duplicate-path rejection remain.
- Corrected repeated persisted context by giving the shared reducer's chunks
  its existing `session_` namespace semantics. Distinct chunk positions and
  repeated content remain intact; no shared validation policy was weakened.
- Minimal-delta validation replays candidate patches without each remove/add
  pair and rejects a pair when omitting it produces the identical complete
  result. This catches unchanged array-item restoration at a different index
  pointer, including when approval occurs between those operations. Actual
  item reordering remains valid. Work stays bounded by the interpreter's
  existing 32-operation limit; replay never invokes the model or effects.
- Added regressions for two appends, both operation orders of redundant item
  restoration around approval, real item movement, and a repeated 9,639-byte
  persisted fact through bounded shared minification. Existing parser/caller
  task boundaries remain as recorded above.

Resume resource inspection: no retained task container; available RAM 24,906 MiB,
swap used 19,686 MiB, disk free 44 GiB. Reused the preceding disposable-runner
plan/image/mounts/budgets; no new host cache or build outputs. Verification and
cleanup results follow below.

Current correction verification (final source, disposable Docker runner):

1. `go test -p 1 ./internal/userintent ./internal/sessionintent ./internal/contextcompiler -count=1 -timeout=90s`
   PASS (0.005s / 0.002s / 0.003s). Includes the supplied-finding regressions,
   literal object-key `-` duplicate-write rejection, and existing ordinary/
   compound single-call, reference/recipient, clarification/no-effects,
   actual-snapshot, unchanged-message, context-bound and failure checks.
2. From `internal/api`,
   `go test -p 1 $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .) user_intent_interpret_test.go -run 'TestSessionUserIntent|TestUserIntentAPI' -count=1 -timeout=90s`
   PASS. Preserved caller production source and interpretation checks compile/run.
3. `git diff --check` PASS. No old reusable-parser selection references found.

The first verification attempt compiled but could not execute binaries because
Docker's default tmpfs was noexec; it is not passing evidence. Removed that
runner, then recreated it with `exec` on its private `/tmp` tmpfs and reran the
checks above. A brief auto-removal name conflict required waiting for the old
runner to disappear; no alternative source tree or retained cache was created.
Peak sampled memory 479.6 MiB (first runner), 380.9 MiB (final runner), each within
2 GiB. These are scripted contract/component checks, not live-model quality or
DB/production activation evidence. No production access occurred.

Final source/test binding: `sha256sum internal/userintent/*.go | sha256sum`
returned `fd6a6af6ffa9cda6a68434a941666e9870e9e499dbf5bdb2779cf0be43d8229b`.
This correction changes only context.go, patch_validation.go and their tests,
plus this existing evidence record; preserves all other pre-existing work.

Stages: implemented task-1 corrections; verified concrete checks above; delivered
reviewable local source/tests and persisted evidence, without commit/push or
activation; stopped task-owned disposable runner and retired its private caches.
No schema, installed binary, production service, settings, other job, AGENTS.md,
duplicate prompt display or unrelated historical test contract was changed.
Independent review/acceptance remain later Omnidex-owned transitions.

Task-1 further supplied-finding correction (job 25 / generation 1 / ledger 23 /
frozen revision 6 / p_f1a58327a78d_1_task, 2026-10-19):

The current execution preserves the existing reusable-parser implementation and
separate task-2 caller boundary. Corrects repeated sequential array removals,
redundant multi-operation array deltas, and blank context partitions. Scope is
internal/userintent source/tests and this existing evidence record only.

Resource plan: owner task 1; purpose disposable component verification; container
omnidex-parser-finding-tests; existing omnidex-r6-snapshot-debug-builder:local;
read-only /src and /go/pkg/mod; network none; 2 CPUs, 2 GiB memory with no swap,
256 PIDs; private /tmp exec and /cache tmpfs each 768 MiB. Tests have 90-second
package timeouts. Container --rm retires private caches/binaries on exit; no host
outputs or additional source tree. Initial available RAM 25,038 MiB, swap used
19,875 MiB, disk free 44 GiB. Existing bin/.agents and unrelated containers are
preserved. Final verification results and source binding follow.

Implementation details: duplicate textual array remove pointers are now checked
against their live array parent, preserving RFC6902 sequential indexing and
object/ancestor overlap protection. Array deltas use a banded edit-distance check
bounded by the existing 32-operation interpreter limit; unchanged subtree replay
also rejects redundant groups without suppressing unrelated intended changes.
Shared context minification skips whitespace-only partitions, retaining nonblank
chunks and the original dedicated-context input limits. The unchanged message
and code-built server snapshot remain outside minification.

The initial disposable component run passed userintent/sessionintent/contextcompiler
(0.006s / 0.002s / 0.004s); sampled memory 357.2 MiB. Since final nested-array
lookup and distance regressions were added while dependency compilation was in
progress, this run alone is not the final source binding. A fresh sequential run
uses the same resource plan and additionally runs the preserved API caller checks.
No task container or private cache survived the initial run.

Final-source verification PASS:

1. `go test -p 1 ./internal/userintent ./internal/sessionintent ./internal/contextcompiler -count=1 -timeout=90s`
   passed (0.005s / 0.002s / 0.003s). Covers repeated removals, nonminimal
   remove/replace/append with an unrelated intended edit, bounded array distance,
   whitespace-only context partitions, and existing actual-snapshot/unchanged
   message, ordinary/compound single interpretation, recipient/reference,
   clarification/no-effects, context bounds, cancellation and failure contracts.
2. From internal/api,
   `go test -p 1 $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .) user_intent_interpret_test.go -run 'TestSessionUserIntent|TestUserIntentAPI' -count=1 -timeout=90s`
   passed (0.003s), preserving the separate caller task's production boundary.
3. `git diff --check` passed. Final `sha256sum internal/userintent/*.go | sha256sum`:
   `c34ae91dfaa7d66657b7d8f076360fa61c92f73a529b4318de57a2037e5069c3`.

Final runner sampled memory 413 MiB / 2 GiB. Its process exited successfully and
--rm removed the task container/private tmpfs caches; final filtered container
listing is empty. Existing shared resources were preserved. No host build output,
source copy, live schema, installed binary, service, settings, other job,
AGENTS.md, duplicate prompt display or unrelated historical test contract changed.
These checks prove scripted parser/component contracts and caller compatibility;
they do not claim live-model semantic quality or activation.

Stages: implemented the supplied task-1 parser corrections; verified final source
with the checks above; delivered local reviewable source/tests and this existing
record without commit/push or activation; cleaned disposable test resources.
No schema activation is required by these corrections. Later caller integration,
independent review and acceptance remain separate Omnidex-owned transitions.

Task-1 sequential-array supplied-finding correction (job 25 / generation 1 /
ledger 23 / frozen revision 6 / p_f1a58327a78d_1_task, 2026-10-19):

Confirmed overlap validation rejected remove-then-replace/edit of the shifted
successor and two numeric-index inserts. Corrected overlap classification for
array removals and insertions while retaining object/ancestor overlap rejection
and existing minimal-delta replay/distance validation. Added Interpret-boundary
regressions asserting exact resulting state, unchanged input snapshot, one
interpretation call and one typed validation for all three supplied cases.

Resource owner/purpose: this task, disposable component verification only.
Container omnidex-parser-sequential-tests uses the existing builder image,
read-only authoritative source/module cache, no network, 2 CPUs, 2 GiB RAM/no
swap, 256 PIDs, /tmp exec and /cache private tmpfs each 768 MiB; 600-second
lifetime, --rm cleanup on exit. No host cache/output/source copy. Initial
available RAM 24,982 MiB, swap used 20,026 MiB, disk free 44 GiB. Existing
bin/.agents, unrelated containers and shared caches retained. Scope remains two
userintent source/test files and this existing evidence record.

Final-source checks PASS in that disposable runner:
1. `go test -p 1 ./internal/userintent ./internal/sessionintent ./internal/contextcompiler -count=1 -timeout=90s`
   (0.007s / 0.002s / 0.004s). Includes all three new Interpret regressions and
   existing minimal-delta, overlap, reference/recipient, bounded context,
   clarification/no-effects, exact snapshot/message and single-call contracts.
2. From internal/api:
   `go test -p 1 $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .) user_intent_interpret_test.go -run 'TestSessionUserIntent|TestUserIntentAPI' -count=1 -timeout=90s`
   PASS (0.003s).
3. `git diff --check` PASS. Final source/test binding via
   `sha256sum internal/userintent/*.go | sha256sum`:
   `764c05d0a50ebb6dd321b015d4738e6666451d0668ea698e649755cf2d9549b1`.

Runner exited 0 and auto-removed; private caches/test binaries retired. Sampled
memory 286.1 MiB / 2 GiB. These are scripted component/contract checks, not live
model language-quality or DB integration evidence. No activation/schema change,
installed binary replacement or production restart occurred. Implemented and
verified task-1 correction; delivered reviewable local source/tests and this
existing record, without commit/push; cleaned task-owned resources. Independent
review/acceptance remain subsequent Omnidex-owned transitions.

Task-1 remaining sequential-index corrections (job 25 / generation 1 / ledger 23 /
frozen revision 6 / p_f1a58327a78d_1_task, 2026-10-19):
Overlap records now follow surviving array elements through insertion/removal.
Array-delta groups capture their actual array baseline at first edit and follow
that array through outer index shifts. This addresses both supplied findings;
Interpret regressions cover separate shifted item edits and nested removals before
and after an outer removal. Existing parser/context implementation is preserved.

Resource plan: task-owned disposable omnidex-parser-index-tests; existing builder
image; read-only authoritative source/module cache; network none; 2 CPUs, 2 GiB
RAM/no swap, 256 PIDs, private /tmp and /cache tmpfs each 768 MiB; 600-second
command lifetime; --rm cleanup on exit. No host output/cache or copied source.
Available RAM 24,929 MiB, swap used 20,023 MiB, disk free 44 GiB. Existing project
artifacts/shared caches remain untouched. Verification results follow.

Verification PASS:
- `go test -p 1 ./internal/userintent ./internal/sessionintent ./internal/contextcompiler -count=1 -timeout=90s`
  passed (0.007s / 0.002s / 0.003s). An additional component invocation after
  final overlap cleanup also passed (0.008s / 0.002s / 0.012s). It overlapped
  the initial dependency compilation in the same bounded container; no separate
  runner/cache was created. Container sampled memory 461.7 MiB / 2 GiB.
- Preserved API caller checks from internal/api:
  `go test -p 1 $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .) user_intent_interpret_test.go -run 'TestSessionUserIntent|TestUserIntentAPI' -count=1 -timeout=90s`
  passed (0.003s).
- `git diff --check` passed. Final parser source/test binding:
  `sha256sum internal/userintent/*.go | sha256sum` =
  `31d91f72ace5622e8fcd09f7530db8ec45cfaa6aafea03894d55e7010b05007d`.

Both command processes exited 0; --rm retired the container/private caches.
Implemented and verified the remaining task-1 corrections. Delivered reviewable
local source/tests and evidence in this existing record; no commit/push or live
activation performed. No activation/schema change is required by these fixes.
Dirty source, other jobs, settings, installed binaries, live schemas, production
services, AGENTS.md, duplicate prompt display and unrelated historical tests were
preserved. These are scripted component/contract checks, not live-model or DB
integration evidence. Independent review/acceptance belongs to Omnidex after this
execution response.

Task-1 append overlap correction (job 25 / generation 1 / ledger 23 /
frozen revision 6 / p_f1a58327a78d_1_task, 2026-10-19):
The supplied finding identified append records retaining `-` rather than the
inserted item's concrete index. Record the concrete index against the current
array before application; existing rebasing then follows subsequent shifts.
Interpret regressions reject appended-item field/whole-item overwrites, numeric
insertion equivalents and an appended item shifted by a later insertion. They
assert one interpretation call, no transition validation, no returned effects and
unchanged snapshot. Existing compound append and literal dash-key tests remain.
Resource plan: task-owned --rm container omnidex-parser-append-tests, existing
builder image, read-only source/module cache, network none, 2 CPUs/2 GiB/no swap,
256 PIDs, private tmp/cache tmpfs each 768 MiB, timeout 600s. Cleanup on command
exit; no host build outputs. Available RAM 25,133 MiB, swap used 20,015 MiB,
disk free 44 GiB. Pre-existing artifacts/caches preserved.

Verification PASS on final source:
- `go test -p 1 ./internal/userintent ./internal/sessionintent ./internal/contextcompiler -count=1 -timeout=90s`
  passed (0.007s / 0.002s / 0.004s), including the new append regressions and
  existing snapshot/message, minimal delta, ordinary/compound single-call,
  recipient/reference, clarification/no-effects and bounded context contracts.
- From internal/api, `go test -p 1 $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .) user_intent_interpret_test.go -run 'TestSessionUserIntent|TestUserIntentAPI' -count=1 -timeout=90s`
  passed (0.003s), verifying the preserved task-2 caller boundary.
- `git diff --check` passed. `sha256sum internal/userintent/*.go | sha256sum`:
  `21c70b813ff8356cc27af5cf138881628c235a621854dcf1de21d63184eceeff`.
The isolated command exited 0 and --rm retired its container/private caches.
These are scripted component/contract checks, not live-model or DB evidence.
Implemented and verified; delivered local source/tests and this existing evidence
record, without commit/push or activation. Cleaned task resources. No live schema,
installed binary, service, settings, other job, AGENTS.md, duplicate prompt display
or unrelated historical test changed. Independent review/acceptance remains
Omnidex-owned after this execution response.

### Job 25 / ledger 23 — task 2 authoritative state application (2026-10-19)

Frozen assignment: replace session action selection/catalog/dispatch and all
callers with typed state proposals; code validates and atomically persists the
exact delta, deriving continuation through existing ledger/lifecycle transactions.
Preserve replay, reconnect, cancellation, clarification, native input, same-job
recovery and one genuine confirmation. Preserve all pre-existing dirty work,
AGENTS.md, duplicate prompt display, settings, other jobs and accepted work.
Root remains /home/gryph/Projects/tools/omnidex. This is execution evidence;
Omnidex's later independent review and acceptance are not part of this assignment.

In progress: direct snapshot DTO and shared validation replace the action menu;
repository applies typed before/after differences using existing transactions.
Verification resource owner/purpose: task 2, disposable Docker tests only.
Budget: one Go runner, 2 CPUs/2 GiB/no swap/256 PIDs, private cache/tmp tmpfs
768 MiB each; PostgreSQL 1 CPU/384 MiB/no swap/128 PIDs with 256 MiB private
data tmpfs, private Unix socket in existing ignored .tmp path (1 MiB budget),
no networking, no production credentials/ports/data. Lifetime <=1200 seconds;
read-only source/module cache; --rm cleanup after checks, remove empty socket
path. Retain only source/tests and this existing evidence record. Pre-existing
bin/.agents/shared caches and running containers are not task-owned; retained.
Initial available RAM 25,148 MiB, swap used 20,011 MiB, disk free 44 GiB.

Task-2 execution completed:

- Removed the session action catalog, opaque selection loop, callback routes,
  dispatch implementation and their obsolete tests. API now uses the reusable
  parser once for a typed snapshot delta or clarification. Result/receipt/client
  DTOs carry RFC6902 patches, never model-selected actions. Existing CLI input,
  retry, cancellation and reconnect paths consume this contract.
- Snapshot authority binds workspace/actor/session, job/generation, ledger and
  revision, displayed plan eligibility, recovery and native input. Item identities,
  text and authority are protected. Only intended state fields are writable;
  required fields, typed values, compatible transitions and exact native choices
  are checked. Relevant conversation facts/history remain in the separately
  bounded context, without bypassing minification through the snapshot.
- Repository validates all proposed data independently of API/model validation,
  then rechecks current locked authority and reapplies the exact stored patch.
  Existing ledger, freeze, resume, interrupt, cancel and message transactions own
  persistence/continuation. A review delta changes only the addressed items;
  freezing derives the existing deterministic executor/reviewer continuation.
  Receipt and effects commit together. No action dispatcher or alternate ledger
  remains. Exact operation identities retain immutable result/receipt replay.
- Code computes confirmation eligibility and human-readable pitches from the
  delta. Confirmation authorizes only its exact stored pending proposal; replacing
  that proposal makes an earlier confirmation stale. Confirmation cannot confirm
  another confirmation, and a confirmation reply requires no recursive approval.
  Declines, native choices, cancellation and same-job recovery remain transactional.
- CLI verification found the persisted context query could return three user
  turns among four recent messages. It now selects at most two per role, preserving
  chronological order and the established two-exchange boundary.

Final isolated Docker verification (all passed, actual database URL only
`postgres://postgres@/postgres?host=/fixture&sslmode=disable`):

1. `go test -p 1 ./internal/userintent ./internal/sessionintent ./internal/contextcompiler -count=1 -timeout=90s`
   passed: 0.006s / 0.004s / 0.004s. Ordinary/compound one-call interpretation,
   exact snapshots/messages, clarification/no effects, writable/typed state,
   protected fields, minimal deltas, arrays, context bounds and cancellation.
2. From internal/queue:
   `go test -p 1 $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .) user_intent_fixture_test.go user_intent_integration_test.go user_intent_native_integration_test.go user_intent_patch_integration_test.go coding_recovery_integration_test.go coding_recovery_input_integration_test.go coding_combined_flow_integration_test.go coding_review_runtime_integration_test.go -count=1 -timeout=120s`
   passed, 30.157s. This compiles every queue production source and exercises
   real disposable PostgreSQL transactions: approval/freeze, exact replay, another
   actor, stale job/generation/workspace/ledger/native snapshots, protected-field
   rejection at intake and execution validation, immutable proposed provenance,
   cancellation before/after intake, genuine confirmation/decline and replacement
   rejection, exact native approval/denial, identity-collision serialization,
   same-job recovery across parser/disconnect/interruption boundaries, and the
   native executor/independent-review deterministic ledger loop.
   New rollback test fails the final receipt write after ledger mutation and proves
   both ledger and receipt roll back, then succeeds without changing task fields
   outside the proposed review state. No trigger/invariant was disabled.
3. `go test -p 1 ./internal/api -run 'Test(SessionUserIntent|UserIntent)' -count=1 -timeout=120s`
   passed, 2.042s. Real HTTP/client/provider-fixture/PostgreSQL checks cover
   clarification without effects, proposal reconnect, one confirmation, lost
   committed response and exact receipt replay, identity collisions and cancellation.
4. `go test -p 1 ./cmd/omni -run 'Test(PausedPlanningOrdinaryUserIntent|ChatPlanReviewNote|PlanReviewNote|PlanReviewConversationCannotBypassPendingReviewKey|PlanReviewNoteBytes)' -count=1 -timeout=120s`
   passed, 1.007s. Actual CLI session input/client/API/provider fixture/database;
   paused planning preserves job/generation/ledger, undisplayed or stale plan
   changes are rejected, and selected-item notes remain on the shared boundary.
5. `git diff --check` passed. Scoped source search found no legacy interpreter,
   action/catalog/selection/route types, callback dispatch, ForwardText or actions
   JSON fields in the replaced packages/callers. AGENTS.md has no diff.

Verification limits and corrected attempts: full queue/client package test
compilation remains blocked by unrelated historical tests referencing removed
EngineeringLedger.Constraints/Objectives/AcceptanceCriteria, codingagent.Check
and older completion signatures. Those tests were left unchanged. Queue checks
therefore explicitly enumerate the affected test files alongside all production
source; API and CLI package test sources compile normally. Model/native runtime
fixtures are scripted component/integration evidence, not live-model quality or
production activation evidence. Early attempts were discarded after noexec tmpfs,
PostgreSQL bootstrap socket-path failures, a later fixture EOF/recovery failure,
and the CLI context bug; none is represented as a passing check. A test initially
attempted stored-provenance tampering; the existing immutability trigger rejected
it. The corrected test asserts that protection and separately tests execution's
untrusted-data validator rather than weakening the trigger.

Final fixture used exec-capable private test tmpfs and both fixture/bootstrap
Unix socket paths. PostgreSQL WAL was bounded at 64 MiB (minimum 32 MiB), with
30-second checkpoints, wal_level=minimal/max_wal_senders=0 and fixture-only
fsync/full_page_writes disabled. These are rollback/transaction checks, not crash
or power-loss durability checks. Private data remained within 256 MiB (sampled
172.6 MiB); sampled runner memory 406.8 MiB and database 238.1 MiB, within budgets.
No production database/schema/data/credentials, Docker socket or network was used.

Source binding: SHA-256 `fa47d69830ce7389db6c442ebfee185b797d307dc3e0674fb0586c3272a96c66`
from `rg --files internal/userintent internal/sessionintent internal/queue internal/api internal/client cmd/omni | rg '\.go$' | sort | xargs sha256sum | sha256sum`.
No source/test edits followed these final checks. Existing go.mod SHA-256
`b2db2bc406cede90c9402676a60f993ed66e5da13168a5e3163f83c9707e4e91`,
go.sum `674645914ab7add7712b83b830fa13d88860bba595f8f8bae2e5833c8a4a543a`,
and database/setup.sql `5714a5bccbbffa3fe57994be4f4ff8cb3fb40f8cb5b3b7b3bcd711d6b43f2e79`
were read, not edited by this task.

Activation facts: this task adds no schema changes; existing JSON proposal and
receipt tables are used. The wire result changes from actions to patches, so
core and CLI source consumers belong to the same candidate build. Old unresolved
action-format results are not translated or treated as state-patch authority;
missing patches fail validation. No live migration, binary replacement, build
installation or service restart was performed or is claimed here.

Stages: implemented the supplied task 2; verified the concrete affected checks
above; delivered local reviewable source/tests and this existing job evidence
record, without commit/push/activation; cleaned all task-owned disposables.
Both final --rm containers were stopped; a filtered final Docker listing was
empty, and the empty .tmp/task2-socket directory was removed. Diagnostic and early
containers also auto-removed. Private database/cache/test outputs retired;
pre-existing bin/.agents/shared caches, containers and dirty/accepted source were
retained. Final available RAM 25,192 MiB, swap used 20,040 MiB, disk free 44 GiB.
No writes to AGENTS.md, duplicate prompt display, unrelated historical contracts,
settings, other jobs, installed binaries or live schemas/services occurred.
Independent review and acceptance recording remain Omnidex-owned later transitions.

### Job 25 / ledger 23 — task 3 isolated semantic provider reuse (2026-10-19)

Frozen scope: keep normal parsing on gpt-6.1-sol low; reuse one bounded,
tool-free provider through the established parser isolation boundary; fresh
threads for unrelated questions; diagnose/remove unrelated model-visible
instructions; preserve signed-in identity/configuration without copying source
or credentials. Verify reuse, separation, isolation, cancellation and shutdown.
Preserve dirty/accepted work, settings, AGENTS.md, duplicate prompt display and
unrelated historical test contracts. No live schemas, installed binaries or
production restart. Execution evidence only; Omnidex owns later independent review.

Finding: API userIntentModel constructed codex.New on every interpretation;
GeneratePreparedExact started a direct nonisolated process on every generation.
The existing startParserConnection privately masks profile AGENTS files, but was
not called by this path. The active profile has AGENTS.md; inspection of config
key names found no instruction/skill/personality/developer/profile overrides.
No profile values, credentials or source were copied. New parser process uses
private empty cwd, explicit base and empty developer instructions, existing
private profile masks, minimal environment and tool-disabled native transport.
Other station clients retain their construction and process lifecycle.

Resource plan: task 3 owns one disposable Docker component/build runner; existing
omnidex-r6-snapshot-debug-builder:local, read-only source/module cache, no network,
2 CPUs/2 GiB RAM/no swap/256 PIDs, private /tmp exec and /cache tmpfs 768 MiB each,
<=600 seconds. Namespace fixture requires SYS_ADMIN and private mount capability
inside this disposable container only. No production mounts/data/credentials.
Cleanup on completion/failure: --rm removes private compiler caches/test binaries;
fixture processes/scratch reaped by tests. Initial available RAM 25,107 MiB,
swap used 20,041 MiB, disk free 44 GiB; preexisting .tmp and bin/agent-runtime empty.

Implementation: API caches a lazy NewParser client per server lifecycle. A
one-slot context-aware gate serializes native RPC; initialize runs once per
process, and each question uses thread/start, ephemeral=true, explicit empty
developer instructions, exact requested model and low effort. Retained-context
requests fail explicitly. At most 64 questions share a process, bounding retained
native thread state; transport failures/cancellation reap it without replaying
inference. Later independent requests may start a new isolated process. Active
cancellation kills blocked reads/process group and joins the cancellation callback
before releasing the gate. Runtime.Close synchronously joins provider shutdown.
Private scratch is removed on reset/shutdown. No other semantic provider opts in.

Instruction isolation: reuse the existing Linux user/mount namespace boundary,
masking both global AGENTS filenames without host writes; suppress project docs,
host skill discovery, tools, apps/plugins/hooks/multi-agent/web and configured MCP
servers. Explicit base/developer instructions and a private cwd exclude unrelated
agent instructions. HOME/CODEX_HOME and normal profile/config identity stay in
place. Installed `codex features list` confirms skip_host_skill_discovery is
supported. No credential/source copy or profile-setting write was performed.

Corrected diagnostic attempts: initial fixture launcher lacked the Go test `--`
separator; then its response lacked required native usage evidence. Neither is
passing evidence. The actual namespace test also found BusyBox mount requires
an explicit remount source and preservation of locked nosuid/nodev/noexec flags
inside a nested user namespace. The shared private-mask remount now supplies
those flags; no host mount changes occur. A blocked fixture uses a sleeping
process (not Go's immediate deadlock exit). All early --rm containers retired.

Intermediate checks: parser process fixtures PASS (0.252s), API state/confirmation/
callback-rejection/server-provider ownership component tests PASS (0.004s), core
build PASS. HTTP/PostgreSQL user-intent integration was explicitly SKIPPED because
OMNI_TEST_DATABASE_URL is absent; no production database was connected. A final
unchanged-source pass includes all codex/llm/llmprovider tests, the affected API
components and core build. New assertions cover 65 questions/2 processes, waiting
cancellation, explicit active close, process reaping and host-profile preservation.

Final capability finding: installed feature registry additionally enables native
browser/computer/image/code-mode/skill-search and related tools by default.
The user-parser-only startup overrides now disable those capabilities, view-image,
sleep/goals, skill dependency installation, elicitation/suggestion, realtime,
workspace/worktree tools and shell snapshots. Shared station defaults are unchanged.
The actual subprocess fixture asserts these startup overrides as well as native
thread/tool configuration. Because this finding changed source after the earlier
pass, a new final-source test/build pass was started; earlier checks are not
represented as proof of these added overrides.

Final-source checks PASS, disposable isolated runner exit 0:
- `go test -p 1 ./internal/codex ./internal/llm ./internal/llmprovider -count=1 -timeout=120s`:
  0.260s / 0.263s / 0.002s. Actual stdio child-process and Linux namespace fixtures
  prove one initialize/process for two questions, different native thread IDs,
  exact gpt-6.1-sol/low requests, tool/instruction overrides, unchanged host fixture
  policy/config/identity, cancellation/replacement, 64-call process bound, canceled
  waiters, active/idle shutdown, child reaping and empty scratch afterward.
- `go test -p 1 ./internal/api -run 'Test(SessionUserIntent|UserIntent)' -count=1 -timeout=120s -v`:
  affected components PASS 0.005s, including concurrent callers sharing exactly one
  lifecycle-owned provider. Existing HTTP/PostgreSQL integration SKIPPED (no
  OMNI_TEST_DATABASE_URL); no database verification or live-model quality claim.
- `go build -p 1 -o /tmp/omnidex ./cmd/omnidex`: PASS inside disposable runner.
- `git diff --check`: PASS. AGENTS.md has no diff. No source/test changes followed
  these checks; only this existing evidence record was appended.

Final source binding: SHA-256 of ordered sha256sum output for
internal/codex/{parser_process.go,parser_process_test.go,parser_isolation.go,client.go,
connection.go,generation.go}, internal/api/{server.go,user_intent.go,
user_intent_provider_test.go}, internal/runtime/runtime.go:
`cc789a7cb5698e59fa7973d81c61c4ee964646f4192c4d2afe7a21a76d98aada`.
API user_intent.go was already untracked owner/task work; only provider selection
and the two lifecycle helper functions changed for this task. Preexisting dirty
Codex agent/parser tests and all other dirty/accepted work were retained.

Activation facts: no schema or wire-contract change. Candidate core must include
these sources; no CLI replacement is required by task 3. The running installed
core remains unchanged. Existing Linux namespace prerequisites (unshare/mount)
remain necessary when profile AGENTS files exist; failure remains explicit and
never falls back to an unisolated provider. No installed binary replacement,
production restart, live schema mutation, credential/source copy or settings
write occurred. Independent review/acceptance remain later Omnidex transitions.

Stages: implemented task 3; verified the concrete process/namespace/component and
build checks above; delivered reviewable local source/tests and this single job
record (no commit/push requested); cleaned. All task3 --rm runners/diagnostics and
private compiler caches/binaries retired. Final filtered Docker listing empty;
.tmp and bin/agent-runtime remain empty. Final RAM available 25,177 MiB, swap used
20,038 MiB, disk free 44 GiB. Preexisting shared caches/artifacts, live services,
other jobs, settings, AGENTS.md, duplicate prompt display and unrelated historical
test contracts were preserved.

Task 3 execution correction — native request isolation (2026-10-19)

This continues Job 25 / ledger 23 task 3, without changing its frozen scope.
The earlier fixture-only instruction-isolation claim above was insufficient:
native Codex 0.163.0 still injected 3,219 bytes of automatic skills/permissions/
collaboration context and five tools via an `additional_tools` input item.
No unrelated station, AGENTS.md, historical test contract, duplicate display,
live schema, installed binary, or production process was changed.

Diagnosis and implementation:
- Automatic context has independent include_* and skills.include_instructions
  switches. Parser-only startup now disables those blocks and bundled skills.
- Native thread config replaces the startup feature table. The parser carries
  its disabled capabilities into each fresh thread, including sleep and input
  request tools. Ordinary semantic clients retain their settings.
- Native gpt-6.1-sol metadata forces tool_mode=code_mode_only, independently of
  capability flags, and advertises the experimental clock tool. Parser startup
  projects the normal model catalog into its existing private scratch directory,
  changing only tool_mode to direct and experimental_supported_tools to empty.
  Model names, reasoning support, limits, and other metadata are preserved.
  It reads the configured model_catalog_json from the profile config when set,
  otherwise the normal models_cache.json. Missing/malformed/empty catalogs fail
  explicitly; there is no unisolated fallback. No credentials or source are
  copied. Host configuration/catalog/identity files are not written.

Native verification uses the installed executable mounted read-only into a
network-disabled disposable Docker runner, a synthetic CODEX_HOME, and an
in-process local HTTP endpoint. Only synthetic request bodies are inspected;
headers/credentials are neither recorded nor needed. The endpoint deliberately
returns 400, so this is native request-construction evidence, not live inference
or semantic-quality evidence. The final assertion requires exactly the semantic
instruction and question, no tools or additional input, gpt-6.1-sol and low effort.
Early diagnostic failures exposed the surviving exec/wait and then clock tools;
these are historical failures, not passing evidence. The initial capture assertion
checked only top-level tools; it was corrected to inspect the complete input.

Final checks (all inside the bounded disposable runner):
- go test -p 1 ./internal/codex ./internal/llm ./internal/llmprovider -count=1
  -timeout=120s: PASS 0.336s / 0.255s / 0.004s.
- After strengthening native test assertions, go test -p 1 ./internal/codex
  -run 'TestParser(Native|Catalog|Process)' -v -count=1 -timeout=60s:
  PASS 0.356s. Native capture passes; fixtures prove reuse, fresh thread IDs,
  64-call recycling, active/waiting cancellation, shutdown/reaping and cleanup.
  Catalog test proves model/limit preservation, unchanged host bytes, configured
  catalog selection, and rejection of empty metadata.
- go test -p 1 ./internal/api -run 'Test(SessionUserIntent|UserIntent)'
  -count=1 -timeout=120s: PASS 0.005s. Database-dependent tests remain skipped
  without OMNI_TEST_DATABASE_URL; no production database was connected.
- go build -p 1 -o /tmp/omnidex ./cmd/omnidex: PASS in the disposable runner.
- git diff --check: PASS; git diff --numstat -- AGENTS.md: empty.

Final SHA-256 of ordered sha256sum output for internal/codex/{parser_process.go,
parser_process_test.go,parser_isolation.go,parser_catalog.go,parser_catalog_test.go,
parser_native_isolation_test.go,generation.go}:
8974a303bbfc83ec35e30a12a782704c036b5aea7c9e31cf533cf72fce2f2230.

Activation facts: candidate core requires the readable normal native model
catalog as well as the existing namespace prerequisites. No schema change,
settings edit, installed-binary replacement or restart was performed. The native
check is opt-in with OMNI_TEST_CODEX_NATIVE and OMNI_TEST_CODEX_CATALOG, and needs
no real authentication. Existing profile-mask fixtures independently preserve
synthetic identity/configuration while suppressing AGENTS files.

Stages: implemented and verified task 3 correction; delivered reviewable local
source/tests and this same job record, with no commit/push requested. Independent
review and acceptance remain Omnidex-owned later transitions. Task schema scratch
was removed; all task3 disposable runners, compiler caches and candidate binary
are retired at execution finish. Preexisting dirty work and shared caches remain.

### Job 25 / ledger 23 — task 4 replacement qualification (2026-10-19)

Frozen scope: qualify the complete task 1–3 replacement through actual parser,
API and CLI paths, real gpt-6.1-sol/low inference and terminal checks. Compare
interpretation count, application prompt bytes, native provider input tokens and
latency with bin/usability-gauntlet-20261019. Keep startup/inference/shutdown
separate; historical no-inference shutdown baseline is 7–16 ms, not five seconds.
This is execution qualification; Omnidex owns independent review/acceptance.
No live schema, installed binary replacement, production restart, AGENTS.md,
duplicate prompt display or unrelated historical test contract changes.

Initial resource inspection: available RAM 25,686 MiB; swap used 20,749 MiB;
workspace disk free 44 GiB; .tmp empty. Existing shared bin artifacts and all
preexisting dirty source retained. Existing running containers belong to other
work; untouched. A short network-none read-only tooling probe auto-removed and
created no retained output.

Resource ownership before fixture creation: task 4 owns only
bin/task4-qualification-20261019 (retained evidence/scripts <=8 MiB),
.tmp/task4-socket (disposable Unix sockets), omnidex-task4-db and
omnidex-task4-runner. DB: network none, read-only root, 1 CPU/512 MiB/no swap,
128 PIDs, private data tmpfs256 MiB, /tmp32 MiB, socket mount only. Runner:
2 CPUs/2 GiB/no swap, 256 PIDs, read-only source/module cache, private /tmp768 MiB,
/cache768 MiB and profile32 MiB; <=1800s. One compiler/test operation at a time.
Runner bridge network is solely for authorized real inference; database uses a
private Unix socket, no host published port/Docker socket/live DB. Native executable
and existing auth/config/catalog/policy files are read-only mounts; credentials
are neither copied nor captured. Remove both --rm containers, socket directory,
private caches, test binaries and fixture data on completion/failure. Retain only
bounded test commands/source and qualification evidence. No persistent host service.

Qualification setup: two early DB bootstrap attempts auto-removed because the
entrypoint's private bootstrap psql expected /var/run/postgresql while only the
fixture socket was configured. Corrected fixture adds a private 1 MiB tmpfs at
that path and includes it in unix_socket_directories. PGHOST alone did not fix the
entrypoint override. DB init uses 1 MiB WAL segments; final WAL bounds remain
32/64 MiB. Fixture-only fsync/full_page_writes=false is transaction evidence,
not crash/power-loss durability evidence. No production DB was contacted.

Initial live API run (live-api-initial.txt) FAIL, 32.940s: one real interpretation
for each of nine messages. Natural approval/approval-plus-start returned
/approval=true without a pending proposal; quoted/resolved Codex instructions
returned a string instead of the typed message object; selected removal returned
"removed" instead of the persisted disposition "tombstoned". Server validation
rejected those outputs. Negated status and focused ambiguity/condition questions
succeeded. Removal failure prevented downstream workflow checks in that run.
This is a replacement qualification failure, not an acceptance claim or permission
to change task 1–3 implementation. The expanded qualification harness continues
past failed prerequisites only with explicitly logged synthetic fixture seeds;
subsequent real-model checks are conditional on those seeds and cannot prove an
uninterrupted natural approval-to-execution workflow. No prompt tuning, model
routing change, enum remapping or validator weakening was performed.

Final qualification outcome: **FAIL — replacement is not qualified for activation.**
The final actual HTTP/native-model run exercised all 14 cases in 55.918s and
returned exit 1. Each message made exactly one real gpt-6.1-sol/low interpretation;
validation failures were retained rather than retried or remapped. Final raw
prompts/results, positive native usage, per-call wall time and assertion failures
are in bin/task4-qualification-20261019/live-api-final.txt. Earlier failed runs
are live-api-initial.txt (32.940s) and live-api.txt (37.261s); they are historical
diagnostics, not current passing proof. The final harness adds labelled fixture
seeds solely to expose downstream checks after live prerequisite failures.

| Final live case | Observed outcome |
| --- | --- |
| Natural approval | FAIL: model used `approved` for plan/review state; validator rejects. |
| Approval plus start | FAIL: `/approval=true` without a pending proposal; no start. |
| Quoted Codex instructions | FAIL: string `/message`, incompatible with Content DTO. No control patch executed. |
| Negated control/status | PASS: only `/status_visible=true`. |
| Resolvable Codex recipient | FAIL: string `/message`, no typed recipient. |
| Ambiguous reference | Empty patch and focused question; no effects. |
| Ambiguous recipient | Empty patch and recipient question; no effects. |
| Conditional start | Empty patch and question distinguishing Codex instruction from conditional control; no effects. Relevance remains for independent review. |
| Selected resolvable item | FAIL: model used `removed`, not `tombstoned`. |
| Pending-proposal confirmation | FAIL: repeated item-removal patch, not confirmation; validator rejects. |
| Approval/start remaining item | FAIL: invalid review state; no freeze/start. |
| Reconnect and same-job continuation | Fixture-created pending removal survived reconnect; seeded confirmation replayed exactly. After seeded freeze/start and real interruption/reconnect, real-model continuation resumed the same job/generation/ledger. Conditional evidence, not a natural end-to-end workflow pass. |
| Cancellation | FAIL: model used `cancelled`, rejected by typed state validator. |
| Cancellation confirmation | FAIL: model proposed cancellation again (requires approval), rather than confirming the stored proposal. Subsequent fixture-only exact confirmation hit `cannot scan NULL into *int` for column `phase`; cancellation completion is unverified. |

There were five explicitly labelled fixture seeds in the final run: removal,
confirmation of removal, frozen/running plan, cancellation, confirmation of
cancellation. They are not model interpretations or successful user-flow evidence.
The cancellation scanner failure occurred at the real repository execution
boundary even after a code-valid fixture confirmation. No runtime source fix was
made under this qualification-only task, and no full successful replacement claim
is supported. Safety rejection is not semantic success.

Other concrete current-source checks:
- `go test -p 1 ./internal/userintent ./internal/sessionintent ./internal/contextcompiler ./internal/codex ./internal/llm ./internal/llmprovider -count=1 -timeout=120s`
  PASS: 0.007/0.004/0.003/0.346/0.301/0.001s (components.txt).
- `go test -p 1 ./internal/api -run 'Test(SessionUserIntent|UserIntent(API|HTTP|Provider))' -v -count=1 -timeout=180s`
  PASS 1.982s with actual disposable PostgreSQL, including HTTP confirmation,
  reconnect, cancellation, immutable replay, stale authority and provider ownership
  fixture checks (api.txt). Scripted semantics do not establish live model quality.
- From internal/queue, `go test -p 1 $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .) user_intent_fixture_test.go user_intent_integration_test.go user_intent_native_integration_test.go user_intent_patch_integration_test.go coding_recovery_integration_test.go coding_recovery_input_integration_test.go coding_combined_flow_integration_test.go coding_review_runtime_integration_test.go -count=1 -timeout=120s`
  PASS 33.317s (queue.txt), all queue production sources plus affected fixtures.
  Full queue/client test packages remain limited by the preexisting unrelated
  historical contract compile errors described in task 2; no full suite claim.
- `go test -p 1 ./cmd/omni -run '^TestPausedPlanningLiveUserIntent$' -v -count=1 -timeout=180s`
  PASS 8.637s (live-cli.txt): actual CLI acceptUserMessage/client/HTTP/live native
  parser/database status path, one interpretation, retained job/generation4/
  revision2, receipt replay and stale/undisplayed authority rejection. This is a
  synthetic paused fixture, not a real application-building or execution trial.
- `go test -p 1 ./cmd/omni -run 'Test(PausedPlanningOrdinaryUserIntent|ChatPlanReviewNote|PlanReviewNote|PlanReviewConversationCannotBypassPendingReviewKey|PlanReviewNoteBytes|WorkspaceFrames|WorkspaceReview)' -count=1 -timeout=120s`
  PASS 1.229s (cli.txt), including actual scripted API/database CLI path.
- `OMNI_TEST_CODEX_NATIVE=/native-codex OMNI_TEST_CODEX_CATALOG=/profile/models_cache.json go test -p 1 ./internal/codex -run '^TestParserNativeInstructionIsolation$' -v -count=1 -timeout=60s`
  PASS 0.078s (native-isolation.txt). Actual installed native executable, synthetic
  local endpoint: exactly one semantic instruction and one question, no tools,
  gpt-6.1-sol/low. This request-capture test performs no real inference.
- `go test -p 1 -c -o /tmp/omni.test ./cmd/omni` and
  `go build -p 1 -o /tmp/omnidex ./cmd/omnidex`: PASS, disposable runner only.
- `python3 bin/task4-qualification-20261019/pty-check.py`: PASS twice, 2.533s each,
  140x40 and 54x18, actual Docker interactive PTY/production console. Exact draft
  `aXb\n  quoted 'value'\n\nend`, cursor edit, bracketed paste, ledger selection,
  refresh, streaming, resize away/back, submit and terminal settings restoration.
  PTY checks have no job authority or API connection; they complement live CLI/API
  evidence, not substitute for an interactive live workflow. Raw bounded terminal
  output is pty-140x40.txt / pty-54x18.txt; results in pty-results.json.
- `git diff --check`: PASS. AGENTS.md diff empty; protected-sha256.txt rechecked
  go.mod/go.sum/AGENTS.md/bin/omnidex and the historical shutdown evidence: all OK.

Measurements (metrics.json; `python3 bin/task4-qualification-20261019/summarize.py`):
The baseline's first nine stored operations each used two interpretations;
median application prompt 5,442 bytes, native input 14,771 tokens and accumulated
provider wall time 14,091 ms (range 12,704–17,294 ms). Final first nine replacement
cases each used one interpretation; median 2,943 bytes, 717 input tokens and
3,221 ms (2,816–5,764 ms). All 14 final cases: one interpretation each,
2,922-byte / 718-token / 3,316.5-ms medians, latency 2,816–6,680 ms. These are
observed workload distributions, not a controlled speedup: fixture sizes, messages,
conversation histories and semantic success differ. Faster failed interpretations
do not qualify the replacement. The full baseline has 11 operations, including
one three-call explicit-start repair; baseline-metrics.json preserves all of them.

Separate lifecycle measurements:
- `go test -p 1 ./internal/codex -run '^TestParserLiveStartupShutdown$' -v -count=1 -timeout=60s`:
  no inference; direct native startup/initialize 32.808/28.169/29.614 ms and
  synchronous shutdown 0.541/0.506/0.496 ms (lifecycle.txt). These fixture-only
  no-thread measurements are not the prior installed launcher timings.
- Final API run uses a disposable transparent native RPC timing executable
  (/tmp/native-timed, source retained in native-timed.go). It forwards unchanged
  bytes and records only method names/IDs/timestamps, never credentials/headers.
  Startup/initialize 24.569 ms once; 14 separate native turn/start→turn/completed
  intervals: 2,806.706–6,668.257 ms, median 3,306.312 ms. These exclude startup and
  shutdown but include native turn dispatch/network/provider inference. Pure
  provider compute time is unavailable and is not claimed. Provider API wall times
  above include native setup as applicable; no double-counted inference total.
- Direct native shutdown after earlier live inference was 6.994 and 6.819 ms.
  Final proxy close 0.611 ms measures the proxy lifecycle, not native child reaping.
  A later runner process listing contained no native/proxy/test processes.
- Preserve the existing installed baseline exactly: startup 459/580/309 ms,
  shutdown 10/16/7 ms, zero model calls and no five-second timeout reached, from
  bin/usability-gauntlet-20261019/intent-transport-latency-probe.json. There is no
  evidence attributing observed language-control latency to that timeout.

Exact identity/delivery binding:
- Authoritative tree /home/gryph/Projects/tools/omnidex, branch main, base HEAD
  3b252b684449447fd84bc122bace2ae494b11bf1 plus preserved dirty work. No commit/push.
  Ordered path/hash manifest SOURCE_SHA256SUMS (internal/, cmd/, database/) SHA-256
  `798faa61671a7ebab4c300f693a06cc106438a1c9a047e67f073c29b47349442`.
  All manifest paths reverified after final checks; no subsequent source/test edits.
- Task-4 new API live qualification test SHA-256
  `d7b1655d9df4e67992df748a013e571ec29974b913e0e73b5df5831e57578dc4`;
  new native lifecycle test
  `8c916b8dfc0a12d242b09b5dfb57baf9011cd979f1ac11389833c55bbda18139`;
  affected CLI paused fixture, factored to add opt-in real-model verification,
  `ded355ab004d693636da9d77008345977dafa74ae135cf4270d11ebb8747f5d7`.
  Task 4 changes no production implementation. Earlier task-4 harness revisions
  terminated sooner; their failures are not final-source success evidence.
- Builder image sha256:4c1b27b8fdbcdd877fef43fef587451d4862384f0509c8c464c18e81b0fe98e5;
  PostgreSQL image sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873.
- Installed native codex-cli 0.163.0, executable SHA-256
  `399ff69c431c066e40de318fba45c2b07f45a76b19939d247ee4828fc70c4215`.
  Runtime command/profile mount identities and resource options are retained in
  run-fixture.sh / run-db.sh. The first CLI/lifecycle checks used /native-codex;
  only the final API run used /tmp/native-timed. No timing-proxy measurements are
  attributed to earlier direct-native runs.
- Disposable candidate core SHA-256
  `dcbfa7171765dc3efc03c7a5e4b25c19b14f82d1ffcd3a91de66512579bceca7`;
  CLI test executable `8348dab1c6998df3a62e71d2b5b77872028020ef3f3d99935114bd291753de54`;
  timing executable `429c40b8d447de7273b56255e97e108d285c38e79017f1260a5dcb6bafd4f24c`.
  All retired. The buffered runner script also produced the same 31 MiB CLI test
  binary briefly at /evidence/omni.test before the /tmp output correction;
  its hash was checked and that task-owned disposable was removed. It is not
  retained evidence. The build commands were `go test -p 1 -c -o /evidence/omni.test
  ./cmd/omni` and then the /tmp command listed above.

Activation facts for review: the candidate remains source only and unqualified.
Tasks 1–3 require matching core/CLI patch-contract consumers, the readable normal
native model catalog and existing Linux namespace prerequisites. No new schema
is introduced by these tasks. Existing pending legacy action-format results do
not become patch authority. No live schema, installed binary, service or settings
was modified, installed, restarted or activated by this assignment. There is no
activation recommendation while the recorded semantic and cancellation failures
remain. Independent review and acceptance are Omnidex-owned later transitions,
not prerequisites for finishing this execution qualification assignment.

Cleanup: all disposable schemas in the live/paused/API fixture namespaces were
absent before shutdown (count 0). Sampled private DB data 173 MiB/256 MiB;
sampled runner memory 467.7 MiB/2 GiB and DB 302 MiB/512 MiB; final private cache
264 MiB, /tmp66 MiB and profile scratch5 MiB, all retired with containers. Native,
proxy and test processes were absent before container shutdown. Both named --rm
containers stopped and auto-removed; an immediate listing briefly saw the runner
in Dead removal state, and the subsequent filtered listing was empty. The empty
.tmp/task4-socket directory and the incidental host test binary were removed.
Only bounded qualification evidence/scripts remain in the existing ignored bin
area, approximately 1 MiB, within the 8 MiB retained budget. Preexisting shared
artifacts/caches, unrelated containers/jobs, dirty/accepted source and settings
were retained. Final available RAM 25,711 MiB, swap used20,875 MiB, disk free44 GiB.
No cache dropping, swap changes, owner-process termination or global pruning.

Stages: implemented task-4 opt-in qualification checks; verified component,
transaction/API, CLI, native isolation and PTY checks, and measured explicit live
qualification FAILURE (no replacement acceptance claim); delivered reviewable
local tests and this existing Job25/ledger23 evidence record, without requested
commit/push/activation; cleaned task-owned disposable resources. Stop here for
Omnidex's independent review; no unrelated work or repair is added to the ledger.

Task 4 owner-authorized continuation — Job25/generation1/ledger23/frozen6,
p_f1a58327a78d_4_task. Prior qualification FAILURE remains genuine and retained.
Owner explicitly authorizes correction of its observed production discrepancies
and fresh qualification, without reopening unrelated accepted work or another job.
Resume inspection: old task containers absent, .tmp empty; old bounded evidence
retained. Available RAM24,925 MiB, swap used19,773 MiB, disk44 GiB free.

Corrections in progress: supply the actual typed state data contract alongside
snapshot/message (DTO-derived shapes, model constants and state-field meanings,
not actions/callbacks or a selector loop). Keep validators and one-call normal
interpretation unchanged. The cancellation NULL scan originates in the source
conversation-event view: resume_job is selected but lacks phase/kind/context CASE
mappings. Add its ordinary same-generation follow-up projection at that owner;
no NULL coalescing or weakening the chronological validator. Source view change
will require coordinated activation review; no live schema alteration is allowed.
The final live harness now fails on prerequisite errors and has no fixture seeds.

Resource ownership/budgets: reuse the same task4 container names and .tmp/task4-socket,
existing builder/PostgreSQL images, source/module-cache read-only and existing
identity/config/catalog/policy as individual read-only mounts. Same limits as above:
runner2CPU/2GiB/no swap/256PIDs, /tmp768MiB/cache768MiB/profile32MiB, lifetime1800s;
DB1CPU/512MiB/no swap/128PIDs, data256MiB, tmp32MiB/bootstrap socket1MiB, network
none/private Unix socket only. Runner egress only for authorized real inference.
One heavy build/test operation at a time; retained continuation outputs share the
existing evidence directory, total <=8MiB. Private compiled outputs stay /tmp.
Stop/remove containers, scratch/socket/compiler/native processes at completion
or failure; preserve old evidence, other jobs/containers/caches and dirty source.

Continuation first fresh live run: continue-live-api.txt FAIL43.341s. All 14
interpretations used correct typed patches/clarification with one call each,
including natural agreement, compound approval/start, planner Content object,
tombstoned selected removal, confirmation via approval=true, resume and canceled.
Natural agreement was executed without freeze/start; removal/confirmation/replay
and resume ran without seeds. Cancellation then exposed the next source-view
chronology error: frozen interruption keeps generation1, but was projected as
phase0/interruption (whose established validator correctly requires generation>=2).
Correct the view's chronological projection for same-generation interrupt_job to
phase1/follow_up with exact feedback text, like resume_job. New-generation
interruptions retain phase0/interruption. No scanner/type/chronology validator
was relaxed. The job lifecycle rows still retain exact interrupt/resume identities;
this changes only their ordinary conversation-context projection. The new focused
regression asserts both same-generation pause/resume projections, cancellation
with one confirmation and exact replay. Prior failed outputs remain unchanged.

The corrected-view live run continue-live-api-final.txt now PASS43.186s with all
14 cases, no diagnostic seeds, one interpretation each, real natural agreement
execution, one removal confirmation, start, interruption/reconnect/same-job
continuation and cancellation confirmation/reconnect/replay. The focused database
regression PASS1.062s and the full affected queue subset PASS34.293s.
The expanded live CLI check exposed a test assumption, not a production failure:
active-planning content is durably queued, so its successful receipt legitimately
has no ai_channel_messages ID yet. The strengthened check now reads it through
ClaimNextStep/NextChannelSessionMessage and verifies admitted job/generation and
quoted bytes, instead of expecting immediate conversation-history publication.
No product behavior/receipt was changed. It also exercises a separately explicit
unknown recipient so recipient ambiguity is not conflated with an ambiguous
subject in the older “Tell them to shorten it” case.

Task 4 continuation final source qualification — PASS (2026-10-19 local date;
checks ran 2026-10-20 UTC). This supersedes the earlier task-4 failure as current
qualification evidence without changing or relabelling any failed run.

Implementation: the shared parser accepts a bounded JSON state_contract alongside
the unchanged message, actual snapshot and existing bounded/minified context.
The session caller always supplies it; construction errors propagate. DTO reflection
derives nullable object/array/scalar shapes and required field names. Persisted
model constants supply review/disposition/job values; field meanings describe
actual state, agreement versus start, the current content recipient, stored pending
confirmation and protected identities. The same recipient function is used by
contract generation and existing typed validation. No action catalog, tool menu,
callback protocol, output repair/remapping, word heuristics, selector loop, weakened
validator or sample-specific production prompt clause was added. A normal compound
message remains one interpretation. Only the nullable DTO/enum/context omission
and observed source-view chronology discrepancies were corrected.

Final checks/commands and observed outcomes:
1. `docker exec omnidex-task4-runner sh /evidence/continue-live.sh` initially
   ran current userintent/sessionintent tests (PASS0.007/0.004s) and then the
   first fresh live API run (FAIL43.341s) described above. The failed output is
   retained in continue-live-api.txt. It exposed the same-generation pause
   projection and justified the precise subsequent source correction.
2. `docker exec omnidex-task4-runner sh /evidence/continue-queue.sh`:
   all queue production sources plus the affected explicit test-file subset,
   adding user_intent_cancel_resume_integration_test.go, PASS34.293s. Exact
   `go test -p 1 $(go list -f '{{range .GoFiles}}{{.}} {{end}}' .) ... -count=1
   -timeout=150s` file list is retained in continue-queue.sh; it is the earlier
   task-4 queue list plus that new file. This covers transaction rollback,
   authority, confirmation/replay, native reply and frozen continuation fixtures.
3. `docker exec omnidex-task4-runner sh /evidence/continue-final-checks.sh`:
   final focused `-run '^TestUserIntentCancellationAfterSameJobResume$' -v
   -count=1 -timeout=90s` PASS1.062s, asserting exact same-generation pause/resume
   context projections, cancellation with one confirmation and replay.
   Final `OMNI_TEST_LIVE_CODEX=/tmp/native-timed go test -p 1 ./internal/api
   -run '^TestUserIntentLiveQualification$' -v -count=1 -timeout=600s`
   PASS43.186s, continue-live-api-final.txt. All 14 real gpt-6.1-sol/low
   interpretations passed, one per message; zero fallback seeds, repairs or retries.
   Natural agreement persisted both review states without starting/freezing;
   compound approval/start produced agreed reviews plus frozen/running state;
   quoted content retained quotation and negation with planner recipient;
   status respected negation; resolved/selected references used exact state;
   unresolved condition asked “Have the tests passed?” with no effects;
   removal had one confirmation and exact receipt replay; the remaining plan
   started, interruption/reconnect resumed the same job/generation/ledger;
   cancellation confirmed once and its receipt replayed after reconnect, with
   canceled job state and no pending proposal. Initial plans are synthetic
   fixture setup before inference, not diagnostic seeds after failures.
   The script then stopped on the expanded CLI's incorrect immediate-history
   assertion (continue-live-cli.txt, FAIL9.431s). Its durable-queue correction is
   documented above; none of the already passing API effects was repeated against
   the same fixture, nor claimed invalid by that separate test assumption.
4. `docker exec omnidex-task4-runner sh -c 'go test -p 1 ./cmd/omni -run
   "^TestPausedPlanningLiveUserIntent$" -v -count=1 -timeout=180s >
   /evidence/continue-live-cli-final.txt 2>&1'`: PASS12.841s on final test source.
   Actual CLI acceptance/client/HTTP/live native parser/database path verifies
   paused status, replay, stale and undisplayed authority, quoted task content
   durably available through the production message consumer at the same admitted
   job/generation, and explicit unknown recipient clarification with no effects.
   The unknown-recipient reply was “Who do you mean by \"the other person\"?”.
   Each of the three inspected CLI inputs has exactly one stored interpretation;
   the existing stale-execution fixture also makes one additional status
   interpretation before deliberately changing authority. The test's old `calls`
   variable counts scripted HTTP-fixture requests only (zero in live mode), not
   real inference; native evidence/usage is authoritative for live call counts.
5. `go test -p 1 ./internal/api -run
   'Test(SessionUserIntent|UserIntent(API|HTTP|Provider))' -v -count=1
   -timeout=180s`: PASS2.025s, continue-api.txt, with actual disposable PostgreSQL.
   `go build -p 1 -o /tmp/omnidex ./cmd/omnidex`: PASS in the same bounded runner.
   Explicit command statuses retained in continue-status.txt. No build/install
   operation touched an installed binary.
6. `python3 bin/task4-qualification-20261019/continue-summary.py` and
   `git diff --check`: PASS. Summary asserts 14 positive native usages, one call
   each, passing API/CLI results and no recovery seeds. Source manifest recheck
   passed after all tests. Protected go.mod/go.sum/AGENTS.md/bin/omnidex and the
   historical shutdown evidence hashes still match the retained baseline.

Completed terminal/isolation work was not repeated. Rechecked exact source hashes
against the retained task-4 manifest: all 107 cmd/omni source paths are unchanged
except the expanded paused-intent test, and all 41 internal/codex source paths
are unchanged. Therefore the earlier actual 140x40/54x18 PTY, draft/cursor/paste/
ledger/resize/restoration and native tool/instruction isolation/process lifecycle
checks remain bound to the current affected implementations. This is recorded in
continue-retained-check-bindings.json. Live CLI input/API evidence and the retained
PTY evidence are complementary; no full installed interactive workflow, production
execution, autonomous application build or full repository-suite pass is claimed.
The unrelated historical queue/client test-contract compile limitations remain;
those tests and duplicate prompt display are untouched. Omnidex's independent
review/acceptance dispatch follows this execution response; it is not self-certified
here and is not a reason to leave the execution implementation incomplete.

Fresh measurements, continue-metrics.json (descriptive, not controlled cohorts):
- All 14 API cases: exactly1 interpretation each; application prompt bytes
  6,005–6,428, median6,168; native provider input tokens1,313–1,439, median1,357;
  provider request wall time2,520–4,069ms, median2,905ms.
- First nine replacement cases: median6,179 application bytes /1,352 native input
  tokens /2,910ms; each one call. Historical first nine stored operations:
  median5,442 bytes /14,771 native input tokens /14,091ms; each two calls.
  Application bytes increased to provide necessary type/value facts; native tokens
  and observed latency decreased. Different messages/fixtures/context and semantic
  outcomes preclude a controlled speedup claim. The full 11-operation baseline
  including the earlier three-call explicit-start repair remains unchanged.
- Separate fresh process/initialize interval27.003ms (one reused process), and
  14 native turn/start→turn/completed intervals2,509.495–4,058.244ms,
  median2,895.799ms, excluding process startup and shutdown. Native intervals
  include dispatch/network/inference; pure provider compute time is unavailable.
  Application wall times include setup as applicable; they are not added to the
  native intervals. Timing executable forwards unchanged RPC bytes and records
  method names/IDs/timestamps only.
- Final proxy close0.608ms is proxy lifecycle timing, not native child reaping.
  Direct native process lifecycle implementation is unchanged; retained direct
  post-inference shutdown6.994/6.819ms and installed baseline10/16/7ms remain
  distinct observations. The installed 7–16ms baseline is retained exactly,
  with startup459/580/309ms, zero model calls and no five-second timeout reached.
  No measured inference latency is attributed to that timeout.

Provenance/current identity: authoritative root/branch remain
/home/gryph/Projects/tools/omnidex, main, base
3b252b684449447fd84bc122bace2ae494b11bf1 plus retained dirty/accepted work.
CONTINUE_SOURCE_SHA256SUMS binds final internal/,cmd/,database/ source and tests;
its SHA-256 is `9ee1b418842bdcd9827410cc08af6738c8dc99030a791da0a28a1267755c3049`.
Final database/setup.sql SHA-256
`2632fee1ac090b22d3872a9a9943c460d158008d6a0c713d37d78bd50b376c06`;
state contract source `808952338bf67f691f8aec219ca3bc6c78e448797196573aeaa63232a67be57e`;
shared parser `e620434fbf650dd3b3fb1123b5d58b5ef839a8388ff8c0f760e247923a94ee5c`.
Only scoped production edits are parser contract transport, session contract/
shared recipient function/API wiring, and the existing conversation-event source
view; related tests were added/strengthened. No unrelated source was rewritten.
Images/native executable are exactly the prior retained identities. Disposable
candidate core hash
`e896c88115b2bb19e9ca0b7fee703d62767d548f39a75fbc1125bbb59078cd03`;
final timing executable
`f5ebd69eb74fa352ce9b98e30f3628d50e0ec763ad3e762b2bda4091d6e8fe06`.
No source/test edits followed the final passing checks.

Only the earlier disposable timing-wrapper source changed among its 40 retained
artifact entries: fixed log path became a linker-set string with the same default.
Build flags `-X main.timingPath=/evidence/continue-native-times.jsonl` and then
`-X main.timingPath=/evidence/continue-final-native-times.jsonl` separate fresh
captures. All 39 other prior artifacts, including raw failures/timings/results/
scripts/source manifests, still match their original hashes; verification is
continue-prior-evidence-verification.json. Earlier SHA256SUMS describes the earlier
artifact versions; CONTINUE_SHA256SUMS binds current continuation artifacts.

Activation facts: candidate now requires the corrected source view definition
from database/setup.sql in addition to matching core/CLI state-patch consumers and
existing namespace/model-catalog prerequisites. View columns/tables are unchanged;
resume and same-generation interrupt context projections are corrected. This is
not a claim that an installed core alone repairs an unchanged live view. Review
and apply that view through the owner's normal coordinated activation procedure
if activation is later authorized. No live view/schema alteration, migration,
installation, binary replacement, settings write, service restart, commit or push
was performed by this source execution. No new approval cycle or deployment work
is manufactured as an execution-completion prerequisite.

Continuation cleanup/stages: implemented the authorized observed corrections;
verified the fresh passing API/CLI and concrete affected checks above; delivered
reviewable local source/tests and this same Job25/ledger23 record (no commit/push
or activation requested); cleaned all task-owned runtime resources. All private
test schemas were absent before shutdown (non-system/non-public namespace count0).
Final sampled runner326.8MiB/2GiB, DB240.8MiB/512MiB; private /tmp36MiB,
/cache275MiB and profile7MiB. Two completed native children of the timing proxy
were zombies under the disposable runner's non-reaping sleep PID1, not active
inference; proxy close timings are not child-reaping proof. Both were retired with
the stopped/auto-removed container. Direct-provider CLI left no native process.
Both named containers are now absent, .tmp is empty, private cache/database/profile/
compiled core/timing executables and sockets are retired. The retained evidence
area is1.5MiB total, within8MiB; no disposable binary is retained there. Final RAM
available25,053MiB, swap used19,799MiB, disk free44GiB. Shared caches/artifacts,
other containers/jobs, accepted/dirty work and settings were preserved. AGENTS.md
has no diff. No global cleanup, host service or production operation was performed.
Independent review/acceptance remain Omnidex's subsequent transitions; execution
qualification is complete and no unrelated work is opened.

Task 4 independent-review finding correction — same Job25/ledger23 assignment.
The fresh independent review correctly rejected the live fixture assumption that
“Tell them to shorten it” must be ambiguous after explicit Codex/report-title
context. Only internal/api/user_intent_live_test.go changes: use the explicitly
unknown “other person” recipient with an explicit report-title subject, and fail
immediately on a non-clarification before attempting execution. This preserves
shared fixture authority and leaves production interpretation semantics unchanged.
The historical failing review capture remains untouched.

This rerun reuses the above task4 resource ownership/budgets and existing scripts:
run-db.sh and continue-fixture.sh, same named disposable containers, private socket,
read-only source/module/identity mounts, 1800-second runner lifetime, one heavy
operation, 2 CPU/2GiB runner and 1 CPU/512MiB database; correction-* retained
captures share the established <=8MiB evidence budget. Cleanup condition is this
qualification's completion/failure. Initial host available RAM25,079MiB, swap
used19,809MiB, disk free44GiB. Initial DB startup failed because the newly recreated
socket directory lacked PostgreSQL write permission; correction-db.txt retains
that failure. Applied mode0777 only to the task-owned private socket directory
and reran run-db.sh, captured in correction-db-retry.txt. No live database touched.

Correction qualification PASS: `docker exec omnidex-task4-runner sh -c
'go build -p 1 -ldflags "-X main.timingPath=/evidence/correction-native-times.jsonl"
-o /tmp/native-timed /evidence/native-timed.go &&
OMNI_TEST_LIVE_CODEX=/tmp/native-timed go test -p 1 ./internal/api
-run "^TestUserIntentLiveQualification$" -v -count=1 -timeout=600s >
/evidence/correction-live-api.txt 2>&1'` exited0. Full14-case flow PASS49.412s,
including natural approval execution, compound approval/start interpretation,
quoted/negated instructions, resolved/selected/ambiguous references, explicit
unknown recipient, focused condition, confirmation/replay, cancellation/reconnect
and same-job continuation. No post-failure seeds or interpretation retries.
Unknown recipient asked “Who do you mean by \"the other person\"?”; condition
asked “Have the tests passed?”. Assertions and production validators preserved.

`python3 bin/task4-qualification-20261019/correction-summary.py` asserts all14
positive provider usages, one call each, full API pass and retained CLI pass.
It emits correction-metrics.json: application bytes6006–6429 (median6172),
provider input tokens1320–1447 (median1364), request wall2663–5446ms
(median3275). First-nine medians: one call/6186 bytes/1362 tokens/3168ms versus
bin/usability-gauntlet-20261019 retained first-nine two calls/5442 bytes/14771
tokens/14091ms. Different corpus/context/outcomes remain a limitation; these are
observations, not a controlled speedup estimate. Startup/initialize47.163ms;
14 native turn intervals2650.988–5436.520ms (median3265.628), including dispatch,
network and inference. Pure provider compute remains unavailable. Proxy provider
close0.504ms is separate from native child reaping. Original installed direct
shutdown10/16/7ms (7–16ms baseline) remains unchanged; no inference latency is
attributed to a five-second timeout.

Exact source: main at3b252b684449447fd84bc122bace2ae494b11bf1 plus preserved dirty
work. correction-source-binding.json verifies every CONTINUE_SOURCE_SHA256SUMS
entry unchanged except internal/api/user_intent_live_test.go, now SHA256
00bd82f59b97ad967d605b6ea642a046c5817aced6e9075f25e268bc4c2b6b36.
Thus retained independent-review passing CLI/API/components/queue/native isolation/
process lifecycle/build checks and actual140x40/54x18 PTY checks still bind to the
current implementations. No full-repository suite, installed workflow or autonomous
application construction is claimed. Existing historical test-contract limitations
and activation prerequisites above still apply. The review failure is retained,
not rewritten as a pass. Fresh full-flow capture SHA256
016266182770e1cbf830218bba80ad498e2dbf02b8c0c415835a5dfe2a975bb9.
CORRECTION_SHA256SUMS binds the correction captures/scripts/measurements.

Cleanup: private namespace count0 before shutdown. Final samples runner225.6MiB,
DB65.52MiB; private /tmp4MiB/cache210MiB/profile4MiB. `docker stop
omnidex-task4-runner omnidex-task4-db` retired both auto-removing containers,
including the timing proxy's remaining native child; `rmdir .tmp/task4-socket`
removed the task socket directory. Both container names absent afterward. Private
cache/profile/build outputs retired with containers. Retained evidence2.5MiB is
within8MiB. Unknown/shared resources and other containers untouched.
`git diff --check` PASS; AGENTS.md has no diff. Only the fixture correction and
this same coding evidence record changed in this execution. Implemented and
verified; delivered reviewable local source/evidence; cleaned task resources.
No commit/push, installed-binary replacement, live schema/settings write or
production restart. Subsequent independent review/acceptance belongs to Omnidex.

## Completed-review classification repair — Job26 / ledger24 / 2026-10-19

Frozen execution: job26 generation1, ledger24 frozen/current revision3,
task `p_b0c9bf02bb31_1_task`, step134 `v3_coding`, authoritative workspace
`/home/gryph/Projects/tools/omnidex`, identity `directory_30_84644881`.
This is the existing approved assignment, not a new proposal/job. No live ledger,
schema, setting, binary, service, approval or acceptance mutation is performed here.
Omnidex owns subsequent independent review/acceptance of this repair assignment.

Task-owned evidence: `bin/review-status-20261019/` (8MiB maximum). Ownership,
pre-resource host measurements, limits, cleanup conditions, initial source hashes
and dirty status are recorded there. Initial available RAM24781MiB, swap used
19828MiB, disk free44GiB; .tmp empty. Runner max2CPU/2GiB/no swap/256PIDs,
private tmp/cache768MiB each/profile32MiB; database max1CPU/512MiB/no swap/128PIDs,
data256MiB/tmp32MiB. Both timeout1800s, named `omnidex-review-status-*`, private
Unix socket `.tmp/review-status-socket`, source/module/profile inputs read-only.
Only the runner requires native inference access; database has no network.
Private builds/cache/database/profile/sockets are disposable, removed at completion
or failure. Existing shared caches and unrelated resources remain untouched.

Immutable historical read-only capture: runtime schema `omnidex_runtime_4ffc6561`,
review-response evidence261396 and262710 associated with interruption receipts
261397 and262711. Their exact `notes[0]` source bytes and frozen task are retained
in `corpus.json`; `historical-progression.json` retains these rows plus
`task_reviewed`263989 and `task_accepted`267135. The historical progression is
pilot-assisted recovery, not evidence of automatic correction by this repair.

Before repair: real production sieve/renderer/native provider on
codex/gpt-6.1-sol/low/default, 12 cases, 33 semantic calls,198923ms summed case
wall time. `before-model.jsonl` records exact code-rendered prompts/context/schema,
provider results/native frames and per-call timings. Both unedited historical
reports returned error with one exact finding each; source-defect and unrelated
date-conversion review also returned error despite completed review work. A test
fixture defect returned done but lost its finding. Genuine unfinished/inaccessible
review, question, permission, missing completion evidence, clean review, and
execution required-check failure controls behaved as expected. Full output:
`before-model.txt`; derived outcomes: `baseline-summary.json`. The expected
regression test failure is retained, not rewritten as success.

Repair changes only the existing review semantic definitions: a completed check
exposing a task-relevant source/test/qualification-fixture defect is completed
review evidence, with its failure preserved for separate exact-quote findings.
Unfinished review work, inaccessible verification and missing review evidence
remain error; owner questions/permission and unknown output retain their existing
paths. No finding-count override, text heuristic, error-to-done conversion, new
station or gate is added. Execution-response instructions, outcome precedence,
controller routing and PostgreSQL acceptance validation remain unchanged.

Fixture startup friction retained: first live attempt could not create scratch
under read-only /src/bin/agent-runtime; no inference occurred. Harness now uses
private temporary client scratch with the same production parser renderer/provider.
First database command wrapped postgres directly and bypassed its image entrypoint,
so root startup was rejected; corrected wrapper times the ordinary entrypoint.
Both failures are retained in before.txt/before.jsonl and database-start.txt;
corrected database startup is database-retry.txt. The earlier extra read-only CLI
inspection session ended without submitting text or changing live state.

Checkpoint resolved below: after-repair boundary, affected regressions, automatic
controller/PostgreSQL continuation, source/evidence binding and cleanup all completed.

Completed qualification: after repair all12 boundary cases PASS. The two unchanged
historical reports now return done with findings byte-identical to baseline,
including their failed-check evidence. All three unrelated completed defect
reviews return done with findings; genuine unfinished/inaccessible review,
question, permission, missing evidence and execution-failure controls remain
explicit. `summarize.py` verifies every original verification string and exact
source quote; it also asserts the execution-failure model-visible prompt/context
are byte-identical before/after. Real route codex/gpt-6.1-sol/low/default;
33 semantic calls before and33 after. Summed case wall198923ms before,
199309ms after (test wall198.938s/199.323s): no latency improvement is claimed.
Observed corpus outcomes improved7/12 to12/12; one run per case is not a model
reliability estimate. Exact envelopes/results/native frames and timings are in
before-model.jsonl/after-model.jsonl; summary.json is reproduced by summarize.py.

Checks PASS, recorded in checks.sh/check-status.txt: complete
`./internal/codingagent` (0.009s), complete non-opt-in `./internal/codex` (0.360s),
the established affected explicit queue fixture subset extended with captured
parser-route acceptance and new review-status tests (34.814s), and the opt-in
`TestReviewStatusLiveAutomaticCorrectionPostgres` (39.175s test wall).
Regression coverage includes unfinished review despite findings, missing evidence,
invalid/missing scan units, unknown meaning, null findings, invented quotes/files,
question/permission precedence, execution failure, recovery without repeated native
effects, accepted replay/state-patch behavior and immutable route authority.
No assertions, authority checks or acceptance validation were weakened.

Automatic-flow evidence is separately labeled: REAL semantic classification and
actual production controller/repository/PostgreSQL with BOUNDED SCRIPTED native
execution/review fixture behavior and actual private fixture byte checks. Initial
execution writes alpha only; finished review reports the missing beta line and
failed exact-byte check. The same job1/generation1/ledger1/task `task`/step2 in the
DISPOSABLE database records `task_reviewed`, dispatches correction with exact
findings, writes alpha+beta, dispatches a fresh isolated review, records
`task_accepted`, and passes the existing repository acceptance journal validator.
Final persisted job/ledger completed, task accepted. The negative acceptance probe
at correction is rejected with “lacks accepted review evidence”; it makes no
mutation. No manual resume, report rewrite, extra approval, replacement job or
pilot intervention occurs between stages. Six REAL semantic calls,38197ms
controller-to-acceptance wall, two executions/two independent review sessions.
`automatic-flow.jsonl` retains exact prompts/results, rejection, durable entry IDs,
identities and zero intervention counts. These private fixture identities do not
replace the authoritative repair assignment Job26/ledger24. Full native coding,
installed-runtime repair/activation, full repository suite and application workload
success are not claimed. Historical263989/267135 remain pilot-assisted recovery;
read-only recheck confirms all six captured historical rows unchanged.

Source is main3b252b684449447fd84bc122bace2ae494b11bf1 plus preserved dirty work.
source-verification.json checks1884 preexisting source files: only the two review
boundary files, existing review scope/routing tests and this evidence-ledger append
changed. New source is the focused live boundary test and PostgreSQL automatic-flow
test. Prior EVIDENCE_LEDGER bytes are hash-verified intact. The execution clause,
controller, recovery, acceptance validator, routing/settings/authentication,
accepted state-patch code, AGENTS.md and all other preexisting source hashes are
preserved. commands.json/runtime-identities.json bind invocations, native binary,
Go1.24.1 builder image, mounts and resource caps. Baseline harness formatting was
normalized while its compiled process ran; semantic fixture logic was unchanged.

Cleanup completed: `docker stop -t 10 omnidex-review-status-runner
omnidex-review-status-db`; both auto-removing containers absent, socket directory
removed, .tmp empty, no task-owned cache/build/profile/database/process remains.
Immediate post-stop assertion raced asynchronous auto-removal; subsequent name
checks confirmed absence without repeating effects. Database had0 private schemas
before shutdown,122MiB private data. Runner had1MiB tmp/207MiB cache/4MiB profile,
no native/test child process; sampled memory runner218.2MiB, database258.2MiB.
No image/network was created. Retained evidence below1MiB, within8MiB; final host
measurements and unchanged observed binary hashes are in cleanup.json. No shared
cache or unrelated resource was removed. Remaining observed pilot friction is the
initial confusion between the frozen assignment and its preserved “Propose” wording,
read-only harness scratch setup, database timeout entrypoint setup, and the
asynchronous cleanup-name check, all recorded without hidden retries or live changes.

Stages: implemented the frozen repair; verified the real boundary and automatic
correction/validated acceptance fixture plus affected regressions; delivered
reviewable local source/tests/evidence only; cleaned task resources. No commit,
push, installation, production restart, direct live database mutation, new live job/ledger,
or new approval. Omnidex's subsequent independent review/acceptance of Job26 remains
its own transition, not an execution-completion prerequisite. This task is finished.


### Job26 same-task correction of independent-review extraction finding

Corrective execution retains Job26/generation1/ledger24/revision3/task
`p_b0c9bf02bb31_1_task` in the same authoritative workspace. No new live ledger,
job, approval or acceptance mutation. The independent review reported that the
real sieve extracted unperformed verification as an artifact finding; its raw
`independent-review-boundary.jsonl` and result remain unchanged.

Repair narrows the existing review-findings semantic contract: review-conduct
limitations do not themselves establish artifact defects; separately reported
concrete artifact defects survive even alongside unfinished review work. This
adds no wording filter, heuristic, blanket outcome conversion, model station or
gate. Scan/execution prompts, routing, controller, recovery and acceptance
validation are unchanged. The existing live test now enforces zero findings for
zero-expected controls; the review-scope regression includes unperformed review
verification. Only these three source files and this ledger append changed.

`bin/review-status-20261019/correction-*` retains ownership and pre-resource
measurements, exact commands/prompts/results, source hashes, runtime identities,
checks and cleanup. Existing bounded runner/database scripts were reused with
all requested CPU/RAM/no-swap/PID/private-storage limits and 1800s lifetimes;
resources were recorded before creation and heavy checks ran sequentially.

The retained independent-review run met11/12 expectations, with the false finding.
Fresh pre-repair and post-repair runs both met12/12, using33 semantic calls each
on codex/gpt-6.1-sol/low/default,195211ms and200240ms summed case wall respectively.
The pre-repair run did not reproduce the intermittent false finding; no measured
reliability or latency improvement is claimed. Post-repair unfinished review has
error status and zero findings. Historical261397/262711 retain byte-identical
findings and complete verification evidence; all completed-defect, question,
permission, missing-evidence and execution-failure controls pass. Code compares
all scan/execution envelopes byte-for-byte unchanged. Original historical
263989/267135 remain preserved pilot-assisted recovery evidence.

Affected checks pass: complete codingagent0.008s, non-opt-in codex0.349s,
established affected queue fixture subset33.991s, including invalid/unknown
output, review precedence, execution failures, recovery, accepted replay/state
patches and routing authority. Actual controller/PostgreSQL continuation also
passes37.311s test wall: same disposable job1/generation1/ledger1/task `task`/step2
records task_reviewed, corrects execution, opens fresh independent review, then
records validated task_accepted and completed job/ledger. Two executions/two
reviews, six real semantic calls,36345ms controller-through-acceptance, zero
manual resumes/report rewrites/additional approvals/replacement jobs/interventions.
The unresolved-acceptance probe is rejected without mutation. This is real
classification/controller/PostgreSQL with bounded scripted native work, not full
native coding or installed-runtime evidence. `correction-summary.py` mechanically
checks these claims while preserving all earlier evidence.

Cleanup: both task-owned auto-removing containers stopped and absent; socket
removed, .tmp empty; private tmp/cache/profile/build/database retired. No task
child processes, images or networks remain. Source preservation and retained
evidence size (below8MiB) are checked in correction-summary.json and
correction-cleanup.json. No new pilot setup failure occurred; previously recorded
pilot friction and reliability limitations remain. Implemented and verified the
frozen correction; delivered local source/tests/evidence; cleaned task resources.
No commit/push, installation, live schema change or production restart. Subsequent
independent review and acceptance of this assignment remain Omnidex-owned.


## Execution obligations through independent review — Job27 / ledger25 / 2026-10-19

Implemented frozen Job27/generation2/ledger25/revision4/task
`p_53248246b740_1_task`, authoritative root
`/home/gryph/Projects/tools/omnidex`, workspace `directory_30_84644881`.
The frozen assignment remains byte-identical with exactly one active task. No
new live job/ledger, approval, revision, schema change, installation or restart.
Omnidex authors implementation, tests, documentation and independent review;
Codex pilots and inspects evidence. Execution implements this frozen task
without deriving another job or ledger. Authorization, revision, immutable
routing, task selection, acceptance and lifecycle validation stay code-owned.

The existing execution semantic contract now uses the code-observed completed
native turn: substantive reports of unfinished implementation, checks not run,
failed checks and missing proof reach independent review without needing a prose
completion announcement. This is report completion, never implementation success
or acceptance. Runtime/transport/infrastructure failures, owner questions,
permissions and malformed/unknown output remain explicit. Review receives the
original execution report with call/round identity, frozen task and durable
clarifications, reconstructed from journal state on continuation/replay. Review
identifies concrete missing assigned work/proof as actionable findings; its own
unfinished conduct remains distinct. Directions to corrective execution are not
owner questions. Same-task correction opens fresh independent review. Acceptance
rejects unresolved findings and altered attached execution evidence; older
accepted journals and accepted task state remain preserved.

Native assignment instructions consume durable owner proof waivers as amendments
to the applicable obligation without approval of the answer. Enabling answers
retain the work. Existing focused input, feedback transaction and clarification
projection are reused; genuine blockers explain the missing owner decision and
retain the frozen task. No wording filter, report rewrite, new station, queue,
approval gate, model-owned authority or additional proof requirement was added.

`bin/execution-proof-20261019/` retains ownership, pre-resource pressure, exact
commands/model envelopes/results/native frames, failures, source hashes, route,
durations, counts, full disposable PostgreSQL journals and cleanup. Original
Job26 decision285235, interruption285429, execution_response285428 and parser
call284838 are read-only captures, including the complete unedited 663-byte
report and original model journal. The entire completed Job26 read response and
original receipts remain byte-identical. Its historical manual reclassification
is preserved as history, not proof of this repair.

Final semantic verification: 23 cases, 49 real semantic calls,
304596ms summed case wall, codex/gpt-6.1-sol/low/default. This includes the
original report, unrelated unfinished/unproven/failed reports with and without
completion declarations, blocker/permission/infrastructure controls, missing
execution-proof findings and all11 prior ReviewResponse controls. The original
failed parser route is retained separately; no route-comparison or statistical
reliability claim is made. Ordinary codingagent/codex checks and the established
affected PostgreSQL subset pass, including full captured-journal acceptance,
invalid/unknown output, permission, cancellation, revisions, immutable routing,
lifecycle, interruption, continuation and accepted replay. Full-repository and
installed-source verification are not claimed; unrelated obsolete test contracts
remain unchanged.

Final real-controller/PostgreSQL flows use bounded scripted native artifact work,
not autonomous native coding. Each private fixture keeps job1/generation1/
ledger1/task `task` and its own recorded workspace identity. Automatic correction:
6 semantic calls, 38695ms,
2 executions/2 reviews, zero owner/pilot intervention. Proof waiver:
7 calls, 45322ms; enabling answer:
7 calls, 45980ms. Each answer flow has
one focused owner intervention, 3 executions/2 reviews, a durable replay-safe
answer, no additional approval, and validated same-task job/ledger completion.
The enabling path performs the missing exact-byte proof; the waiver path removes
that obligation. Required missing proof is rejected before either answer.
Controller restart/reconstruction and accepted replay repeat no completed work.

Retained failures/pilot friction: the first live missing-proof correction direction
was classified as an owner question; terse unfinished/failed reports exposed a
second completion-announcement assumption. Both semantic defects were fixed at
the existing boundary and reverified on final source. A fixture variable-shadowing
assertion and an incomplete replay-projection capture were corrected, retaining
all failed attempts and separate full-journal runs. Historical API evidence
reading also failed explicitly; scoped read-only database captures supplied the
original records. No failed run was silently discarded or retried unchanged until
success. `pilot-friction.json`, `check-status.txt` and `summary.json` retain details.

Resources were declared before creation: reused runner2CPU/2GiB/no swap/256PIDs,
private tmp/cache768MiB each/profile32MiB; database1CPU/512MiB/no swap/128PIDs,
private data256MiB/tmp32MiB, no database network; both1800s lifetime. Checks ran
one heavy operation at a time. Cleanup removed both auto-removing task containers,
the scoped socket and all private builds/cache/profile/tmp/database/processes.
No image/network was created; unrelated containers, shared caches, dirty source
and prior evidence remain. Retained evidence is below8MiB; cleanup and source
preservation are mechanically checked. No commit/push or live delivery action.

Stages: implemented and verified the frozen execution assignment; delivered
reviewable local source/tests/documentation/evidence; cleaned task resources.
Omnidex's subsequent independent review and code-owned acceptance close this same
task; those later transitions are not represented as this execution's result.

## Bounded semantic task ordering — 2026-10-20

Implemented `codingagent.OrderUnorderedTasks` over caller-selected eligible
unordered tasks using the existing lightweight `userintent.Ask` transport.
Multiple tasks cause one ordering-only question containing code-owned IDs and
exact goal instructions. The response is only a proposed JSON ID sequence.
Code validates a complete exact permutation before assigning consecutive
`LedgerItem.Rank` values. Slice positions, identities and every other task field
remain unchanged. Zero/one task makes no inference. Missing, duplicate, foreign,
malformed, oversized, transport-failed or canceled responses apply no order.
Eligibility and persistence remain caller-owned; this assignment adds the
operation and tests, without parser-classification or queue integration changes.

Focused tests exercise all six permutations of three tasks, content preservation,
missing/duplicate/foreign IDs, malformed/trailing/null output, transport failure,
input bounds, cancellation and zero/one tasks. Evidence and source identities
are retained in `bin/task-ordering-20261020/` within 8 MiB. Tests use injected
transport responses; no live-model reliability or installed-source claim.

Disposable Docker ownership, paths, limits, lifetime and cleanup conditions were
declared before creation; RAM/swap/disk/cache pressure was measured first.
Runner limits: 2 CPU/2 GiB/no swap/256 PIDs, private tmp256 MiB/cache768 MiB,
1800-second lifetime, source and existing module cache read-only. No PostgreSQL
resource was needed. Heavy checks ran sequentially. Initial package check passed;
its retained output also records formatting differences observed before formatting
completed. The final formatted-source check and cleanup result are recorded in
the same evidence directory. Existing dirty source, jobs25/26/27, prior evidence,
settings/authentication/routes and parser/execution/review repairs are preserved.
No commit, push, installation, live schema mutation or restart. Fresh independent
review and code-owned acceptance follow this execution response; they are not
claimed here.

Final checks: complete `internal/codingagent` package passes on final source
(0.010s); formatting and `git diff --check` pass. Source hashes match the tested
revision. Auto-removing runner is absent; private tmp/cache/build/process resources
retired with it. No host socket, image, network or volume was created. Retained
evidence is approximately 28 KiB. Stages: implemented and verified this bounded
operation/tests; delivered local reviewable source/evidence; cleaned task resources.

## Persisted proposal ordering — Job28 / ledger26 / task2 / 2026-10-20

Implemented frozen Job28/generation2/ledger26/revision8/task
`p_d9022b779337_1_task` in the authoritative workspace. The frozen assignment
remains unchanged. No live job/ledger, revision, schema, approval, installation,
restart, commit or push was created by this execution.

Initial multi-task proposals invoke the existing bounded ordering operation before
ledger creation. Newly added unordered tasks are ordered within their new portion;
existing ranks and explicit owner reorder operations remain authoritative. Code
validates the complete permutation, retains its exact question/result and origin
operation in the existing evidence journal, and persists ranks through the existing
create/mutation transaction. Canonical content fingerprints reuse a resolved order
across reconstructed controllers, changed parser slice positions and unchanged
resumed operations. Changed instructions do not reuse an unrelated resolution.
Zero/one eligible task, ordinary review-state changes, amendments, restores,
removals, explicit reorder, freeze, execution and accepted replay introduce no
ordering inference. Ordering uses the existing lightweight semantic transport;
Codex gets a scoped instruction-isolated parser client which is closed afterward.

Unpublished planner prose remains `planning_draft` evidence. The owner proposal
is rendered from the committed ledger in its rank/ID display order, with ledger
identity/revision attached. Publication rechecks the current projection under the
ledger lock and is idempotent by operation. A stale result cannot mutate a newer
revision or publish an older projection. Queued planning replies publish their
committed projection before resolution. Existing historical planning responses
remain readable without modifying accepted journals.

A demonstrated continuation discrepancy was corrected: unchanged owner replanning
leaves the ledger `revising`; the worker incorrectly checked `frozen` before
calling the existing unchanged-proposal transaction. The focused worker check
now reaches `proposed` while preserving rank. PostgreSQL continuation/refreeze
retains accepted tasks and selects no completed work. Rank-tie inspection and real
freeze checks establish that freeze inserts the persisted rank/ID-sorted snapshot,
so existing fairness ordering agrees with display. The scheduler query and immutable
fairness identities remain unchanged; no additional queue or priority system exists.

`bin/proposal-ordering-20261020/` retains the frozen assignment, exact commands,
source hashes, resource declarations/pressure/limits, successful and failed checks,
synthetic semantic receipts, actual disposable PostgreSQL planning journals,
approved/frozen/carried projections, preservation checks and cleanup. Both ordinary
and tied-rank fixtures intake z,a and persist/present/freeze/select a,z. Each has
one injected ordering call, two scripted executions and two separate task-local
review calls; reconstruction, queued owner reorder, freeze, execution, accepted
replay and carry-forward add zero ordering calls. The stale-result fixture changes
the owner rank to42 during ordering, then rejects both stale mutation and stale
publication, preserving that newer rank and committing no candidate additions.
These are real controller/repository checks with scripted planning, ordering and
native work, not live natural-language ordering, autonomous native coding,
installed-source verification or statistical-reliability evidence.

Final affected checks pass: full codingagent0.011s; non-opt-in Codex0.275s;
current Ollama raw/context/timeout and ordering-transport checks0.864s; established
affected PostgreSQL subset38.189s; production worker files plus focused unchanged
continuation0.003s. Final retained-journal integration checks pass3.154s. Existing
permission, cancellation, revision, immutable-routing, state-PATCH, execution-proof,
review, recovery and acceptance checks remain in the affected subset. Formatting
and `git diff --check` pass. Existing obsolete test contracts remain unchanged;
no full-repository-suite claim. Prior execution-proof and bounded-ordering source
hashes match their retained accepted evidence.

Actual pilot friction is retained in `pilot-friction.json`: workspace-read and
fixture identity/schema/response mistakes; rejected fixture manipulation of
immutable fairness identity; initial runner missing private-mount privileges;
auto-removal/name-reuse race; obsolete Ollama test compilation; and error shadowing
introduced during scoped transport hookup. The invalid-permutation check caught
that last defect; error propagation was corrected and final checks passed. One
unchanged rerun confirmed that failure before correction. No protection or test
assertion was weakened to obtain a pass.

Resources were declared before creation and pressure measured first. Runner:
2CPU/2GiB/no swap/256PIDs, private tmp256MiB/cache768MiB; PostgreSQL:
1CPU/512MiB/no swap/128PIDs, private data256MiB/tmp32MiB/socket1MiB and no network;
1800-second lifetimes, one heavy operation at a time. The replacement runner
restored the established namespace-test privileges within Docker only. Both
auto-removing containers are absent, their private caches/builds/data/processes
retired, and the scoped socket removed; project `.tmp` is empty. No image,
network or volume was created. Necessary retained evidence is below8MiB.

Stages: implemented and verified this frozen integration assignment; delivered
reviewable local source/tests/evidence; cleaned task resources. Omnidex dispatches
fresh independent review and records code-owned acceptance after this execution
response; those subsequent transitions are not claimed here.


## Production semantic and PostgreSQL boundaries — Job28 / ledger26 / task3 / 2026-10-20

Verified frozen generation2/revision8/task `p_d9022b779337_6_task` against the
existing authoritative source. No production source repair was needed. Necessary
pilot code, exact inputs/results, raw provider responses, planning journals,
PostgreSQL projections, commands, limits, source hashes and failed attempts are
retained in ignored `bin/semantic-boundaries-20261020/` (below8MiB).

Two unrelated natural-language examples used the unchanged production ordering
renderer and `codex.OrderingClient` instruction-isolated native transport, actual
`gpt-6.1-sol`, low effort, native Codex request protocol. Intake and actual ledger
item insertion order were a,b,c in both cases (the latter queried by PostgreSQL
ctid before subsequent changes). Document instructions were publication,
proofreading/author approval, then drafting; the model returned c,b,a in5255ms.
Device instructions were final functional check, fault inspection, then replacement;
the model returned b,c,a in4807ms. Each final flow made exactly one ordering call.
Exact task instructions and every production-rendered byte are retained; IDs carry
no domain instructions. These finite observations make no statistical-reliability
or installed-source claim.

For each result, real controller/repository transactions persisted ranks before
publication, published that projection, applied scripted owner agreement/freeze,
and selected the exact approved sequence through PostgreSQL
`NextIncompleteCodingLedgerTask`. Reconstructed controllers reused the durable
ordering receipt for unchanged tasks despite permuted intake and a new operation;
reconstructed repositories selected the same persisted sequence. This is controller/
repository reconstruction against the same disposable database, not a production
process or database restart. Each flow performed three scripted native executions
and three separate scripted task reviews. Accepted replay repeated no completed
work. Post-approval ordering calls: zero. Planning intake, owner approval controls,
native artifact work and task review are scripted boundary fixtures, explicitly
separate from the live natural-language ordering evidence; no autonomous workload
construction claim is made.

An additional actual PostgreSQL/manual-control pilot changed the resolved a,z
proposal to z,a, published and approved/froze that owner projection, then selected
z,a. Manual reorder and all later stages added zero ordering calls. Existing fresh
checks additionally prove stale semantic results and stale publication cannot
replace the newer owner rank42, queued replies and rank ties retain display/selection
agreement, and accepted carry-forward selects no completed task. Affected authority,
stale-revision, state-PATCH, permission, cancellation/resume, parser/native recovery,
execution-proof and task-review checks pass without weakening assertions.

Final checks: semantic/PostgreSQL pilot12.160s; manual override1.026s; complete
codingagent0.011s; non-opt-in Codex0.272s; current Ollama raw/context/timeout and
ordering transport0.842s; established affected PostgreSQL subset37.685s; worker
unchanged-continuation0.003s. No full-repository-suite claim. Baseline hashes prove
all pre-existing source and prior ledger bytes were preserved before this append;
`git diff --check` passes. Frozen assignment/settings/authentication/routes, dirty
and accepted work, jobs25/26/27 and previous evidence were not changed.

Actual pilot friction is retained: initial evidence extraction selected a tombstoned
rank3 item and was corrected to the active frozen task; a capture wrapper omitted
native request preparation and was rejected before inference; the first successful
pilot deferred journal reading beyond planner authority and captured mutated replay
intake. The final pilot records original intake, raw responses and journals under
active authority. All attempts remain labeled. Docker auto-removal briefly raced
the first cleanup assertion; subsequent scoped inspection confirmed removal.

Resource ownership, paths, budgets, lifetime and cleanup were declared before
creation; RAM/swap/disk/cache pressure was measured first. High host swap use was
observed; no host memory/settings changes were made. Runner2CPU/2GiB/no swap/256PIDs,
private tmp/cache768MiB each/profile32MiB; PostgreSQL1CPU/512MiB/no swap/128PIDs,
private data256MiB/tmp32MiB/socket1MiB, no database network; both1800s lifetimes.
Checks ran sequentially. Both auto-removing task containers, socket, private builds,
caches, profile, database and processes are gone; project `.tmp` is empty. No image,
network or volume was created. No commit/push, installation, live schema change,
production restart, source copy/worktree/archive/backup or live job/ledger creation.

Stages: implemented the scoped evidence pilot and ledger append; verified the
requested production boundaries; delivered reviewable local evidence; cleaned task
resources. Omnidex dispatches the fresh independent assignment review and records
code-owned acceptance after this execution response; neither is claimed as completed
by the scripted task reviews above.
