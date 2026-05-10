.PHONY: tidy build core cli run fmt

tidy:
	go mod tidy

build: core cli

core:
	./scripts/build-core.sh

cli:
	go build -o bin/agent-cli ./cmd/cli

run:
	go run ./cmd/core

fmt:
	gofmt -w ./cmd ./internal
