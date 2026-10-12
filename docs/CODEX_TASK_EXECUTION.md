# Persistent task execution with Codex

## Active task: governed agent execution

Root: /home/gryph/Projects/tools/omnidex. Branch: feat/governed-agent-mode,
based on 2fb65541; the prior provider/persistence PR remains its intended base.
Goal: add explicit Normal and Agents coding modes. Agents mode finalizes a plan,
dispatches narrowly scoped environment-capable execution jobs, uses an independent
reviewer to return concrete findings/questions, repeats only unresolved work, and
performs an overall review before completion. The existing queue remains authority.

Acceptance: consumed mode selection; frozen task/file/check contracts; replaceable
executor/reviewer providers; enforced tool/file boundaries; durable task/run/review
state and cancellation/recovery; finding-driven correction and persisted questions;
overall review and real verification; existing Normal behavior preserved; actual
Codex environment execution and independent review qualified; scoped PR and cleanup.

User authorization explicitly supersedes semantic-only restrictions for Agents
mode. Normal mode retains those restrictions. No development copies/worktrees,
unrelated owner changes, or Daemonet work. Implementation first, affected tests
at the end. State: portable scoped planner/executor/reviewer contracts, native
Codex sandbox calls, durable queue records and mode selection are implemented.
Atomic plan finalization, question replay, completion gates, environment ownership
and disposable tool caches are complete. Native Codex inspected files, performed
governed source edits, passed deterministic checks, and reviewed in a fresh thread.
Its file-root sandbox cannot grant individual writable files safely; execution
therefore uses read-only native source mounts and the typed omnidex_write_file
tool through the existing mutation fence, with exact original-content validation.
Planning/review grants no writer. Native running-command cancellation passed.
Controller/component and affected queue/SQL checks passed, including preservation
of rejected requirements, same-generation answers, stale pause/generation rejection,
historical replay, cancellation/replan during pending resume, and completion guards.
Browser mode selection, pause/answer/reload and lifecycle controls pass their
affected checks. The owner added independent per-role Codex model/effort/speed
selection, model refresh and managed CLI updates. Those controls, live discovery,
typed saved settings and new-job configuration snapshots are implemented; 115
frontend tests and type checks pass. Native discovery reports 10 models, with
efforts through max/ultra and Fast/priority advertised by this installation.

The original Charmeleon requirement intake, authorization sieve and carried
decisions remain authoritative. Native task decomposition receives only finalized
approved criteria and its file catalog, never the original request. Execution and
task review reject parent objectives/workloads/requirements and unrelated files,
checks/findings; answered context is scoped to the assigned step. Overall outcome
review alone receives the original objective. A proposed intake shortcut was
removed before delivery. The first ordinary qualification reached an artificial
six-minute test deadline during original intake. Per owner direction, the fresh
unsteered qualification now has no overall deadline and Go's test timeout is off.
The second qualification completed intake but paused at task decomposition because
artifact bindings, construction constraints and supported runtime/file authority
were missing. Original intake context is now snapshotted with the frozen criteria.
A separate bounded semantic leaf extracts only literal construction constraints;
the planner receives those facts, resolved artifact values and code-owned runtime
capabilities. Narrow execution/review still receive only their assigned task.
Queue snapshot/replay checks and affected Go suites passed. A real browser check
also exposed extra native catalog metadata rejected by the frontend DTO parser;
that exact contract mismatch is fixed and focused settings checks pass.
Source is frozen for a fresh ordinary-request native qualification with no overall
deadline. Final UI/core builds are activated in the retained native installation. Actual
Chromium verified Normal/Agents keyboard selection and 1280/375px layouts, all
three live model/effort/speed controls (including max, ultra and Fast/priority),
Save, Refresh and reload persistence. Production routes remained identical when
saved. All six affected Go suites passed with the race detector. The next unsteered qualification completed intake, native planning and source
execution, then independent task review paused over an unrequested historical
TDD proof criterion. Task-planning acceptance provenance is being corrected:
implementation methodology must not become additional acceptance unless required.
Final review also identified two bounded guards: completion must reuse the existing
narrow-context validator, and installed updates must serialize with all native-job
admission/resume paths and include Normal Codex jobs in the idle check. These exact
fixes are implemented; queue guard regressions and real PostgreSQL admission
serialization checks for both Normal and Agents passed. The managed updater
completed successfully on the existing signed-in installation, which remains
Codex 0.162.1. A fresh ordinary qualification is running from frozen source;
Source commit: bae509dd. Delivered as draft stacked PR
https://github.com/ccordine/omnidex/pull/6, based on the retained provider PR #5.
Actual PostgreSQL API/race checks, current builds and vet pass. That qualification
finished intake but the native planner stopped on an inference notification. The
adapter treated every native error as terminal and returned before retaining the
original error frame. It now honors the native willRetry flag within the same turn,
retains original retry/failure frames, and surfaces bounded terminal diagnostics.
Protocol regressions passed. A fresh ordinary native run is underway using the
advertised priority tier for task planning/execution/review; retained production
settings stay low/default. That run completed native planning, first task acceptance and targeted correction,
but task-2 review repeatedly paraphrased/combined the exact criterion label and its
valid numeric finding was structurally rejected. The task-owned run was canceled
through its real cancellation API after reproducing that nonconvergence; cleanup
completed. Native review output schemas are constrained to code-owned exact
criterion/task references (bound parent outcomes for overall review). Semantic
observations remain free text. Holdout now checks actual conversion values and
finite numeric edge cases, rather than inverse consistency alone. The subsequent run completed every narrow task and review, then overall
verification failed because the command adapter relabeled OverallReview as narrow
Review. The adapter now preserves the overall scope and strips write access;
execution checks still become read-only review of the same task. Narrow guards
remain enforced. Real native source editing, independent review, sandbox/env
isolation and running-command cancellation passed again (74.217s), as did scoped
projection regressions and affected race checks. A fresh full ordinary qualification
is running from frozen source; final evidence and cleanup remain before ready.

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
