# RivetQ

[![CI](https://github.com/rivetq/rivetq/actions/workflows/ci.yml/badge.svg)](https://github.com/rivetq/rivetq/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/rivetq/rivetq)](https://goreportcard.com/report/github.com/rivetq/rivetq)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**RivetQ** is a production-grade, single-node durable task queue with delayed jobs, priorities, retries, visibility timeouts, dead-letter queues, REST+gRPC APIs, Prometheus metrics, and a beautiful Next.js admin UI.

## Features

- **Durable Storage**: Write-ahead log (WAL) with segmented log files and Pebble KV for indexes
- **Delayed Jobs**: Schedule jobs to execute at a specific time
- **Priority Queues**: Jobs ordered by priority (0-9), ETA, and enqueue time
- **Retry Logic**: Configurable retry policies with exponential backoff and jitter
- **Visibility Timeout**: Lease-based job processing with automatic timeout handling
- **Dead Letter Queue**: Failed jobs moved to DLQ after max retries
- **Rate Limiting**: Token bucket rate limiting per queue
- **Idempotency**: Optional idempotency keys to prevent duplicate processing
- **REST + gRPC APIs**: Full-featured APIs with OpenAPI docs and gRPC reflection
- **Observability**: Prometheus metrics, structured logging (zerolog), pprof profiling
- **Admin UI**: Modern Next.js dashboard with real-time stats
- **Client SDKs**: Go and Python clients included

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         RivetQ Server                            │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────────────────┐│
│  │  REST API   │  │  gRPC API   │  │  Metrics (/metrics)      ││
│  │  (chi)      │  │  (protobuf) │  │  Pprof (/debug/pprof)    ││
│  └──────┬──────┘  └──────┬──────┘  └──────────────────────────┘│
│         │                │                                       │
│         └────────┬───────┘                                       │
│                  │                                               │
│         ┌────────▼────────────────────────────────┐             │
│         │      Queue Manager                      │             │
│         │  - Priority queues (min-heap)           │             │
│         │  - Inflight tracking                    │             │
│         │  - Dead letter queue                    │             │
│         │  - Rate limiting (token bucket)         │             │
│         │  - Lease timeout worker                 │             │
│         └────────┬────────────────────────────────┘             │
│                  │                                               │
│         ┌────────┴─────────┬─────────────────────┐             │
│         │                  │                     │              │
│    ┌────▼────┐      ┌──────▼──────┐      ┌──────▼──────┐      │
│    │   WAL   │      │    Store    │      │   Metrics   │      │
│    │ Segments│      │  (Pebble)   │      │ (Prometheus)│      │
│    │  + CRC  │      │  Indexes    │      └─────────────┘      │
│    └─────────┘      └─────────────┘                            │
└─────────────────────────────────────────────────────────────────┘
```

## Quick Start

### Using Docker Compose (Recommended)

```bash
# Start RivetQ server and UI
docker compose -f docker/docker-compose.yml up --build

# Server will be available at:
# - HTTP API:  http://localhost:8080
# - gRPC API:  localhost:9090
# - Admin UI:  http://localhost:3000
# - Metrics:   http://localhost:8080/metrics
```

### Building from Source

```bash
# Install dependencies
go mod download

# Build binaries
make build

# Run server
./rivetqd --data-dir=./data --http-addr=:8080 --grpc-addr=:9090

# Or use the dev script
chmod +x scripts/dev_up.sh
./scripts/dev_up.sh
```

## Usage Examples

### REST API

```bash
# Enqueue a job
curl -X POST http://localhost:8080/v1/queues/emails/enqueue \
  -H 'Content-Type: application/json' \
  -d '{
    "payload": {"to": "user@example.com", "subject": "Hello"},
    "priority": 7,
    "delay_ms": 0,
    "max_retries": 3
  }'

# Response: {"job_id": "550e8400-e29b-41d4-a716-446655440000"}

# Lease a job (with 30s visibility timeout)
curl -X POST http://localhost:8080/v1/queues/emails/lease \
  -H 'Content-Type: application/json' \
  -d '{
    "max_jobs": 1,
    "visibility_ms": 30000
  }'

# Response:
# {
#   "jobs": [{
#     "id": "550e8400-e29b-41d4-a716-446655440000",
#     "queue": "emails",
#     "payload": {"to": "user@example.com", "subject": "Hello"},
#     "priority": 7,
#     "tries": 0,
#     "lease_id": "lease-123"
#   }]
# }

# Acknowledge job completion
curl -X POST http://localhost:8080/v1/ack \
  -H 'Content-Type: application/json' \
  -d '{
    "job_id": "550e8400-e29b-41d4-a716-446655440000",
    "lease_id": "lease-123"
  }'

