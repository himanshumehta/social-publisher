package service

import (
	"testing"
	"time"
)

func newTestCB(threshold int, resetAfter time.Duration) *circuitBreaker {
	return &circuitBreaker{
		threshold:  threshold,
		resetAfter: resetAfter,
	}
}

func TestCircuitBreaker_StartsClosed(t *testing.T) {
	cb := newTestCB(3, time.Second)
	if cb.isOpen() {
		t.Fatal("expected circuit breaker to start closed (isOpen=false)")
	}
}

func TestCircuitBreaker_OpensAfterThresholdFailures(t *testing.T) {
	cb := newTestCB(3, time.Second)

	for i := 0; i < 3; i++ {
		cb.recordFailure()
	}

	if !cb.isOpen() {
		t.Fatal("expected circuit breaker to be open after reaching failure threshold")
	}
}

func TestCircuitBreaker_ClosesAfterResetDuration(t *testing.T) {
	cb := newTestCB(3, time.Millisecond)

	for i := 0; i < 3; i++ {
		cb.recordFailure()
	}

	if !cb.isOpen() {
		t.Fatal("expected circuit breaker to be open immediately after failures")
	}

	time.Sleep(5 * time.Millisecond)

	if cb.isOpen() {
		t.Fatal("expected circuit breaker to be closed after resetAfter duration elapsed")
	}
}

func TestCircuitBreaker_RecordSuccessResetsFailures(t *testing.T) {
	cb := newTestCB(3, time.Second)

	cb.recordFailure()
	cb.recordFailure()
	cb.recordSuccess()

	// After reset, even one more failure should not open the circuit.
	cb.recordFailure()
	if cb.isOpen() {
		t.Fatal("expected circuit breaker to remain closed after success reset and one new failure")
	}
}

func TestCircuitBreaker_SingleFailureDoesNotOpen(t *testing.T) {
	cb := newTestCB(3, time.Second)

	cb.recordFailure()
	if cb.isOpen() {
		t.Fatal("expected circuit breaker to remain closed after a single failure")
	}
}
