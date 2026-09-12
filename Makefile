.PHONY: tidy build omnidex omni fmt ui ui-dev

tidy:
	go mod tidy

ui:
	./scripts/build-ui.sh

ui-dev:
	cd internal/api/web && npm install && npm run dev

build: omnidex omni

omnidex:
	./scripts/build-core.sh

omni:
	./scripts/build-core.sh --package ./cmd/omni

fmt:
	gofmt -w ./cmd ./internal
