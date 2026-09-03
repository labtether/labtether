package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/hubapi/testutil"
)

type oidcSettingsStoreStub struct {
	value json.RawMessage
}

func (s *oidcSettingsStoreStub) GetSystemSetting(context.Context, string) (json.RawMessage, bool, error) {
	return append(json.RawMessage(nil), s.value...), len(s.value) > 0, nil
}

func (s *oidcSettingsStoreStub) PutSystemSetting(_ context.Context, _ string, value json.RawMessage) error {
	s.value = append(s.value[:0], value...)
	return nil
}

func TestHandleOIDCSettingsApplyKeepsActiveProviderWhenReplacementIsRejected(t *testing.T) {
	activeProvider, _ := newMobileOIDCTestProvider(t)
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LINK_LOCAL", "false")
	t.Setenv("LABTETHER_OIDC_ISSUER_URL", "")
	t.Setenv("LABTETHER_OIDC_CLIENT_ID", "")
	t.Setenv("LABTETHER_OIDC_CLIENT_SECRET", "")
	t.Setenv("LABTETHER_OIDC_ALLOWED_ENDPOINT_ORIGINS", "")

	stored, err := json.Marshal(oidcDBSettings{
		Enabled:      boolPointer(true),
		IssuerURL:    "http://169.254.169.254/identity",
		ClientID:     "replacement-client",
		ClientSecret: "replacement-secret", // #nosec G101 -- test-only fixture
	})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	ref := NewOIDCProviderRef(activeProvider, true)
	deps := &Deps{
		OIDCRef:          ref,
		SettingsStore:    &oidcSettingsStoreStub{value: stored},
		EnforceRateLimit: testutil.NoopRateLimit,
	}
	req := httptest.NewRequest(http.MethodPost, "/settings/oidc/apply", nil)
	rec := httptest.NewRecorder()

	deps.HandleOIDCSettingsApply(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid oidc configuration") || strings.Contains(rec.Body.String(), "169.254.169.254") {
		t.Fatalf("unexpected stable error body: %s", rec.Body.String())
	}
	gotProvider, gotAutoProvision := ref.Get()
	if gotProvider != activeProvider || !gotAutoProvision {
		t.Fatal("rejected replacement changed the active OIDC provider")
	}
}

func TestHandleOIDCSettingsApplyHidesDiscoveryResponseBody(t *testing.T) {
	activeProvider, _ := newMobileOIDCTestProvider(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "private upstream diagnostic", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	t.Setenv("LABTETHER_OIDC_ISSUER_URL", "")
	t.Setenv("LABTETHER_OIDC_CLIENT_ID", "")
	t.Setenv("LABTETHER_OIDC_CLIENT_SECRET", "")
	t.Setenv("LABTETHER_OIDC_ALLOWED_ENDPOINT_ORIGINS", "")

	stored, err := json.Marshal(oidcDBSettings{
		Enabled:      boolPointer(true),
		IssuerURL:    upstream.URL,
		ClientID:     "replacement-client",
		ClientSecret: "replacement-secret", // #nosec G101 -- test-only fixture
	})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	ref := NewOIDCProviderRef(activeProvider, false)
	deps := &Deps{
		OIDCRef:          ref,
		SettingsStore:    &oidcSettingsStoreStub{value: stored},
		EnforceRateLimit: testutil.NoopRateLimit,
	}
	req := httptest.NewRequest(http.MethodPost, "/settings/oidc/apply", nil)
	rec := httptest.NewRecorder()

	deps.HandleOIDCSettingsApply(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "private upstream diagnostic") {
		t.Fatalf("unexpected stable error body: %s", rec.Body.String())
	}
	gotProvider, gotAutoProvision := ref.Get()
	if gotProvider != activeProvider || gotAutoProvision {
		t.Fatal("failed discovery changed the active OIDC provider")
	}
}

func boolPointer(value bool) *bool {
	return &value
}
