# Benchmarks

Benchmarks are replayable task definitions for measuring Omnidex behavior.

List benchmark manifests:

```bash
omni bench list
```

Run a benchmark:

```bash
omni bench run npm-stimulus-tailwind-calculator
```

Prepare an isolated workspace and print the run packet without model execution:

```bash
omni bench run npm-stimulus-tailwind-calculator --dry-run --json
```

Inspect recent run telemetry:

```bash
omni run:trace latest
omni bench report
```

Current manifests live under `benchmarks/*/benchmark.json`.

## Manifest Fields

- `id`: stable benchmark identifier
- `description`: human-readable purpose
- `workspace`: workspace mode, such as `tmp`
- `prompt`: prompt to run
- `recipe`: optional recipe ID
- `success_criteria`: evidence that must be true for a pass

## Intended Report Metrics

The benchmark harness should record:

- success or failure
- commands run
- rejected commands
- elapsed time
- model/endpoint where available
- files created or modified
- tests/builds/checks passed
- final objective ledger state
- early completion decisions
- repeated command rejections

The evidence ledger format in `docs/EVIDENCE_LEDGER.md` is the intended benchmark output foundation.
The run trace format in `docs/RUN_TRACE.md` is the intended latency/waste summary foundation.

## Current Run Foundation

`omni bench run` currently:

- loads a benchmark manifest by ID
- prepares an isolated temporary workspace for `workspace: "tmp"`
- executes the prompt through the structured command loop when a model client is available
- records a benchmark session and trace-derived report
- supports `--dry-run` for manifest/workspace validation without model execution
