package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// DiscoveryConfig holds discovery configuration
type DiscoveryConfig struct {
	SeedAddrs        []string
	DiscoveryInterval time.Duration
	RetryInterval     time.Duration
	Timeout           time.Duration
}

// DefaultDiscoveryConfig returns default discovery configuration
func DefaultDiscoveryConfig() DiscoveryConfig {
	return DiscoveryConfig{
		DiscoveryInterval: 30 * time.Second,
		RetryInterval:     5 * time.Second,
		Timeout:           3 * time.Second,
	}
}

// Discovery handles node discovery and joining
type Discovery struct {
	mu     sync.RWMutex
	config DiscoveryConfig
	node   *Node
	member *Membership

	localAddr string
	localID   string

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewDiscovery creates a new discovery manager
func NewDiscovery(config DiscoveryConfig, node *Node, member *Membership, localAddr, localID string) *Discovery {
	return &Discovery{
		config:    config,
		node:      node,
		member:    member,
		localAddr: localAddr,
		localID:   localID,
		stopCh:    make(chan struct{}),
	}
}

// Start starts the discovery process
func (d *Discovery) Start() error {
	// Try to join existing cluster
	if len(d.config.SeedAddrs) > 0 {
		if err := d.joinCluster(); err != nil {
			log.Warn().Err(err).Msg("failed to join cluster via discovery")
		}
	}

	// Start periodic discovery
	d.wg.Add(1)
	go d.discoveryLoop()

	return nil
}

// Stop stops the discovery process
func (d *Discovery) Stop() {
	close(d.stopCh)
	d.wg.Wait()
}

// joinCluster attempts to join an existing cluster
func (d *Discovery) joinCluster() error {
	for _, seedAddr := range d.config.SeedAddrs {
		log.Info().Str("seed", seedAddr).Msg("attempting to join cluster")

		if err := d.requestJoin(seedAddr); err != nil {
			log.Warn().Err(err).Str("seed", seedAddr).Msg("failed to join via seed")
			continue
		}

		log.Info().Str("seed", seedAddr).Msg("successfully joined cluster")
		return nil
	}

	return fmt.Errorf("failed to join cluster via any seed node")
}

// requestJoin sends a join request to a seed node
func (d *Discovery) requestJoin(seedAddr string) error {
	client := &http.Client{
		Timeout: d.config.Timeout,
	}

	// Get cluster info from seed
	resp, err := client.Get(fmt.Sprintf("http://%s/v1/cluster/info", seedAddr))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var info MembershipInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return err
	}

	// Find the leader
	leaderAddr := ""
	for _, member := range info.Members {
		if member.IsLeader {
			leaderAddr = member.Addr
			break
		}
	}

	if leaderAddr == "" {
		return fmt.Errorf("no leader found in cluster")
	}

	// Send join request to leader
	joinReq := struct {
		NodeID   string `json:"node_id"`
		Addr     string `json:"addr"`
		RaftAddr string `json:"raft_addr"`
	}{
		NodeID:   d.localID,
		Addr:     d.localAddr,
		RaftAddr: d.node.config.RaftAddr,
	}

	reqBody, err := json.Marshal(joinReq)
	if err != nil {
		return err
	}

	resp, err = client.Post(
		fmt.Sprintf("http://%s/v1/cluster/join", leaderAddr),
		"application/json",
		nil,
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("join request failed: %d", resp.StatusCode)
	}

	// Update local membership
	d.member.AddMember(&Member{
		ID:       d.localID,
		Addr:     d.localAddr,
		RaftAddr: d.node.config.RaftAddr,
	})

	return nil
}

// discoveryLoop periodically discovers new nodes
func (d *Discovery) discoveryLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.DiscoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.discoverNodes()
		}
	}
}

// discoverNodes discovers nodes from seed addresses
func (d *Discovery) discoverNodes() {
	if len(d.config.SeedAddrs) == 0 {
		return
	}

	client := &http.Client{
		Timeout: d.config.Timeout,
	}

	for _, seedAddr := range d.config.SeedAddrs {
		resp, err := client.Get(fmt.Sprintf("http://%s/v1/cluster/members", seedAddr))
		if err != nil {
			log.Debug().Err(err).Str("seed", seedAddr).Msg("failed to discover nodes")
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		var members []*Member
		if err := json.NewDecoder(resp.Body).Decode(&members); err != nil {
			log.Debug().Err(err).Msg("failed to decode members")
			continue
		}

		// Add newly discovered members
		for _, member := range members {
			if member.ID == d.localID {
				continue // Skip self
			}

			existing, err := d.member.GetMember(member.ID)
			if err != nil || existing == nil {
				log.Info().
					Str("member_id", member.ID).
					Str("addr", member.Addr).
					Msg("discovered new cluster member")

				d.member.AddMember(member)
			}
		}
	}
}

// Announce announces this node to the cluster
func (d *Discovery) Announce(ctx context.Context) error {
	if len(d.config.SeedAddrs) == 0 {
		return nil
	}

	announcement := struct {
		NodeID   string `json:"node_id"`
		Addr     string `json:"addr"`
		RaftAddr string `json:"raft_addr"`
		Version  string `json:"version"`
	}{
		NodeID:   d.localID,
		Addr:     d.localAddr,
		RaftAddr: d.node.config.RaftAddr,
		Version:  "1.0.0",
	}

	data, err := json.Marshal(announcement)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: d.config.Timeout,
	}

	for _, seedAddr := range d.config.SeedAddrs {
		req, err := http.NewRequestWithContext(
			ctx,
			"POST",
			fmt.Sprintf("http://%s/v1/cluster/announce", seedAddr),
			nil,
		)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Debug().Err(err).Str("seed", seedAddr).Msg("failed to announce")
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			log.Info().Str("seed", seedAddr).Msg("announced to cluster")
			return nil
		}
	}

	return fmt.Errorf("failed to announce to any seed node")
}
