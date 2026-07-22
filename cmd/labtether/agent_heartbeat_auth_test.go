package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
)

type blockingAuthenticatedHeartbeatStore struct {
	persistence.EnrollmentStore
	persistence.AgentEnrollmentTransactionStore
	entered chan struct{}
	release chan struct{}
}

func (s *blockingAuthenticatedHeartbeatStore) CommitAuthenticatedAgentHeartbeat(ctx context.Context, tokenID string, req assets.HeartbeatRequest) (assets.Asset, error) {
	close(s.entered)
	select {
	case <-s.release:
	case <-ctx.Done():
		return assets.Asset{}, ctx.Err()
	}
	return s.AgentEnrollmentTransactionStore.CommitAuthenticatedAgentHeartbeat(ctx, tokenID, req)
}

func TestAgentHeartbeatAuthBindsTokenToExactAssetAndRoute(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true

	raw, hash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("generate agent token: %v", err)
	}
	agentToken, err := sut.enrollmentStore.CreateAgentToken("node-allowed", hash, "test", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("create agent token: %v", err)
	}
	trustedVerifiedAt := "2026-07-14T00:00:00Z"
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-allowed", Type: "host", Name: "Test node", Source: "agent", Status: "online", Platform: "linux",
		Metadata: map[string]string{
			assets.MetadataKeyAgentDeviceFingerprint:  "LT-TRUSTED",
			assets.MetadataKeyAgentDeviceKeyAlgorithm: "ed25519",
			assets.MetadataKeyAgentIdentityVerifiedAt: trustedVerifiedAt,
		},
		AllowAgentIdentityRotation: true,
	}); err != nil {
		t.Fatalf("seed token-bound asset: %v", err)
	}

	handlers := sut.buildHTTPHandlers(nil, nil, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/assets/heartbeat", handlers["/assets/heartbeat"])
	mux.HandleFunc("/assets/", handlers["/assets/"])

	heartbeat := func(assetID string) *httptest.ResponseRecorder {
		t.Helper()
		payload := []byte(`{"asset_id":"` + assetID + `","type":"host","name":"Test node","source":"agent","status":"online","platform":"linux","metadata":{"agent_identity_verified_at":"2099-01-01T00:00:00Z"}}`)
		req := httptest.NewRequest(http.MethodPost, "https://labtether.test/assets/heartbeat", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+raw)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := heartbeat("node-allowed"); rec.Code != http.StatusAccepted {
		t.Fatalf("token-bound heartbeat status=%d body=%s, want 202", rec.Code, rec.Body.String())
	}
	if _, ok, err := sut.assetStore.GetAsset("node-allowed"); err != nil || !ok {
		t.Fatalf("token-bound heartbeat asset lookup ok=%v err=%v, want stored asset", ok, err)
	}
	if stored, _, _ := sut.assetStore.GetAsset("node-allowed"); stored.Metadata[assets.MetadataKeyAgentIdentityVerifiedAt] != trustedVerifiedAt {
		t.Fatalf("agent heartbeat forged verified_at=%q", stored.Metadata[assets.MetadataKeyAgentIdentityVerifiedAt])
	}

	if rec := heartbeat("node-other"); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-asset heartbeat status=%d body=%s, want 403", rec.Code, rec.Body.String())
	}
	if _, ok, err := sut.assetStore.GetAsset("node-other"); err != nil || ok {
		t.Fatalf("cross-asset heartbeat lookup ok=%v err=%v, want no stored asset", ok, err)
	}

	assetReq := httptest.NewRequest(http.MethodGet, "https://labtether.test/assets/node-allowed", nil)
	assetReq.Header.Set("Authorization", "Bearer "+raw)
	assetRec := httptest.NewRecorder()
	mux.ServeHTTP(assetRec, assetReq)
	if assetRec.Code != http.StatusUnauthorized {
		t.Fatalf("agent token on non-heartbeat asset route status=%d body=%s, want 401", assetRec.Code, assetRec.Body.String())
	}

	if err := sut.enrollmentStore.RevokeAgentToken(agentToken.ID); err != nil {
		t.Fatalf("revoke agent token: %v", err)
	}
	if rec := heartbeat("node-allowed"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked agent heartbeat status=%d body=%s, want 401", rec.Code, rec.Body.String())
	}
}

