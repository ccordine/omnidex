.PHONY: tidy build core cli omni run fmt

tidy:
	go mod tidy

build: core cli omni

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
