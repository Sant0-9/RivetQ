#!/bin/bash
set -e

echo "Starting RivetQ development environment..."

# Build binaries
echo "Building binaries..."
make build

# Start server in background
echo "Starting RivetQ server..."
./rivetqd --data-dir=./data --http-addr=:8080 --grpc-addr=:9090 --log-level=debug &
SERVER_PID=$!

# Wait for server to be ready
echo "Waiting for server to be ready..."
until curl -s http://localhost:8080/healthz > /dev/null; do
  sleep 1
done

echo "Server is ready!"
echo ""
echo "RivetQ is running:"
echo "  - HTTP API:  http://localhost:8080"
echo "  - gRPC API:  localhost:9090"
echo "  - Metrics:   http://localhost:8080/metrics"
echo "  - Pprof:     http://localhost:8080/debug/pprof"
echo ""
echo "Example commands:"
echo ""
echo "  # Enqueue a job"
echo "  curl -X POST http://localhost:8080/v1/queues/emails/enqueue \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"payload\":{\"to\":\"user@example.com\",\"subject\":\"Hello\"},\"priority\":7}'"
echo ""
echo "  # Lease a job"
echo "  curl -X POST http://localhost:8080/v1/queues/emails/lease \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"max_jobs\":1,\"visibility_ms\":30000}'"
echo ""
echo "  # Get stats"
echo "  curl http://localhost:8080/v1/queues/emails/stats"
echo ""
echo "Press Ctrl+C to stop the server"

# Wait for Ctrl+C
trap "echo ''; echo 'Stopping server...'; kill $SERVER_PID; exit 0" INT
wait $SERVER_PID
