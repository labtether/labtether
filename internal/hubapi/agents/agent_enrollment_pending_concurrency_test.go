package agents

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPendingAgentsTryAddEnforcesCapsAtomically(t *testing.T) {
	registry := NewPendingAgents()
	const attempts = 100
	var admitted atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			if err := registry.TryAdd(&PendingAgent{
				AssetID:  "pending-" + time.Now().Add(time.Duration(index)).Format("150405.000000000"),
				RemoteIP: "192.0.2.10",
			}, 200, 5); err == nil {
				admitted.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if got := admitted.Load(); got != 5 {
		t.Fatalf("admitted=%d, want hard per-IP cap 5", got)
	}
	if got := registry.Count(); got != 5 {
		t.Fatalf("registry count=%d, want 5", got)
	}
}

func TestPendingAgentsDecisionClaimIsExclusive(t *testing.T) {
	registry := NewPendingAgents()
	registry.Add(&PendingAgent{AssetID: "pending-node", IdentityVerified: true})
	const attempts = 32
	var claimed atomic.Int32
	var claim PendingDecisionClaim
	var claimMu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := registry.ClaimDecision("pending-node", true)
			if err == nil {
				claimed.Add(1)
				claimMu.Lock()
				claim = got
				claimMu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := claimed.Load(); got != 1 {
		t.Fatalf("successful claims=%d, want exactly 1", got)
	}
	if !registry.CompleteDecision(claim) {
		t.Fatal("exclusive claim could not complete")
	}
}

func TestEnrollmentHostnameBounds(t *testing.T) {
	if got, ok := validEnrollmentHostname(string(make([]byte, maxEnrollmentHostnameBytes)), false); ok || got != "" {
		t.Fatal("NUL-filled hostname must be rejected")
	}
	valid := make([]byte, maxEnrollmentHostnameBytes)
	for i := range valid {
		valid[i] = 'a'
	}
	if got, ok := validEnrollmentHostname(string(valid), false); !ok || len(got) != maxEnrollmentHostnameBytes {
		t.Fatalf("253-byte hostname rejected: len=%d ok=%v", len(got), ok)
	}
	tooLong := append(valid, 'a')
	if _, ok := validEnrollmentHostname(string(tooLong), false); ok {
		t.Fatal("254-byte hostname accepted")
	}
	if _, ok := validEnrollmentHostname("safe\nunsafe", false); ok {
		t.Fatal("control-character hostname accepted")
	}
}
