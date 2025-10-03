package cluster

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
)

// VirtualNodes is the number of virtual nodes per physical node
const VirtualNodes = 150

// ConsistentHash implements consistent hashing for queue distribution
type ConsistentHash struct {
	mu      sync.RWMutex
	ring    []uint32
	members map[uint32]string // hash -> member ID
	nodes   map[string]bool   // member ID -> exists
}

// NewConsistentHash creates a new consistent hash ring
func NewConsistentHash() *ConsistentHash {
	return &ConsistentHash{
		ring:    make([]uint32, 0),
		members: make(map[uint32]string),
		nodes:   make(map[string]bool),
	}
}

// AddNode adds a node to the hash ring
func (ch *ConsistentHash) AddNode(nodeID string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if ch.nodes[nodeID] {
		return // Already exists
	}

	ch.nodes[nodeID] = true

	// Add virtual nodes
	for i := 0; i < VirtualNodes; i++ {
		hash := ch.hashKey(fmt.Sprintf("%s:%d", nodeID, i))
		ch.ring = append(ch.ring, hash)
		ch.members[hash] = nodeID
	}

	// Sort the ring
	sort.Slice(ch.ring, func(i, j int) bool {
		return ch.ring[i] < ch.ring[j]
	})
}

// RemoveNode removes a node from the hash ring
func (ch *ConsistentHash) RemoveNode(nodeID string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if !ch.nodes[nodeID] {
		return // Doesn't exist
	}

	delete(ch.nodes, nodeID)

	// Remove virtual nodes
	newRing := make([]uint32, 0)
	for _, hash := range ch.ring {
		if ch.members[hash] != nodeID {
			newRing = append(newRing, hash)
		} else {
			delete(ch.members, hash)
		}
	}

	ch.ring = newRing
}

// GetNode returns the node responsible for a given key
func (ch *ConsistentHash) GetNode(key string) (string, error) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if len(ch.ring) == 0 {
		return "", fmt.Errorf("no nodes available")
	}

	hash := ch.hashKey(key)

	// Binary search for the first node >= hash
	idx := sort.Search(len(ch.ring), func(i int) bool {
		return ch.ring[i] >= hash
	})

	// Wrap around if necessary
	if idx == len(ch.ring) {
		idx = 0
	}

	nodeID := ch.members[ch.ring[idx]]
	return nodeID, nil
}

// GetNodes returns N nodes for replication
func (ch *ConsistentHash) GetNodes(key string, n int) ([]string, error) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if len(ch.ring) == 0 {
		return nil, fmt.Errorf("no nodes available")
	}

	if n > len(ch.nodes) {
		n = len(ch.nodes)
	}

	hash := ch.hashKey(key)
	idx := sort.Search(len(ch.ring), func(i int) bool {
		return ch.ring[i] >= hash
	})

	if idx == len(ch.ring) {
		idx = 0
	}

	seen := make(map[string]bool)
	nodes := make([]string, 0, n)

	// Walk the ring until we have n unique nodes
	for len(nodes) < n {
		nodeID := ch.members[ch.ring[idx]]
		if !seen[nodeID] {
			nodes = append(nodes, nodeID)
			seen[nodeID] = true
		}

		idx = (idx + 1) % len(ch.ring)
	}

	return nodes, nil
}

// hashKey computes hash for a key
func (ch *ConsistentHash) hashKey(key string) uint32 {
	h := sha256.New()
	h.Write([]byte(key))
	sum := h.Sum(nil)
	return binary.BigEndian.Uint32(sum[:4])
}

// NodeCount returns the number of nodes
func (ch *ConsistentHash) NodeCount() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.nodes)
}

// Nodes returns all node IDs
func (ch *ConsistentHash) Nodes() []string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	nodes := make([]string, 0, len(ch.nodes))
	for nodeID := range ch.nodes {
		nodes = append(nodes, nodeID)
	}
	return nodes
}

// Sharding manages queue distribution across cluster nodes
type Sharding struct {
	mu           sync.RWMutex
	hashRing     *ConsistentHash
	localNodeID  string
	replication  int // Number of replicas
}

// NewSharding creates a new sharding manager
func NewSharding(localNodeID string, replication int) *Sharding {
	return &Sharding{
		hashRing:    NewConsistentHash(),
		localNodeID: localNodeID,
		replication: replication,
	}
}

// AddNode adds a node to the shard ring
func (s *Sharding) AddNode(nodeID string) {
	s.hashRing.AddNode(nodeID)
}

// RemoveNode removes a node from the shard ring
func (s *Sharding) RemoveNode(nodeID string) {
	s.hashRing.RemoveNode(nodeID)
}

// GetQueueNode returns the primary node for a queue
func (s *Sharding) GetQueueNode(queueName string) (string, error) {
	return s.hashRing.GetNode(queueName)
}

// GetQueueNodes returns all nodes (primary + replicas) for a queue
func (s *Sharding) GetQueueNodes(queueName string) ([]string, error) {
	return s.hashRing.GetNodes(queueName, s.replication)
}

// IsLocalQueue returns true if this node is responsible for the queue
func (s *Sharding) IsLocalQueue(queueName string) bool {
	node, err := s.GetQueueNode(queueName)
	if err != nil {
		return false
	}
	return node == s.localNodeID
}

// GetLocalQueues returns all queues this node is responsible for
func (s *Sharding) GetLocalQueues(allQueues []string) []string {
	local := make([]string, 0)
	for _, queue := range allQueues {
		if s.IsLocalQueue(queue) {
			local = append(local, queue)
		}
	}
	return local
}

// RebalanceInfo provides information about queue rebalancing
type RebalanceInfo struct {
	TotalQueues   int               `json:"total_queues"`
	LocalQueues   int               `json:"local_queues"`
	QueuesByNode  map[string]int    `json:"queues_by_node"`
	QueueMappings map[string]string `json:"queue_mappings,omitempty"`
}

// GetRebalanceInfo returns rebalancing information
func (s *Sharding) GetRebalanceInfo(allQueues []string) RebalanceInfo {
	queuesByNode := make(map[string]int)
	queueMappings := make(map[string]string)

	for _, queue := range allQueues {
		node, err := s.GetQueueNode(queue)
		if err != nil {
			continue
		}

		queuesByNode[node]++
		queueMappings[queue] = node
	}

	return RebalanceInfo{
		TotalQueues:   len(allQueues),
		LocalQueues:   len(s.GetLocalQueues(allQueues)),
		QueuesByNode:  queuesByNode,
		QueueMappings: queueMappings,
	}
}
