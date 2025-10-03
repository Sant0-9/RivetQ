package backoff

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculate(t *testing.T) {
	cfg := Config{
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   10 * time.Second,
		Multiplier: 2.0,
		Jitter:     0, // No jitter for predictable tests
	}

	tests := []struct {
		attempt  uint32
		expected time.Duration
	}{
		{0, 0},
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, 1600 * time.Millisecond},
	}

	for _, tt := range tests {
		result := Calculate(cfg, tt.attempt)
		assert.Equal(t, tt.expected, result, "attempt %d", tt.attempt)
	}
}

func TestCalculateMaxDelay(t *testing.T) {
	cfg := Config{
		BaseDelay:  1 * time.Second,
		MaxDelay:   5 * time.Second,
		Multiplier: 2.0,
		Jitter:     0,
	}

	// Should cap at max delay
	result := Calculate(cfg, 10)
	assert.Equal(t, 5*time.Second, result)
}

func TestCalculateWithJitter(t *testing.T) {
	cfg := Config{
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   10 * time.Second,
		Multiplier: 2.0,
		Jitter:     0.1, // 10% jitter
	}

	// Run multiple times to check jitter variance
	results := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		results[i] = Calculate(cfg, 3)
	}

	// Should have variance (not all the same)
	allSame := true
	first := results[0]
	for _, r := range results[1:] {
		if r != first {
			allSame = false
			break
		}
	}
	assert.False(t, allSame, "results should vary due to jitter")

	// All should be within jitter range of expected (400ms ± 10%)
	expected := 400 * time.Millisecond
	minExpected := float64(expected) * 0.9
	maxExpected := float64(expected) * 1.1

	for _, r := range results {
		assert.GreaterOrEqual(t, float64(r), minExpected)
		assert.LessOrEqual(t, float64(r), maxExpected)
	}
}
