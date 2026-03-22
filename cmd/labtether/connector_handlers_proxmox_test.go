package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxmoxconnector "github.com/labtether/labtether/internal/connectors/proxmox"
)

func TestHandleProxmoxConnectorTestRedactsTokenSecret(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	const leakedSecret = "proxmox-token-secret-should-not-leak"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, fmt.Sprintf(`{"error":"token_secret=%s"}`, leakedSecret), http.StatusBadGateway)
	}))
	defer server.Close()

	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/connectors/proxmox/test", strings.NewReader(fmt.Sprintf(`{
		"base_url":"%s",
		"token_id":"labtether@pve!agent",
		"token_secret":"%s"
	}`, server.URL, leakedSecret)))
	rec := httptest.NewRecorder()
	sut.handleProxmoxConnectorTest(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "proxmox api returned 502")
	if strings.Contains(rec.Body.String(), leakedSecret) {
		t.Fatalf("expected response to redact leaked token secret, got %s", rec.Body.String())
	}
}

func TestHandleProxmoxConnectorTestRedactsPassword(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	const leakedPassword = "proxmox-password-should-not-leak"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, fmt.Sprintf(`{"error":"password=%s"}`, leakedPassword), http.StatusBadGateway)
	}))
	defer server.Close()

	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/connectors/proxmox/test", strings.NewReader(fmt.Sprintf(`{
		"base_url":"%s",
		"auth_method":"password",
		"username":"root@pam",
		"password":"%s"
	}`, server.URL, leakedPassword)))
	rec := httptest.NewRecorder()
	sut.handleProxmoxConnectorTest(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "ticket auth failed")
	if strings.Contains(rec.Body.String(), leakedPassword) {
		t.Fatalf("expected response to redact leaked password, got %s", rec.Body.String())
	}
}

func TestHandleConnectorActionsProxmoxTestRateLimit(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	sut.connectorRegistry.Register(proxmoxconnector.New())

	for i := 0; i < 12; i++ {
		req := httptest.NewRequest(http.MethodPost, "/connectors/proxmox/test", strings.NewReader(`{"base_url":"https://proxmox.local:8006"}`))
		req.RemoteAddr = "203.0.113.43:4403"
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("request %d: expected 400 before rate limit, got %d", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/connectors/proxmox/test", strings.NewReader(`{"base_url":"https://proxmox.local:8006"}`))
	req.RemoteAddr = "203.0.113.43:4403"
	rec := httptest.NewRecorder()
	sut.handleConnectorActions(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after connector test burst, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "rate limit exceeded")
}
