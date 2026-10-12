# Persistent task execution with Codex

## Active task: terminal Agents in the invoking directory

Root: /home/gryph/Projects/tools/omnidex. Branch: feat/local-cli-agent-chat,
based on main 1169030c. Goal: plain omni chat runs scoped Agents directly from any
local directory on this device: request, original Charmeleon research/planning,
scoped execution, independent verification, recap, then another task in the same
terminal. Deliver a solid MVP through the relevant gauntlets, then stop; subsequent
self-improvement will be performed through Omnidex, not added to this task.

Acceptance: CLI chooses Agents for new jobs and freezes mode in idempotent turns;
server uses existing saved Codex role settings; exact live local-client/physical
workspace binding allows native execution outside the static service root while
foreign/unbound jobs retain existing restrictions; terminal questions resume the
same job; existing cancellation/replan and mutation fences stay intact; installed
omni resolves to the current CLI; a real existing-project terminal update passes;
affected checks, preserving activation, scoped PR and cleanup complete.

Constraints: preserve original Charmeleon intake, narrow job contexts and owner
uncommitted inference work. No global filesystem grant, alternate ledger, source
copy/worktree, broad model context or unrelated feature work. One source branch.
Source implemented first; affected verification at the end. Existing native install
and credentials are retained. Resources before creation: one ignored
bin/cli-agent-qualification fixture set (32 MiB, retired on completion), two candidate
binaries (256 MiB, activated then retired), and one disposable PostgreSQL container
(512 MiB RAM, two CPUs, 1 GiB tmpfs, auto-removed). One local compiler operation,
two workers. Actual native inference uses one producer, no overall deadline.

State: implemented terminal Agents dispatch, exact mode/retry identity, paused-question
projection/rendering, live local-directory ownership and disconnect cancellation,
observed-file/check recap and managed CLI PATH binding. Focused failing reproductions
preceded fixes. Affected cmd/client/codingagent/transport/API/queue/worker race checks
passed. Original-plan-pause regression passed against PostgreSQL with the race detector.
Full CLI/client/codingagent/runtime package checks, affected vet and builds passed.
One task-owned red-test cache, bin/terminal-chat-go-cache (129 MiB observed, 512 MiB
budget), will be retired after checks. Current binaries and CLI launchers are activated. The authoritative operation
validator from setup.sql was replaced transactionally in the retained runtime schema;
Normal descriptors remain unchanged. No job/data/config/credential rows were changed.
The isolated PostgreSQL fixture was recycled after completed tests before native
qualification. The first real PTY run reached original intake, frozen planning and
existing-file execution, then native review rejected a quoted criterion in its strict
output-schema enum (400 invalid_json_schema). Safe code-owned reference mapping is
implemented and unit/race/vet checked in the native adapter; portable criteria and original wire evidence stay
exact. PTY dimensions are explicit and failure diagnostics are bounded.
A fresh two-task PTY acceptance run is active from frozen source.
No separate Charmeleon gauntlet executable is registered; the relevant application,
queue/step lifecycle, workspace, pause, cancellation and real-terminal checks are the
applicable proof. No original intake/planner shortcut is introduced.

## Completed task: governed agent execution

Authoritative root: /home/gryph/Projects/tools/omnidex.
Branch: feat/governed-agent-mode, based on 2fb65541 and stacked on provider PR #5.
Goal: implement Normal/Agents coding modes with narrow execution, independent
review/correction, overall review and durable code-owned task/evidence state.

Acceptance: mode selection; frozen task/file/check contracts; replaceable providers;
enforced scope and read-only review; durable questions, findings, cancellation and
recovery; completion guarded by observed checks and exact requirement coverage;
existing Normal/Charmeleon intake preserved; per-role Codex models, effort and Fast,
model refresh and managed CLI updates; actual native qualification; preserving
installation activation, one scoped PR and task-resource cleanup.

Authorization: Agents may use the granted environment. Normal retains semantic-only
restrictions. No source copies/worktrees, owner-file changes or Daemonet work.
No overall qualification deadline. Narrow execution/task review receive only the
assigned task, relevant files/checks/findings/answers; parent context is rejected.
Task decomposition receives finalized requirements, resolved artifact bindings,
literal construction facts and code-owned runtime authority, never the original
request. Original Charmeleon requirement intake remains unchanged. Overall review
alone receives the original objective. Native source mounts are read-only; execution
edits use the existing Omnidex mutation fence with exact original-content validation.

Implemented: provider contracts, native sandbox/tool adapter, independent fresh
review turns, exact criterion-reference schemas, scoped correction/question replay,
durable queue completion gates, cancellation/recovery, mode and settings UI, live
model catalog, saved per-role routes and serialized managed updates/job admission.
Native retry notifications retain the same turn and original frames. Verification
preserves overall scope while stripping write access. Review now explicitly runs
independent checks over declared input classes/ranges and representation boundaries.

