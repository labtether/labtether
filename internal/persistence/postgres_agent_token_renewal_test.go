package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
)

// Agents have no way to fetch a new credential, so a token that is never
// extended takes every agent offline when it expires. Heartbeats must slide
// the expiry forward for live credentials only.
func TestPostgresAuthenticatedHeartbeatSlidesTokenExpiry(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	assetID := "token-renewal-agent-" + suffix
	tokenHash := "token-renewal-hash-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
		_, _ = store.pool.Exec(ctx, `DELETE FROM assets WHERE id = $1`, assetID)
	})
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID, Type: "node", Name: assetID, Source: "agent",
	}); err != nil {
		t.Fatal(err)
	}
	token, err := store.CreateAgentToken(assetID, tokenHash, "test", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	heartbeat := func(ttl time.Duration) error {
		_, err := store.CommitAuthenticatedAgentHeartbeat(ctx, token.ID, ttl, assets.HeartbeatRequest{
			AssetID: assetID, Type: "node", Name: assetID, Source: "agent",
		})
		return err
	}
	expiresAt := func() time.Time {
		var got time.Time
		if err := store.pool.QueryRow(ctx, `SELECT expires_at FROM agent_tokens WHERE id = $1`, token.ID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}

	const ttl = 30 * 24 * time.Hour
	if err := heartbeat(ttl); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	renewed := expiresAt()
	if renewed.Before(time.Now().Add(ttl - time.Hour)) {
		t.Fatalf("expiry after heartbeat=%s, want about %s from now", renewed, ttl)
	}
	if err := heartbeat(time.Minute); err != nil {
		t.Fatalf("short-TTL heartbeat: %v", err)
	}
	if got := expiresAt(); got.Before(renewed) {
		t.Fatalf("heartbeat shortened expiry from %s to %s", renewed, got)
	}

	if err := store.RevokeAgentToken(token.ID); err != nil {
		t.Fatal(err)
	}
	before := expiresAt()
	if err := heartbeat(2 * ttl); !errors.Is(err, ErrAgentCredentialInactive) {
		t.Fatalf("revoked token heartbeat error=%v", err)
	}
	if got := expiresAt(); !got.Equal(before) {
		t.Fatalf("revoked token expiry moved from %s to %s", before, got)
	}
}
