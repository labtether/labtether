package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apikeys"
)

func TestAPIKeyStore_CreateAndLookup(t *testing.T) {
	store := NewMemoryAPIKeyStore()
	ctx := context.Background()

	key := apikeys.APIKey{
		ID:            "key_test1",
		Name:          "test-key",
		Prefix:        "ab12",
		SecretHash:    "deadbeef0123456789abcdef0123456789abcdef0123456789abcdef01234567",
		Role:          "operator",
		Scopes:        []string{"assets:read", "assets:exec"},
		AllowedAssets: []string{"server1"},
		CreatedBy:     "admin",
		CreatedAt:     time.Now().UTC(),
	}

	if err := store.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	got, ok, err := store.LookupAPIKeyByHash(ctx, key.SecretHash)
	if err != nil {
		t.Fatalf("LookupAPIKeyByHash: %v", err)
	}
	if !ok {
		t.Fatal("key should be found")
	}
	if got.Name != "test-key" {
		t.Errorf("name = %q, want test-key", got.Name)
	}
	if got.Role != "operator" {
		t.Errorf("role = %q, want operator", got.Role)
	}
	if len(got.Scopes) != 2 {
		t.Errorf("scopes len = %d, want 2", len(got.Scopes))
	}
}

func TestAPIKeyStore_ListAndDelete(t *testing.T) {
	store := NewMemoryAPIKeyStore()
	ctx := context.Background()

	key := apikeys.APIKey{
		ID:         "key_list1",
		Name:       "list-test",
		Prefix:     "cd34",
		SecretHash: "aaaa000011112222333344445555666677778888",
		Role:       "viewer",
		Scopes:     []string{"assets:read"},
		CreatedBy:  "admin",
		CreatedAt:  time.Now().UTC(),
	}
	_ = store.CreateAPIKey(ctx, key)

	keys, err := store.ListAPIKeys(ctx)
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	found := false
	for _, k := range keys {
		if k.ID == "key_list1" {
			found = true
		}
	}
	if !found {
		t.Error("created key should appear in list")
	}

	if err := store.DeleteAPIKey(ctx, "key_list1"); err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}

	_, ok, err := store.LookupAPIKeyByHash(ctx, key.SecretHash)
	if err != nil {
		t.Fatalf("LookupAPIKeyByHash after delete: %v", err)
	}
	if ok {
		t.Error("key should not be found after delete")
	}
}

func TestAPIKeyStore_TouchLastUsed(t *testing.T) {
	store := NewMemoryAPIKeyStore()
	ctx := context.Background()

	key := apikeys.APIKey{
		ID:         "key_touch1",
		Name:       "touch-test",
		Prefix:     "ef56",
		SecretHash: "bbbb000011112222333344445555666677778888",
		Role:       "operator",
		Scopes:     []string{"*"},
		CreatedBy:  "admin",
		CreatedAt:  time.Now().UTC(),
	}
	_ = store.CreateAPIKey(ctx, key)

	if err := store.TouchAPIKeyLastUsed(ctx, "key_touch1"); err != nil {
		t.Fatalf("TouchAPIKeyLastUsed: %v", err)
	}

	got, _, _ := store.LookupAPIKeyByHash(ctx, key.SecretHash)
	if got.LastUsedAt == nil {
		t.Error("last_used_at should be set after touch")
	}
}
