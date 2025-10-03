package ratelimit

import (
	"sync"
	"time"
)

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	mu           sync.Mutex
	capacity     float64
	tokens       float64
	refillRate   float64 // tokens per second
	lastRefill   time.Time
	enabled      bool
}

// NewTokenBucket creates a new token bucket rate limiter
// capacity: maximum number of tokens
// refillRate: tokens added per second
func NewTokenBucket(capacity, refillRate float64) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
		enabled:    capacity > 0 && refillRate > 0,
	}
}

// Allow checks if an operation is allowed under the rate limit
func (tb *TokenBucket) Allow() bool {
	return tb.AllowN(1)
}

// AllowN checks if N operations are allowed
func (tb *TokenBucket) AllowN(n float64) bool {
	if !tb.enabled {
		return true
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= n {
		tb.tokens -= n
		return true
	}

	return false
}

// refill adds tokens based on elapsed time
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()

	// Add tokens based on elapsed time
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	tb.lastRefill = now
}

// SetRate updates the rate limit parameters
func (tb *TokenBucket) SetRate(capacity, refillRate float64) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill() // Refill with old rate first

	tb.capacity = capacity
	tb.refillRate = refillRate
	tb.enabled = capacity > 0 && refillRate > 0

	// Adjust current tokens if needed
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}
}

// GetRate returns current rate limit settings
func (tb *TokenBucket) GetRate() (capacity, refillRate float64) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.capacity, tb.refillRate
}

// Tokens returns the current number of available tokens
func (tb *TokenBucket) Tokens() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	return tb.tokens
}

// Limiter manages rate limiters for multiple queues
type Limiter struct {
	mu      sync.RWMutex
	buckets map[string]*TokenBucket
}

// NewLimiter creates a new rate limiter manager
func NewLimiter() *Limiter {
	return &Limiter{
		buckets: make(map[string]*TokenBucket),
	}
}

// Allow checks if operation is allowed for a queue
func (l *Limiter) Allow(queue string) bool {
	l.mu.RLock()
	bucket, exists := l.buckets[queue]
	l.mu.RUnlock()

	if !exists {
		return true // No limit set
	}

	return bucket.Allow()
}

// SetRate sets rate limit for a queue
func (l *Limiter) SetRate(queue string, capacity, refillRate float64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, exists := l.buckets[queue]
	if !exists {
		bucket = NewTokenBucket(capacity, refillRate)
		l.buckets[queue] = bucket
	} else {
		bucket.SetRate(capacity, refillRate)
	}
}

// GetRate gets rate limit for a queue
func (l *Limiter) GetRate(queue string) (capacity, refillRate float64, exists bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	bucket, exists := l.buckets[queue]
	if !exists {
		return 0, 0, false
	}

	capacity, refillRate = bucket.GetRate()
	return capacity, refillRate, true
}

// Tokens returns available tokens for a queue
func (l *Limiter) Tokens(queue string) float64 {
	l.mu.RLock()
	bucket, exists := l.buckets[queue]
	l.mu.RUnlock()

	if !exists {
		return -1 // No limit
	}

	return bucket.Tokens()
}
