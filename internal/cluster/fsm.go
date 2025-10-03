package cluster

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/hashicorp/raft"
	"github.com/rivetq/rivetq/internal/queue"
	"github.com/rs/zerolog/log"
)

// CommandType represents the type of command
type CommandType uint8

const (
	CommandEnqueue CommandType = iota + 1
	CommandAck
	CommandNack
	CommandSetRateLimit
)

// Command represents a replicated command
type Command struct {
	Type CommandType `json:"type"`
	Data []byte      `json:"data"`
}

// EnqueueCommand contains enqueue data
type EnqueueCommand struct {
	Queue          string            `json:"queue"`
	JobID          string            `json:"job_id"`
	Payload        []byte            `json:"payload"`
	Headers        map[string]string `json:"headers"`
	Priority       uint8             `json:"priority"`
	DelayMs        int64             `json:"delay_ms"`
	MaxRetries     uint32            `json:"max_retries"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
}

// AckCommand contains ack data
type AckCommand struct {
	JobID   string `json:"job_id"`
	LeaseID string `json:"lease_id"`
}

// NackCommand contains nack data
type NackCommand struct {
	JobID   string `json:"job_id"`
	LeaseID string `json:"lease_id"`
	Reason  string `json:"reason"`
}

// RateLimitCommand contains rate limit data
type RateLimitCommand struct {
	Queue      string  `json:"queue"`
	Capacity   float64 `json:"capacity"`
	RefillRate float64 `json:"refill_rate"`
}

// FSM implements raft.FSM for the finite state machine
type FSM struct {
	mu      sync.RWMutex
	manager *queue.Manager
}

// NewFSM creates a new FSM
func NewFSM(manager *queue.Manager) *FSM {
	return &FSM{
		manager: manager,
	}
}

// Apply applies a Raft log entry to the FSM
func (f *FSM) Apply(l *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(l.Data, &cmd); err != nil {
		log.Error().Err(err).Msg("failed to unmarshal command")
		return err
	}

	switch cmd.Type {
	case CommandEnqueue:
		return f.applyEnqueue(cmd.Data)
	case CommandAck:
		return f.applyAck(cmd.Data)
	case CommandNack:
		return f.applyNack(cmd.Data)
	case CommandSetRateLimit:
		return f.applySetRateLimit(cmd.Data)
	default:
		return fmt.Errorf("unknown command type: %d", cmd.Type)
	}
}

func (f *FSM) applyEnqueue(data []byte) interface{} {
	var cmd EnqueueCommand
	if err := json.Unmarshal(data, &cmd); err != nil {
		return err
	}

	retryPolicy := queue.RetryPolicy{
		MaxRetries: cmd.MaxRetries,
	}

	jobID, err := f.manager.Enqueue(
		cmd.Queue,
		cmd.Payload,
		cmd.Headers,
		cmd.Priority,
		cmd.DelayMs,
		retryPolicy,
		cmd.IdempotencyKey,
	)

	if err != nil {
		log.Error().Err(err).Str("queue", cmd.Queue).Msg("failed to enqueue job")
		return err
	}

	return jobID
}

func (f *FSM) applyAck(data []byte) interface{} {
	var cmd AckCommand
	if err := json.Unmarshal(data, &cmd); err != nil {
		return err
	}

	if err := f.manager.Ack(cmd.JobID, cmd.LeaseID); err != nil {
		log.Error().Err(err).Str("job_id", cmd.JobID).Msg("failed to ack job")
		return err
	}

	return nil
}

func (f *FSM) applyNack(data []byte) interface{} {
	var cmd NackCommand
	if err := json.Unmarshal(data, &cmd); err != nil {
		return err
	}

	if err := f.manager.Nack(cmd.JobID, cmd.LeaseID, cmd.Reason); err != nil {
		log.Error().Err(err).Str("job_id", cmd.JobID).Msg("failed to nack job")
		return err
	}

	return nil
}

func (f *FSM) applySetRateLimit(data []byte) interface{} {
	var cmd RateLimitCommand
	if err := json.Unmarshal(data, &cmd); err != nil {
		return err
	}

	f.manager.SetRateLimit(cmd.Queue, cmd.Capacity, cmd.RefillRate)
	return nil
}

// Snapshot returns a snapshot of the FSM state
func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Create snapshot of current state
	snapshot := &FSMSnapshot{
		queues: f.manager.ListQueues(),
	}

	// Collect stats for all queues
	snapshot.stats = make(map[string]QueueStats)
	for _, queueName := range snapshot.queues {
		ready, inflight, dlq, err := f.manager.Stats(queueName)
		if err != nil {
			continue
		}
		snapshot.stats[queueName] = QueueStats{
			Ready:    ready,
			Inflight: inflight,
			DLQ:      dlq,
		}

		// Get rate limits
		capacity, refillRate, exists := f.manager.GetRateLimit(queueName)
		if exists {
			snapshot.stats[queueName] = QueueStats{
				Ready:      ready,
				Inflight:   inflight,
				DLQ:        dlq,
				Capacity:   capacity,
				RefillRate: refillRate,
			}
		}
	}

	return snapshot, nil
}

// Restore restores the FSM from a snapshot
func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()

	var snapshot FSMSnapshot
	if err := json.NewDecoder(rc).Decode(&snapshot); err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Restore rate limits
	for queue, stats := range snapshot.stats {
		if stats.Capacity > 0 {
			f.manager.SetRateLimit(queue, stats.Capacity, stats.RefillRate)
		}
	}

	log.Info().Int("queues", len(snapshot.queues)).Msg("restored FSM from snapshot")
	return nil
}

// QueueStats holds queue statistics for snapshots
type QueueStats struct {
	Ready      int     `json:"ready"`
	Inflight   int     `json:"inflight"`
	DLQ        int     `json:"dlq"`
	Capacity   float64 `json:"capacity,omitempty"`
	RefillRate float64 `json:"refill_rate,omitempty"`
}

// FSMSnapshot represents a point-in-time snapshot
type FSMSnapshot struct {
	queues []string
	stats  map[string]QueueStats
}

// Persist writes the snapshot to the sink
func (s *FSMSnapshot) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		data := struct {
			Queues []string                `json:"queues"`
			Stats  map[string]QueueStats `json:"stats"`
		}{
			Queues: s.queues,
			Stats:  s.stats,
		}

		if err := json.NewEncoder(sink).Encode(data); err != nil {
			return err
		}

		return sink.Close()
	}()

	if err != nil {
		sink.Cancel()
		return err
	}

	return nil
}

// Release releases the snapshot resources
func (s *FSMSnapshot) Release() {}
