# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial release of RivetQ
- Core queue operations (enqueue, lease, ack, nack)
- Delayed job scheduling with ETA support
- Priority queues (0-9 priority levels)
- Configurable retry policies with exponential backoff
- Dead letter queue (DLQ) for failed jobs
- Visibility timeout with automatic lease expiration
- Rate limiting per queue (token bucket algorithm)
- Idempotency keys for duplicate prevention
- Write-ahead log (WAL) for durability
- Segmented log files with CRC32 checksums
- Pebble KV store for indexes
- WAL replay on startup
- REST API with JSON
- gRPC API with protocol buffers
- Prometheus metrics
- Structured logging (zerolog)
- pprof profiling endpoints
- Next.js admin UI
- Go SDK client
- Python SDK client
- CLI tool (rivetqctl)
- Docker support
- Docker Compose setup
- GitHub Actions CI/CD
- k6 load testing scripts
- Comprehensive test suite
- API documentation

### Technical Details
- Heap-based priority queue implementation
- Concurrent queue sharding support
- Background lease timeout worker
- Graceful shutdown handling
- CORS support for REST API
- Health check endpoint

## [0.1.0] - 2025-10-03

### Added
- Initial project setup
- Basic architecture and design

[Unreleased]: https://github.com/rivetq/rivetq/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/rivetq/rivetq/releases/tag/v0.1.0