Verified: affected Go suites/race checks/vet; isolated PostgreSQL queue/API admission,
replay, question, completion and settings checks; frontend suites/type checks and
UI/core/CLI builds. Actual native source edits, fresh independent review, sandbox/env
isolation and running-command cancellation passed (latest complete check 74.217s).
Actual Chromium checked keyboard mode selection, desktop/mobile layouts, live model,
effort (including max/ultra), Fast/priority, Save, Refresh and reload persistence.
Managed update succeeded on the existing signed-in Codex 0.162.1 installation.
Production routes remain unchanged; retained production rows match the baseline.

Qualification: PASS on source 0eea5f26, 2026-10-12. The ordinary production
request passed original intake, task planning, three narrow execution/review pairs
and overall review: 8 retained native calls, 783.602 seconds. Independent holdout
passed actual results, return types, inverse consistency, representable large
integers/floats, signed overflow and invalid-input exceptions. A preserving runtime
restart retained the same completion calls. No intermediate plan, feedback or
application edits were injected. The request explicitly declares overflow behavior;
all earlier holdout assertions remain, with overflow checks added. Generic production
prompts contain no workload hints. Reviewer used high effort and advertised priority;
ultra routing was observed in the preceding legitimate question-producing run.

Delivered: commits pushed to feat/governed-agent-mode; stacked PR
https://github.com/ccordine/omnidex/pull/6, based on provider PR #5. Installed production
behavior is 5b1ab835; 0eea5f26 changes only qualification/documentation. All three native
units are active/enabled and readiness passes. Production routes remain unchanged.
Original jobs, messages, calls, receipts, outcomes and completion rows match the
pre-activation baseline byte-for-byte after the final preserving activation.
Installed binary SHA-256:
- bin/omnidex: 5c5977fca5e58b158c795839cb5816888995fa049ae4a95bd600effe81f9badd
- bin/omni: aaf48e4d5942a1ca0ddb4fdf470645ba09d94001b1ba677d666bbf70c347be9c

Cleaned: task-owned PostgreSQL fixture/container, request schemas/files, browser
profile/script, generated protocol, stale/native tool caches and PR-body scratch
retired. Candidate binaries were activated. No task-owned processes or disposable
resources remain. Pre-existing owner changes and shared caches remain preserved.
Stages: implemented, verified, delivered, installed, cleaned. No remaining task work.

Resources: generated installed protocol under bin/agent-protocol, at most 32 MiB,
removed after qualification. Native task tool caches are under
bin/agent-runtime/call-*; one call at a time, two workers, bounded by the existing
request lifetime and workspace/artifact limits, removed on call exit. Disposable
qualification fixtures use bin/agent-qualification and bin/agent-test-fixtures
(32 MiB, removed at completion)
and one isolated auto-removed PostgreSQL test container (512 MiB RAM, two CPUs).
One headless Chromium profile and UI qualification script/output may use that
same ignored qualification directory (128 MiB, one browser, removed after checks).
One core/CLI candidate binary set uses bin/omnidex-agents-candidate and
bin/omni-agents-candidate (256 MiB total, retired when activated). Native provider
qualification uses one process at a time; local compilation uses two workers and
one compiler check/build at a time. Test resources retire after actual completion
or explicit cancellation rather than an arbitrary whole-job time cutoff.
Native runtime inputs include the resolved Codex executable, installed Go tools
and module cache (read-only, excluding VCS credentials), and existing selected
workspace dependency directories. Authentication remains outside command grants.
Source and
durable native services remain in the established authoritative locations.

## Completed foundation (historical)

Authoritative workspace: /home/gryph/Projects/tools/omnidex.
Branch: feat/codex-persistent-task-execution, based on 82add43f.

Goal: use this device's signed-in Codex for bounded semantic work while
Omnidex owns task scope, context, execution, evidence, and completion. Keep
authoritative PostgreSQL state across service restarts and run Omnidex as an
automatically started native user service on this device.

Acceptance:
- A registered Codex provider executes one bounded responsibility through the
  existing signed-in installation; no model-directed filesystem or tool effects.
- Actual provider output, completion, usage, cancellation, and correction context
  are validated and recorded through the existing provider/evidence boundaries.
- Restart preserves jobs, accepted work, evidence, and context. Expired execution
  leases reconcile through the existing queue; completed work is not regenerated.
- Existing task isolation and evidence-controlled completion remain enforced.
- Native installed service and required state services start automatically,
  report real health, and survive a preserving restart on this device.
- Focused integration/security checks and complete affected checks pass before
  delivery; exact commits, installation, and remaining limits are recorded here.

