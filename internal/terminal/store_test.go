package terminal

import "testing"

func TestStoreCreateSessionAndCommand(t *testing.T) {
	store := NewStore()

	session := store.CreateSession(CreateSessionRequest{ActorID: "owner", Target: "host-1", Mode: "interactive"})
	if session.ID == "" {
		t.Fatalf("expected session ID")
	}

	cmd, err := store.AddCommand(session.ID, CreateCommandRequest{ActorID: "owner", Command: "uptime"}, session.Target, session.Mode)
	if err != nil {
		t.Fatalf("expected no error adding command, got: %v", err)
	}

	if cmd.Status != "queued" {
		t.Fatalf("expected queued command status, got: %s", cmd.Status)
	}
}

func TestStoreCreateOrUpdatePersistentSessionReusesActorTarget(t *testing.T) {
	store := NewStore()

	first := store.CreateOrUpdatePersistentSession(CreatePersistentSessionRequest{
		ActorID: "owner",
		Target:  "host-1",
		Title:   "Ops Shell",
	})
	second := store.CreateOrUpdatePersistentSession(CreatePersistentSessionRequest{
		ActorID: "owner",
		Target:  "host-1",
		Title:   "Renamed Ops Shell",
	})

	if first.ID != second.ID {
		t.Fatalf("expected same persistent session ID, got %q and %q", first.ID, second.ID)
	}
	if second.Title != "Renamed Ops Shell" {
		t.Fatalf("expected updated title, got %q", second.Title)
	}
}

func TestStorePersistentSessionAttachDetachLifecycle(t *testing.T) {
	store := NewStore()
	persistent := store.CreateOrUpdatePersistentSession(CreatePersistentSessionRequest{
		ActorID: "owner",
		Target:  "host-2",
		Title:   "Detached Shell",
	})

	attachedAt := persistent.CreatedAt.Add(5)
	attached, err := store.MarkPersistentSessionAttached(persistent.ID, attachedAt)
	if err != nil {
		t.Fatalf("attach failed: %v", err)
	}
	if attached.Status != "attached" {
		t.Fatalf("expected attached status, got %q", attached.Status)
	}
	if attached.LastAttachedAt == nil || !attached.LastAttachedAt.Equal(attachedAt) {
		t.Fatalf("expected attached timestamp %v, got %+v", attachedAt, attached.LastAttachedAt)
	}

	detachedAt := attachedAt.Add(10)
	detached, err := store.MarkPersistentSessionDetached(persistent.ID, detachedAt)
	if err != nil {
		t.Fatalf("detach failed: %v", err)
	}
	if detached.Status != "detached" {
		t.Fatalf("expected detached status, got %q", detached.Status)
	}
	if detached.LastDetachedAt == nil || !detached.LastDetachedAt.Equal(detachedAt) {
		t.Fatalf("expected detached timestamp %v, got %+v", detachedAt, detached.LastDetachedAt)
	}
}
