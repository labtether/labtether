package auth

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/audit"
	internalauth "github.com/labtether/labtether/internal/auth"
)

const testOIDCClientID = "labtether-mobile-test"

type mobileOIDCTestProvider struct {
	server *httptest.Server
	key    *rsa.PrivateKey

	mu            sync.Mutex
	expectedNonce string
	tokenForms    []url.Values
}

func newMobileOIDCTestProvider(t *testing.T) (*internalauth.OIDCProvider, *mobileOIDCTestProvider) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test oidc key: %v", err)
	}
	mock := &mobileOIDCTestProvider{key: privateKey}
	var issuer string
	mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeTestJSON(w, map[string]any{
				"issuer":                                issuer,
				"authorization_endpoint":                issuer + "/authorize",
				"token_endpoint":                        issuer + "/token",
				"jwks_uri":                              issuer + "/keys",
				"response_types_supported":              []string{"code"},
				"subject_types_supported":               []string{"public"},
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/keys":
			writeTestJSON(w, map[string]any{"keys": []any{testRSAJWK(privateKey)}})
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			formCopy := make(url.Values, len(r.PostForm))
			for key, values := range r.PostForm {
				formCopy[key] = append([]string(nil), values...)
			}
			mock.mu.Lock()
			mock.tokenForms = append(mock.tokenForms, formCopy)
			nonce := mock.expectedNonce
			mock.mu.Unlock()
			idToken, signErr := signTestIDToken(privateKey, issuer, nonce)
			if signErr != nil {
				http.Error(w, "sign token", http.StatusInternalServerError)
				return
			}
			writeTestJSON(w, map[string]any{
				"access_token": "test-access-token",
				"token_type":   "Bearer",
				"expires_in":   300,
				"id_token":     idToken,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	issuer = mock.server.URL
	t.Cleanup(mock.server.Close)

	provider, err := internalauth.NewOIDCProvider(context.Background(), internalauth.OIDCSettings{
		Enabled:      true,
		IssuerURL:    issuer,
		ClientID:     testOIDCClientID,
		ClientSecret: "test-client-secret", // #nosec G101 -- test-only fixture
		DefaultRole:  internalauth.RoleViewer,
		RoleClaim:    "labtether_role",
	})
	if err != nil {
		t.Fatalf("create test oidc provider: %v", err)
	}
	return provider, mock
}

func (m *mobileOIDCTestProvider) setExpectedNonce(nonce string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expectedNonce = nonce
}

func (m *mobileOIDCTestProvider) receivedTokenForms() []url.Values {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]url.Values, len(m.tokenForms))
	copy(out, m.tokenForms)
	return out
}

func TestValidateMobileOIDCRedirectURIRequiresExactCallback(t *testing.T) {
	if got, err := ValidateMobileOIDCRedirectURI(MobileOIDCRedirectURI); err != nil || got != MobileOIDCRedirectURI {
		t.Fatalf("valid callback returned %q, %v", got, err)
	}
	for _, redirectURI := range []string{
		"com.labtether.mobile:/oauth2redirect/",
		"com.labtether.mobile://oauth2redirect",
		"com.labtether.mobile:/oauth2redirect?next=/",
		"COM.LABTETHER.MOBILE:/oauth2redirect",
		"com.labtether.beta:/oauth2redirect",
		"labtether://auth/oidc/callback",
		"https://hub.example/api/auth/oidc/callback",
	} {
		if _, err := ValidateMobileOIDCRedirectURI(redirectURI); err == nil {
			t.Fatalf("expected redirect URI %q to be rejected", redirectURI)
		}
	}
}

func TestMobileOIDCStateCannotCrossCallbackFlows(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	expiresAt := time.Now().UTC().Add(time.Minute)
	if !deps.StoreOIDCState("mobile-state", OIDCAuthState{
		RedirectURI: MobileOIDCRedirectURI,
		Flow:        OIDCAuthFlowMobile,
		ExpiresAt:   expiresAt,
	}) {
		t.Fatal("store mobile state")
	}
	if _, ok := deps.ConsumeOIDCState("mobile-state", MobileOIDCRedirectURI); ok {
		t.Fatal("web callback consumed mobile state")
	}

	const webRedirect = "https://hub.example/api/auth/oidc/callback"
	if !deps.StoreOIDCState("web-state", OIDCAuthState{
		RedirectURI: webRedirect,
		Flow:        OIDCAuthFlowWeb,
		ExpiresAt:   expiresAt,
	}) {
		t.Fatal("store web state")
	}
	if _, ok := deps.ConsumeMobileOIDCState("web-state", webRedirect); ok {
		t.Fatal("mobile callback consumed web state")
	}
}

func TestHandleAuthOIDCMobileStartBuildsBoundPKCERequest(t *testing.T) {
	provider, _ := newMobileOIDCTestProvider(t)
	deps, _ := newTestAuthDeps(t)
	deps.OIDCRef.Swap(provider, true)
	var globalBucket string
	deps.EnforceRateLimitGlobal = func(_ http.ResponseWriter, bucket string, _ int, _ time.Duration) bool {
		globalBucket = bucket
		return true
	}

	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge, err := internalauth.PKCECodeChallengeS256(verifier)
	if err != nil {
		t.Fatal(err)
	}
	body := mustTestJSON(t, map[string]any{
		"redirect_uri":          MobileOIDCRedirectURI,
		"code_challenge":        challenge,
		"code_challenge_method": internalauth.PKCECodeChallengeMethodS256,
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/oidc/mobile/start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleAuthOIDCMobileStart(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if globalBucket != "auth.oidc.mobile.start.global" {
		t.Fatalf("global rate-limit bucket = %q", globalBucket)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}

	var response struct {
		AuthURL   string    `json:"auth_url"`
		State     string    `json:"state"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.State == "" || response.ExpiresAt.Before(time.Now().UTC().Add(4*time.Minute)) {
		t.Fatalf("invalid state metadata: %#v", response)
	}
	authURL, err := url.Parse(response.AuthURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	query := authURL.Query()
	for key, want := range map[string]string{
		"state":                 response.State,
		"redirect_uri":          MobileOIDCRedirectURI,
		"code_challenge":        challenge,
		"code_challenge_method": internalauth.PKCECodeChallengeMethodS256,
	} {
		if got := query.Get(key); got != want {
			t.Fatalf("auth URL %s = %q, want %q", key, got, want)
		}
	}
	if query.Get("nonce") == "" {
		t.Fatal("auth URL missing nonce")
	}
	if query.Has("access_type") {
		t.Fatalf("native authorization unnecessarily requests offline access: %q", query.Get("access_type"))
	}

	deps.OIDCStateMu.Lock()
	stored := deps.OIDCStates[response.State]
	deps.OIDCStateMu.Unlock()
	if stored.Flow != OIDCAuthFlowMobile || stored.CodeChallenge != challenge || stored.RedirectURI != MobileOIDCRedirectURI {
		t.Fatalf("unexpected stored state: %#v", stored)
	}
}

func TestHandleAuthProvidersAdvertisesNativePKCEContract(t *testing.T) {
	provider, _ := newMobileOIDCTestProvider(t)
	deps, _ := newTestAuthDeps(t)
	deps.OIDCRef.Swap(provider, true)
	rec := httptest.NewRecorder()
	deps.HandleAuthProviders(rec, httptest.NewRequest(http.MethodGet, "/auth/providers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload struct {
		OIDC struct {
			MobileSupported      bool     `json:"mobile_supported"`
			MobileRedirectURI    string   `json:"mobile_redirect_uri"`
			PKCEMethodsSupported []string `json:"pkce_methods_supported"`
		} `json:"oidc"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OIDC.MobileSupported || payload.OIDC.MobileRedirectURI != MobileOIDCRedirectURI {
		t.Fatalf("unexpected mobile metadata: %#v", payload.OIDC)
	}
	if len(payload.OIDC.PKCEMethodsSupported) != 1 || payload.OIDC.PKCEMethodsSupported[0] != "S256" {
		t.Fatalf("unexpected PKCE methods: %#v", payload.OIDC.PKCEMethodsSupported)
	}
}

func TestHandleAuthOIDCMobileCallbackCreatesCookieSessionWithPKCE(t *testing.T) {
	provider, mock := newMobileOIDCTestProvider(t)
	deps, store := newTestAuthDeps(t)
	deps.OIDCRef.Swap(provider, true)
	deps.EnforceRateLimitGlobal = func(_ http.ResponseWriter, _ string, _ int, _ time.Duration) bool { return true }
	ownerHash, err := internalauth.HashPassword("owner-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUserWithRole("owner", ownerHash, internalauth.RoleOwner, "local", ""); err != nil {
		t.Fatal(err)
	}

	const (
		state    = "mobile-callback-state"
		nonce    = "mobile-callback-nonce"
		verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	)
	challenge, err := internalauth.PKCECodeChallengeS256(verifier)
	if err != nil {
		t.Fatal(err)
	}
	if !deps.StoreOIDCState(state, OIDCAuthState{
		Nonce:         nonce,
		RedirectURI:   MobileOIDCRedirectURI,
		Flow:          OIDCAuthFlowMobile,
		CodeChallenge: challenge,
		ExpiresAt:     time.Now().UTC().Add(time.Minute),
	}) {
		t.Fatal("store state")
	}
	mock.setExpectedNonce(nonce)
	var auditEvents []audit.Event
	deps.AppendAuditEventBestEffort = func(event audit.Event, _ string) {
		auditEvents = append(auditEvents, event)
	}

	body := mustTestJSON(t, map[string]any{
		"code":          "authorization-code",
		"state":         state,
		"redirect_uri":  MobileOIDCRedirectURI,
		"code_verifier": verifier,
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/oidc/mobile/callback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleAuthOIDCMobileCallback(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	var response struct {
		SessionID string                `json:"session_id"`
		Created   bool                  `json:"created"`
		User      internalauth.UserInfo `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.SessionID == "" || !response.Created || response.User.Role != internalauth.RoleOperator {
		t.Fatalf("unexpected response: %#v", response)
	}
	createdUser, found, err := store.GetUserByOIDCIdentity("oidc", mock.server.URL, "mobile-user-subject")
	if err != nil || !found || createdUser.ID != response.User.ID {
		t.Fatalf("verified issuer was not persisted with the OIDC subject: found=%t user=%#v err=%v", found, createdUser, err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != internalauth.SessionCookieName || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatalf("unexpected session cookies: %#v", cookies)
	}
	forms := mock.receivedTokenForms()
	if len(forms) != 1 {
		t.Fatalf("token endpoint calls = %d", len(forms))
	}
	if forms[0].Get("code_verifier") != verifier || forms[0].Get("redirect_uri") != MobileOIDCRedirectURI {
		t.Fatalf("token exchange was not PKCE-bound: %#v", forms[0])
	}
	if len(auditEvents) != 1 || auditEvents[0].Decision != "allow" || auditEvents[0].ActorID == "" || auditEvents[0].SessionID != response.SessionID {
		t.Fatalf("unexpected audit events: %#v", auditEvents)
	}
	if details := fmt.Sprint(auditEvents[0].Details); strings.Contains(details, verifier) || strings.Contains(details, "authorization-code") {
		t.Fatal("audit details contain oauth secrets")
	}

	replayRec := httptest.NewRecorder()
	replayReq := httptest.NewRequest(http.MethodPost, "/auth/oidc/mobile/callback", bytes.NewReader(body))
	replayReq.Header.Set("Content-Type", "application/json")
	deps.HandleAuthOIDCMobileCallback(replayRec, replayReq)
	if replayRec.Code != http.StatusBadRequest || len(mock.receivedTokenForms()) != 1 {
		t.Fatalf("replay status = %d, token calls = %d", replayRec.Code, len(mock.receivedTokenForms()))
	}
}

func TestHandleAuthOIDCMobileCallbackConsumesStateOnVerifierMismatch(t *testing.T) {
	provider, mock := newMobileOIDCTestProvider(t)
	deps, _ := newTestAuthDeps(t)
	deps.OIDCRef.Swap(provider, true)
	deps.EnforceRateLimitGlobal = func(_ http.ResponseWriter, _ string, _ int, _ time.Duration) bool { return true }
	const goodVerifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const wrongVerifier = "aBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge, err := internalauth.PKCECodeChallengeS256(goodVerifier)
	if err != nil {
		t.Fatal(err)
	}
	if !deps.StoreOIDCState("mismatch-state", OIDCAuthState{
		Nonce:         "nonce",
		RedirectURI:   MobileOIDCRedirectURI,
		Flow:          OIDCAuthFlowMobile,
		CodeChallenge: challenge,
		ExpiresAt:     time.Now().UTC().Add(time.Minute),
	}) {
		t.Fatal("store state")
	}
	body := mustTestJSON(t, map[string]any{
		"code":          "authorization-code",
		"state":         "mismatch-state",
		"redirect_uri":  MobileOIDCRedirectURI,
		"code_verifier": wrongVerifier,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/oidc/mobile/callback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	deps.HandleAuthOIDCMobileCallback(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid or expired") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(mock.receivedTokenForms()) != 0 {
		t.Fatal("provider was contacted for mismatched verifier")
	}
	if _, ok := deps.ConsumeMobileOIDCState("mismatch-state", MobileOIDCRedirectURI); ok {
		t.Fatal("mismatched state was not consumed")
	}
}

func TestMobileOIDCEndpointsEnforceGlobalRateLimit(t *testing.T) {
	provider, _ := newMobileOIDCTestProvider(t)
	for _, test := range []struct {
		name    string
		path    string
		handler http.HandlerFunc
		bucket  string
	}{
		{name: "start", path: "/auth/oidc/mobile/start", bucket: "auth.oidc.mobile.start.global"},
		{name: "callback", path: "/auth/oidc/mobile/callback", bucket: "auth.oidc.mobile.callback.global"},
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
			if test.name == "start" {
				test.handler = deps.HandleAuthOIDCMobileStart
			} else {
				test.handler = deps.HandleAuthOIDCMobileCallback
			}
			rec := httptest.NewRecorder()
			test.handler(rec, httptest.NewRequest(http.MethodPost, test.path, nil))
			if rec.Code != http.StatusTooManyRequests || gotBucket != test.bucket || calledLocal {
				t.Fatalf("status=%d bucket=%q local=%t", rec.Code, gotBucket, calledLocal)
			}
		})
	}
}

func writeTestJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func mustTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testRSAJWK(key *rsa.PrivateKey) map[string]any {
	exponent := big.NewInt(int64(key.PublicKey.E)).Bytes()
	return map[string]any{
		"kty": "RSA",
		"kid": "mobile-test-key",
		"use": "sig",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(exponent),
	}
}

func signTestIDToken(key *rsa.PrivateKey, issuer, nonce string) (string, error) {
	header, err := json.Marshal(map[string]any{
		"alg": "RS256",
		"kid": "mobile-test-key",
		"typ": "JWT",
	})
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	claims, err := json.Marshal(map[string]any{
		"iss":                issuer,
		"sub":                "mobile-user-subject",
		"aud":                testOIDCClientID,
		"exp":                now.Add(5 * time.Minute).Unix(),
		"iat":                now.Unix(),
		"nonce":              nonce,
		"preferred_username": "mobile-operator",
		"labtether_role":     "operator",
	})
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
