package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTokenBucketAllow(t *testing.T) {
	tb := NewTokenBucket(10, 1) // 10 capacity, 1 token/sec

	// Should allow up to capacity
	for i := 0; i < 10; i++ {
		assert.True(t, tb.Allow())
	}

	// Should reject after capacity exhausted
	assert.False(t, tb.Allow())
}

func TestTokenBucketRefill(t *testing.T) {
	tb := NewTokenBucket(10, 10) // 10 capacity, 10 tokens/sec

	// Consume all tokens
	for i := 0; i < 10; i++ {
		tb.Allow()
	}

	// Should reject
	assert.False(t, tb.Allow())

	// Wait for refill
	time.Sleep(200 * time.Millisecond) // Should refill ~2 tokens

	// Should allow again
	assert.True(t, tb.Allow())
}

func TestTokenBucketNoLimit(t *testing.T) {
	tb := NewTokenBucket(0, 0) // No limit

	// Should always allow
	for i := 0; i < 100; i++ {
		assert.True(t, tb.Allow())
	}
}

func TestLimiter(t *testing.T) {
	limiter := NewLimiter()

	// No limit by default
	assert.True(t, limiter.Allow("queue1"))

	// Set limit
	limiter.SetRate("queue1", 5, 1)

	// Should allow up to capacity
	for i := 0; i < 5; i++ {
		assert.True(t, limiter.Allow("queue1"))
	}

	// Should reject
	assert.False(t, limiter.Allow("queue1"))

	// Other queues should not be affected
	assert.True(t, limiter.Allow("queue2"))
}
