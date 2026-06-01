package agents

import (
	"math"
	"testing"
	"time"
)

func TestAgentCommandTimeoutFromSecondsBoundsBeforeDurationConversion(t *testing.T) {
	if got := agentCommandTimeoutFromSeconds(0, 30*time.Second); got != 30*time.Second {
		t.Fatalf("agentCommandTimeoutFromSeconds(0) = %s, want 30s", got)
	}
	if got := agentCommandTimeoutFromSeconds(2, 30*time.Second); got != 2*time.Second {
		t.Fatalf("agentCommandTimeoutFromSeconds(2) = %s, want 2s", got)
	}
	if got := agentCommandTimeoutFromSeconds(math.MaxInt, 30*time.Second); got != maxAgentCommandTimeout {
		t.Fatalf("agentCommandTimeoutFromSeconds(MaxInt) = %s, want %s", got, maxAgentCommandTimeout)
	}
}

func TestAgentCommandTimeoutFromSecondsCapsFallback(t *testing.T) {
	got := agentCommandTimeoutFromSeconds(0, 10*time.Minute)
	if got != maxAgentCommandTimeout {
		t.Fatalf("agentCommandTimeoutFromSeconds(fallback) = %s, want %s", got, maxAgentCommandTimeout)
	}
}
