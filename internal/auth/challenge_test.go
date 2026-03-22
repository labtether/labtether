package auth

import (
	"testing"
	"time"
)

func TestChallengeStore_CreateAndConsume(t *testing.T) {
	store := NewChallengeStore()
	token := store.Create("usr_123")

	userID, ok := store.Validate(token)
	if !ok {
		t.Fatal("expected valid challenge")
	}
	if userID != "usr_123" {
		t.Errorf("userID = %q, want usr_123", userID)
	}

	userID2, ok2 := store.Consume(token)
	if !ok2 || userID2 != "usr_123" {
		t.Error("Consume should succeed")
	}

	_, ok3 := store.Consume(token)
	if ok3 {
		t.Error("second Consume should fail")
	}
}

func TestChallengeStore_ConsumeIsOneShot(t *testing.T) {
	store := NewChallengeStore()
	token := store.Create("usr_456")

	// First Consume succeeds.
	userID, ok := store.Consume(token)
	if !ok || userID != "usr_456" {
		t.Fatal("first Consume should succeed")
	}

	// Second Consume (even immediately after) must fail — token is spent.
	_, ok2 := store.Consume(token)
	if ok2 {
		t.Error("second Consume on same token should fail")
	}

	// Validate also fails on a consumed token.
	_, ok3 := store.Validate(token)
	if ok3 {
		t.Error("Validate on consumed token should fail")
	}
}

func TestChallengeStore_RevokeForUser(t *testing.T) {
	store := NewChallengeStore()
	t1 := store.Create("usr_A")
	t2 := store.Create("usr_A")
	t3 := store.Create("usr_B")

	store.RevokeForUser("usr_A")

	if _, ok := store.Validate(t1); ok {
		t.Error("t1 should be revoked")
	}
	if _, ok := store.Validate(t2); ok {
		t.Error("t2 should be revoked")
	}
	if _, ok := store.Validate(t3); !ok {
		t.Error("t3 for usr_B should still be valid")
	}
}

func TestChallengeStore_Expiry(t *testing.T) {
	store := NewChallengeStore()
	token := store.Create("usr_789")

	store.mu.Lock()
	ch := store.challenges[token]
	ch.ExpiresAt = time.Now().Add(-time.Second)
	store.challenges[token] = ch
	store.mu.Unlock()

	_, ok := store.Validate(token)
	if ok {
		t.Error("expired challenge should not validate")
	}
}
