package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// Proxy handles forwarding requests to the appropriate node
type Proxy struct {
	sharding   *Sharding
	membership *Membership
	client     *http.Client
}

// NewProxy creates a new cluster proxy
func NewProxy(sharding *Sharding, membership *Membership) *Proxy {
	return &Proxy{
		sharding:   sharding,
		membership: membership,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ForwardRequest forwards a request to the appropriate node
func (p *Proxy) ForwardRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	// Extract queue name from path (assumes /v1/queues/{queue}/...)
	queueName := extractQueueName(path)
	if queueName == "" {
		return nil, fmt.Errorf("could not determine queue from path: %s", path)
	}

	// Get the node responsible for this queue
	targetNode, err := p.sharding.GetQueueNode(queueName)
	if err != nil {
		return nil, fmt.Errorf("failed to find node for queue: %w", err)
	}

	// Get member info
	member, err := p.membership.GetMember(targetNode)
	if err != nil {
		return nil, fmt.Errorf("failed to get member info: %w", err)
	}

	if member.Status != MemberStatusAlive {
		return nil, fmt.Errorf("target node is not alive: %s", targetNode)
	}

	// Forward the request
	targetURL := fmt.Sprintf("http://%s%s", member.Addr, path)
	log.Debug().
		Str("queue", queueName).
		Str("target_node", targetNode).
		Str("url", targetURL).
		Msg("forwarding request")

	req, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-By", "rivetq-cluster")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to forward request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("forwarded request failed: %d - %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// BroadcastCommand broadcasts a command to all nodes
func (p *Proxy) BroadcastCommand(ctx context.Context, path string, body interface{}) error {
	members := p.membership.GetAliveMembers()

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	errors := make([]error, 0)

	for _, member := range members {
		targetURL := fmt.Sprintf("http://%s%s", member.Addr, path)

		req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(data))
		if err != nil {
			errors = append(errors, fmt.Errorf("node %s: %w", member.ID, err))
			continue
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := p.client.Do(req)
		if err != nil {
			errors = append(errors, fmt.Errorf("node %s: %w", member.ID, err))
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 400 {
			errors = append(errors, fmt.Errorf("node %s: status %d", member.ID, resp.StatusCode))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("broadcast failed on %d nodes: %v", len(errors), errors)
	}

	return nil
}

// extractQueueName extracts queue name from URL path
func extractQueueName(path string) string {
	// Parse paths like /v1/queues/{queue}/enqueue
	// Simple parsing - could be more robust
	const prefix = "/v1/queues/"
	if len(path) <= len(prefix) {
		return ""
	}

	remaining := path[len(prefix):]
	for i, c := range remaining {
		if c == '/' {
			return remaining[:i]
		}
	}

	return remaining
}

// ProxyStats holds proxy statistics
type ProxyStats struct {
	ForwardedRequests  int64            `json:"forwarded_requests"`
	FailedForwards     int64            `json:"failed_forwards"`
	ForwardsByNode     map[string]int64 `json:"forwards_by_node"`
	AverageLatencyMs   float64          `json:"average_latency_ms"`
}

// GetStats returns proxy statistics (placeholder for now)
func (p *Proxy) GetStats() ProxyStats {
	return ProxyStats{
		ForwardsByNode: make(map[string]int64),
	}
}
