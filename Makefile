# QTunnel Makefile

.PHONY: all build test test-race test-cover test-fuzz test-bench test-integration lint vet fmt clean install generate certs help

# Variables
BINARY_NAME = qtunnel
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME = $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS = -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)"

# Default target
all: build

# Build all binaries
build: build-client build-server build-diagnose

build-client:
	go build $(LDFLAGS) -o bin/qtunnel-client ./cmd/qtunnel-client

build-server:
	go build $(LDFLAGS) -o bin/qtunnel-server ./cmd/qtunnel-server

build-diagnose:
	go build $(LDFLAGS) -o bin/qtunnel-diagnose ./cmd/qtunnel-diagnose

# Install to GOPATH/bin
install:
	go install $(LDFLAGS) ./cmd/qtunnel-client
	go install $(LDFLAGS) ./cmd/qtunnel-server
	go install $(LDFLAGS) ./cmd/qtunnel-diagnose

# Run tests
test:
	go test -v ./...

test-race:
	go test -race ./...

test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-fuzz:
	@echo "Running fuzz tests (30s each)..."
	go test -fuzz=FuzzDecode -fuzztime=30s ./internal/frame 2>&1 | head -50
	go test -fuzz=FuzzConfig -fuzztime=30s ./internal/config 2>&1 | head -50

test-bench:
	go test -bench=. -benchmem -benchtime=3s ./internal/...

test-integration:
	go test -tags=integration -v ./internal/integration/...

# Linting and formatting
lint:
	golangci-lint run ./...

vet:
	go vet ./...

fmt:
	@gofmt -l . | grep -v vendor | tee /dev/stderr | test -z

fmt-fix:
	gofmt -w .

# Generate code (if needed)
generate:
	go generate ./...

# Generate test certificates
certs:
	@mkdir -p testdata/certs
	@openssl req -x509 -newkey rsa:2048 -keyout testdata/certs/server.key -out testdata/certs/server.crt -days 365 -nodes -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
	@cp testdata/certs/server.crt testdata/certs/ca.crt
	@echo "Generated test certificates in testdata/certs/"

# Generate secret token
secret:
	@go run ./cmd/qtunnel-diagnose secret

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html
	go clean -cache -testcache -modcache

# Tidy dependencies
tidy:
	go mod tidy
	go mod verify

# Download dependencies
deps:
	go mod download

# Verify dependencies
verify:
	go mod verify

# Run server (for testing)
run-server: build-server certs
	./bin/qtunnel-server -c configs/server.example.yaml

# Run client (for testing)
run-client: build-client certs
	./bin/qtunnel-client -c configs/client.example.yaml

# Help
help:
	@echo "QTunnel Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build           - Build all binaries"
	@echo "  build-client    - Build client binary"
	@echo "  build-server    - Build server binary"
	@echo "  build-diagnose  - Build diagnose binary"
	@echo "  install         - Install to GOPATH/bin"
	@echo "  test            - Run unit tests"
	@echo "  test-race       - Run tests with race detector"
	@echo "  test-cover      - Run tests with coverage"
	@echo "  test-fuzz       - Run fuzz tests (30s each)"
	@echo "  test-bench      - Run benchmarks"
	@echo "  test-integration - Run integration tests"
	@echo "  lint            - Run golangci-lint"
	@echo "  vet             - Run go vet"
	@echo "  fmt             - Check formatting"
	@echo "  fmt-fix         - Fix formatting"
	@echo "  generate        - Run go generate"
	@echo "  certs           - Generate test certificates"
	@echo "  secret          - Generate secret token"
	@echo "  clean           - Clean build artifacts"
	@echo "  tidy            - Tidy and verify dependencies"
	@echo "  deps            - Download dependencies"
	@echo "  run-server      - Run server with example config"
	@echo "  run-client      - Run client with example config"
	@echo "  help            - Show this help"