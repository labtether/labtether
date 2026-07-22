package persistence

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/labtether/labtether/internal/auth"
)

func TestMemoryAuthStoreConsumesRecoveryCodeOnceUnderConcurrency(t *testing.T) {
	store := NewMemoryAuthStore()
	user, err := store.CreateUserWithRole("recovery-user", "unused", auth.RoleViewer, "local", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	const recoveryCode = "12345678-90abcdef"
	hash, err := auth.HashRecoveryCode(recoveryCode)
	if err != nil {
		t.Fatalf("hash recovery code: %v", err)
	}
	encoded, err := json.Marshal([]string{hash})
	if err != nil {
		t.Fatalf("encode recovery codes: %v", err)
	}
	if err := store.ConfirmUserTOTP(user.ID, string(encoded)); err != nil {
		t.Fatalf("seed recovery code: %v", err)
	}

	var consumed atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, consumeErr := store.ConsumeRecoveryCode(user.ID, recoveryCode)
			if consumeErr != nil {
				t.Errorf("consume recovery code: %v", consumeErr)
				return
			}
			if ok {
				consumed.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := consumed.Load(); got != 1 {
		t.Fatalf("successful recovery-code consumptions = %d, want 1", got)
	}
}
