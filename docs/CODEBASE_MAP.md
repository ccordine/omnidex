# Codebase Map

The codebase map is Omnidex's durable, incrementally updated model of a workspace: what exists, what it does, how pieces relate, and where future tasks should start.

It complements the workspace index. The workspace index is deterministic file state: paths, hashes, manifests, and package probes. The codebase map adds routing meaning on top: modules, files, symbols, entrypoints, tests, commands, risks, and task routes.

## Commands

```sh
omni map build
omni map update
omni map query "where is scope drift handled?"
omni map route "fix repeated command loop recovery"
```

By default the map is written to:

```text
.omni/codebase-map.json
```

Use `--workspace PATH`, `--out PATH`, `--max-files N`, and `--json` as needed.

## Design

The map is advisory routing context, not execution permission.

Execution still requires worksite survey, command policy, objective ledger, progression gate, and evidence verification. A stale or incorrect map can suggest where to inspect first, but it cannot authorize scope drift or completion.

## Incrementality

`omni map update` reuses the workspace index update path. File summaries carry `sha256`, `summary_generated_for_hash`, and `stale`. If a file hash changes from the previous map, the summary is marked stale so downstream code-owned workflows know to inspect or regenerate before trusting it.

## Task Routes

Task routing returns:

- likely files
- relevant modules
- verification commands
- known risks
- reasons
- confidence

The code-owned repository workflow may load `.omni/codebase-map.json` and validate a
compact `task_route` before using it as advisory evidence. The map never chooses an
operation, grants mutation authority, or enters model context as a project plan.
