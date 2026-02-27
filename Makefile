.PHONY: build build-server run-server restart kill proto test cover vet lint clean

# ── Build ─────────────────────────────────────────────────────────────────────

## Verify the module compiles cleanly.
build:
	go build ./...

## Build the service binary to bin/codevaldwork-server.
build-server:
	go build -o bin/codevaldwork-server ./cmd/server

## Build and run the service.
## Override config with env vars or a .env file.
run-server: build-server
	@if [ -f .env ]; then \
		set -a && . ./.env && set +a; \
	fi; \
	CODEVALDWORK_PORT=$(or $(CODEVALDWORK_PORT),50054) \
	./bin/codevaldwork-server

## Stop any running instance, rebuild, and run.
restart: kill build-server
	@echo "Running codevaldwork-server..."
	@if [ -f .env ]; then \
		set -a && . ./.env && set +a; \
	fi; \
	./bin/codevaldwork-server

## Stop any running instances of codevaldwork.
kill:
	@echo "Stopping any running instances..."
	-@pkill -9 -f "bin/codevaldwork" 2>/dev/null || true
	@sleep 1

# ── Proto Codegen ─────────────────────────────────────────────────────────────

## Regenerate Go stubs from proto/codevaldwork/v1/*.proto.
## Requires: buf, protoc-gen-go, protoc-gen-go-grpc on PATH.
## Install: go install github.com/bufbuild/buf/cmd/buf@latest
##          go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
##          go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
proto:
	buf generate

# ── Tests ─────────────────────────────────────────────────────────────────────

## Run all unit tests with race detector.
test:
	go test -v -race -count=1 ./...

## Run ArangoDB integration tests.
## Loads .env if it exists, otherwise falls back to environment variables.
test-arango:
	@if [ -f .env ]; then \
		set -a && . ./.env && set +a; \
	fi; \
	go test -v -race -count=1 ./storage/arangodb/

## Run tests and produce an HTML coverage report (coverage.html).
cover:
	go test -v -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# ── Quality ───────────────────────────────────────────────────────────────────

vet:
	go vet ./...

lint:
	golangci-lint run ./...

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	go clean ./...
	rm -f coverage.out coverage.html
	rm -rf bin/
