package store

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/cockroachdb/pebble"
)

// Store provides KV storage using Pebble
type Store struct {
	db *pebble.DB
	mu sync.RWMutex
}

// New creates a new Store instance
func New(path string) (*Store, error) {
	db, err := pebble.Open(path, &pebble.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to open pebble db: %w", err)
	}

	return &Store{
		db: db,
	}, nil
}

// Set stores a key-value pair
func (s *Store) Set(key, value []byte) error {
	return s.db.Set(key, value, pebble.Sync)
}

// Get retrieves a value by key
func (s *Store) Get(key []byte) ([]byte, error) {
	value, closer, err := s.db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	defer closer.Close()

	// Copy value since it's only valid until closer is called
	result := make([]byte, len(value))
	copy(result, value)
	return result, nil
}

// Delete removes a key
func (s *Store) Delete(key []byte) error {
	return s.db.Delete(key, pebble.Sync)
}

// Scan iterates over keys with a prefix
func (s *Store) Scan(prefix []byte, callback func(key, value []byte) error) error {
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixUpperBound(prefix),
	})
	if err != nil {
		return err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		// Copy key and value
		key := make([]byte, len(iter.Key()))
		copy(key, iter.Key())
		value := make([]byte, len(iter.Value()))
		copy(value, iter.Value())

		if err := callback(key, value); err != nil {
			return err
		}
	}

	return iter.Error()
}

// Close closes the store
func (s *Store) Close() error {
	return s.db.Close()
}

// prefixUpperBound returns the upper bound for a prefix scan
func prefixUpperBound(prefix []byte) []byte {
	end := make([]byte, len(prefix))
	copy(end, prefix)
	for i := len(end) - 1; i >= 0; i-- {
		end[i]++
		if end[i] != 0 {
			return end
		}
	}
	return nil
}

// JobMetadata stores metadata about a job
type JobMetadata struct {
	JobID      string            `json:"job_id"`
	Queue      string            `json:"queue"`
	Payload    []byte            `json:"payload"`
	Headers    map[string]string `json:"headers"`
	Priority   uint8             `json:"priority"`
	Tries      uint32            `json:"tries"`
	MaxRetries uint32            `json:"max_retries"`
	ETA        int64             `json:"eta"` // Unix milliseconds
	LeaseID    string            `json:"lease_id,omitempty"`
	LeaseUntil int64             `json:"lease_until,omitempty"` // Unix milliseconds
	Status     string            `json:"status"`                // ready, inflight, dlq
}

// SetJob stores job metadata
func (s *Store) SetJob(jobID string, meta *JobMetadata) error {
	key := []byte(fmt.Sprintf("job:%s", jobID))
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return s.Set(key, data)
}

// GetJob retrieves job metadata
func (s *Store) GetJob(jobID string) (*JobMetadata, error) {
	key := []byte(fmt.Sprintf("job:%s", jobID))
	data, err := s.Get(key)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}

	var meta JobMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// DeleteJob removes job metadata
func (s *Store) DeleteJob(jobID string) error {
	key := []byte(fmt.Sprintf("job:%s", jobID))
	return s.Delete(key)
}

// ScanJobs scans all jobs
func (s *Store) ScanJobs(callback func(*JobMetadata) error) error {
	prefix := []byte("job:")
	return s.Scan(prefix, func(key, value []byte) error {
		var meta JobMetadata
		if err := json.Unmarshal(value, &meta); err != nil {
			return err
		}
		return callback(&meta)
	})
}

// SetIdempotencyKey stores the result for an idempotency key
func (s *Store) SetIdempotencyKey(key, jobID string) error {
	k := []byte(fmt.Sprintf("idempotency:%s", key))
	v := []byte(jobID)
	return s.Set(k, v)
}

// GetIdempotencyKey retrieves the job ID for an idempotency key
func (s *Store) GetIdempotencyKey(key string) (string, error) {
	k := []byte(fmt.Sprintf("idempotency:%s", key))
	v, err := s.Get(k)
	if err != nil {
		return "", err
	}
	if v == nil {
		return "", nil
	}
	return string(v), nil
}
