package main

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/credentials"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/terminal"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoadProxmoxRuntimeBranches(t *testing.T) {
	sut := newTestAPIServer(t)

	if _, err := sut.loadProxmoxRuntime(""); err == nil || !strings.Contains(err.Error(), "hub collector store unavailable") {
		t.Fatalf("expected missing hub collector store error, got %v", err)
	}

	sut.hubCollectorStore = &stubHubCollectorStore{collectors: nil}
	if _, err := sut.loadProxmoxRuntime(""); err == nil || !strings.Contains(err.Error(), "no active proxmox collector configured") {
		t.Fatalf("expected no active collector error, got %v", err)
	}

	sut = newTestAPIServer(t)
	sut.credentialStore = nil
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{{
			ID:            "collector-proxmox-1",
			CollectorType: hubcollector.CollectorTypeProxmox,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      "https://proxmox.example.local",
				"credential_id": "cred-proxmox-1",
			},
		}},
	}
	if _, err := sut.loadProxmoxRuntime(""); err == nil || !strings.Contains(err.Error(), "credential store unavailable") {
		t.Fatalf("expected missing credential store error, got %v", err)
	}

	sut = newTestAPIServer(t)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{{
			ID:            "collector-proxmox-1",
			CollectorType: hubcollector.CollectorTypeProxmox,
			Enabled:       true,
			Config: map[string]any{
				"base_url": "https://proxmox.example.local",
			},
		}},
	}
	if _, err := sut.loadProxmoxRuntime(""); err == nil || !strings.Contains(err.Error(), "config is incomplete") {
		t.Fatalf("expected incomplete config error, got %v", err)
	}

	sut = newTestAPIServer(t)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{{
			ID:            "collector-proxmox-1",
			CollectorType: hubcollector.CollectorTypeProxmox,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      "https://proxmox.example.local",
				"credential_id": "missing-credential",
			},
		}},
	}
	if _, err := sut.loadProxmoxRuntime(""); err == nil || !strings.Contains(err.Error(), "credential profile not found") {
		t.Fatalf("expected missing credential error, got %v", err)
	}

	sut = newTestAPIServer(t)
	if _, err := sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               "cred-bad-cipher",
		Name:             "bad-cipher",
		Kind:             credentials.KindProxmoxAPIToken,
		Username:         "labtether@pve!agent",
		Status:           "active",
		SecretCiphertext: "not-valid-ciphertext",
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("failed to create invalid ciphertext profile: %v", err)
	}
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{{
			ID:            "collector-proxmox-1",
			CollectorType: hubcollector.CollectorTypeProxmox,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      "https://proxmox.example.local",
				"credential_id": "cred-bad-cipher",
			},
		}},
	}
	if _, err := sut.loadProxmoxRuntime(""); err == nil || !strings.Contains(err.Error(), "failed to decrypt proxmox credential") {
		t.Fatalf("expected decrypt error, got %v", err)
	}

	sut = newTestAPIServer(t)
	createProxmoxCredentialProfile(t, sut, "cred-password-missing-user", "", "secret", "https://proxmox.example.local")
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{{
			ID:            "collector-proxmox-1",
			CollectorType: hubcollector.CollectorTypeProxmox,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      "https://proxmox.example.local",
				"credential_id": "cred-password-missing-user",
				"auth_method":   "password",
			},
		}},
	}
	if _, err := sut.loadProxmoxRuntime(""); err == nil || !strings.Contains(err.Error(), "username missing") {
		t.Fatalf("expected password-mode username missing error, got %v", err)
	}

	sut = newTestAPIServer(t)
	createProxmoxCredentialProfile(t, sut, "cred-token-missing-id", "", "secret", "https://proxmox.example.local")
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{{
			ID:            "collector-proxmox-1",
			CollectorType: hubcollector.CollectorTypeProxmox,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      "https://proxmox.example.local",
				"credential_id": "cred-token-missing-id",
			},
		}},
	}
	if _, err := sut.loadProxmoxRuntime(""); err == nil || !strings.Contains(err.Error(), "token_id missing") {
		t.Fatalf("expected token_id missing error, got %v", err)
	}

	sut = newTestAPIServer(t)
	createProxmoxCredentialProfile(t, sut, "cred-cache-hit", "labtether@pve!agent", "secret", "https://proxmox.example.local")
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{{
			ID:            "collector-proxmox-1",
			CollectorType: hubcollector.CollectorTypeProxmox,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      "https://proxmox.example.local",
				"token_id":      "labtether@pve!agent",
				"credential_id": "cred-cache-hit",
			},
		}},
	}

	// Pin the proxmox deps so the cache persists across calls.
	sut.proxmoxDeps = sut.buildProxmoxDeps()

	first, err := sut.loadProxmoxRuntime("")
	if err != nil {
		t.Fatalf("first loadProxmoxRuntime failed: %v", err)
	}
	second, err := sut.loadProxmoxRuntime("")
	if err != nil {
		t.Fatalf("second loadProxmoxRuntime failed: %v", err)
	}
	if first != second {
		t.Fatalf("expected loadProxmoxRuntime cache hit to reuse runtime pointer")
	}
	if first.SkipVerify() {
		t.Fatalf("expected skipVerify default to false when collector config omits skip_verify")
	}
}