# Nack (requeue with backoff)
curl -X POST http://localhost:8080/v1/nack \
  -H 'Content-Type: application/json' \
  -d '{
    "job_id": "550e8400-e29b-41d4-a716-446655440000",
    "lease_id": "lease-123",
    "reason": "temporary error"
  }'

# Get queue stats
curl http://localhost:8080/v1/queues/emails/stats

# Set rate limit (100 capacity, 10 jobs/sec)
curl -X POST http://localhost:8080/v1/queues/emails/rate_limit \
  -H 'Content-Type: application/json' \
  -d '{
    "capacity": 100,
    "refill_rate": 10
  }'
```

### CLI

```bash
# Enqueue a job
./rivetqctl enqueue -q emails -p '{"to":"user@example.com"}' --priority 7

# Lease jobs
./rivetqctl lease -q emails --max-jobs 5 --visibility 30000

# Get stats
./rivetqctl stats -q emails

# List all queues
./rivetqctl list
```

### Go SDK

```go
package main

import (
    "context"
    "fmt"
    "log"

    rivetq "github.com/rivetq/rivetq/clients/go"
)

func main() {
    client := rivetq.NewClient("http://localhost:8080")

    // Enqueue a job
    jobID, err := client.Enqueue(context.Background(), "emails", map[string]interface{}{
        "to":      "user@example.com",
        "subject": "Hello from RivetQ",
    }, &rivetq.EnqueueOptions{
        Priority:   7,
        MaxRetries: 3,
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Enqueued job: %s\n", jobID)

    // Lease and process jobs
    jobs, err := client.Lease(context.Background(), "emails", 1, 30000)
    if err != nil {
        log.Fatal(err)
    }

    for _, job := range jobs {
        fmt.Printf("Processing job: %s\n", job.ID)
        
        // Do work...
        
        // Acknowledge completion
        if err := client.Ack(context.Background(), job.ID, job.LeaseID); err != nil {
            log.Printf("Failed to ack: %v", err)
        }
    }
}
```

## Testing

```bash
# Run all tests with race detection
make test

# Run tests with coverage
make test-cover

# Run benchmarks
make bench

# Run load tests with k6
k6 run scripts/k6_load.js
```

## Benchmarks

Example results from k6 load test (50 concurrent users):

```
Load Test Summary
==================================================

Enqueue Success Rate: 99.87%
Lease Success Rate: 99.92%
Ack Success Rate: 99.95%

Lease Latency:
  p50: 12.34ms
  p95: 45.67ms
  p99: 89.12ms
```

## Configuration

Create a `config.yaml`:

```yaml
server:
  http_addr: ":8080"
  grpc_addr: ":9090"

storage:
  data_dir: "./data"

wal:
  segment_size: 67108864  # 64MB
  fsync: true

queue:
  shards: 4
  lease_check_interval: 1s

logging:
  level: info
  format: console
```

Or use environment variables and flags:

```bash
./rivetqd \
  --data-dir=/var/lib/rivetq \
  --http-addr=:8080 \
  --grpc-addr=:9090 \
  --log-level=debug \
  --fsync=true
```

## Monitoring

RivetQ exposes Prometheus metrics at `/metrics`:

```
# Job metrics
rivetq_jobs_enqueued_total{queue="emails"}
rivetq_jobs_leased_total{queue="emails"}
rivetq_jobs_acked_total{queue="emails"}
rivetq_jobs_nacked_total{queue="emails"}

# Queue gauges
rivetq_jobs_ready{queue="emails"}
rivetq_jobs_inflight{queue="emails"}
rivetq_jobs_dlq{queue="emails"}

# WAL metrics
rivetq_wal_segments
rivetq_wal_size_bytes

# Rate limiting
rivetq_rate_limit_rejections_total{queue="emails"}
```

## Roadmap

- [x] Core queue operations (enqueue, lease, ack, nack)
- [x] Delayed jobs and priorities
- [x] Retry logic with exponential backoff
- [x] Dead letter queue
- [x] Rate limiting
- [x] WAL and durable storage
- [x] REST and gRPC APIs
- [x] Admin UI
- [x] Prometheus metrics
- [ ] **Phase 2**: Clustering with Raft consensus
- [ ] **Phase 2**: Multi-node sharding
- [ ] **Phase 2**: S3-compatible WAL snapshots
- [ ] **Phase 2**: Pluggable storage backends
- [ ] **Phase 2**: WebSocket API for real-time updates
- [ ] **Phase 2**: Advanced monitoring dashboards

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built with [Pebble](https://github.com/cockroachdb/pebble) for KV storage
- Inspired by AWS SQS, Google Cloud Tasks, and BullMQ
- UI powered by [Next.js](https://nextjs.org/) and [Tailwind CSS](https://tailwindcss.com/)

---

Built with Go
