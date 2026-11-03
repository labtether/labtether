package persistence

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Owner decision 2026-11-03: a removed device can be added again, but only
// with a new single-use token made after the removal. Its old agent token
// stays revoked. Before, the name was refused for good.
func TestMemoryRemovedDeviceComesBackOnlyWithANewSingleUseToken(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryEnrollmentStore(NewMemoryAssetStore())
	now := time.Now().UTC()
	enroll := func(tokenHash, agentHash string) (AgentEnrollmentCommitResult, error) {
		return store.CommitAgentEnrollment(ctx, AgentEnrollmentCommitRequest{
			AssetID: "node-a", Hostname: "node-a", EnrollmentTokenHash: tokenHash,
			AgentTokenHash: agentHash, AgentTokenExpiresAt: now.Add(time.Hour),
		})
	}
	mustToken := func(hash string, maxUses int, expiresAt time.Time) {
		t.Helper()
		if _, err := store.CreateEnrollmentToken(testEnrollmentTokenParams(hash, hash, expiresAt, maxUses)); err != nil {
			t.Fatal(err)
		}
	}
	mustToken("first", 1, now.Add(time.Hour))
	mustToken("made-before-removal", 1, now.Add(time.Hour))
	first, err := enroll("first", "old-agent")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := store.DecommissionAgentAsset(ctx, "node-a"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	mustToken("multi-use", 5, now.Add(time.Hour))
	mustToken("fresh", 1, now.Add(time.Hour))

	for _, hash := range []string{"multi-use", "made-before-removal"} {
		if _, err := enroll(hash, "agent-"+hash); !errors.Is(err, ErrAgentIdentityRetired) {
			t.Fatalf("%s token re-admitted a removed device: err=%v", hash, err)
		}
	}
	back, err := enroll("fresh", "new-agent")
	if err != nil || !back.Readmitted {
		t.Fatalf("fresh single-use token did not re-admit: result=%+v err=%v", back, err)
	}
	if retired, err := store.IsAgentIdentityRetired(ctx, "node-a"); err != nil || retired {
		t.Fatalf("still retired after re-admission: %v %v", retired, err)
	}
	if err := store.ValidateActiveAgentTokenID(ctx, first.AgentToken.ID, "node-a"); err == nil {
		t.Fatal("the removed device's old agent token works again")
	}
	if err := store.ValidateActiveAgentTokenID(ctx, back.AgentToken.ID, "node-a"); err != nil {
		t.Fatalf("new agent token: %v", err)
	}
	if _, err := enroll("fresh", "again"); !errors.Is(err, ErrEnrollmentTokenUsed) {
		t.Fatalf("reused token: err=%v, want ErrEnrollmentTokenUsed", err)
	}
	mustToken("expired", 1, now.Add(time.Millisecond))
	time.Sleep(5 * time.Millisecond)
	if _, err := enroll("expired", "late"); !errors.Is(err, ErrEnrollmentTokenExpired) || !errors.Is(err, ErrEnrollmentTokenInvalid) {
		t.Fatalf("expired token: err=%v, want ErrEnrollmentTokenExpired", err)
	}
}
