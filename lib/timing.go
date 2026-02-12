package lib

import (
	"fmt"
	"math"
	"time"
)

// TimingConfig holds configurable tolerance settings for timing assertions.
type TimingConfig struct {
	// TolerancePct is the percentage tolerance for approximate timing assertions.
	// Default: 50 (meaning +-50% of expected value)
	TolerancePct float64

	// MinToleranceMs is the minimum absolute tolerance in milliseconds.
	// Ensures very small timing values still have reasonable tolerance.
	// Default: 100
	MinToleranceMs float64

	// MaxWaitMs is the maximum wait time for delayed assertions.
	// Default: 30000 (30 seconds)
	MaxWaitMs int64
}

// DefaultTimingConfig returns default timing configuration.
func DefaultTimingConfig() TimingConfig {
	return TimingConfig{
		TolerancePct:   50,
		MinToleranceMs: 100,
		MaxWaitMs:      30000,
	}
}

// AssertApproximateDuration checks if a duration is approximately equal to an expected value.
func (tc TimingConfig) AssertApproximateDuration(expected time.Duration, actual time.Duration) error {
	expectedMs := float64(expected.Milliseconds())
	actualMs := float64(actual.Milliseconds())

	tolerance := math.Max(
		expectedMs*tc.TolerancePct/100.0,
		tc.MinToleranceMs,
	)

	diff := math.Abs(actualMs - expectedMs)
	if diff > tolerance {
		return fmt.Errorf(
			"timing assertion failed: expected ~%dms (tolerance: %.0fms), got %dms (diff: %.0fms)",
			expected.Milliseconds(), tolerance, actual.Milliseconds(), diff,
		)
	}
	return nil
}

// AssertApproximateMs checks if a millisecond value is approximately equal.
func (tc TimingConfig) AssertApproximateMs(expectedMs float64, actualMs float64) error {
	tolerance := math.Max(
		expectedMs*tc.TolerancePct/100.0,
		tc.MinToleranceMs,
	)

	diff := math.Abs(actualMs - expectedMs)
	if diff > tolerance {
		return fmt.Errorf(
			"timing assertion failed: expected ~%.0fms (tolerance: %.0fms), got %.0fms (diff: %.0fms)",
			expectedMs, tolerance, actualMs, diff,
		)
	}
	return nil
}

// AssertLessThan checks if a duration is less than a threshold.
func AssertLessThan(threshold time.Duration, actual time.Duration) error {
	if actual >= threshold {
		return fmt.Errorf(
			"timing assertion failed: expected < %dms, got %dms",
			threshold.Milliseconds(), actual.Milliseconds(),
		)
	}
	return nil
}

// AssertGreaterThan checks if a duration is greater than a threshold.
func AssertGreaterThan(threshold time.Duration, actual time.Duration) error {
	if actual <= threshold {
		return fmt.Errorf(
			"timing assertion failed: expected > %dms, got %dms",
			threshold.Milliseconds(), actual.Milliseconds(),
		)
	}
	return nil
}

// WaitForCondition polls a check function until it passes or timeout is reached.
func WaitForCondition(timeout time.Duration, interval time.Duration, check func() error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		lastErr = check()
		if lastErr == nil {
			return nil
		}
		time.Sleep(interval)
	}

	return fmt.Errorf("condition not met within %v: %w", timeout, lastErr)
}
