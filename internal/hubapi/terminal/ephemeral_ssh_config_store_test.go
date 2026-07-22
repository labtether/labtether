package terminal

import (
	"testing"
	"time"

	terminalmodel "github.com/labtether/labtether/internal/terminal"
)

func TestEphemeralSSHConfigStoreCopiesExpiresAndDeletes(t *testing.T) {
	now := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	store := NewEphemeralSSHConfigStore(time.Minute)
	store.now = func() time.Time { return now }
	original := &terminalmodel.SSHConfig{Host: "host.example", Port: 22, User: "qa", Password: "memory-only-secret", StrictHostKey: true}
	if err := store.Put("session-1", original); err != nil {
		t.Fatal(err)
	}
	original.Password = "mutated-after-put"

	first, ok := store.Get("session-1")
	if !ok || first.Password != "memory-only-secret" {
		t.Fatalf("stored config was not copied: ok=%t config=%#v", ok, first)
	}
	first.Password = "mutated-after-get"
	second, ok := store.Get("session-1")
	if !ok || second.Password != "memory-only-secret" {
		t.Fatal("returned config mutation changed the stored credential")
	}

	now = now.Add(2 * time.Minute)
	if _, ok := store.Get("session-1"); ok {
		t.Fatal("expired credential remained retrievable")
	}

	if err := store.Put("session-2", &terminalmodel.SSHConfig{Password: "delete-me"}); err != nil {
		t.Fatal(err)
	}
	store.Delete("session-2")
	if _, ok := store.Get("session-2"); ok {
		t.Fatal("deleted credential remained retrievable")
	}
}
