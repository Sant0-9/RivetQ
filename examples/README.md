# RivetQ Examples

This directory contains example applications demonstrating RivetQ usage.

## Producer Example

Enqueues jobs to a queue with varying priorities.

```bash
cd examples/producer
go run main.go
```

## Consumer Example

Continuously leases and processes jobs from a queue.

```bash
cd examples/consumer
go run main.go
```

## Running Both

In separate terminals:

```bash
# Terminal 1: Start RivetQ server
make dev

# Terminal 2: Start consumer
cd examples/consumer && go run main.go

# Terminal 3: Start producer
cd examples/producer && go run main.go
```

Then visit http://localhost:3000 (if UI is running) to see the jobs being processed in real-time!

## Python Examples

See `clients/python/README.md` for Python client usage examples.
