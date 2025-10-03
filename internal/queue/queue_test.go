package queue

import (
	"testing"
	"time"

	"github.com/rivetq/rivetq/internal/store"
	"github.com/rivetq/rivetq/internal/wal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnqueueAndLease(t *testing.T) {
	dir := t.TempDir()

	// Setup
	walInst, err := wal.New(wal.Config{
		Dir:         dir + "/wal",
		SegmentSize: 1024,
		Fsync:       false,
	})
	require.NoError(t, err)
	defer walInst.Close()

	storeInst, err := store.New(dir + "/store")
	require.NoError(t, err)
	defer storeInst.Close()

	mgr := NewManager(storeInst, walInst)
	err = mgr.Start()
	require.NoError(t, err)
	defer mgr.Stop()

	// Enqueue a job
	jobID, err := mgr.Enqueue("test-queue", []byte("payload"), nil, 5, 0, DefaultRetryPolicy(), "")
	require.NoError(t, err)
	assert.NotEmpty(t, jobID)

	// Lease the job
	jobs, err := mgr.Lease("test-queue", 1, 30000)
	require.NoError(t, err)
	assert.Len(t, jobs, 1)
	assert.Equal(t, jobID, jobs[0].ID)
	assert.Equal(t, "test-queue", jobs[0].Queue)
	assert.Equal(t, []byte("payload"), jobs[0].Payload)
	assert.NotEmpty(t, jobs[0].LeaseID)

	// Stats should show 1 inflight
	ready, inflight, dlq, err := mgr.Stats("test-queue")
	require.NoError(t, err)
	assert.Equal(t, 0, ready)
	assert.Equal(t, 1, inflight)
	assert.Equal(t, 0, dlq)

	// Ack the job
	err = mgr.Ack(jobs[0].ID, jobs[0].LeaseID)
	require.NoError(t, err)

	// Stats should now be empty
	ready, inflight, dlq, err = mgr.Stats("test-queue")
	require.NoError(t, err)
	assert.Equal(t, 0, ready)
	assert.Equal(t, 0, inflight)
	assert.Equal(t, 0, dlq)
}

func TestPriorityOrdering(t *testing.T) {
	dir := t.TempDir()

	walInst, err := wal.New(wal.Config{
		Dir:         dir + "/wal",
		SegmentSize: 1024,
		Fsync:       false,
	})
	require.NoError(t, err)
	defer walInst.Close()

	storeInst, err := store.New(dir + "/store")
	require.NoError(t, err)
	defer storeInst.Close()

	mgr := NewManager(storeInst, walInst)
	err = mgr.Start()
	require.NoError(t, err)
	defer mgr.Stop()

	// Enqueue jobs with different priorities
	_, err = mgr.Enqueue("test", []byte("low"), nil, 2, 0, DefaultRetryPolicy(), "")
	require.NoError(t, err)
	_, err = mgr.Enqueue("test", []byte("high"), nil, 9, 0, DefaultRetryPolicy(), "")
	require.NoError(t, err)
	_, err = mgr.Enqueue("test", []byte("medium"), nil, 5, 0, DefaultRetryPolicy(), "")
	require.NoError(t, err)

	// Lease should return high priority first
	jobs, err := mgr.Lease("test", 3, 30000)
	require.NoError(t, err)
	require.Len(t, jobs, 3)

	assert.Equal(t, uint8(9), jobs[0].Priority)
	assert.Equal(t, uint8(5), jobs[1].Priority)
	assert.Equal(t, uint8(2), jobs[2].Priority)
}

func TestDelayedJobs(t *testing.T) {
	dir := t.TempDir()

	walInst, err := wal.New(wal.Config{
		Dir:         dir + "/wal",
		SegmentSize: 1024,
		Fsync:       false,
	})
	require.NoError(t, err)
	defer walInst.Close()

	storeInst, err := store.New(dir + "/store")
	require.NoError(t, err)
	defer storeInst.Close()

	mgr := NewManager(storeInst, walInst)
	err = mgr.Start()
	require.NoError(t, err)
	defer mgr.Stop()

	// Enqueue a delayed job (500ms delay)
	_, err = mgr.Enqueue("test", []byte("delayed"), nil, 5, 500, DefaultRetryPolicy(), "")
	require.NoError(t, err)

	// Should not be available immediately
	jobs, err := mgr.Lease("test", 1, 30000)
	require.NoError(t, err)
	assert.Len(t, jobs, 0)

	// Wait for delay to pass
	time.Sleep(600 * time.Millisecond)

	// Should now be available
	jobs, err = mgr.Lease("test", 1, 30000)
	require.NoError(t, err)
	assert.Len(t, jobs, 1)
}

