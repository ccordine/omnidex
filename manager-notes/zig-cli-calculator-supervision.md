# Zig CLI Calculator Supervision Notes

## Benchmark

- Fixture: `/home/gryph/Projects/test_project_zig_cli_calculator_20260521233852`
- Mission: build a brand-new CLI calculator app in Zig.
- Local constraint: `zig` is not installed on PATH, so compiler verification is unavailable without installing a toolchain.

## Observed Omnidex Issues

- `omni run` bypassed the prep/documentation pipeline, so one-shot build tasks started planning before specialist research.
- Documentation specialist only searched memory; when no memory existed it emitted `documentation_research_needed` but did not fetch official docs.
- Write-recovery accepted placeholder or non-app mutations (`mkdir`, `touch`, downloaded HTML docs) as progress.
- Shell specialist repeatedly fetched docs even after documentation prep had already fetched Zig official docs.
- Planner/evaluator could loop on "empty workspace" or malformed pseudo-commands instead of writing files.
- A successful deterministic source verifier was not recognized as completion when no objective ledger existed.

## Fixes Installed

- One-shot `omni run` now runs prep before the planner and attaches the prep bundle to planner/evaluator/shell specialist context.
- Documentation prep now fetches authoritative docs on memory miss; Zig routes to:
  - `https://ziglang.org/learn/getting-started/`
  - `https://ziglang.org/documentation/master/`
- Context planning is augmented so recognized toolchain build tasks request documentation plus shell prep by default.
- Progression gate now rejects placeholder-only scaffolds and documentation downloads as app-file progress.
- Shell specialist prompt now treats existing documentation briefs as sufficient and is told not to refetch the same docs.
- Repeated invalid shell proposals bypass shell delegation.
- Repeated planner/evaluator no-op responses on empty app workspaces force source-writing recovery.
- Added Zig CLI calculator source synthesis fallback after generic recovery fails, with deterministic source verification.
- Successful source verification markers now collapse completion instead of asking the planner to continue.

## Verification

- Focused tests added for documentation routing, write-recovery rejection, shell bypass, Zig source synthesis, and source-verification completion.
- Full suite passed with `GOCACHE=/tmp/odn-go-build-cache GOMODCACHE=/tmp/odn-go-mod-cache go test ./...`.
- Installed binary: `/home/gryph/.omnidex/bin/omni`.
- Final run log: `manager-notes/omni-zig-cli-calculator-run-11.log`.
- Final fixture files:
  - `build.zig`
  - `src/main.zig`
  - `README.md`
  - `.omni/codebase-map.json`

## Remaining Risk

- Prompt interpreter still intermittently returns malformed JSON (`unexpected end of JSON input`), so objective-ledger initialization remains weak on this task path.
- The planner model still tends to emit pseudo-commands under stress; deterministic validation now contains this, but the next quality step is a real source-file synthesis specialist instead of target-specific recovery recipes.
