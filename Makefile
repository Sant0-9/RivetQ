.PHONY: all build test bench proto lint clean dev install-tools

# Build variables
BINARY_SERVER := rivetqd
BINARY_CLI := rivetqctl
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

all: build

# Install development tools
install-tools:
	@echo "Installing development tools..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Build binaries
build:
	@echo "Building binaries..."
	go build $(LDFLAGS) -o $(BINARY_SERVER) ./cmd/rivetqd
	go build $(LDFLAGS) -o $(BINARY_CLI) ./cmd/rivetqctl

# Generate protobuf
proto:
	@echo "Generating protobuf files..."
	mkdir -p api/gen
	protoc --go_out=api/gen --go_opt=paths=source_relative \
		--go-grpc_out=api/gen --go-grpc_opt=paths=source_relative \
		api/queue.proto

# Run tests
test:
	@echo "Running tests..."
	go test ./... -v -race -cover

# Run tests with coverage
test-cover:
	@echo "Running tests with coverage..."
	go test ./... -v -race -coverprofile=coverage.txt -covermode=atomic
	go tool cover -html=coverage.txt -o coverage.html

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	go test ./... -bench=. -benchmem -run=^$

# Lint
lint:
	@echo "Running linter..."
	golangci-lint run ./...

# Run development server
dev: build
	@echo "Starting development server..."
	./$(BINARY_SERVER) --data-dir=./data --http-addr=:8080 --grpc-addr=:9090 --log-level=debug

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_SERVER) $(BINARY_CLI)
	rm -rf data/
	rm -f coverage.txt coverage.html

# Docker build
docker-build:
	docker build -f docker/Dockerfile -t rivetq:latest .

# Docker compose up
docker-up:
	docker compose -f docker/docker-compose.yml up --build

# Docker compose down
docker-down:
	docker compose -f docker/docker-compose.yml down -v
