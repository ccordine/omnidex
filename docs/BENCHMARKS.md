# Autonomy benchmarks

The repository contains a rubric-blind benchmark foundation. It does not currently expose an `omni bench` CLI command, production builder adapter, or black-box application evaluator. Do not claim an app benchmark has run until those concrete boundaries exist and a frozen production build completes them.

## Single build

`internal/autonomybench.Run` receives only:

- one request identity;
- the unchanged ordinary user request;
- one fresh workspace;
- a production builder;
- a separate evaluation loader and evaluator.

The builder returns before the runner loads the evaluation plan. A failed build is still evaluated so accepted partial behavior remains measurable.

## Baseline versus thinking assistance

`internal/autonomybench.RunComparison` is the authoritative A/B coordinator. It enforces:

1. The baseline receives the unchanged user request and its fresh workspace.
2. The assisted builder receives the same unchanged bytes and a distinct fresh workspace.
3. Both builds stop before the withheld evaluation loader is called.
4. The same black-box checks evaluate both finished workspaces.
5. The result records both complete build observations, build failures, per-check evidence, weighted scores, and the assisted-minus-baseline delta.

The `BuildInput` type deliberately contains only `UserRequest` and `Workspace`. It has no variant, rubric, expected feature, plan, or memo field. Baseline and assisted routing must be frozen in their separate builder instances before the run starts.

## Required production adapter

A valid app-build comparison still requires one checked-in adapter that:

- submits the ordinary request through the production front door;
- waits for the authoritative job to stop without source edits or steering;
- uses a verified empty workspace;
- reports exact model calls, prompt bytes, accepted/rejected units, corrections, verification runs, and file changes from immutable evidence;
- configures the assisted build so R1 memos can advise only registered narrow stations and never choose a plan, graph, path, repair target, or completion state.

There must be one production execution path. Do not implement comparison by shelling out to an alternate agent, replaying hand-authored intermediate prompts, or maintaining a benchmark-only builder.

## Required evaluator

The evaluator loads only after both builds stop. It must use typed black-box checks of user-visible behavior and permit any implementation satisfying the ordinary request. Private file names, component structures, source snippets, or implementation-specific selectors are not acceptance criteria unless the request explicitly required them.

Every build is measured, including failures. Report working and missing weighted capabilities, stop reason, calls by station and model, context volume, accepted work, corrections, verification runs, elapsed time, and human interventions.
