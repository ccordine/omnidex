# Omnidex Integration Notes

This document records the internal integration pass that brought the local deterministic CLI and service-backed queue/runtime into one Omnidex repository before public release.

## Base

- Base project: `omnidex`
- Module path: `github.com/gryph/omnidex`
- Go version normalized to `go 1.23` to avoid automatic Go 1.24 toolchain downloads on systems that do not already have Go 1.24 installed.

## Integrated Components

Copied into this tree:

- `cmd/omni` -> deterministic Omnidex CLI entrypoint
- `internal/omni` -> Omnidex app, router, permissions, run logs, migrations, policies, tools, verification
- `docs/omni/*` -> Omnidex docs: contracts, roadmap, dev bible
- `database/migrations` -> Omnidex migration directory placeholder

Adjusted:

- `cmd/omni/main.go` import path changed from `omnidex/internal/omni` to `github.com/gryph/omnidex/internal/omni`.

## Validation

Focused validation:

```bash
GOTOOLCHAIN=local go build ./cmd/omni
GOTOOLCHAIN=local go test ./internal/omni ./cmd/omni
```

Full validation:

```bash
go mod tidy
go test ./...
go build ./cmd/core
go build ./cmd/cli
go build ./cmd/omni
```

## Architecture Direction

Treat `cmd/omni` / `internal/omni` as the lightweight local deterministic CLI and `cmd/core` / `internal/worker` as the heavier queue/runtime engine. The clean path is to make Omnidex's deterministic permission/router/logging model a front door that can call the queue runtime stages.
