package rivetq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a RivetQ client
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new RivetQ client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Job represents a job
type Job struct {
	ID       string            `json:"id"`
	Queue    string            `json:"queue"`
	Payload  json.RawMessage   `json:"payload"`
	Headers  map[string]string `json:"headers,omitempty"`
	Priority uint8             `json:"priority"`
	Tries    uint32            `json:"tries"`
	LeaseID  string            `json:"lease_id"`
}

// EnqueueOptions for enqueuing jobs
type EnqueueOptions struct {
	Priority       uint8
	DelayMs        int64
	MaxRetries     uint32
	IdempotencyKey string
	Headers        map[string]string
}

// Enqueue adds a job to a queue
func (c *Client) Enqueue(ctx context.Context, queue string, payload interface{}, opts *EnqueueOptions) (string, error) {
	if opts == nil {
		opts = &EnqueueOptions{
			Priority:   5,
			MaxRetries: 3,
		}
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	req := map[string]interface{}{
		"payload":     json.RawMessage(payloadBytes),
		"priority":    opts.Priority,
		"delay_ms":    opts.DelayMs,
		"max_retries": opts.MaxRetries,
	}

	if opts.IdempotencyKey != "" {
		req["idempotency_key"] = opts.IdempotencyKey
	}

	if opts.Headers != nil {
		req["headers"] = opts.Headers
	}

	var resp struct {
		JobID string `json:"job_id"`
	}

	if err := c.doRequest(ctx, "POST", fmt.Sprintf("/v1/queues/%s/enqueue", queue), req, &resp); err != nil {
		return "", err
	}

	return resp.JobID, nil
}

// Lease leases jobs from a queue
func (c *Client) Lease(ctx context.Context, queue string, maxJobs int, visibilityMs int64) ([]*Job, error) {
	if maxJobs <= 0 {
		maxJobs = 1
	}
	if visibilityMs <= 0 {
		visibilityMs = 30000
	}

	req := map[string]interface{}{
		"max_jobs":      maxJobs,
		"visibility_ms": visibilityMs,
	}

	var resp struct {
		Jobs []*Job `json:"jobs"`
	}

	if err := c.doRequest(ctx, "POST", fmt.Sprintf("/v1/queues/%s/lease", queue), req, &resp); err != nil {
		return nil, err
	}

	return resp.Jobs, nil
}

// Ack acknowledges job completion
func (c *Client) Ack(ctx context.Context, jobID, leaseID string) error {
	req := map[string]interface{}{
		"job_id":   jobID,
		"lease_id": leaseID,
	}

	return c.doRequest(ctx, "POST", "/v1/ack", req, nil)
}

// Nack negatively acknowledges a job
func (c *Client) Nack(ctx context.Context, jobID, leaseID, reason string) error {
	req := map[string]interface{}{
		"job_id":   jobID,
		"lease_id": leaseID,
		"reason":   reason,
	}

	return c.doRequest(ctx, "POST", "/v1/nack", req, nil)
}

// Stats returns queue statistics
func (c *Client) Stats(ctx context.Context, queue string) (ready, inflight, dlq int, err error) {
	var resp struct {
		Ready    int `json:"ready"`
		Inflight int `json:"inflight"`
		DLQ      int `json:"dlq"`
	}

	if err := c.doRequest(ctx, "GET", fmt.Sprintf("/v1/queues/%s/stats", queue), nil, &resp); err != nil {
		return 0, 0, 0, err
	}

	return resp.Ready, resp.Inflight, resp.DLQ, nil
}

// ListQueues returns all queue names
func (c *Client) ListQueues(ctx context.Context) ([]string, error) {
	var resp struct {
		Queues []string `json:"queues"`
	}

	if err := c.doRequest(ctx, "GET", "/v1/queues/", nil, &resp); err != nil {
		return nil, err
	}

	return resp.Queues, nil
}

// doRequest performs an HTTP request
func (c *Client) doRequest(ctx context.Context, method, path string, body, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server error (%d): %s", resp.StatusCode, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}
