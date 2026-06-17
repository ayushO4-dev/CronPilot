BIN     := bin/cronpilotd
PKG     := ./cmd/cronpilotd
GOFLAGS ?=

.PHONY: all build build-linux build-linux-amd64 build-linux-arm64 \
        run dev web web-install test tidy lint fmt clean

## all: build the frontend and the binary.
all: web build

## build: compile the native binary (embeds web/dist).
build:
	go build $(GOFLAGS) -o $(BIN) $(PKG)

## build-linux: build Linux binaries for all supported architectures.
build-linux: build-linux-amd64 build-linux-arm64

## build-linux-amd64: cross-compile a cgo-free Linux amd64 binary.
build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build $(GOFLAGS) -o bin/cronpilotd-linux-amd64 $(PKG)

## build-linux-arm64: cross-compile a cgo-free Linux arm64 binary.
build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
		go build $(GOFLAGS) -o bin/cronpilotd-linux-arm64 $(PKG)

## run: build then run in dev mode.
run: build
	./$(BIN) -dev

## dev: run the backend from source in dev mode.
dev:
	go run $(PKG) -dev

## web-install: install frontend dependencies.
web-install:
	cd web && npm install

## web: build the frontend into web/dist.
web:
	cd web && npm run build

## test: run Go unit tests.
test:
	go test ./cmd/... ./internal/...

## tidy: resolve and pin module dependencies.
tidy:
	go mod tidy

## lint: run linters.
lint:
	@command -v golangci-lint >/dev/null 2>&1 && \
		golangci-lint run ./cmd/... ./internal/... || \
		echo "golangci-lint not installed; skipping Go lint"
	cd web && npm run lint

## fmt: format Go sources.
fmt:
	gofmt -w .

## clean: remove build artifacts.
clean:
	rm -rf bin
	find web/dist -mindepth 1 ! -name .gitkeep -delete
