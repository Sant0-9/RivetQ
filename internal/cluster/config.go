package cluster

import (
	"time"
)

// Config holds cluster configuration
type Config struct {
	// Node identification
	NodeID   string
	RaftAddr string
	RaftDir  string

	// Cluster settings
	Bootstrap bool     // Is this node bootstrapping a new cluster
	JoinAddrs []string // Addresses of existing nodes to join

	// Raft tuning
	HeartbeatTimeout time.Duration
	ElectionTimeout  time.Duration
	SnapshotInterval time.Duration
	SnapshotThreshold uint64

	// Replication
	MaxAppendEntries int
	TrailingLogs     uint64
}

// DefaultConfig returns default cluster configuration
func DefaultConfig() Config {
	return Config{
		HeartbeatTimeout:  1 * time.Second,
		ElectionTimeout:   1 * time.Second,
		SnapshotInterval:  120 * time.Second,
		SnapshotThreshold: 8192,
		MaxAppendEntries:  64,
		TrailingLogs:      10240,
	}
}
