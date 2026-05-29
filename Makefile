.PHONY: tidy build core cli omni run fmt ui ui-dev

tidy:
	go mod tidy

ui:
	cd internal/api/web && npm install && npm run build

ui-dev:
	cd internal/api/web && npm install && npm run dev

build: ui core cli omni

core:
	./scripts/build-core.sh

cli:
	go build -o bin/agent-cli ./cmd/cli

omni:
	go build -o bin/omni ./cmd/omni

run:
	go run ./cmd/core

fmt:
	gofmt -w ./cmd ./internal
