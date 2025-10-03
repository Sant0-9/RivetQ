# RivetQ Architecture

This document provides a detailed overview of RivetQ's architecture and design decisions.

## Overview

RivetQ is a production-grade, single-node durable task queue system built in Go. It provides reliable job processing with features like delayed execution, priorities, retries, and dead-letter queues.

## Core Components

### 1. Queue Manager (`internal/queue/`)

The Queue Manager is the heart of RivetQ, coordinating all queue operations.

**Responsibilities:**
- Manage multiple named queues
- Handle enqueue/lease/ack/nack operations
- Track job states (ready, inflight, DLQ)
- Coordinate with WAL and storage
- Run background workers (lease timeout checker)

**Key Design Decisions:**
- Each queue uses a min-heap for priority ordering
- Jobs ordered by: priority (DESC) → ETA (ASC) → enqueue time (ASC)
- Inflight jobs tracked in a map with lease deadlines
- Background worker checks for expired leases every second

### 2. Write-Ahead Log (`internal/wal/`)

The WAL provides durability through append-only log files.

**Features:**
- Segmented log files (default 64MB per segment)
- CRC32C checksums for data integrity
- Record types: Enqueue, Ack, Nack, Requeue, Tombstone
- Optional fsync for guaranteed durability
- Automatic segment rotation
- Replay on startup

**Record Format:**
```
[length:4][crc32:4][record_data...]
```

**Record Data:**
```
[type:1][queue_len:2][queue][job_id_len:2][job_id]
[priority:1][tries:4][max_retries:4][eta:8]
[payload_len:4][payload][headers_count:2][headers...]
[lease_id_len:2][lease_id][reason_len:2][reason]
```

### 3. Storage Layer (`internal/store/`)

Pebble KV store for indexes and metadata.

**Stored Data:**
- Job metadata (indexed by job ID)
- Idempotency keys → job IDs
- Future: Queue configurations, statistics

**Why Pebble?**
- LSM-tree based (efficient for write-heavy workloads)
- Built-in compression
- Stable and battle-tested (used in CockroachDB)
- Good Go integration

### 4. Priority Queue (`internal/queue/heap.go`)

Custom min-heap implementation using Go's container/heap.

**Ordering:**
1. Higher priority first (9 > 5 > 0)
2. Earlier ETA first (for delayed jobs)
3. Earlier enqueue time first (FIFO within priority)

**Operations:**
- Push: O(log n)
- Pop: O(log n)
- Peek: O(1)
- Remove by ID: O(log n)

### 5. Rate Limiting (`internal/ratelimit/`)

Token bucket algorithm for per-queue rate limiting.

**Algorithm:**
- Bucket has a capacity of tokens
- Tokens refill at a constant rate (tokens/second)
- Each operation consumes tokens
- Operations rejected when bucket is empty

**Implementation:**
- Lock-based for simplicity
- Monotonic time for refill calculation
- Per-queue buckets in a map

### 6. Retry & Backoff (`internal/backoff/`)

Exponential backoff with jitter for failed jobs.

**Formula:**
```
delay = min(base * multiplier^(tries-1), max_delay) + jitter
```

**Default Config:**
- Base delay: 100ms
- Multiplier: 2.0
- Max delay: 60s
- Jitter: ±10%

**Example Progression:**
- Try 1: 100ms (±10ms)
- Try 2: 200ms (±20ms)
- Try 3: 400ms (±40ms)
- Try 4: 800ms (±80ms)
- ...capped at 60s

### 7. API Layer

#### REST API (`internal/rest/`)
- Chi router for routing
- JSON request/response
- CORS enabled
- Middleware: logging, recovery, request ID

#### gRPC API (`internal/api/`)
- Protocol Buffers definitions in `api/queue.proto`
- gRPC reflection enabled
- Streaming support (future)

### 8. Observability

#### Metrics (`internal/metrics/`)
Prometheus metrics:
- Counters: enqueued, leased, acked, nacked
- Gauges: ready, inflight, dlq counts
- WAL metrics: segment count, size
- Rate limit rejections

#### Logging
- Structured logging with zerolog
- Configurable levels: debug, info, warn, error
- Console or JSON format
- Request IDs for tracing

#### Profiling
- pprof endpoints at `/debug/pprof`
- CPU, memory, goroutine, mutex profiling
- Always enabled (low overhead)

## Data Flow

### Enqueue Operation

```
Client → REST/gRPC Handler
  → Queue Manager
    → Check rate limit
    → Generate job ID
    → Check idempotency key (if provided)
    → Write to WAL (Enqueue record)
    → Store idempotency key (if provided)
    → Add to priority queue
  → Return job ID
```

### Lease Operation

```
Client → REST/gRPC Handler
  → Queue Manager
    → Pop ready jobs from heap (by priority & ETA)
    → Generate lease ID for each job
    → Set lease deadline (now + visibility timeout)
    → Move jobs to inflight map
    → Return jobs with lease IDs
```

### Ack Operation