func TestLoadProxmoxRuntimeAdditionalBranches(t *testing.T) {
	t.Run("collector list failure", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.hubCollectorStore = &listErrorHubCollectorStore{
			listErr: errors.New("list failed"),
		}
		if _, err := sut.loadProxmoxRuntime(""); err == nil || !strings.Contains(err.Error(), "failed to list hub collectors") {
			t.Fatalf("expected list hub collectors error, got %v", err)
		}
	})

	t.Run("skip non proxmox collector", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.hubCollectorStore = &stubHubCollectorStore{
			collectors: []hubcollector.Collector{{
				ID:            "collector-docker-1",
				CollectorType: hubcollector.CollectorTypeDocker,
				Enabled:       true,
				Config: map[string]any{
					"base_url": "http://docker.example.local",
				},
			}},
		}
		if _, err := sut.loadProxmoxRuntime(""); err == nil || !strings.Contains(err.Error(), "no active proxmox collector configured") {
			t.Fatalf("expected no proxmox collector configured error, got %v", err)
		}
	})

	t.Run("password auth success path", func(t *testing.T) {
		sut := newTestAPIServer(t)
		createProxmoxCredentialProfile(t, sut, "cred-password-ok", "root@pam", "secret", "https://proxmox.example.local")
		sut.hubCollectorStore = &stubHubCollectorStore{
			collectors: []hubcollector.Collector{{
				ID:            "collector-proxmox-password",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      "https://proxmox.example.local",
					"credential_id": "cred-password-ok",
					"auth_method":   "password",
				},
			}},
		}

		runtime, err := sut.loadProxmoxRuntime("")
		if err != nil {
			t.Fatalf("expected password auth runtime to load, got %v", err)
		}
		if runtime.AuthMode() != proxmox.AuthModePassword {
			t.Fatalf("expected password auth mode, got %q", runtime.AuthMode())
		}
	})

	t.Run("password auth prefers collector username over credential username", func(t *testing.T) {
		sut := newTestAPIServer(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api2/json/access/ticket" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if got := r.Form.Get("username"); got != "root@pam" {
				t.Fatalf("expected collector username override, got %q", got)
			}
			if got := r.Form.Get("password"); got != "secret" {
				t.Fatalf("expected password secret, got %q", got)
			}
			_, _ = w.Write([]byte(`{"data":{"ticket":"PVE:ticket-override","CSRFPreventionToken":"csrf-override"}}`))
		}))
		defer server.Close()

		createProxmoxCredentialProfile(t, sut, "cred-password-stale-user", "labtether@pve!monitoring", "secret", server.URL)
		sut.hubCollectorStore = &stubHubCollectorStore{
			collectors: []hubcollector.Collector{{
				ID:            "collector-proxmox-password-override",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      server.URL,
					"credential_id": "cred-password-stale-user",
					"auth_method":   "password",
					"username":      "root@pam",
				},
			}},
		}

		runtime, err := sut.loadProxmoxRuntime("")
		if err != nil {
			t.Fatalf("expected password auth runtime to load, got %v", err)
		}
		ticket, _, err := runtime.Client().GetTicket(context.Background())
		if err != nil {
			t.Fatalf("expected ticket acquisition to succeed, got %v", err)
		}
		if ticket != "PVE:ticket-override" {
			t.Fatalf("unexpected ticket: %q", ticket)
		}
	})

	t.Run("invalid client config", func(t *testing.T) {
		sut := newTestAPIServer(t)
		createProxmoxCredentialProfile(t, sut, "cred-invalid-client", "labtether@pve!agent", "secret", "https://proxmox.example.local")
		sut.hubCollectorStore = &stubHubCollectorStore{
			collectors: []hubcollector.Collector{{
				ID:            "collector-proxmox-invalid",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      "https://proxmox.example.local",
					"token_id":      "labtether@pve!agent",
					"credential_id": "cred-invalid-client",
					"ca_pem":        "not-valid-pem",
				},
			}},
		}
		if _, err := sut.loadProxmoxRuntime(""); err == nil {
			t.Fatalf("expected proxmox client initialization to fail for invalid ca_pem")
		}
	})
}

