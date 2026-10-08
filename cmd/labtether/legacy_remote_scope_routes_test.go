package main

import (
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/credentials"
	"net/http"
	"strings"
	"testing"
)

func TestRemoteSessionRoutesEnforceAllowedAssets(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKey(t, sut, []string{"terminal:read", "terminal:write"}, []string{"srv1"})
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	for _, id := range []string{"srv1", "srv2"} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: id,
			Name:    strings.ToUpper(id),
			Source:  "agent",
			Type:    "host",
			Status:  "online",
		}); err != nil {
			t.Fatalf("seed asset %s: %v", id, err)
		}
	}

	terminalRec := invokeLegacyRoute(t, handlers["/terminal/sessions"], http.MethodPost, "/terminal/sessions", key, `{"target":"srv2","mode":"interactive"}`)
	if terminalRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed terminal target, got %d: %s", terminalRec.Code, terminalRec.Body.String())
	}

	desktopRec := invokeLegacyRoute(t, handlers["/desktop/sessions"], http.MethodPost, "/desktop/sessions", key, `{"target":"srv2"}`)
	if desktopRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed desktop target, got %d: %s", desktopRec.Code, desktopRec.Body.String())
	}
}

func TestRemoteSessionRoutesRequireCredentialUseForStoredProfiles(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKey(t, sut, []string{"terminal:read", "terminal:write"}, []string{"srv1"})
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1",
		Name:    "SRV1",
		Source:  "agent",
		Type:    "host",
		Status:  "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if _, err := sut.credentialStore.SaveAssetTerminalConfig(credentials.AssetTerminalConfig{
		AssetID:             "srv1",
		Host:                "192.0.2.10",
		Port:                22,
		CredentialProfileID: "cred-private",
	}); err != nil {
		t.Fatalf("seed terminal credential binding: %v", err)
	}

	terminalRec := invokeLegacyRoute(t, handlers["/terminal/sessions"], http.MethodPost, "/terminal/sessions", key, `{"target":"srv1","mode":"interactive"}`)
	if terminalRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without credentials:use for terminal, got %d: %s", terminalRec.Code, terminalRec.Body.String())
	}

	desktopRec := invokeLegacyRoute(t, handlers["/desktop/sessions"], http.MethodPost, "/desktop/sessions", key, `{"target":"srv1"}`)
	if desktopRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without credentials:use for desktop, got %d: %s", desktopRec.Code, desktopRec.Body.String())
	}
}

func TestTerminalConfigPatchCannotRetainCredentialWithoutUseScope(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKey(t, sut, []string{"terminal:write"}, []string{"srv1"})
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1",
		Name:    "SRV1",
		Source:  "agent",
		Type:    "host",
		Status:  "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if _, err := sut.credentialStore.SaveAssetTerminalConfig(credentials.AssetTerminalConfig{
		AssetID:             "srv1",
		Host:                "192.0.2.10",
		Port:                22,
		CredentialProfileID: "cred-private",
	}); err != nil {
		t.Fatalf("seed terminal credential binding: %v", err)
	}

	rec := invokeLegacyRoute(t, handlers["/assets/"], http.MethodPatch, "/assets/srv1/terminal/config", key, `{"host":"192.0.2.11"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when retained binding lacks credentials:use, got %d: %s", rec.Code, rec.Body.String())
	}
	cfg, ok, err := sut.credentialStore.GetAssetTerminalConfig("srv1")
	if err != nil || !ok {
		t.Fatalf("reload terminal config: ok=%v err=%v", ok, err)
	}
	if cfg.Host != "192.0.2.10" {
		t.Fatalf("forbidden patch mutated stored config host to %q", cfg.Host)
	}
}

func TestDesktopCredentialRetrieveRequiresUseScope(t *testing.T) {
	sut := newTestAPIServer(t)
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1", Name: "SRV1", Source: "agent", Type: "host", Status: "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	writeKey := createLegacyRouteAPIKey(t, sut, []string{"credentials:write"}, []string{"srv1"})
	rec := invokeLegacyRoute(t, handlers["/assets/"], http.MethodPost, "/assets/srv1/desktop/credentials/retrieve", writeKey, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when credentials:write lacks credentials:use, got %d: %s", rec.Code, rec.Body.String())
	}

	useKey := createLegacyRouteAPIKey(t, sut, []string{"credentials:use"}, []string{"srv1"})
	rec = invokeLegacyRoute(t, handlers["/assets/"], http.MethodPost, "/assets/srv1/desktop/credentials/retrieve", useKey, "")
	if rec.Code == http.StatusForbidden {
		t.Fatalf("credentials:use should pass the scope boundary, got %d: %s", rec.Code, rec.Body.String())
	}
}
