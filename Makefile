.PHONY: tidy build omnidex omni fmt ui ui-dev

tidy:
	go mod tidy

ui:
	./scripts/build-ui.sh

ui-dev:
	cd internal/api/web && npm install && npm run dev

build: omnidex omni

omnidex:
	go build -trimpath -o bin/omnidex ./cmd/omnidex

omni:
	go build -trimpath -o bin/omni ./cmd/omni

fmt:
	gofmt -w ./cmd ./internal
