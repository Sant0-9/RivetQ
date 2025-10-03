package cluster

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
	"github.com/rs/zerolog/log"
)

// Node represents a cluster node
type Node struct {
	config Config
	raft   *raft.Raft
	fsm    *FSM
	trans  *raft.NetworkTransport
}

// NewNode creates a new cluster node
func NewNode(cfg Config, fsm *FSM) (*Node, error) {
	if err := os.MkdirAll(cfg.RaftDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create raft directory: %w", err)
	}

	node := &Node{
		config: cfg,
		fsm:    fsm,
	}

	// Setup Raft configuration
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(cfg.NodeID)
	raftConfig.HeartbeatTimeout = cfg.HeartbeatTimeout
	raftConfig.ElectionTimeout = cfg.ElectionTimeout
	raftConfig.SnapshotInterval = cfg.SnapshotInterval
	raftConfig.SnapshotThreshold = cfg.SnapshotThreshold
	raftConfig.MaxAppendEntries = cfg.MaxAppendEntries
	raftConfig.TrailingLogs = cfg.TrailingLogs

	// Setup transport
	addr, err := net.ResolveTCPAddr("tcp", cfg.RaftAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve raft address: %w", err)
	}

	trans, err := raft.NewTCPTransport(cfg.RaftAddr, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}
	node.trans = trans

	// Create stable store (for Raft logs and stable storage)
	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.RaftDir, "raft.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to create stable store: %w", err)
	}

	// Create log store
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.RaftDir, "raft-log.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to create log store: %w", err)
	}

	// Create snapshot store
	snapshotStore, err := raft.NewFileSnapshotStore(cfg.RaftDir, 2, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot store: %w", err)
	}

	// Create Raft instance
	r, err := raft.NewRaft(raftConfig, fsm, logStore, stableStore, snapshotStore, trans)
	if err != nil {
		return nil, fmt.Errorf("failed to create raft: %w", err)
	}
	node.raft = r

	// Bootstrap or join cluster
	if cfg.Bootstrap {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raft.ServerID(cfg.NodeID),
					Address: raft.ServerAddress(cfg.RaftAddr),
				},
			},
		}
		f := r.BootstrapCluster(configuration)
		if err := f.Error(); err != nil {
			return nil, fmt.Errorf("failed to bootstrap cluster: %w", err)
		}
		log.Info().Str("node_id", cfg.NodeID).Msg("bootstrapped new cluster")
	}

	return node, nil
}

// IsLeader returns true if this node is the Raft leader
func (n *Node) IsLeader() bool {
	return n.raft.State() == raft.Leader
}

// Leader returns the current leader address
func (n *Node) Leader() string {
	addr, _ := n.raft.LeaderWithID()
	return string(addr)
}

// Apply applies a command to the Raft log
func (n *Node) Apply(cmd []byte, timeout time.Duration) error {
	if !n.IsLeader() {
		return fmt.Errorf("not the leader")
	}

	f := n.raft.Apply(cmd, timeout)
	if err := f.Error(); err != nil {
		return err
	}

	return nil
}

// Join adds a new node to the cluster
func (n *Node) Join(nodeID, addr string) error {
	if !n.IsLeader() {
		return fmt.Errorf("not the leader")
	}

	log.Info().Str("node_id", nodeID).Str("addr", addr).Msg("adding node to cluster")

	f := n.raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(addr), 0, 0)
	if err := f.Error(); err != nil {
		return fmt.Errorf("failed to add voter: %w", err)
	}

	return nil
}

// Remove removes a node from the cluster
func (n *Node) Remove(nodeID string) error {
	if !n.IsLeader() {
		return fmt.Errorf("not the leader")
	}

	log.Info().Str("node_id", nodeID).Msg("removing node from cluster")

	f := n.raft.RemoveServer(raft.ServerID(nodeID), 0, 0)
	if err := f.Error(); err != nil {
		return fmt.Errorf("failed to remove server: %w", err)
	}

	return nil
}

// Stats returns Raft stats
func (n *Node) Stats() map[string]string {
	return n.raft.Stats()
}

// Shutdown gracefully shuts down the node
func (n *Node) Shutdown() error {
	log.Info().Msg("shutting down raft node")

	if err := n.raft.Shutdown().Error(); err != nil {
		return err
	}

	if err := n.trans.Close(); err != nil {
		return err
	}

	return nil
}

// WaitForLeader waits for a leader to be elected
func (n *Node) WaitForLeader(timeout time.Duration) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			if leader := n.Leader(); leader != "" {
				return nil
			}
		case <-timer.C:
			return fmt.Errorf("timeout waiting for leader")
		}
	}
}