func TestAgentHeartbeatAuthRejectsOrphanActiveToken(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	raw, hash, _ := auth.GenerateSessionToken()
	if _, err := sut.enrollmentStore.CreateAgentToken("missing-agent-asset", hash, "legacy", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"asset_id":"missing-agent-asset","type":"host","name":"Missing","source":"agent"}`)
	req := httptest.NewRequest(http.MethodPost, "https://labtether.test/assets/heartbeat", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	sut.withAgentHeartbeatAuth(sut.handleAssetActions)(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("orphan active token heartbeat status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, exists, _ := sut.assetStore.GetAsset("missing-agent-asset"); exists {
		t.Fatal("orphan HTTP agent token recreated asset")
	}
}

func TestAgentHeartbeatAuthIgnoresStaleClientGroup(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	raw, hash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	agentToken, err := sut.enrollmentStore.CreateAgentToken("stale-group-agent", hash, "test", time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "stale-group-agent", Type: "host", Name: "Stale group agent", Source: "agent", Status: "offline",
	}); err != nil {
		t.Fatal(err)
	}

	payload := []byte(`{"asset_id":"stale-group-agent","type":"host","name":"Stale group agent","source":"agent","group_id":"deleted-group","status":"online"}`)
	req := httptest.NewRequest(http.MethodPost, "https://labtether.test/assets/heartbeat", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	sut.withAgentHeartbeatAuth(sut.handleAssetActions)(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("stale-group heartbeat status=%d body=%s, want 202", rec.Code, rec.Body.String())
	}
	stored, exists, err := sut.assetStore.GetAsset("stale-group-agent")
	if err != nil || !exists {
		t.Fatalf("stored asset exists=%v err=%v", exists, err)
	}
	if stored.GroupID != "" || stored.Status != "online" {
		t.Fatalf("stale group affected canonical heartbeat: %+v", stored)
	}
	validated, valid, err := sut.enrollmentStore.ValidateAgentToken(hash)
	if err != nil || !valid || validated.ID != agentToken.ID || validated.LastUsedAt == nil {
		t.Fatalf("agent token was not committed by heartbeat: token=%+v valid=%v err=%v", validated, valid, err)
	}
}

func TestAgentHeartbeatDeleteRaceCannotResurrectAsset(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	underlying := sut.enrollmentStore
	transactions := underlying.(persistence.AgentEnrollmentTransactionStore)
	raw, hash, _ := auth.GenerateSessionToken()
	if _, err := underlying.CreateAgentToken("http-race-node", hash, "legacy", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "http-race-node", Type: "host", Name: "HTTP race", Source: "agent",
	}); err != nil {
		t.Fatal(err)
	}
	wrapped := &blockingAuthenticatedHeartbeatStore{
		EnrollmentStore: underlying, AgentEnrollmentTransactionStore: transactions,
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	sut.enrollmentStore = wrapped
	payload := []byte(`{"asset_id":"http-race-node","type":"host","name":"HTTP race","source":"agent"}`)
	req := httptest.NewRequest(http.MethodPost, "https://labtether.test/assets/heartbeat", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		sut.withAgentHeartbeatAuth(sut.handleAssetActions)(rec, req)
	}()
	select {
	case <-wrapped.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("authenticated HTTP heartbeat did not reach atomic commit")
	}
	if err := transactions.DecommissionAgentAsset(context.Background(), "http-race-node"); err != nil {
		t.Fatal(err)
	}
	close(wrapped.release)
	<-done
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("racing heartbeat status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, exists, _ := sut.assetStore.GetAsset("http-race-node"); exists {
		t.Fatal("HTTP heartbeat resurrected decommissioned asset")
	}
}

func TestAgentHeartbeatAuthRequiresHTTPS(t *testing.T) {
	sut := newTestAPIServer(t)
	raw, hash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("generate agent token: %v", err)
	}
	if _, err := sut.enrollmentStore.CreateAgentToken("node-allowed", hash, "test", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create agent token: %v", err)
	}

	payload := []byte(`{"asset_id":"node-allowed","type":"host","name":"Test node","source":"agent"}`)
	req := httptest.NewRequest(http.MethodPost, "http://labtether.test/assets/heartbeat", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	sut.withAgentHeartbeatAuth(sut.handleAssetActions)(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("plain-HTTP agent heartbeat status=%d body=%s, want 403", rec.Code, rec.Body.String())
	}
}

func TestHeartbeatHonorsAuthenticatedAssetAllowlist(t *testing.T) {
	sut := newTestAPIServer(t)
	payload := []byte(`{"asset_id":"node-other","type":"host","name":"Other node","source":"agent"}`)
	req := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader(payload))
	ctx := contextWithScopes(req.Context(), []string{"assets:write"})
	ctx = contextWithAllowedAssets(ctx, []string{"node-allowed"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("allowlisted credential cross-asset heartbeat status=%d body=%s, want 403", rec.Code, rec.Body.String())
	}
	if _, ok, err := sut.assetStore.GetAsset("node-other"); err != nil || ok {
		t.Fatalf("cross-asset heartbeat lookup ok=%v err=%v, want no stored asset", ok, err)
	}
}

func TestOwnerHeartbeatCannotCreateOrResurrectAgentSource(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.authValidator = auth.NewTokenValidator("owner-token")
	sut.allowLegacySharedAgentAuth = true
	handler := sut.buildHTTPHandlers(nil, nil, nil)["/assets/heartbeat"]
	post := func(payload string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "https://labtether.test/assets/heartbeat", bytes.NewBufferString(payload))
		req.Header.Set("Authorization", "Bearer owner-token")
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec
	}

	if rec := post(`{"asset_id":"owner-missing-agent","type":"host","name":"Missing","source":"agent"}`); rec.Code != http.StatusConflict {
		t.Fatalf("missing owner agent heartbeat status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, exists, _ := sut.assetStore.GetAsset("owner-missing-agent"); exists {
		t.Fatal("owner heartbeat created an unenrolled agent source")
	}
	if rec := post(`{"asset_id":"operator-smoke-fixture","type":"host","name":"Fixture","source":"smoke-test"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("explicit smoke fixture status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "owner-existing-agent", Type: "host", Name: "Existing", Source: "agent", Status: "offline",
	}); err != nil {
		t.Fatal(err)
	}
	if rec := post(`{"asset_id":"owner-existing-agent","type":"host","name":"Existing","source":"agent","status":"online","metadata":{"agent_device_fingerprint":"FORGED","agent_identity_verified_at":"2099-01-01T00:00:00Z"}}`); rec.Code != http.StatusAccepted {
		t.Fatalf("existing owner agent refresh status=%d body=%s", rec.Code, rec.Body.String())
	}
	stored, exists, err := sut.assetStore.GetAsset("owner-existing-agent")
	if err != nil || !exists {
		t.Fatalf("existing asset lookup exists=%v err=%v", exists, err)
	}
	if stored.Source != "agent" || stored.Metadata[assets.MetadataKeyAgentDeviceFingerprint] != "" || stored.Metadata[assets.MetadataKeyAgentIdentityVerifiedAt] != "" {
		t.Fatalf("owner refresh trusted unverified identity/source: %+v", stored)
	}
	transactions := sut.enrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	if err := transactions.DecommissionAgentAsset(context.Background(), "owner-existing-agent"); err != nil {
		t.Fatal(err)
	}
	if rec := post(`{"asset_id":"owner-existing-agent","type":"host","name":"Existing","source":"agent","status":"online"}`); rec.Code != http.StatusConflict {
		t.Fatalf("decommissioned owner agent heartbeat status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, exists, _ := sut.assetStore.GetAsset("owner-existing-agent"); exists {
		t.Fatal("owner heartbeat resurrected decommissioned agent")
	}
}

func TestOwnerHeartbeatSharedCredentialIsDisabledByDefault(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.authValidator = auth.NewTokenValidator("owner-token")
	req := httptest.NewRequest(http.MethodPost, "https://labtether.test/assets/heartbeat", bytes.NewBufferString(`{"asset_id":"legacy-agent","type":"node","name":"legacy-agent","source":"agent"}`))
	req.Header.Set("Authorization", "Bearer owner-token")
	rec := httptest.NewRecorder()
	sut.withAgentHeartbeatAuth(sut.handleAssetActions)(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("shared owner heartbeat status=%d body=%s", rec.Code, rec.Body.String())
	}
	fixtureReq := httptest.NewRequest(http.MethodPost, "https://labtether.test/assets/heartbeat", bytes.NewBufferString(`{"asset_id":"operator-fixture","type":"node","name":"operator-fixture","source":"smoke-test"}`))
	fixtureReq.Header.Set("Authorization", "Bearer owner-token")
	fixtureRec := httptest.NewRecorder()
	sut.withAgentHeartbeatAuth(sut.handleAssetActions)(fixtureRec, fixtureReq)
	if fixtureRec.Code != http.StatusAccepted {
		t.Fatalf("non-agent operator heartbeat status=%d body=%s", fixtureRec.Code, fixtureRec.Body.String())
	}
}
