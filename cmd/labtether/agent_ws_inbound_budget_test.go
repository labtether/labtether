package main

import (
	"testing"
	"time"
)

// A fast agent stream (a large file download) must be slowed to the
// configured rate, not disconnected; only a frame that can never fit is refused.
func TestAgentWSInboundBudgetThrottlesStreamAndRefusesImpossibleFrame(t *testing.T) {
	clock := time.Unix(1_700_000_000, 0)
	budget := &agentWSInboundBudget{
		messagesPerSecond: 1000, messageBurst: 1000,
		bytesPerSecond: 1024, byteBurst: 2048,
		messageTokens: 1000, byteTokens: 2048,
		lastRefill: clock,
	}
	start := clock
	for frame := 0; frame < 10; frame++ {
		wait, ok := budget.reserve(1024, clock)
		if !ok {
			t.Fatalf("frame %d was refused; a stream above the rate must be delayed", frame)
		}
		if wait > maxAgentWSInboundWait {
			t.Fatalf("frame %d wait %s exceeds the bound", frame, wait)
		}
		clock = clock.Add(wait)
	}
	// 10 KiB at 1 KiB/s with a 2 KiB burst takes 8 s to pay for.
	if elapsed := clock.Sub(start); elapsed < 8*time.Second-time.Millisecond || elapsed > 8*time.Second+time.Millisecond {
		t.Fatalf("stream took %s, want 8s at the configured rate", elapsed)
	}

	if _, ok := budget.reserve(2049, clock.Add(time.Hour)); ok {
		t.Fatal("a frame larger than the byte burst was admitted")
	}
	slow := &agentWSInboundBudget{
		messagesPerSecond: 1, messageBurst: 1, bytesPerSecond: 1, byteBurst: 1 << 20,
		messageTokens: 1, byteTokens: 1 << 20, lastRefill: clock,
	}
	if _, ok := slow.reserve(64<<10, clock); ok {
		t.Fatal("a frame needing a pause past the read deadline was admitted")
	}
}