func TestLoadProxmoxRuntimeSeparatesAPIAndSPICECertificateBypasses(t *testing.T) {
	sut := newTestAPIServer(t)
	createProxmoxCredentialProfile(t, sut, "cred-spice-tls-split", "labtether@pve!agent", "secret", "https://proxmox.example.local")
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{{
			ID:            "collector-proxmox-spice-tls-split",
			CollectorType: hubcollector.CollectorTypeProxmox,
			Enabled:       true,
			Config: map[string]any{
				"base_url":          "https://proxmox.example.local",
				"token_id":          "labtether@pve!agent",
				"credential_id":     "cred-spice-tls-split",
				"skip_verify":       true,
				"spice_skip_verify": false,
			},
		}},
	}

	runtime, err := sut.loadProxmoxRuntime("")
	if err != nil {
		t.Fatalf("load Proxmox runtime: %v", err)
	}
	if !runtime.SkipVerify() {
		t.Fatal("API skip_verify setting was not preserved")
	}
	if runtime.SPICESkipVerify() {
		t.Fatal("API skip_verify silently disabled SPICE certificate verification")
	}
}

func TestProxmoxSPICETicketRejectsCertificateDataFromUnverifiedAPI(t *testing.T) {
	var apiCalls atomic.Int32
	forgedAPI := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		apiCalls.Add(1)
		_, _ = w.Write([]byte(`{"data":{"host":"pvespiceproxy:68b8d480:101:pve01::aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","tls-port":61000,"proxy":"http://attacker.example:3128","host-subject":"CN=attacker","ca":"attacker CA"}}`))
	}))
	defer forgedAPI.Close()

	sut := newTestAPIServer(t)
	createProxmoxCredentialProfile(t, sut, "cred-spice-untrusted-api", "labtether@pve!agent", "secret", forgedAPI.URL)
	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{{
			ID:            "collector-spice-untrusted-api",
			CollectorType: hubcollector.CollectorTypeProxmox,
			Enabled:       true,
			Config: map[string]any{
				"base_url":          forgedAPI.URL,
				"token_id":          "labtether@pve!agent",
				"credential_id":     "cred-spice-untrusted-api",
				"skip_verify":       true,
				"spice_skip_verify": false,
			},
		}},
	}
	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-vm-101",
		Type:    "vm",
		Name:    "pve01",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "qemu",
			"node":         "pve01",
			"vmid":         "101",
			"collector_id": "collector-spice-untrusted-api",
		},
	})
	if err != nil {
		t.Fatalf("seed Proxmox VM: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/desktop/sessions/session-untrusted-api/spice-ticket", nil)
	rec := httptest.NewRecorder()
	sut.handleDesktopSPICETicket(rec, req, terminal.Session{
		ID:     "session-untrusted-api",
		Target: "proxmox-vm-101",
		Mode:   "desktop",
	})

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "API certificate") {
		t.Fatalf("unverified API SPICE status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := apiCalls.Load(); got != 0 {
		t.Fatalf("fetched %d forged SPICE ticket responses before enforcing API trust", got)
	}
}

func TestNewProxmoxDialerInvalidPEM(t *testing.T) {
	if _, err := proxmoxpkg.NewProxmoxDialer(true, "not-a-pem"); err == nil {
		t.Fatalf("expected invalid ca_pem to fail")
	}
}

func TestNewProxmoxDialerValidPEM(t *testing.T) {
	caPEM := testCAPEM(t)
	dialer, err := proxmoxpkg.NewProxmoxDialer(false, caPEM)
	if err != nil {
		t.Fatalf("expected valid ca_pem to succeed, got %v", err)
	}
	if dialer.TLSClientConfig == nil || dialer.TLSClientConfig.RootCAs == nil {
		t.Fatalf("expected RootCAs to be configured from ca_pem")
	}
}

func TestOpenProxmoxTerminalTicketUnsupportedKind(t *testing.T) {
	sut := newTestAPIServer(t)
	_, err := sut.openProxmoxTerminalTicket(context.Background(), nil, proxmoxSessionTarget{Kind: "unsupported"})
	if err == nil || !strings.Contains(err.Error(), "unsupported proxmox terminal kind") {
		t.Fatalf("expected unsupported kind error, got %v", err)
	}
}

func TestOpenProxmoxTerminalTicketSupportedKinds(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/termproxy",
			"/api2/json/nodes/pve01/qemu/100/termproxy",
			"/api2/json/nodes/pve01/lxc/200/termproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:ticket","user":"root@pam"}}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
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
	sut := newTestAPIServer(t)

	cases := []proxmoxSessionTarget{
		{Kind: "node", Node: "pve01"},
		{Kind: "qemu", Node: "pve01", VMID: "100"},
		{Kind: "lxc", Node: "pve01", VMID: "200"},
	}
	for _, tc := range cases {
		ticket, err := sut.openProxmoxTerminalTicket(context.Background(), runtime, tc)
		if err != nil {
			t.Fatalf("openProxmoxTerminalTicket(%s) failed: %v", tc.Kind, err)
		}
		if ticket.Port.Int() != 5900 || strings.TrimSpace(ticket.Ticket) == "" {
			t.Fatalf("unexpected proxy ticket for %s: %+v", tc.Kind, ticket)
		}
	}
}
