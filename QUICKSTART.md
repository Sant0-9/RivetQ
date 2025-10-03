# RivetQ Quick Start Guide

Get RivetQ up and running in 5 minutes!

## Prerequisites

- Go 1.22+
- Docker & Docker Compose (optional)
- curl (for testing)

## Option 1: Docker Compose (Easiest)

```bash
# Start everything (server + UI)
docker compose -f docker/docker-compose.yml up --build

# Server: http://localhost:8080
# Admin UI: http://localhost:3000
# Metrics: http://localhost:8080/metrics
```

That's it! Skip to [Testing](#testing).

## Option 2: Build from Source

### Step 1: Install Dependencies

```bash
# Download Go dependencies
go mod download

# Install development tools (optional)
make install-tools
```

### Step 2: Generate Protocol Buffers (Optional)

If you want to modify the gRPC API:

```bash
# Install protoc first: https://grpc.io/docs/protoc-installation/
make proto
```

Otherwise, you can skip this step and use REST API only.

### Step 3: Build

```bash
make build
```

This creates:
- `rivetqd` - server daemon
- `rivetqctl` - CLI client

### Step 4: Run Server

```bash
# Quick start
./rivetqd

# Or with custom config
./rivetqd --data-dir=./mydata --http-addr=:8080 --grpc-addr=:9090 --log-level=debug

# Or use the dev script
chmod +x scripts/dev_up.sh
./scripts/dev_up.sh
```

The server is now running:
- REST API: http://localhost:8080
- gRPC API: localhost:9090
- Health: http://localhost:8080/healthz
- Metrics: http://localhost:8080/metrics
- Pprof: http://localhost:8080/debug/pprof

### Step 5: Run UI (Optional)

```bash
cd ui
npm install
npm run dev
```

Admin UI: http://localhost:3000

## Testing

### Using curl

```bash
# 1. Enqueue a job
curl -X POST http://localhost:8080/v1/queues/emails/enqueue \
  -H 'Content-Type: application/json' \
  -d '{
    "payload": {"to": "user@example.com", "subject": "Welcome!"},
    "priority": 7,
    "max_retries": 3
  }'

# Response: {"job_id":"550e8400-e29b-41d4-a716-446655440000"}

# 2. Lease a job
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
#     "payload": {"to":"user@example.com","subject":"Welcome!"},
#     "priority": 7,
#     "tries": 0,
#     "lease_id": "abc123..."
#   }]
# }

# 3. Acknowledge completion
curl -X POST http://localhost:8080/v1/ack \
  -H 'Content-Type: application/json' \
  -d '{
    "job_id": "550e8400-e29b-41d4-a716-446655440000",
    "lease_id": "abc123..."
  }'

# Response: {"success":true}

# 4. Get queue stats
curl http://localhost:8080/v1/queues/emails/stats

# Response: {"ready":0,"inflight":0,"dlq":0}
```

### Using CLI

```bash
# Enqueue
./rivetqctl enqueue -q emails -p '{"to":"user@example.com"}' --priority 7

# Lease
./rivetqctl lease -q emails --max-jobs 5

# Stats
./rivetqctl stats -q emails

# List queues
./rivetqctl list
```

### Using Go SDK

```go
package main

import (
    "context"
    "fmt"
    rivetq "github.com/rivetq/rivetq/clients/go"
)

func main() {
    client := rivetq.NewClient("http://localhost:8080")

    // Enqueue
    jobID, _ := client.Enqueue(context.Background(), "emails", 
        map[string]string{"to": "user@example.com"}, 
        &rivetq.EnqueueOptions{Priority: 7})
    
    fmt.Println("Job ID:", jobID)

    // Lease
    jobs, _ := client.Lease(context.Background(), "emails", 1, 30000)
    
    // Process and ack
    for _, job := range jobs {
        fmt.Println("Processing:", job.ID)
        // ... do work ...
        client.Ack(context.Background(), job.ID, job.LeaseID)
    }
}
```

### Using Python SDK

```bash
cd clients/python
pip install -e .
```

