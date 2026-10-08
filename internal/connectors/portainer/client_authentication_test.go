package portainer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestValidatePortainerJWTRejectsEmptyOversizeAndControlCharacters(t *testing.T) {
	if err := validatePortainerJWT("header.payload.signature"); err != nil {
		t.Fatalf("valid JWT rejected: %v", err)
	}
	for _, jwt := range []string{
		"",
		strings.Repeat("a", maxJWTBytes+1),
		"header.payload.signature\nreflected-header",
	} {
		if err := validatePortainerJWT(jwt); err == nil {
			t.Fatalf("invalid JWT of length %d accepted", len(jwt))
		}
	}
}

func TestAPIKeyAuth(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	const apiKey = "ptr_abc123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("expected X-API-Key=%q, got %q", apiKey, got)
		}
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Fatalf("expected no Authorization header, got %q", auth)
		}
		_, _ = w.Write([]byte(`{"ServerVersion":"2.21.0","DatabaseVersion":"100","Build":{"BuildNumber":"1234","GoVersion":"go1.21"}}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		APIKey:  apiKey,
		Timeout: 5 * time.Second,
	})

	info, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if info.ServerVersion != "2.21.0" {
		t.Fatalf("unexpected server version: %s", info.ServerVersion)
	}
}

func TestJWTAuth(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	var authCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth":
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST for /api/auth, got %s", r.Method)
			}
			var creds struct {
				Username string `json:"Username"`
				Password string `json:"Password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
				t.Fatalf("decode auth body: %v", err)
			}
			if creds.Username != "admin" || creds.Password != "secret" {
				t.Fatalf("unexpected credentials: %+v", creds)
			}
			authCalls.Add(1)
			_, _ = w.Write([]byte(`{"jwt":"eyJhbGciOiJIUzI1NiJ9.test.sig"}`))

		case "/api/system/version":
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				t.Fatalf("expected Bearer token, got %q", auth)
			}
			_, _ = w.Write([]byte(`{"ServerVersion":"2.21.0","DatabaseVersion":"100","Build":{"BuildNumber":"1234","GoVersion":"go1.21"}}`))

		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:  server.URL,
		Username: "admin",
		Password: "secret",
		Timeout:  5 * time.Second,
	})

	// First call should trigger auth.
	info, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("first GetVersion failed: %v", err)
	}
	if info.ServerVersion != "2.21.0" {
		t.Fatalf("unexpected server version: %s", info.ServerVersion)
	}

	// Second call should reuse cached JWT (no additional auth call).
	_, err = client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("second GetVersion failed: %v", err)
	}

	if authCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 auth call, got %d", authCalls.Load())
	}
}

func TestJWTRetryOn401(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	var requestCount atomic.Int32
	var authCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth":
			authCount.Add(1)
			_, _ = w.Write([]byte(`{"jwt":"fresh-token"}`))

		case "/api/system/version":
			count := requestCount.Add(1)
			if count == 1 {
				// First request: return 401 to trigger re-auth.
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"Token is expired"}`))
				return
			}
			// Second request (after re-auth): succeed.
			_, _ = w.Write([]byte(`{"ServerVersion":"2.21.0","DatabaseVersion":"100","Build":{}}`))

		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:  server.URL,
		Username: "admin",
		Password: "secret",
		Timeout:  5 * time.Second,
	})

	info, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if info.ServerVersion != "2.21.0" {
		t.Fatalf("unexpected version: %s", info.ServerVersion)
	}

	// Should have called auth twice: initial + retry after 401.
	if authCount.Load() != 2 {
		t.Fatalf("expected 2 auth calls, got %d", authCount.Load())
	}
}

func TestJWTRetryOn401PreservesJSONBody(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	var requestCount atomic.Int32
	var authCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth":
			authCount.Add(1)
			_, _ = w.Write([]byte(`{"jwt":"fresh-token"}`))

		case "/api/stacks/5/git/redeploy":
			call := requestCount.Add(1)
			if call == 1 {
				// Force retry path.
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"token expired"}`))
				return
			}

			rawBody, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}
			if len(strings.TrimSpace(string(rawBody))) == 0 {
				t.Fatalf("expected JSON body on retry request, got empty payload")
			}

			var payload map[string]any
			if err := json.Unmarshal(rawBody, &payload); err != nil {
				t.Fatalf("failed to decode retry payload: %v", err)
			}
			if pullImage, ok := payload["pullImage"].(bool); !ok || !pullImage {
				t.Fatalf("expected pullImage=true in retry payload, got %#v", payload["pullImage"])
			}

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))

		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:  server.URL,
		Username: "admin",
		Password: "secret",
		Timeout:  5 * time.Second,
	})

	if err := client.RedeployStack(context.Background(), 5, 1, true); err != nil {
		t.Fatalf("RedeployStack failed: %v", err)
	}

	if requestCount.Load() != 2 {
		t.Fatalf("expected 2 stack requests, got %d", requestCount.Load())
	}
	if authCount.Load() != 2 {
		t.Fatalf("expected 2 auth calls (initial + retry), got %d", authCount.Load())
	}
}

func TestAuthenticateErrorPaths(t *testing.T) {
	allowInsecureTransportForPortainerTests(t)
	t.Run("create auth request error", func(t *testing.T) {
		client := NewClient(Config{
			BaseURL:  "https://example.com",
			Username: "admin",
			Password: "secret",
		})
		client.baseURL = "://bad-url"

		if err := client.authenticate(context.Background()); err == nil || !strings.Contains(err.Error(), "create auth request") {
			t.Fatalf("expected create auth request error, got %v", err)
		}
	})

	t.Run("auth request transport error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		baseURL := server.URL
		server.Close()

		client := NewClient(Config{
			BaseURL:  baseURL,
			Username: "admin",
			Password: "secret",
			Timeout:  50 * time.Millisecond,
		})
		if err := client.authenticate(context.Background()); err == nil || !strings.Contains(err.Error(), "auth request") {
			t.Fatalf("expected auth request error, got %v", err)
		}
	})

	t.Run("read auth response error", func(t *testing.T) {
		client := NewClient(Config{
			BaseURL:  "https://portainer.local",
			Username: "admin",
			Password: "secret",
		})
		client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       errorReadCloser{},
				Header:     make(http.Header),
			}, nil
		})

		if err := client.authenticate(context.Background()); err == nil || !strings.Contains(err.Error(), "read auth response") {
			t.Fatalf("expected read auth response error, got %v", err)
		}
	})

	t.Run("auth non-success status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"invalid credentials"}`))
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:  server.URL,
			Username: "admin",
			Password: "wrong",
		})

		if err := client.authenticate(context.Background()); err == nil || !strings.Contains(err.Error(), "auth failed") {
			t.Fatalf("expected auth failed error, got %v", err)
		}
	})

	t.Run("decode auth response error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{not-json`))
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:  server.URL,
			Username: "admin",
			Password: "secret",
		})
		if err := client.authenticate(context.Background()); err == nil || !strings.Contains(err.Error(), "decode auth response") {
			t.Fatalf("expected decode auth response error, got %v", err)
		}
	})

	t.Run("empty jwt response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"jwt":""}`))
		}))
		defer server.Close()

		client := NewClient(Config{
			BaseURL:  server.URL,
			Username: "admin",
			Password: "secret",
		})
		if err := client.authenticate(context.Background()); err == nil || !strings.Contains(err.Error(), "empty JWT") {
			t.Fatalf("expected empty JWT error, got %v", err)
		}
	})
}
