# RivetQ Python Client

Python client library for RivetQ task queue.

## Installation

```bash
pip install rivetq
```

Or from source:

```bash
cd clients/python
pip install -e .
```

## Usage

```python
from rivetq import Client

# Create client
client = Client("http://localhost:8080")

# Enqueue a job
job_id = client.enqueue(
    "emails",
    {"to": "user@example.com", "subject": "Hello"},
    priority=7,
    max_retries=3
)
print(f"Enqueued: {job_id}")

# Lease and process jobs
jobs = client.lease("emails", max_jobs=5, visibility_ms=30000)

for job in jobs:
    print(f"Processing {job['id']}: {job['payload']}")
    
    try:
        # Do work...
        process_email(job['payload'])
        
        # Acknowledge success
        client.ack(job['id'], job['lease_id'])
    except Exception as e:
        # Nack on failure (will retry with backoff)
        client.nack(job['id'], job['lease_id'], str(e))

# Get queue stats
stats = client.stats("emails")
print(f"Ready: {stats['ready']}, Inflight: {stats['inflight']}, DLQ: {stats['dlq']}")
```

## API Reference

### Client(base_url, timeout=30)

Create a new RivetQ client.

### enqueue(queue, payload, priority=5, delay_ms=0, max_retries=3, idempotency_key=None, headers=None)

Enqueue a job. Returns job ID.

### lease(queue, max_jobs=1, visibility_ms=30000)

Lease jobs from queue. Returns list of jobs.

### ack(job_id, lease_id)

Acknowledge job completion.

### nack(job_id, lease_id, reason="")

Negatively acknowledge job (requeue with backoff).

### stats(queue)

Get queue statistics.

### list_queues()

List all queues.

### set_rate_limit(queue, capacity, refill_rate)

Set rate limit for queue.

### get_rate_limit(queue)

Get rate limit for queue.
