package rest

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rivetq/rivetq/internal/cluster"
	"github.com/rs/zerolog/log"
)

// ClusterServer provides cluster management REST API
type ClusterServer struct {
	node       *cluster.Node
	membership *cluster.Membership
	sharding   *cluster.Sharding
	discovery  *cluster.Discovery
}

// NewClusterServer creates a new cluster API server
func NewClusterServer(node *cluster.Node, membership *cluster.Membership, sharding *cluster.Sharding, discovery *cluster.Discovery) *ClusterServer {
	return &ClusterServer{
		node:       node,
		membership: membership,
		sharding:   sharding,
		discovery:  discovery,
	}
}

// RegisterRoutes registers cluster routes
func (cs *ClusterServer) RegisterRoutes(r chi.Router) {
	r.Route("/v1/cluster", func(r chi.Router) {
		r.Get("/info", cs.getInfo)
		r.Get("/members", cs.listMembers)
		r.Get("/stats", cs.getStats)
		r.Get("/sharding", cs.getSharding)
		r.Post("/join", cs.joinNode)
		r.Post("/leave", cs.leaveNode)
		r.Post("/announce", cs.announceNode)
	})
}

// getInfo returns cluster information
func (cs *ClusterServer) getInfo(w http.ResponseWriter, r *http.Request) {
	info := cs.membership.GetInfo()
	respondJSON(w, http.StatusOK, info)
}

// listMembers returns all cluster members
func (cs *ClusterServer) listMembers(w http.ResponseWriter, r *http.Request) {
	members := cs.membership.ListMembers()
	respondJSON(w, http.StatusOK, members)
}

// getStats returns cluster statistics
func (cs *ClusterServer) getStats(w http.ResponseWriter, r *http.Request) {
	stats := cs.node.Stats()
	respondJSON(w, http.StatusOK, stats)
}

// getSharding returns sharding information
func (cs *ClusterServer) getSharding(w http.ResponseWriter, r *http.Request) {
	// This would need access to queue manager to get all queues
	// For now return basic info
	info := struct {
		NodeCount   int      `json:"node_count"`
		Replication int      `json:"replication"`
		Nodes       []string `json:"nodes"`
	}{
		NodeCount: cs.sharding.hashRing.NodeCount(),
		Nodes:     cs.sharding.hashRing.Nodes(),
	}

	respondJSON(w, http.StatusOK, info)
}

// JoinRequest represents a node join request
type JoinRequest struct {
	NodeID   string `json:"node_id"`
	Addr     string `json:"addr"`
	RaftAddr string `json:"raft_addr"`
}

// joinNode handles node join requests
func (cs *ClusterServer) joinNode(w http.ResponseWriter, r *http.Request) {
	if !cs.node.IsLeader() {
		respondError(w, http.StatusServiceUnavailable, "not the leader")
		return
	}

	var req JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Add to Raft cluster
	if err := cs.node.Join(req.NodeID, req.RaftAddr); err != nil {
		log.Error().Err(err).Msg("failed to join node to cluster")
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Add to membership
	if err := cs.membership.AddMember(&cluster.Member{
		ID:       req.NodeID,
		Addr:     req.Addr,
		RaftAddr: req.RaftAddr,
	}); err != nil {
		log.Error().Err(err).Msg("failed to add member")
	}

	// Add to sharding ring
	cs.sharding.AddNode(req.NodeID)

	respondJSON(w, http.StatusOK, map[string]string{
		"status": "joined",
		"node_id": req.NodeID,
	})
}

// LeaveRequest represents a node leave request
type LeaveRequest struct {
	NodeID string `json:"node_id"`
}

// leaveNode handles node leave requests
func (cs *ClusterServer) leaveNode(w http.ResponseWriter, r *http.Request) {
	if !cs.node.IsLeader() {
		respondError(w, http.StatusServiceUnavailable, "not the leader")
		return
	}

	var req LeaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Remove from Raft cluster
	if err := cs.node.Remove(req.NodeID); err != nil {
		log.Error().Err(err).Msg("failed to remove node from cluster")
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Remove from membership
	if err := cs.membership.RemoveMember(req.NodeID); err != nil {
		log.Error().Err(err).Msg("failed to remove member")
	}

	// Remove from sharding ring
	cs.sharding.RemoveNode(req.NodeID)

	respondJSON(w, http.StatusOK, map[string]string{
		"status": "left",
		"node_id": req.NodeID,
	})
}

// AnnounceRequest represents a node announcement
type AnnounceRequest struct {
	NodeID   string `json:"node_id"`
	Addr     string `json:"addr"`
	RaftAddr string `json:"raft_addr"`
	Version  string `json:"version"`
}

// announceNode handles node announcements
func (cs *ClusterServer) announceNode(w http.ResponseWriter, r *http.Request) {
	var req AnnounceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	log.Info().
		Str("node_id", req.NodeID).
		Str("addr", req.Addr).
		Msg("received node announcement")

	// Update or add member
	member := &cluster.Member{
		ID:       req.NodeID,
		Addr:     req.Addr,
		RaftAddr: req.RaftAddr,
		Version:  req.Version,
	}

	if err := cs.membership.AddMember(member); err != nil {
		cs.membership.UpdateMemberStatus(req.NodeID, cluster.MemberStatusAlive)
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"status": "acknowledged",
	})
}
