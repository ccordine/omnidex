.PHONY: tidy build core cli omni run fmt ui ui-dev

tidy:
	go mod tidy

ui:
	./scripts/build-ui.sh

ui-dev:
	cd internal/api/web && npm install && npm run dev

build: core cli omni

core:
	./scripts/build-core.sh

cli:
	./scripts/build-core.sh --package ./cmd/cli --output bin/agent-cli

omni:
	./scripts/build-core.sh --package ./cmd/omni --output bin/omni

run: core
	./bin/agent-core

fmt:
	gofmt -w ./cmd ./internal
