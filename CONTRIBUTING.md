# Contributing to RivetQ

Thank you for your interest in contributing to RivetQ! This document provides guidelines and instructions for contributing.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/rivetq.git`
3. Create a feature branch: `git checkout -b feature/my-feature`
4. Make your changes
5. Run tests: `make test`
6. Run linter: `make lint`
7. Commit your changes: `git commit -am 'Add new feature'`
8. Push to your fork: `git push origin feature/my-feature`
9. Create a Pull Request

## Development Setup

### Prerequisites

- Go 1.22 or later
- Node.js 20 or later (for UI development)
- Docker and Docker Compose (optional, for integration tests)
- Protocol Buffers compiler (`protoc`)
- k6 (for load testing)

### Install Development Tools

```bash
make install-tools
```

This will install:
- protoc-gen-go
- protoc-gen-go-grpc
- golangci-lint

### Building

```bash
# Build binaries
make build

# Generate protobuf code
make proto

# Build Docker image
make docker-build
```

### Running Tests

```bash
# Run all tests with race detection
make test

# Run tests with coverage
make test-cover

# Run benchmarks
make bench

# Run load tests
k6 run scripts/k6_load.js
```

### Running Locally

```bash
# Start development server
make dev

# Or use Docker Compose
docker compose -f docker/docker-compose.yml up --build
```

## Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Run `golangci-lint` before committing
- Write tests for new features
- Add comments for exported functions

## Testing Guidelines

- Write unit tests for all new functionality
- Ensure race detector passes: `go test -race`
- Aim for >80% code coverage
- Include integration tests for API changes
- Test edge cases and error conditions

## Commit Messages

Follow conventional commit format:

```
feat: add new queue priority algorithm
fix: resolve memory leak in WAL compaction
docs: update API documentation
test: add integration tests for rate limiting
chore: update dependencies
```

## Pull Request Process

1. Update documentation for any API changes
2. Add tests for new features
3. Ensure all tests pass
4. Update CHANGELOG.md
5. Request review from maintainers

## Areas for Contribution

- **Core Features**: Queue operations, storage, WAL
- **Performance**: Optimizations, benchmarks
- **Testing**: More test coverage, integration tests
- **Documentation**: Examples, tutorials, API docs
- **Client Libraries**: Additional language SDKs
- **Admin UI**: New features, improvements
- **Monitoring**: Dashboards, alerting

## Questions?

- Open an issue for bugs or feature requests
- Join discussions for design questions
- Check existing issues before creating new ones

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
