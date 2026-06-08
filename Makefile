# CronPilot — build & dev helpers.
#
# The Go targets must run on Linux (WSL2 with systemd, or a VM) because the
# daemon uses systemd, /proc and PTYs. The frontend (web/) needs Node; if your
# WSL/Linux lacks Node, build the frontend on the host and run the Go targets in
# WSL — web/dist is shared via the filesystem.

BIN     := bin/cronpilotd
PKG     := ./cmd/cronpilotd
GOFLAGS ?=

.PHONY: all build build-linux run dev web web-install test tidy lint fmt clean

## all: build the frontend and the binary.
all: web build

## build: compile the single binary (embeds web/dist — build the frontend first).
build:
	go build $(GOFLAGS) -o $(BIN) $(PKG)

## build-linux: cross-compile a cgo-free Linux amd64 binary.
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -o bin/cronpilotd-linux-amd64 $(PKG)

## run: build then run in dev mode.
run: build
	./$(BIN) -dev

## dev: run the backend from source in dev mode (Vite serves the UI separately).
dev:
	go run $(PKG) -dev

## web-install: install frontend dependencies (requires Node).
web-install:
	cd web && npm install

## web: build the frontend into web/dist (requires Node).
web:
	cd web && npm run build

## test: run Go unit tests (scoped to avoid walking web/node_modules).
test:
	go test ./cmd/... ./internal/...

## tidy: resolve and pin module dependencies.
tidy:
	go mod tidy

## lint: run golangci-lint (if present) and the frontend linter.
lint:
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./cmd/... ./internal/... || echo "golangci-lint not installed; skipping Go lint"
	cd web && npm run lint

## fmt: format Go sources.
fmt:
	gofmt -w .

## clean: remove build artifacts (keeps the web/dist .gitkeep).
clean:
	rm -rf bin
	find web/dist -mindepth 1 ! -name .gitkeep -delete
