# RivetQ Clustering Guide

This document describes RivetQ's clustering capabilities introduced in Phase 2.

## Overview

RivetQ clustering provides:
- **High Availability**: Multiple nodes with automatic failover
- **Horizontal Scalability**: Distribute queues across nodes
- **Consensus**: Raft-based distributed consensus
- **Consistent Hashing**: Automatic queue distribution
- **Replication**: Configurable replication factor

## Architecture

### Components

1. **Raft Consensus**
   - Leader election
   - Log replication
   - Snapshot management
   - Built on HashiCorp Raft

2. **Consistent Hashing**
   - SHA256-based hashing
   - Virtual nodes (150 per physical node)
   - Minimal disruption on rebalancing

3. **Membership**
   - Node discovery
   - Health checking
   - Status tracking (alive/suspect/dead)

4. **Request Forwarding**
   - Automatic routing to correct node
   - Transparent to clients
   - Broadcast support for admin operations

## Setup

### Single Node (Bootstrap)

Create `config.yaml`:

```yaml
cluster:
  enabled: true
  node_id: "node1"
  raft_addr: ":7000"
  bootstrap: true
  seed_nodes: []
  replication: 3
```

Start the node:

```bash
./rivetqd --config=config.yaml
```

### Adding Nodes

For node2, create `config.node2.yaml`:

```yaml
server:
  http_addr: ":8081"
  grpc_addr: ":9091"

storage:
  data_dir: "./data-node2"

cluster:
  enabled: true
  node_id: "node2"
  raft_addr: ":7001"
  bootstrap: false
  seed_nodes:
    - "localhost:8080"  # Point to node1
  replication: 3
```

Start node2:

```bash
./rivetqd --config=config.node2.yaml
```

Node2 will automatically:
1. Discover node1 via seed_nodes
2. Request to join the cluster
3. Sync state from the leader
4. Participate in Raft consensus

### Three-Node Cluster Example

```bash
# Terminal 1: Node 1 (bootstrap)
./rivetqd --data-dir=./data1 --http-addr=:8080 --grpc-addr=:9090 \
  --config=config.node1.yaml

# Terminal 2: Node 2
./rivetqd --data-dir=./data2 --http-addr=:8081 --grpc-addr=:9091 \
  --config=config.node2.yaml

# Terminal 3: Node 3
./rivetqd --data-dir=./data3 --http-addr=:8082 --grpc-addr=:9092 \
  --config=config.node3.yaml
```

## Queue Distribution

Queues are distributed using consistent hashing:

```
Queue "emails"   -> Hash -> Node2 (primary), Node3 (replica)
Queue "orders"   -> Hash -> Node1 (primary), Node2 (replica)
Queue "webhooks" -> Hash -> Node3 (primary), Node1 (replica)
```

### How It Works

1. **Enqueue**: Client sends to any node
2. **Hash**: Node computes hash(queue_name)
3. **Route**: Forwards to responsible node
4. **Replicate**: Raft replicates to followers
5. **Ack**: Returns success to client

## API Operations

### Check Cluster Status

```bash
curl http://localhost:8080/v1/cluster/info
```

Response:
```json
{
  "local_id": "node1",
  "leader": "127.0.0.1:7000",
  "member_count": 3,
  "members": [
    {
      "id": "node1",
      "addr": "localhost:8080",
      "raft_addr": ":7000",
      "status": "alive",
      "is_leader": true,
      "last_seen_unix": 1696800000
    },
    ...
  ]
}
```

### List Members

```bash
curl http://localhost:8080/v1/cluster/members
```

### Join Cluster (Manual)

```bash
curl -X POST http://localhost:8080/v1/cluster/join \
  -H 'Content-Type: application/json' \
  -d '{
    "node_id": "node4",
    "addr": "localhost:8083",
    "raft_addr": ":7003"
  }'
```

### Leave Cluster

```bash
curl -X POST http://localhost:8080/v1/cluster/leave \
  -H 'Content-Type: application/json' \
  -d '{"node_id": "node4"}'
```

### Sharding Info

```bash
curl http://localhost:8080/v1/cluster/sharding
```

## Metrics

New Prometheus metrics for clustering:

