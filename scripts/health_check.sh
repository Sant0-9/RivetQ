#!/bin/bash
# Health check script for RivetQ

set -e

API_URL="${RIVETQ_URL:-http://localhost:8080}"
GRPC_URL="${RIVETQ_GRPC:-localhost:9090}"

echo "RivetQ Health Check"
echo "====================="
echo ""

# Check HTTP server
echo -n "HTTP Server... "
if curl -sf "${API_URL}/healthz" > /dev/null; then
    echo "OK"
else
    echo "FAILED"
    exit 1
fi

# Check metrics endpoint
echo -n "Metrics endpoint... "
if curl -sf "${API_URL}/metrics" | grep -q "rivetq"; then
    echo "OK"
else
    echo "FAILED"
    exit 1
fi

# Try to enqueue a test job
echo -n "Enqueue operation... "
RESPONSE=$(curl -sf -X POST "${API_URL}/v1/queues/_health_check/enqueue" \
    -H 'Content-Type: application/json' \
    -d '{"payload":{"test":true},"priority":5}')

if echo "$RESPONSE" | grep -q "job_id"; then
    JOB_ID=$(echo "$RESPONSE" | grep -o '"job_id":"[^"]*"' | cut -d'"' -f4)
    echo "OK (job_id: $JOB_ID)"
else
    echo "FAILED"
    exit 1
fi

# Try to lease the job
echo -n "Lease operation... "
LEASE_RESPONSE=$(curl -sf -X POST "${API_URL}/v1/queues/_health_check/lease" \
    -H 'Content-Type: application/json' \
    -d '{"max_jobs":1,"visibility_ms":5000}')

if echo "$LEASE_RESPONSE" | grep -q "jobs"; then
    echo "OK"
    
    # Extract lease ID and ack the job
    LEASE_ID=$(echo "$LEASE_RESPONSE" | grep -o '"lease_id":"[^"]*"' | cut -d'"' -f4)
    
    if [ -n "$LEASE_ID" ]; then
        echo -n "Ack operation... "
        ACK_RESPONSE=$(curl -sf -X POST "${API_URL}/v1/ack" \
            -H 'Content-Type: application/json' \
            -d "{\"job_id\":\"$JOB_ID\",\"lease_id\":\"$LEASE_ID\"}")
        
        if echo "$ACK_RESPONSE" | grep -q "success"; then
            echo "OK"
        else
            echo "WARNING (ack failed)"
        fi
    fi
else
    echo "FAILED"
    exit 1
fi

# Get queue stats
echo -n "Stats endpoint... "
STATS=$(curl -sf "${API_URL}/v1/queues/_health_check/stats")
if echo "$STATS" | grep -q "ready"; then
    echo "OK"
else
    echo "FAILED"
    exit 1
fi

echo ""
echo "All health checks passed!"
echo ""
echo "System Info:"
echo "  API URL: $API_URL"
echo "  gRPC URL: $GRPC_URL"
echo ""

# Show queue stats
echo "Queue Stats:"
curl -sf "${API_URL}/v1/queues/" | grep -o '"queues":\[[^]]*\]' || echo "  No queues"
echo ""
