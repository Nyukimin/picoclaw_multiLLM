package main

import (
	"testing"
	"time"
)

func TestClaimLLMWorkerRecoveryThrottlesByAlias(t *testing.T) {
	llmWorkerRecoveryState.Lock()
	llmWorkerRecoveryState.last = map[string]time.Time{}
	llmWorkerRecoveryState.Unlock()

	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	if !claimLLMWorkerRecovery("Worker", now) {
		t.Fatal("first recovery claim should pass")
	}
	if claimLLMWorkerRecovery("Worker", now.Add(time.Second)) {
		t.Fatal("second recovery claim inside throttle window should be suppressed")
	}
	if !claimLLMWorkerRecovery("Worker", now.Add(llmWorkerRecoveryThrottle+time.Second)) {
		t.Fatal("recovery claim after throttle window should pass")
	}
}
