package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/policy"
)

func TestDesktopCredentialEndpointsApplyDesktopAuthorizationBoundary(t *testing.T) {
	sut := newTestAPIServer(t)
	cfg := policy.DefaultEvaluatorConfig()
	cfg.InteractiveEnabled = false
	sut.policyState = newPolicyRuntimeState(cfg)

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "desktop-node-creds",
		Type:    "node",
		Name:    "Desktop Credentials Node",
		Source:  "manual",
		Status:  "online",
	}); err != nil {
		t.Fatalf("failed to seed asset: %v", err)
	}

	testCases := []struct {
		name    string
		path    string
		method  string
		body    string
		handler func(*apiServer, http.ResponseWriter, *http.Request)
	}{
		{
			name:   "get",
			path:   "/assets/desktop-node-creds/desktop/credentials",
			method: http.MethodGet,
			handler: func(s *apiServer, w http.ResponseWriter, r *http.Request) {
				s.handleDesktopCredentials(w, r)
			},
		},
		{
			name:   "save",
			path:   "/assets/desktop-node-creds/desktop/credentials",
			method: http.MethodPost,
			body:   `{"username":"operator","password":"secret"}`,
			handler: func(s *apiServer, w http.ResponseWriter, r *http.Request) {
				s.handleDesktopCredentials(w, r)
			},
		},
		{
			name:   "delete",
			path:   "/assets/desktop-node-creds/desktop/credentials",
			method: http.MethodDelete,
			handler: func(s *apiServer, w http.ResponseWriter, r *http.Request) {
				s.handleDesktopCredentials(w, r)
			},
		},
		{
			name:   "retrieve",
			path:   "/assets/desktop-node-creds/desktop/credentials/retrieve",
			method: http.MethodPost,
			handler: func(s *apiServer, w http.ResponseWriter, r *http.Request) {
				s.handleRetrieveDesktopCredentials(w, r)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			tc.handler(sut, rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for %s, got %d: %s", tc.name, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "interactive mode disabled") {
				t.Fatalf("expected interactive policy denial for %s, got %s", tc.name, rec.Body.String())
			}
		})
	}
}