```
# Number of nodes in cluster
rivetq_cluster_nodes

# Is this node the leader (1 = yes, 0 = no)
rivetq_cluster_is_leader

# Raft log indexes
rivetq_raft_committed_index
rivetq_raft_applied_index

# Forwarded requests
rivetq_proxy_forwarded_total{target_node="node2"}
rivetq_proxy_forward_errors_total
```

## Failure Scenarios

### Leader Failure

1. Followers detect missed heartbeats
2. Election timeout triggers
3. New leader elected (majority vote)
4. Clients automatically discover new leader
5. Operations resume

**Recovery Time**: ~1-2 seconds

### Follower Failure

1. Leader marks follower as unavailable
2. Queues rebalance to remaining nodes
3. Clients unaffected (leader still operational)
4. Node rejoins when recovered

**Impact**: Minimal, queues redistributed

### Network Partition

- Minority partition: Read-only mode
- Majority partition: Continues operations
- Partition heals: Automatic reconciliation

### Split Brain Prevention

Raft guarantees single leader via majority consensus.
Cannot have two leaders simultaneously.

## Best Practices

### Node Count

- **Odd numbers**: 3, 5, 7 nodes
- **Minimum for HA**: 3 nodes
- **Recommended**: 3-5 nodes for most workloads
- **Maximum tested**: 7 nodes

### Replication Factor

- **Default**: 3 (can tolerate 1 failure)
- **High availability**: 5 (can tolerate 2 failures)
- **Lower latency**: 1 (no replication, single node)

### Network

- **Low latency**: < 10ms between nodes
- **Same datacenter**: Recommended for Raft
- **Cross-region**: Possible but adds latency

### Monitoring

Watch these metrics:
- `rivetq_cluster_nodes`: Should match expected count
- `rivetq_cluster_is_leader`: Exactly one node = 1
- `rivetq_raft_committed_index`: Should increase steadily
- `rivetq_proxy_forward_errors_total`: Should be low

### Capacity Planning

- Queues distributed by consistent hashing
- Adding nodes: Queues rebalance automatically
- Removing nodes: Queues move to remaining nodes
- Plan for ~20% overhead for rebalancing

## Troubleshooting

### Node Won't Join

Check:
- Seed nodes are reachable
- Raft ports are not firewalled
- Node ID is unique
- Leader is available

### Split Cluster

Symptoms: Multiple nodes think they're leader

Solution:
1. Stop all nodes
2. Delete Raft data: `rm -rf data/raft`
3. Re-bootstrap with single node
4. Re-join other nodes

### Slow Operations

Causes:
- Network latency between nodes
- Disk I/O (fsync enabled)
- Too many nodes (7+)

Solutions:
- Reduce inter-node latency
- Use faster disks (SSD)
- Disable fsync for non-critical workloads

### Queue Imbalance

If queues not evenly distributed:
- Normal with consistent hashing
- Adding/removing nodes rebalances
- Use rebalance endpoint to check distribution

## Admin UI

Navigate to http://localhost:3000/cluster

Features:
- View all cluster nodes
- See leader status
- Monitor node health
- Real-time updates

## Performance

### Latency Impact

- Single node: ~1-5ms
- Clustered (local): ~2-10ms
- Clustered (remote): +network latency

### Throughput

- Linear scaling up to 5 nodes
- Bottleneck: Leader write capacity
- Read operations: Distributed across followers

### Benchmark Results

3-node cluster (same datacenter):
- Enqueue: ~2,000 ops/sec
- Lease: ~5,000 ops/sec (distributed)
- Ack: ~8,000 ops/sec

## Migration

### Single to Cluster

1. Stop single node
2. Enable clustering in config
3. Set bootstrap = true
4. Start node (now node1)
5. Add additional nodes

### Cluster to Single

1. Stop all nodes except one
2. Set cluster.enabled = false
3. Restart remaining node

Note: Queue data preserved, but cluster metadata lost.

## Future Enhancements

- Cross-datacenter replication
- Read replicas (non-voting members)
- Automatic node discovery (mDNS)
- Dynamic replication factor
- Queue migration API

## Support

For issues or questions:
- GitHub Issues: https://github.com/Sant0-9/RivetQ
- Check logs: `rivetqd --log-level=debug`
- Prometheus metrics: `/metrics` endpoint
