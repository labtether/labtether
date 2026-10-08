package main

import (
	"crypto/tls"
	"encoding/pem"
	"errors"
	"github.com/labtether/labtether/internal/terminal"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDesktopSPICEProxyTargetIsConsumedOnce(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.setDesktopSPICEProxyTarget("sess-1", desktopSPICEProxyTarget{
		Host:    "127.0.0.1",
		TLSPort: 61000,
	})

	target, ok := sut.takeDesktopSPICEProxyTarget("sess-1")
	if !ok {
		t.Fatal("expected spice proxy target to be available")
	}
	if target.Host != "127.0.0.1" || target.TLSPort != 61000 {
		t.Fatalf("unexpected spice target: %+v", target)
	}

	if _, ok := sut.takeDesktopSPICEProxyTarget("sess-1"); ok {
		t.Fatal("expected spice proxy target to be one-time consumable")
	}
}

func TestHandleDesktopStreamSPICERequiresPreissuedTicketFlow(t *testing.T) {
	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/desktop/sessions/sess-spice/stream?protocol=spice", nil)
	rec := httptest.NewRecorder()

	sut.handleDesktopStream(rec, req, terminal.Session{
		ID:     "sess-spice",
		Target: "desktop-node-01",
		Mode:   "desktop",
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when spice stream target is missing, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "spice ticket") {
		t.Fatalf("expected spice ticket guidance error, got %s", rec.Body.String())
	}
}

func TestHandleProxmoxSPICEStreamRequiresGlobalGateForSkipVerify(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "")
	sut := newTestAPIServer(t)
	sut.setDesktopSPICEProxyTarget("sess-spice-unverified", desktopSPICEProxyTarget{
		Host:       "192.0.2.90",
		TLSPort:    61000,
		SkipVerify: true,
	})
	req := httptest.NewRequest(http.MethodGet, "/desktop/sessions/sess-spice-unverified/stream", nil)
	rec := httptest.NewRecorder()
	sut.handleDesktopSPICEStream(rec, req, terminal.Session{ID: "sess-spice-unverified", Mode: "desktop"})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unverified Proxmox SPICE status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "certificate checks") {
		t.Fatalf("unverified Proxmox SPICE error was unclear: %s", rec.Body.String())
	}
}

func TestNewProxmoxTLSConfigAcceptsProvidedCAPEM(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cert := server.Certificate()
	if cert == nil {
		t.Fatal("expected test tls server certificate")
	}

	tlsConfig, err := newProxmoxTLSConfig(false, string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})))
	if err != nil {
		t.Fatalf("newProxmoxTLSConfig returned error: %v", err)
	}

	host, _, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}
	if net.ParseIP(host) == nil {
		tlsConfig.ServerName = host
	}

	conn, err := tls.Dial("tcp", server.Listener.Addr().String(), tlsConfig)
	if err != nil {
		t.Fatalf("expected provided ca pem to allow tls handshake: %v", err)
	}
	_ = conn.Close()
}

func TestNewProxmoxTLSConfigRejectsInvalidCAPEM(t *testing.T) {
	if _, err := newProxmoxTLSConfig(false, "not a pem"); err == nil {
		t.Fatal("expected invalid ca pem to be rejected")
	}
}

func TestProxmoxSPICEOpenErrorResponse(t *testing.T) {
	tests := []struct {
		name       string
		input      error
		wantStatus int
		wantSubstr string
	}{
		{
			name:       "no spice port",
			input:      errors.New(`proxmox api returned 500: {"message":"no spice port\n","data":null}`),
			wantStatus: http.StatusConflict,
			wantSubstr: "not configured for SPICE",
		},
		{
			name:       "vm not running",
			input:      errors.New(`proxmox api returned 500: {"message":"VM 102 not running\n","data":null}`),
			wantStatus: http.StatusConflict,
			wantSubstr: "must be running",
		},
		{
			name:       "generic upstream error",
			input:      errors.New("upstream dial timeout"),
			wantStatus: http.StatusBadGateway,
			wantSubstr: "failed to open SPICE proxy",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotStatus, gotMessage := proxmoxSPICEOpenErrorResponse(tc.input)
			if gotStatus != tc.wantStatus {
				t.Fatalf("status = %d, want %d", gotStatus, tc.wantStatus)
			}
			if !strings.Contains(strings.ToLower(gotMessage), strings.ToLower(tc.wantSubstr)) {
				t.Fatalf("message = %q, want substring %q", gotMessage, tc.wantSubstr)
			}
		})
	}
}