```
Client → REST/gRPC Handler
  → Queue Manager
    → Verify job is inflight
    → Verify lease ID matches
    → Write to WAL (Ack record)
    → Remove from inflight map
  → Success
```

### Nack Operation

```
Client → REST/gRPC Handler
  → Queue Manager
    → Verify job is inflight
    → Verify lease ID matches
    → Increment tries
    → Calculate backoff delay
    → Write to WAL (Nack record)
    → If tries < max_retries:
        → Set new ETA (now + backoff)
        → Move back to ready queue
      Else:
        → Move to DLQ map
  → Success
```

### Lease Timeout (Background Worker)

```
Every 1 second:
  For each queue:
    For each inflight job:
      If lease_deadline < now:
        → Increment tries
        → Calculate backoff
        → Write to WAL (Requeue record)
        → If tries < max_retries:
            → Move back to ready queue
          Else:
            → Move to DLQ
```

### Startup Replay

```
On server start:
  Load WAL segments from disk
  For each segment:
    For each record:
      Switch on record type:
        Enqueue → Add job to ready queue
        Ack → Remove from inflight
        Nack/Requeue → Update tries, move to ready or DLQ
        Tombstone → Remove completely
```

## Durability Guarantees

### With Fsync Enabled (Default)
- All writes fsynced to disk before returning
- Survives crashes, power loss
- Trade-off: Higher latency (~1-5ms per operation)

### With Fsync Disabled
- Writes buffered in OS page cache
- Much faster (~0.1ms per operation)
- Risk: Data loss if OS crashes before flush
- Good for: Development, non-critical queues

## Performance Characteristics

### Enqueue
- **Complexity:** O(log n) - heap insertion
- **Latency:** 1-5ms (with fsync), <1ms (without)
- **Throughput:** ~1000-5000 ops/sec per queue

### Lease
- **Complexity:** O(k log n) - k jobs popped from heap
- **Latency:** 5-20ms
- **Throughput:** ~500-2000 ops/sec

### Ack/Nack
- **Complexity:** O(1) - map deletion
- **Latency:** 1-5ms (with fsync)
- **Throughput:** ~2000-10000 ops/sec

### Memory Usage
- Per job: ~200-500 bytes (depending on payload)
- Per queue: ~1KB overhead
- WAL: Buffered in memory, flushed periodically
- Pebble: Uses bloom filters, block cache

## Scalability Considerations

### Current (Single Node)
- Vertical scaling: More CPU, RAM, faster disk
- Sharding: Multiple queues on same node
- Good for: 100K-1M jobs/day

### Future (Multi-Node)
- Raft consensus for replication
- Sharding queues across nodes
- Distributed WAL (S3 snapshots)
- Good for: 10M+ jobs/day

## Failure Modes & Recovery

### Server Crash
- WAL replay on restart
- In-flight jobs requeued (with backoff)
- No data loss (with fsync)

### Disk Corruption
- CRC32 checksums detect corruption
- Skip corrupted records, continue replay
- Manual recovery may be needed

### Network Partition (Future: Multi-Node)
- Raft ensures consistency
- Minority partitions become read-only
- Automatic recovery on heal

## Security Considerations

### Current
- No authentication/authorization (single-node, trusted network)
- CORS enabled (configurable)
- No encryption at rest or in transit

### Future
- API key authentication
- TLS for REST/gRPC
- Encryption at rest
- Rate limiting per client

## Testing Strategy

### Unit Tests
- WAL read/write, CRC validation
- Priority queue ordering
- Backoff calculations
- Rate limiter behavior

### Integration Tests
- Full enqueue → lease → ack flow
- Retry and DLQ scenarios
- Delayed job execution
- Idempotency key handling
- WAL replay

### Load Tests (k6)
- Concurrent producers/consumers
- Measure p50/p95/p99 latencies
- Throughput under load
- Memory usage profiling

## Monitoring & Operations

### Key Metrics to Watch
- **Queue depth:** ready + inflight counts
- **DLQ size:** indicates systemic failures
- **Lease timeouts:** high rate = slow consumers
- **Rate limit rejections:** capacity issues

### Operational Tasks
- WAL compaction (automatic, configurable)
- Log rotation
- Metrics scraping (Prometheus)
- Backup (copy data directory)

## Future Enhancements

1. **Clustering:**
   - Raft for consensus
   - Leader election
   - Follower replication

2. **Advanced Features:**
   - Job dependencies (DAG)
   - Scheduled cron jobs
   - Batch operations
   - Message deduplication

3. **Performance:**
   - Zero-copy optimizations
   - Parallel WAL writes
   - Read replicas

4. **Observability:**
   - Distributed tracing
   - Grafana dashboards
   - Alert rules

## Conclusion

RivetQ is designed for production use with a focus on:
- **Durability:** WAL + checksums
- **Reliability:** Retries + DLQ
- **Performance:** Heap-based scheduling
- **Observability:** Metrics + logs + profiling
- **Simplicity:** Single binary, easy deployment

The architecture balances these concerns while remaining maintainable and extensible.