```python
from rivetq import Client

client = Client("http://localhost:8080")

# Enqueue
job_id = client.enqueue("emails", {"to": "user@example.com"}, priority=7)
print(f"Job ID: {job_id}")

# Lease
jobs = client.lease("emails", max_jobs=5, visibility_ms=30000)

# Process and ack
for job in jobs:
    print(f"Processing {job['id']}")
    # ... do work ...
    client.ack(job['id'], job['lease_id'])
```

## Example Applications

Run the included producer/consumer examples:

```bash
# Terminal 1: Start server
make dev

# Terminal 2: Start consumer
cd examples/consumer && go run main.go

# Terminal 3: Start producer
cd examples/producer && go run main.go
```

Watch the jobs flow through the system!

## Advanced Features

### Delayed Jobs

```bash
curl -X POST http://localhost:8080/v1/queues/scheduled/enqueue \
  -H 'Content-Type: application/json' \
  -d '{
    "payload": {"task": "send_reminder"},
    "delay_ms": 300000
  }'
# Job will be available in 5 minutes
```

### Rate Limiting

```bash
# Set limit: 100 jobs capacity, 10 jobs/sec refill
curl -X POST http://localhost:8080/v1/queues/api-calls/rate_limit \
  -H 'Content-Type: application/json' \
  -d '{
    "capacity": 100,
    "refill_rate": 10
  }'
```

### Idempotency

```bash
curl -X POST http://localhost:8080/v1/queues/orders/enqueue \
  -H 'Content-Type: application/json' \
  -d '{
    "payload": {"order_id": "12345"},
    "idempotency_key": "order-12345-v1"
  }'
# Duplicate requests with same key return same job_id
```

## Monitoring

### Prometheus Metrics

```bash
curl http://localhost:8080/metrics
```

Key metrics:
- `rivetq_jobs_enqueued_total{queue="emails"}`
- `rivetq_jobs_ready{queue="emails"}`
- `rivetq_jobs_inflight{queue="emails"}`
- `rivetq_jobs_dlq{queue="emails"}`

### Health Check

```bash
curl http://localhost:8080/healthz
# Response: OK
```

### Profiling

```bash
# CPU profile
curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof

# Heap profile
curl http://localhost:8080/debug/pprof/heap > heap.prof
go tool pprof heap.prof

# Goroutines
curl http://localhost:8080/debug/pprof/goroutine
```

## Load Testing

```bash
# Install k6: https://k6.io/docs/getting-started/installation/

# Run load test
k6 run scripts/k6_load.js

# Custom test
k6 run --vus 50 --duration 2m scripts/k6_load.js
```

## Configuration

Create `config.yaml`:

```yaml
server:
  http_addr: ":8080"
  grpc_addr: ":9090"

storage:
  data_dir: "./data"

wal:
  segment_size: 67108864  # 64MB
  fsync: true  # false for faster, less durable

logging:
  level: info
  format: console
```

Run with config:

```bash
./rivetqd --config=config.yaml
```

## Running Tests

```bash
# All tests
make test

# With coverage
make test-cover

# Benchmarks
make bench

# Race detector
go test ./... -race
```

## Troubleshooting

### "Failed to open pebble db"
- Check data directory permissions
- Delete data directory if corrupted: `rm -rf data/`

### "Port already in use"
- Change ports: `--http-addr=:8081 --grpc-addr=:9091`
- Kill existing process: `pkill rivetqd`

### High memory usage
- Check WAL segment count: `ls -lh data/wal/`
- WAL will compact automatically
- Reduce segment size in config

### Jobs stuck in DLQ
- Check application logs for errors
- Inspect DLQ via UI or API
- Manually requeue if needed (future feature)

## Next Steps

- Read [ARCHITECTURE.md](ARCHITECTURE.md) for deep dive
- Check [examples/](examples/) for more examples
- See [CONTRIBUTING.md](CONTRIBUTING.md) to contribute
- Star the repo if you like it! ⭐

## Getting Help

- GitHub Issues: Report bugs or request features
- Discussions: Ask questions, share ideas
- Documentation: README.md, ARCHITECTURE.md

Happy queuing! 
