# Roadmap

This roadmap tracks the work needed to make Omnidex faster, more deterministic, and easier to measure.

## Status Key

- `done`: implemented and covered by tests
- `active`: current implementation target
- `planned`: not started

## Optimization Track

| Status | Item | Target |
| --- | --- | --- |
| done | Evidence ledger | Export objectives, commands, rejected commands, failures, and summary counts. |
| done | Run trace | Summarize model calls, command counts, rejections, loop exhaustion, and completion pressure from session events. |
| done | Recipe DAG foundation | Recipes can express objective dependencies and are validated for cycles. |
| done | Benchmark run foundation | Run benchmark manifests into isolated workspaces and emit reports. |
| done | Deterministic fast-path resolver | Execute safe, structured, non-LLM actions when intent is already explicit. |
| done | Workspace index foundation | Persist file hashes, manifests, and deterministic package probes. |
| done | Patch mode | Apply and dry-run workspace-bounded unified diffs as the constrained source-editing path. |
| done | Failure fingerprint foundation | Classify command failures into known categories with deterministic remediation hints. |
| done | Command/result cache reuse | Opt-in runtime reuse for eligible verification/read commands when indexed inputs are unchanged. |
| done | Adaptive role collapse | Skip summarizer, done-check, and planner calls when deterministic recipe probes already satisfy selected objectives. |
| done | Deterministic recipe completion probes | Satisfy selected recipe objectives from completion probes before asking done-check models. |
| done | Ollama prewarm/stability profile | `omni ollama prewarm` reports model load/profile timings and deterministic failure diagnosis. |
| done | Incremental index updates | Update an existing workspace index by rehashing changed files only. |
| done | Roadmap foundation | Current optimization track has concrete CLI surfaces, docs, and tests for each listed item. |

## Completion Criteria

The track is complete when Omnidex can:

- run and report benchmark tasks reproducibly
- show where time and retries went
- avoid model calls for explicit deterministic actions
- resume from observed state instead of restarting tasks
- verify recipe objectives with deterministic evidence where possible
- classify common failures before asking a model to guess