Preserve the pre-existing CHANGELOG.md modification and untracked stateless
inference source, documentation, contract, and SDK files. They are not this task's
changes. Do not resume Daemonet login, phone, audio, or monitor work.

The user requested implementation first and verification at the end. The request
for permanence supersedes the previous reset-on-start database policy; update
that project contract without altering the owner's global prime directive.

State: implemented, verified, delivered, installed and cleaned.
Actual signed-in Codex generation, process-exit continuation, pre-dispatch
cancellation and running-process cancellation passed. Original native receipts
reported 7050/7075 input tokens and 9 visible output tokens per retained turn.
Independent turns are ephemeral; only source-correction contexts are retained.

Core and embedded UI builds passed. Final LLM/Codex/config/provider/runtime/CLI/API
checks passed. All database and queue integration checks passed. The worker
integration suite passed except a pre-existing publication fixture using obsolete
Go signatures; the fixture was corrected to the current generated function and
oracle contracts, including empty argument handling, and its Docker publication
rerun passed. Scope/isolation, interrupted attempts, accepted-leaf reuse, and
completion guards passed. These constructed fixtures are not autonomous-build proof.
Focused race checks and go vet passed. Native installation was rerun twice
without changing retained configuration, credentials or application state.

Installed production request: job 1, channel omnidex-installation-check,
"Answer with exactly the single word READY." completed with output READY.
Two original Codex calls (objective kind and conversation response) were accepted;
one completion evidence set sealed the step. Restarting PostgreSQL, Valkey and
the core preserved all job, message, call, raw receipt, outcome and completion
evidence rows byte-for-byte, with no additional provider calls.

Installed units omnidex.service, omnidex-postgres.service and
omnidex-valkey.service are active and enabled; user lingering is enabled.
Readiness and PostgreSQL/Redis health are OK at http://127.0.0.1:18090.
Port 8090 belonged to an existing service and was left intact. The native
launcher uses the same runtime URL and preserves the old omni installation.
Native task enforcement applies to requests executed by Omnidex; an unrelated
Codex chat or process does not become ledger-controlled merely by installing it.
No new development copy or worktree has been created.

Delivery: implementation commit 2ac81e23 plus the final pre-dispatch failure
receipt correction on this branch; pushed to the existing origin. One PR:
https://github.com/ccordine/omnidex/pull/5 (base main).
The final cancellation check validates its original not-dispatched failure
receipt rather than leaving an incomplete opening after process-start failure.
All pre-existing CHANGELOG and stateless-inference work remains uncommitted.

Cleanup: disposable PostgreSQL fixture removed; generated protocol schemas,
package downloads and verification logs/receipt retired. Only required native
binaries, durable database/session state, user units and launcher remain.
Existing shared caches, Codex-owned retained contexts, old installations and
unrelated services were preserved.

Resource ownership: Go/compiler checks use the established ignored bin directory,
two build workers, one heavy operation at a time. Task fixtures are disposable
containers only, with explicit CPU/memory/storage/lifetime bounds before creation.
Installed binaries and durable application/database state are necessary outputs,
not disposable fixtures. Retire test artifacts and task processes after checks;
preserve existing shared caches and unrelated services.

Protocol qualification artifact: bin/codex-protocol, generated by installed
codex-cli 0.162.1, at most 32 MiB. It establishes supported RPC fields and is
removed after the adapter is verified. It is not another source workspace.

Native dependencies: signed distribution PostgreSQL 18.4 and Valkey 9.1
packages matching this device's installed libraries, extracted under bin/native.
Budget 256 MiB; package downloads under bin/native-downloads are disposable and
removed after signature verification and installation. Native binaries are
necessary installed outputs. Durable state belongs to ~/.local/state/omnidex;
user units belong to ~/.config/systemd/user and configuration to ~/.config/omnidex.
The native client launcher is ~/.local/bin/omnidex; existing omni installations
remain intact. The installation qualification job and original evidence remain
in the application database; its disposable receipt file in bin is removed.
Only Omnidex-owned units and state are changed; no unrelated service is replaced.

Ledger verification fixture: task-owned Docker container omnidex-codex-ledger-test,
existing postgres:18.4-bookworm image, CPU 2, memory 2 GiB, tmpfs data 1 GiB,
loopback-only port 15433, auto-removal, no host mounts or owner data, removed
after database/queue/worker verification. Go checks use two workers and an
eight-minute deadline. Existing shared Go/npm caches remain owned by the host.
Bounded verification output: bin/codex-ledger-check.log and bin/codex-final-check.log,
at most 2 MiB each, summarized here and removed after delivery.
PR body preparation: bin/codex-pr-body.txt, at most 8 KiB, removed after the
single PR is created. Necessary binaries and durable state remain installed.
