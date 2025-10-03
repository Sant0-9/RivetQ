package backoff

import (
	"math"
	"math/rand"
	"time"
)

// Config for exponential backoff
type Config struct {
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Multiplier float64
	Jitter     float64 // Jitter factor (0.0 to 1.0)
}

// DefaultConfig returns default backoff configuration
func DefaultConfig() Config {
	return Config{
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   60 * time.Second,
		Multiplier: 2.0,
		Jitter:     0.1, // ±10% jitter
	}
}

// Calculate computes the backoff delay for a given attempt
// Formula: min(base * multiplier^attempt, maxDelay) + jitter
func Calculate(cfg Config, attempt uint32) time.Duration {
	if attempt == 0 {
		return 0
	}

	// Calculate exponential backoff
	delay := float64(cfg.BaseDelay) * math.Pow(cfg.Multiplier, float64(attempt-1))

	// Cap at max delay
	if delay > float64(cfg.MaxDelay) {
		delay = float64(cfg.MaxDelay)
	}

	// Add jitter (±jitter%)
	if cfg.Jitter > 0 {
		jitterRange := delay * cfg.Jitter
		jitterDelta := (rand.Float64()*2 - 1) * jitterRange // -jitterRange to +jitterRange
		delay += jitterDelta
	}

	// Ensure non-negative
	if delay < 0 {
		delay = 0
	}

	return time.Duration(delay)
}

// CalculateDefault calculates backoff with default config
func CalculateDefault(attempt uint32) time.Duration {
	return Calculate(DefaultConfig(), attempt)
}
