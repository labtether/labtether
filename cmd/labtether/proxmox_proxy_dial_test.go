package main

import (
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDialProxmoxProxySocketValidationAndErrorBranches(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	if _, err := proxmoxpkg.DialProxmoxProxySocket(nil, "pve01", "node", "", proxmox.ProxyTicket{
		Port:   5900,
		Ticket: "PVEVNC:test",
	}); err == nil || !strings.Contains(err.Error(), "runtime unavailable") {
		t.Fatalf("expected runtime unavailable error, got %v", err)
	}

	if _, err := proxmoxpkg.DialProxmoxProxySocket(proxmoxpkg.NewProxmoxRuntime(nil), "pve01", "node", "", proxmox.ProxyTicket{
		Port:   5900,
		Ticket: "PVEVNC:test",
	}); err == nil || !strings.Contains(err.Error(), "runtime unavailable") {
		t.Fatalf("expected runtime unavailable error for missing client, got %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	runtime := proxmoxpkg.NewProxmoxRuntime(client)

	if _, err := proxmoxpkg.DialProxmoxProxySocket(runtime, "pve01", "node", "", proxmox.ProxyTicket{}); err == nil || !strings.Contains(err.Error(), "invalid proxmox ticket payload") {
		t.Fatalf("expected invalid ticket payload error, got %v", err)
	}

	if _, err := proxmoxpkg.DialProxmoxProxySocket(runtime, "pve01", "node", "", proxmox.ProxyTicket{
		Port:   5900,
		Ticket: "PVEVNC:test",
	}); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("expected websocket HTTP error detail, got %v", err)
	}
}

func TestDialProxmoxProxySocketAdditionalBranches(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/vncwebsocket") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if _, err := proxmoxpkg.DialProxmoxProxySocket(proxmoxpkg.NewProxmoxRuntime(client), "pve01", "invalid-kind", "", proxmox.ProxyTicket{
		Port:   5900,
		Ticket: "PVEVNC:test",
	}); err == nil {
		t.Fatalf("expected BuildVNCWebSocketURL error for invalid kind")
	}

	if _, err := proxmoxpkg.DialProxmoxProxySocket(proxmoxpkg.NewProxmoxRuntimeOpts(proxmoxpkg.ProxmoxRuntimeOpts{Client: client, CAPEM: "not-a-valid-ca"}), "pve01", "node", "", proxmox.ProxyTicket{
		Port:   5900,
		Ticket: "PVEVNC:test",
	}); err == nil || !strings.Contains(err.Error(), "invalid proxmox ca_pem") {
		t.Fatalf("expected invalid ca_pem failure, got %v", err)
	}

	if _, err := proxmoxpkg.DialProxmoxProxySocket(proxmoxpkg.NewProxmoxRuntimeOpts(proxmoxpkg.ProxmoxRuntimeOpts{Client: client, AuthMode: proxmox.AuthModeAPIToken, TokenID: "id"}), "pve01", "node", "", proxmox.ProxyTicket{
		Port:   5900,
		Ticket: "PVEVNC:test",
	}); err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("expected HTTP 403 error detail for empty-body websocket failure, got %v", err)
	}
}

func TestDialProxmoxProxySocketBuildURLFailure(t *testing.T) {
	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     "://not-a-valid-url",
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient should not validate base URL parseability, got %v", err)
	}

	_, err = proxmoxpkg.DialProxmoxProxySocket(proxmoxpkg.NewProxmoxRuntime(client), "pve01", "node", "", proxmox.ProxyTicket{
		Port:   5900,
		Ticket: "PVEVNC:test",
	})
	if err == nil {
		t.Fatalf("expected BuildVNCWebSocketURL parse failure")
	}
}

func TestDialProxmoxProxySocketPasswordSessionTicketFailure(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/access/ticket" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errors":"invalid credentials"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:  server.URL,
		AuthMode: proxmox.AuthModePassword,
		Username: "root@pam",
		Password: "wrong",
		Timeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	_, err = proxmoxpkg.DialProxmoxProxySocket(proxmoxpkg.NewProxmoxRuntimeOpts(proxmoxpkg.ProxmoxRuntimeOpts{Client: client, AuthMode: proxmox.AuthModePassword}), "pve01", "node", "", proxmox.ProxyTicket{
		Port:   5900,
		Ticket: "PVEVNC:test",
	})
	if err == nil || !strings.Contains(err.Error(), "acquire session ticket for websocket") {
		t.Fatalf("expected session-ticket acquisition failure, got %v", err)
	}
}

func TestDialProxmoxProxySocketTokenAndPasswordAuth(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	var tokenDialed bool
	var passwordDialed bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/access/ticket":
			_, _ = w.Write([]byte(`{"data":{"ticket":"PVE:session-ticket","CSRFPreventionToken":"csrf-token"}}`))
			return
		case "/api2/json/nodes/pve01/vncwebsocket":
			if websocket.IsWebSocketUpgrade(r) {
				if strings.Contains(r.Header.Get("Authorization"), "PVEAPIToken=id=secret") {
					tokenDialed = true
				}
				if strings.Contains(r.Header.Get("Cookie"), "PVEAuthCookie=PVE:session-ticket") {
					passwordDialed = true
				}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Fatalf("upgrade failed: %v", err)
				}
				defer conn.Close()
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	tokenClient, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient token mode failed: %v", err)
	}
	tokenRuntime := proxmoxpkg.NewProxmoxRuntimeOpts(proxmoxpkg.ProxmoxRuntimeOpts{Client: tokenClient, AuthMode: proxmox.AuthModeAPIToken, TokenID: "id", TokenSecret: "secret"})
	conn, err := proxmoxpkg.DialProxmoxProxySocket(tokenRuntime, "pve01", "node", "", proxmox.ProxyTicket{
		Port:   5900,
		Ticket: "PVEVNC:token",
	})
	if err != nil {
		t.Fatalf("proxmoxpkg.DialProxmoxProxySocket token mode failed: %v", err)
	}
	_ = conn.Close()
	if !tokenDialed {
		t.Fatalf("expected token-auth websocket dial to set Authorization header")
	}

	passwordClient, err := proxmox.NewClient(proxmox.Config{
		BaseURL:  server.URL,
		AuthMode: proxmox.AuthModePassword,
		Username: "root@pam",
		Password: "secret",
		Timeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient password mode failed: %v", err)
	}
	passwordRuntime := proxmoxpkg.NewProxmoxRuntimeOpts(proxmoxpkg.ProxmoxRuntimeOpts{Client: passwordClient, AuthMode: proxmox.AuthModePassword})
	conn, err = proxmoxpkg.DialProxmoxProxySocket(passwordRuntime, "pve01", "node", "", proxmox.ProxyTicket{
		Port:   5900,
		Ticket: "PVEVNC:password",
	})
	if err != nil {
		t.Fatalf("proxmoxpkg.DialProxmoxProxySocket password mode failed: %v", err)
	}
	_ = conn.Close()
	if !passwordDialed {
		t.Fatalf("expected password-auth websocket dial to send PVEAuthCookie")
	}
}