func TestRetryAndDLQ(t *testing.T) {
	dir := t.TempDir()

	walInst, err := wal.New(wal.Config{
		Dir:         dir + "/wal",
		SegmentSize: 1024,
		Fsync:       false,
	})
	require.NoError(t, err)
	defer walInst.Close()

	storeInst, err := store.New(dir + "/store")
	require.NoError(t, err)
	defer storeInst.Close()

	mgr := NewManager(storeInst, walInst)
	err = mgr.Start()
	require.NoError(t, err)
	defer mgr.Stop()

	// Enqueue with max 2 retries
	retryPolicy := RetryPolicy{MaxRetries: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 1 * time.Second}
	_, err = mgr.Enqueue("test", []byte("retry-test"), nil, 5, 0, retryPolicy, "")
	require.NoError(t, err)

	// Lease and nack 3 times
	for i := 0; i < 3; i++ {
		jobs, err := mgr.Lease("test", 1, 30000)
		require.NoError(t, err)
		
		if i < 2 {
			// First 2 nacks should requeue
			require.Len(t, jobs, 1)
			err = mgr.Nack(jobs[0].ID, jobs[0].LeaseID, "test failure")
			require.NoError(t, err)
			
			// Wait for backoff
			time.Sleep(50 * time.Millisecond)
		} else {
			// Third nack should move to DLQ
			require.Len(t, jobs, 1)
			err = mgr.Nack(jobs[0].ID, jobs[0].LeaseID, "test failure")
			require.NoError(t, err)
		}
	}

	// Should be in DLQ now
	ready, inflight, dlq, err := mgr.Stats("test")
	require.NoError(t, err)
	assert.Equal(t, 0, ready)
	assert.Equal(t, 0, inflight)
	assert.Equal(t, 1, dlq)

	// Should not be leaseable
	jobs, err := mgr.Lease("test", 1, 30000)
	require.NoError(t, err)
	assert.Len(t, jobs, 0)
}

func TestIdempotency(t *testing.T) {
	dir := t.TempDir()

	walInst, err := wal.New(wal.Config{
		Dir:         dir + "/wal",
		SegmentSize: 1024,
		Fsync:       false,
	})
	require.NoError(t, err)
	defer walInst.Close()

	storeInst, err := store.New(dir + "/store")
	require.NoError(t, err)
	defer storeInst.Close()

	mgr := NewManager(storeInst, walInst)
	err = mgr.Start()
	require.NoError(t, err)
	defer mgr.Stop()

	// Enqueue with idempotency key
	jobID1, err := mgr.Enqueue("test", []byte("payload"), nil, 5, 0, DefaultRetryPolicy(), "idempotency-key-123")
	require.NoError(t, err)

	// Enqueue again with same key
	jobID2, err := mgr.Enqueue("test", []byte("different-payload"), nil, 7, 0, DefaultRetryPolicy(), "idempotency-key-123")
	require.NoError(t, err)

	// Should return same job ID
	assert.Equal(t, jobID1, jobID2)

	// Should only have 1 job
	ready, _, _, err := mgr.Stats("test")
	require.NoError(t, err)
	assert.Equal(t, 1, ready)
}

func TestWALReplay(t *testing.T) {
	dir := t.TempDir()

	// Create and populate
	walInst, err := wal.New(wal.Config{
		Dir:         dir + "/wal",
		SegmentSize: 1024,
		Fsync:       false,
	})
	require.NoError(t, err)

	storeInst, err := store.New(dir + "/store")
	require.NoError(t, err)

	mgr := NewManager(storeInst, walInst)
	err = mgr.Start()
	require.NoError(t, err)

	// Enqueue some jobs
	_, err = mgr.Enqueue("test", []byte("job1"), nil, 5, 0, DefaultRetryPolicy(), "")
	require.NoError(t, err)
	_, err = mgr.Enqueue("test", []byte("job2"), nil, 7, 0, DefaultRetryPolicy(), "")
	require.NoError(t, err)

	ready, _, _, _ := mgr.Stats("test")
	assert.Equal(t, 2, ready)

	// Close
	mgr.Stop()
	walInst.Close()
	storeInst.Close()

	// Reopen and replay
	walInst2, err := wal.New(wal.Config{
		Dir:         dir + "/wal",
		SegmentSize: 1024,
		Fsync:       false,
	})
	require.NoError(t, err)
	defer walInst2.Close()

	storeInst2, err := store.New(dir + "/store")
	require.NoError(t, err)
	defer storeInst2.Close()

	mgr2 := NewManager(storeInst2, walInst2)
	err = mgr2.Start()
	require.NoError(t, err)
	defer mgr2.Stop()

	// Should have same jobs
	ready, _, _, _ = mgr2.Stats("test")
	assert.Equal(t, 2, ready)
}
