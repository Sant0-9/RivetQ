package cluster

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// MemberStatus represents the status of a cluster member
type MemberStatus string

const (
	MemberStatusAlive   MemberStatus = "alive"
	MemberStatusSuspect MemberStatus = "suspect"
	MemberStatusDead    MemberStatus = "dead"
)

// Member represents a cluster member
type Member struct {
	ID         string       `json:"id"`
	Addr       string       `json:"addr"`
	RaftAddr   string       `json:"raft_addr"`
	Status     MemberStatus `json:"status"`
	IsLeader   bool         `json:"is_leader"`
	LastSeen   time.Time    `json:"last_seen"`
	JoinedAt   time.Time    `json:"joined_at"`
	Version    string       `json:"version,omitempty"`
}

// Membership manages cluster membership
type Membership struct {
	mu      sync.RWMutex
	node    *Node
	members map[string]*Member
	localID string

	// Health checking
	healthCheckInterval time.Duration
	healthTimeout       time.Duration
	stopCh              chan struct{}
	wg                  sync.WaitGroup
}

// NewMembership creates a new membership manager
func NewMembership(node *Node, localID string) *Membership {
	return &Membership{
		node:                node,
		members:             make(map[string]*Member),
		localID:             localID,
		healthCheckInterval: 5 * time.Second,
		healthTimeout:       2 * time.Second,
		stopCh:              make(chan struct{}),
	}
}

// Start starts the membership manager
func (m *Membership) Start() {
	m.wg.Add(1)
	go m.healthCheckLoop()
}

// Stop stops the membership manager
func (m *Membership) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// AddMember adds a new member to the cluster
func (m *Membership) AddMember(member *Member) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.members[member.ID]; exists {
		return fmt.Errorf("member already exists: %s", member.ID)
	}

	member.JoinedAt = time.Now()
	member.LastSeen = time.Now()
	member.Status = MemberStatusAlive

	m.members[member.ID] = member

	log.Info().
		Str("member_id", member.ID).
		Str("addr", member.Addr).
		Msg("added cluster member")

	return nil
}

// RemoveMember removes a member from the cluster
func (m *Membership) RemoveMember(memberID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.members[memberID]; !exists {
		return fmt.Errorf("member not found: %s", memberID)
	}

	delete(m.members, memberID)

	log.Info().Str("member_id", memberID).Msg("removed cluster member")

	return nil
}

// GetMember returns a member by ID
func (m *Membership) GetMember(memberID string) (*Member, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	member, exists := m.members[memberID]
	if !exists {
		return nil, fmt.Errorf("member not found: %s", memberID)
	}

	return member, nil
}

// ListMembers returns all cluster members
func (m *Membership) ListMembers() []*Member {
	m.mu.RLock()
	defer m.mu.RUnlock()

	members := make([]*Member, 0, len(m.members))
	for _, member := range m.members {
		memberCopy := *member
		memberCopy.IsLeader = (m.node.Leader() == member.RaftAddr)
		members = append(members, &memberCopy)
	}

	return members
}

// GetAliveMembers returns all alive members
func (m *Membership) GetAliveMembers() []*Member {
	m.mu.RLock()
	defer m.mu.RUnlock()

	members := make([]*Member, 0)
	for _, member := range m.members {
		if member.Status == MemberStatusAlive {
			memberCopy := *member
			members = append(members, &memberCopy)
		}
	}

	return members
}

// UpdateMemberStatus updates a member's status
func (m *Membership) UpdateMemberStatus(memberID string, status MemberStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if member, exists := m.members[memberID]; exists {
		member.Status = status
		if status == MemberStatusAlive {
			member.LastSeen = time.Now()
		}

		log.Debug().
			Str("member_id", memberID).
			Str("status", string(status)).
			Msg("updated member status")
	}
}

// healthCheckLoop periodically checks member health
func (m *Membership) healthCheckLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkMemberHealth()
		}
	}
}

// checkMemberHealth checks health of all members
func (m *Membership) checkMemberHealth() {
	members := m.ListMembers()

	for _, member := range members {
		// Skip self
		if member.ID == m.localID {
			continue
		}

		// Check health
		if m.isHealthy(member) {
			m.UpdateMemberStatus(member.ID, MemberStatusAlive)
		} else {
			// Mark as suspect first, then dead if still unhealthy
			m.mu.RLock()
			currentStatus := m.members[member.ID].Status
			m.mu.RUnlock()

			if currentStatus == MemberStatusAlive {
				m.UpdateMemberStatus(member.ID, MemberStatusSuspect)
			} else if currentStatus == MemberStatusSuspect {
				m.UpdateMemberStatus(member.ID, MemberStatusDead)
				log.Warn().Str("member_id", member.ID).Msg("member marked as dead")
			}
		}
	}
}

// isHealthy checks if a member is healthy
func (m *Membership) isHealthy(member *Member) bool {
	if member.Addr == "" {
		return false
	}

	client := &http.Client{
		Timeout: m.healthTimeout,
	}

	resp, err := client.Get(fmt.Sprintf("http://%s/healthz", member.Addr))
	if err != nil {
		log.Debug().Err(err).Str("member_id", member.ID).Msg("health check failed")
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// MembershipInfo returns membership information
type MembershipInfo struct {
	LocalID     string    `json:"local_id"`
	Leader      string    `json:"leader"`
	MemberCount int       `json:"member_count"`
	Members     []*Member `json:"members"`
}

// GetInfo returns cluster membership information
func (m *Membership) GetInfo() MembershipInfo {
	members := m.ListMembers()

	return MembershipInfo{
		LocalID:     m.localID,
		Leader:      m.node.Leader(),
		MemberCount: len(members),
		Members:     members,
	}
}

// MarshalJSON implements json.Marshaler
func (m *Member) MarshalJSON() ([]byte, error) {
	type Alias Member
	return json.Marshal(&struct {
		LastSeenUnix int64 `json:"last_seen_unix"`
		JoinedAtUnix int64 `json:"joined_at_unix"`
		*Alias
	}{
		LastSeenUnix: m.LastSeen.Unix(),
		JoinedAtUnix: m.JoinedAt.Unix(),
		Alias:        (*Alias)(m),
	})
}
