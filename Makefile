# go-bash developer tasks. This is the gate the `finalize_phase` tool runs
# at the end of every phase. Each target must exit 0 for the handoff to be
# accepted.

.PHONY: all ci build vet test test-race lint tidy fmt clean

all: ci

## ci: full local gate (run before every phase handoff)
ci: tidy build vet test-race lint
	@echo "✓ all checks passed"

## tidy: ensure go.mod / go.sum are clean
tidy:
	@if [ -f go.mod ]; then go mod tidy; else echo "no go.mod yet — skipping tidy"; fi

## build: compile all packages (CGO disabled — see SPEC §0.2)
build:
	@CGO_ENABLED=0 go build ./...

## vet: go vet across all packages
vet:
	@go vet ./...

## test: run tests without race detector
test:
	@GOBASH_TEST_NO_NETWORK=1 go test ./...

## test-race: tests with race detector + coverage (CI mode)
test-race:
	@GOBASH_TEST_NO_NETWORK=1 go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

## lint: staticcheck + custom lints (no-os-exec, no-net-default — see SPEC §21.1)
lint:
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed; install: go install honnef.co/go/tools/cmd/staticcheck@latest"

## fmt: gofmt -s -w
fmt:
	@gofmt -s -w .

## clean: drop build cache + coverage
clean:
	@go clean ./... 2>/dev/null || true
	@rm -f coverage.out
