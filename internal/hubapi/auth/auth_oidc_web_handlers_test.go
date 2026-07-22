package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/audit"
	internalauth "github.com/labtether/labtether/internal/auth"
)

const webOIDCRedirectURI = "https://hub.example/api/auth/oidc/callback"

func TestHandleAuthOIDCWebStartIsBoundedAndNeverCacheable(t *testing.T) {
	provider, _ := newMobileOIDCTestProvider(t)
	deps, _ := newTestAuthDeps(t)
	deps.OIDCRef.Swap(provider, true)
	var globalBucket string
	deps.EnforceRateLimitGlobal = func(_ http.ResponseWriter, bucket string, _ int, _ time.Duration) bool {
		globalBucket = bucket
		return true
	}
	var localBucket string
	deps.EnforceRateLimit = func(_ http.ResponseWriter, _ *http.Request, bucket string, _ int, _ time.Duration) bool {
		localBucket = bucket
		return true
	}

	body := mustTestJSON(t, map[string]any{
		"redirect_uri": webOIDCRedirectURI,
		"next":         "/nodes?filter=online",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/oidc/start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleAuthOIDCStart(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if globalBucket != "auth.oidc.web.start.global" || localBucket != "auth.oidc.start" {
		t.Fatalf("rate-limit buckets global=%q local=%q", globalBucket, localBucket)
	}
	assertOIDCNoStoreHeaders(t, rec)

	var response struct {
		AuthURL string `json:"auth_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	authURL, err := url.Parse(response.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	state := authURL.Query().Get("state")
	if state == "" || authURL.Query().Get("nonce") == "" || authURL.Query().Get("redirect_uri") != webOIDCRedirectURI {
		t.Fatalf("authorization URL is not state/nonce/redirect bound: %q", response.AuthURL)
	}
	deps.OIDCStateMu.Lock()
	stored := deps.OIDCStates[state]
	deps.OIDCStateMu.Unlock()
	if stored.Flow != OIDCAuthFlowWeb || stored.RedirectURI != webOIDCRedirectURI || stored.NextPath != "/nodes?filter=online" {
		t.Fatalf("unexpected stored web state: %#v", stored)
	}
}

func TestHandleAuthOIDCWebCallbackCreatesAuditedNonCacheableSessionAndRejectsReplay(t *testing.T) {
	provider, mock := newMobileOIDCTestProvider(t)
	deps, store := newTestAuthDeps(t)
	deps.OIDCRef.Swap(provider, true)
	deps.EnforceRateLimitGlobal = func(_ http.ResponseWriter, _ string, _ int, _ time.Duration) bool { return true }
	var localBucket string
	deps.EnforceRateLimit = func(_ http.ResponseWriter, _ *http.Request, bucket string, _ int, _ time.Duration) bool {
		localBucket = bucket
		return true
	}
	ownerHash, err := internalauth.HashPassword("owner-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUserWithRole("owner", ownerHash, internalauth.RoleOwner, "local", ""); err != nil {
		t.Fatal(err)
	}

	const state = "web-callback-state"
	const nonce = "web-callback-nonce"
	if !deps.StoreOIDCState(state, OIDCAuthState{
		Nonce:       nonce,
		NextPath:    "/topology",
		RedirectURI: webOIDCRedirectURI,
		Flow:        OIDCAuthFlowWeb,
		ExpiresAt:   time.Now().UTC().Add(time.Minute),
	}) {
		t.Fatal("store state")
	}
	mock.setExpectedNonce(nonce)
	var auditEvents []audit.Event
	deps.AppendAuditEventBestEffort = func(event audit.Event, _ string) {
		auditEvents = append(auditEvents, event)
	}

	body := mustTestJSON(t, map[string]any{
		"code":         "web-authorization-code",
		"state":        state,
		"redirect_uri": webOIDCRedirectURI,
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/oidc/callback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleAuthOIDCCallback(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if localBucket != "auth.oidc.callback" {
		t.Fatalf("local callback rate-limit bucket = %q", localBucket)
	}
	assertOIDCNoStoreHeaders(t, rec)

	var response struct {
		SessionID string                `json:"session_id"`
		Created   bool                  `json:"created"`
		Next      string                `json:"next"`
		User      internalauth.UserInfo `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.SessionID == "" || !response.Created || response.Next != "/topology" || response.User.Role != internalauth.RoleOperator {
		t.Fatalf("unexpected callback response: %#v", response)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != internalauth.SessionCookieName || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatalf("unexpected session cookies: %#v", cookies)
	}
	if len(auditEvents) != 1 || auditEvents[0].Type != "auth.oidc.login" || auditEvents[0].Decision != "allow" || auditEvents[0].ActorID == "" || auditEvents[0].SessionID != response.SessionID {
		t.Fatalf("unexpected audit events: %#v", auditEvents)
	}
	if clientType := fmt.Sprint(auditEvents[0].Details["client_type"]); clientType != "web" {
		t.Fatalf("audit client type = %q", clientType)
	}
	if details := fmt.Sprint(auditEvents[0].Details); strings.Contains(details, "web-authorization-code") || strings.Contains(details, nonce) {
		t.Fatal("audit details contain OAuth secrets")
	}

	replayRec := httptest.NewRecorder()
	replayReq := httptest.NewRequest(http.MethodPost, "/auth/oidc/callback", bytes.NewReader(body))
	replayReq.Header.Set("Content-Type", "application/json")
	deps.HandleAuthOIDCCallback(replayRec, replayReq)
	if replayRec.Code != http.StatusBadRequest || len(mock.receivedTokenForms()) != 1 {
		t.Fatalf("replay status = %d, token calls = %d", replayRec.Code, len(mock.receivedTokenForms()))
	}
}

func TestWebOIDCEndpointsEnforceGlobalRateLimitBeforeLocalWork(t *testing.T) {
	provider, _ := newMobileOIDCTestProvider(t)
	for _, test := range []struct {
		name   string
		path   string
		bucket string
	}{
		{name: "start", path: "/auth/oidc/start", bucket: "auth.oidc.web.start.global"},
		{name: "callback", path: "/auth/oidc/callback", bucket: "auth.oidc.web.callback.global"},
	} {
		t.Run(test.name, func(t *testing.T) {
			deps, _ := newTestAuthDeps(t)
			deps.OIDCRef.Swap(provider, true)
			calledLocal := false
			deps.EnforceRateLimit = func(_ http.ResponseWriter, _ *http.Request, _ string, _ int, _ time.Duration) bool {
				calledLocal = true
				return true
			}
			var gotBucket string
			deps.EnforceRateLimitGlobal = func(w http.ResponseWriter, bucket string, _ int, _ time.Duration) bool {
				gotBucket = bucket
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return false
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, test.path, nil)
			if test.name == "start" {
				deps.HandleAuthOIDCStart(rec, req)
			} else {
				deps.HandleAuthOIDCCallback(rec, req)
			}
			if rec.Code != http.StatusTooManyRequests || gotBucket != test.bucket || calledLocal {
				t.Fatalf("status=%d bucket=%q local=%t", rec.Code, gotBucket, calledLocal)
			}
			assertOIDCNoStoreHeaders(t, rec)
		})
	}
}

func assertOIDCNoStoreHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q", got)
	}
}
