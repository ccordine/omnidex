# Omnidex Recovery Merge Notes

This recovered tree merges the most complete recovered Omnidex V2 codebase with the Omnidex Neo deterministic CLI scaffold.

## Base

- Base project: `omnidex-v2`
- Module path: `github.com/gryph/omnidex`
- Go version normalized to `go 1.23` to avoid automatic Go 1.24 toolchain downloads on systems that do not already have Go 1.24 installed.

## Merged from Omnidex Neo

Copied into this tree:

- `cmd/odn` -> deterministic Neo CLI entrypoint
- `internal/odn` -> Neo app, router, permissions, run logs, migrations, policies, tools, verification
- `docs/neo/*` -> Neo docs: contracts, roadmap, dev bible
- `database/migrations` -> Neo-style migration directory placeholder

Adjusted:

- `cmd/odn/main.go` import path changed from `omnidexneo/internal/odn` to `github.com/gryph/omnidex/internal/odn`.

## Validation performed in recovery environment

The recovery environment had Go 1.23.2 and no internet access, so full `go test ./...` could not fetch external dependencies (`pgx`, `ledongthuc/pdf`) or download the Go 1.24.1 toolchain.

Validated locally without external dependency fetch:

```bash
GOTOOLCHAIN=local go build ./cmd/odn
GOTOOLCHAIN=local go test ./internal/odn ./cmd/odn
```

Both passed.

Full validation to run on your machine:

```bash
go mod tidy
go test ./...
go build ./cmd/core
go build ./cmd/cli
go build ./cmd/odn
```

## Suggested next step

Treat `cmd/odn` / `internal/odn` as the lightweight local deterministic CLI and `cmd/core` / `internal/worker` as the heavier queue/runtime engine. The clean path is to make Neo's deterministic permission/router/logging model a front door that can call Omnidex V2 runtime stages.
