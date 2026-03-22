package main

import (
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"context"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/terminal"
)

type listErrorHubCollectorStore struct {
	stubHubCollectorStore
	listErr error
}

func (s *listErrorHubCollectorStore) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.stubHubCollectorStore.ListHubCollectors(limit, enabledOnly)
}

func TestResolveProxmoxSessionTargetStorageFallbacks(t *testing.T) {
	sut := newTestAPIServer(t)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-local-zfs",
		Type:    "storage-pool",
		Name:    "storage/pve01/local-zfs",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "storage",
			"storage_id":   "storage/pve01/local-zfs",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed storage asset: %v", err)
	}

	target, ok, err := sut.resolveProxmoxSessionTarget("proxmox-storage-local-zfs")
	if err != nil {
		t.Fatalf("resolveProxmoxSessionTarget returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected proxmox storage target to resolve")
	}
	if target.Kind != "storage" || target.Node != "pve01" || target.StorageName != "local-zfs" {
		t.Fatalf("unexpected resolved storage target: %+v", target)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-fast-ssd",
		Type:    "storage-pool",
		Name:    "storage/pve02/fast-ssd",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "storage",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed second storage asset: %v", err)
	}
	target, ok, err = sut.resolveProxmoxSessionTarget("proxmox-storage-fast-ssd")
	if err != nil {
		t.Fatalf("resolveProxmoxSessionTarget returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected second proxmox storage target to resolve")
	}
	if target.Node != "pve02" || target.StorageName != "fast-ssd" {
		t.Fatalf("unexpected fallback target parse: %+v", target)
	}
}

func TestResolveProxmoxSessionTargetStorageMissingNode(t *testing.T) {
	sut := newTestAPIServer(t)
	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-broken",
		Type:    "storage-pool",
		Name:    "",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "storage",
			"storage_id":   "local-zfs",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed broken storage asset: %v", err)
	}

	_, ok, err := sut.resolveProxmoxSessionTarget("proxmox-storage-broken")
	if !ok {
		t.Fatalf("expected proxmox asset detection to be true")
	}
	if err == nil || !strings.Contains(err.Error(), "missing node metadata") {
		t.Fatalf("expected missing node metadata error, got %v", err)
	}
}

func TestResolveProxmoxSessionTargetAdditionalBranches(t *testing.T) {
	sut := newTestAPIServer(t)

	target, ok, err := sut.resolveProxmoxSessionTarget("missing-asset")
	if err != nil || ok || target != (proxmoxSessionTarget{}) {
		t.Fatalf("expected unresolved missing asset, got target=%+v ok=%v err=%v", target, ok, err)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "agent-host-01",
		Type:    "server",
		Name:    "agent-host-01",
		Source:  "agent",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("failed to seed non-proxmox asset: %v", err)
	}
	target, ok, err = sut.resolveProxmoxSessionTarget("agent-host-01")
	if err != nil || ok || target != (proxmoxSessionTarget{}) {
		t.Fatalf("expected non-proxmox asset to be ignored, got target=%+v ok=%v err=%v", target, ok, err)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-vm-110",
		Type:    "vm",
		Name:    "pve01",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"node": "pve01",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed vm asset: %v", err)
	}
	target, ok, err = sut.resolveProxmoxSessionTarget("proxmox-vm-110")
	if err != nil || !ok || target.Kind != "qemu" || target.VMID != "110" {
		t.Fatalf("expected inferred qemu target with vmid 110, got target=%+v ok=%v err=%v", target, ok, err)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-vm-",
		Type:    "vm",
		Name:    "pve01",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"node": "pve01",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed broken vm asset: %v", err)
	}
	_, ok, err = sut.resolveProxmoxSessionTarget("proxmox-vm-")
	if !ok || err == nil || !strings.Contains(err.Error(), "missing vmid metadata") {
		t.Fatalf("expected missing vmid metadata error for broken vm id, got ok=%v err=%v", ok, err)
	}

	withoutAssetStore := newTestAPIServer(t)
	withoutAssetStore.assetStore = nil
	target, ok, err = withoutAssetStore.resolveProxmoxSessionTarget("anything")
	if err != nil || ok || target != (proxmoxSessionTarget{}) {
		t.Fatalf("expected nil asset store to short-circuit, got target=%+v ok=%v err=%v", target, ok, err)
	}
}

func TestResolveProxmoxSessionTargetInferenceBranches(t *testing.T) {
	sut := newTestAPIServer(t)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-ct-200",
		Type:    "container",
		Name:    "pve01",
		Source:  "proxmox",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("failed to seed inferred container asset: %v", err)
	}

	target, ok, err := sut.resolveProxmoxSessionTarget("proxmox-ct-200")
	if err != nil || !ok {
		t.Fatalf("expected inferred lxc target, got target=%+v ok=%v err=%v", target, ok, err)
	}
	if target.Kind != "lxc" || target.VMID != "200" {
		t.Fatalf("expected inferred lxc vmid 200, got %+v", target)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-fast",
		Type:    "storage-pool",
		Name:    "fast",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"storage_id": "pve02/fast",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed inferred storage asset: %v", err)
	}

	target, ok, err = sut.resolveProxmoxSessionTarget("proxmox-storage-fast")
	if err != nil || !ok {
		t.Fatalf("expected inferred storage target, got target=%+v ok=%v err=%v", target, ok, err)
	}
	if target.Kind != "storage" || target.Node != "pve02" || target.StorageName != "fast" {
		t.Fatalf("unexpected inferred storage target from two-part storage_id: %+v", target)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-node-inferred",
		Type:    "host",
		Name:    "pve03",
		Source:  "proxmox",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("failed to seed inferred node asset: %v", err)
	}

	target, ok, err = sut.resolveProxmoxSessionTarget("proxmox-node-inferred")
	if err != nil || !ok {
		t.Fatalf("expected inferred node target, got target=%+v ok=%v err=%v", target, ok, err)
	}
	if target.Kind != "node" || target.Node != "pve03" {
		t.Fatalf("unexpected inferred node target: %+v", target)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-name-fallback",
		Type:    "storage-pool",
		Name:    "pve04/archive",
		Source:  "proxmox",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("failed to seed storage name-fallback asset: %v", err)
	}

	target, ok, err = sut.resolveProxmoxSessionTarget("proxmox-storage-name-fallback")
	if err != nil || !ok {
		t.Fatalf("expected storage name-fallback target, got target=%+v ok=%v err=%v", target, ok, err)
	}
	if target.Node != "pve04" || target.StorageName != "archive" {
		t.Fatalf("unexpected storage name fallback parse: %+v", target)
	}
}

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

func TestTranslateBrowserToProxmoxTerm(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		want    string
		nilOut  bool
	}{
		{
			name:    "resize",
			payload: []byte(`{"type":"resize","cols":120,"rows":40}`),
			want:    "1:120:40:",
		},
		{
			name:    "ping",
			payload: []byte(`{"type":"ping"}`),
			want:    "2",
		},
		{
			name:    "input",
			payload: []byte(`{"type":"input","data":"ls\n"}`),
			want:    "0:3:ls\n",
		},
		{
			name:    "resize-invalid",
			payload: []byte(`{"type":"resize","cols":0,"rows":40}`),
			nilOut:  true,
		},
		{
			name:    "input-empty",
			payload: []byte(`{"type":"input","data":""}`),
			nilOut:  true,
		},
		{
			name:    "unknown-control",
			payload: []byte(`{"type":"unknown"}`),
			nilOut:  true,
		},
		{
			name:    "raw",
			payload: []byte("pwd\n"),
			want:    "0:4:pwd\n",
		},
		{
			name:    "empty",
			payload: []byte(""),
			nilOut:  true,
		},
	}

	for _, tc := range cases {
		got := proxmoxpkg.TranslateBrowserToProxmoxTerm(tc.payload)
		if tc.nilOut {
			if got != nil {
				t.Fatalf("%s: expected nil output, got %q", tc.name, string(got))
			}
			continue
		}
		if string(got) != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, string(got))
		}
	}
}

func TestProxmoxVNCUtilityFunctions(t *testing.T) {
	if got := proxmoxpkg.VNCReverseBits(0x12); got != 0x48 {
		t.Fatalf("expected bit reverse 0x12 -> 0x48, got %#x", got)
	}
	key := proxmoxpkg.VNCDESKey("password")
	if key[0] != proxmoxpkg.VNCReverseBits('p') {
		t.Fatalf("unexpected DES key first byte: %#x", key[0])
	}
	response := proxmoxpkg.VNCEncryptChallenge([]byte("0123456789ABCDEF"), "password")
	if len(response) != 16 {
		t.Fatalf("expected 16-byte encrypted challenge, got %d", len(response))
	}
	second := proxmoxpkg.VNCEncryptChallenge([]byte("0123456789ABCDEF"), "password")
	if string(response) != string(second) {
		t.Fatalf("expected deterministic encryption output")
	}

	withProxmoxStreamHooks(
		t,
		nil,
		nil,
		nil,
		func([]byte) (cipher.Block, error) {
			return nil, errors.New("forced cipher construction failure")
		},
	)
	fallback := proxmoxpkg.VNCEncryptChallenge([]byte("0123456789ABCDEF"), "password")
	if len(fallback) != 16 {
		t.Fatalf("expected 16-byte fallback challenge, got %d", len(fallback))
	}
	for i, b := range fallback {
		if b != 0 {
			t.Fatalf("expected zeroed fallback response byte at %d, got %d", i, b)
		}
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

func TestPerformProxmoxVNCAuthNone(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()

		version := []byte("RFB 003.008\n")
		if err := conn.WriteMessage(websocket.BinaryMessage, version); err != nil {
			t.Fatalf("failed to send version: %v", err)
		}
		_, echoedVersion, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read echoed version: %v", err)
		}
		if string(echoedVersion) != string(version) {
			t.Fatalf("unexpected echoed version: %q", string(echoedVersion))
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 1}); err != nil {
			t.Fatalf("failed to send security types: %v", err)
		}
		_, selection, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read selection: %v", err)
		}
		if len(selection) != 1 || selection[0] != 1 {
			t.Fatalf("unexpected security selection: %v", selection)
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0, 0}); err != nil {
			t.Fatalf("failed to send auth result: %v", err)
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()

	version, err := proxmoxpkg.PerformProxmoxVNCAuth(conn, "")
	if err != nil {
		t.Fatalf("proxmoxpkg.PerformProxmoxVNCAuth failed: %v", err)
	}
	if version != "RFB 003.008" {
		t.Fatalf("unexpected RFB version: %s", version)
	}
}

func TestPerformProxmoxVNCAuthVNCAuthChallenge(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	password := "s3cret!"
	challenge := []byte("0123456789ABCDEF")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()

		version := []byte("RFB 003.008\n")
		if err := conn.WriteMessage(websocket.BinaryMessage, version); err != nil {
			t.Fatalf("failed to send version: %v", err)
		}
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("failed to read echoed version: %v", err)
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2}); err != nil {
			t.Fatalf("failed to send VNC auth security type: %v", err)
		}
		_, selection, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read security selection: %v", err)
		}
		if len(selection) != 1 || selection[0] != 2 {
			t.Fatalf("unexpected VNC auth selection: %v", selection)
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, challenge); err != nil {
			t.Fatalf("failed to send challenge: %v", err)
		}
		_, response, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read challenge response: %v", err)
		}
		expected := proxmoxpkg.VNCEncryptChallenge(challenge, password)
		if string(response) != string(expected) {
			t.Fatalf("unexpected challenge response")
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0, 0}); err != nil {
			t.Fatalf("failed to send auth result: %v", err)
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()

	if _, err := proxmoxpkg.PerformProxmoxVNCAuth(conn, password); err != nil {
		t.Fatalf("proxmoxpkg.PerformProxmoxVNCAuth VNC auth branch failed: %v", err)
	}
}

func TestPerformProxmoxVNCAuthFailureBranches(t *testing.T) {
	type scenario struct {
		name        string
		handler     func(t *testing.T, conn *websocket.Conn)
		expectError string
	}

	scenarios := []scenario{
		{
			name: "empty-security-message",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{}); err != nil {
					t.Fatalf("failed to write empty security message: %v", err)
				}
			},
			expectError: "empty security message",
		},
		{
			name: "incomplete-security-types",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{2, 1}); err != nil {
					t.Fatalf("failed to write incomplete security types: %v", err)
				}
			},
			expectError: "incomplete security types",
		},
		{
			name: "zero-security-types",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0}); err != nil {
					t.Fatalf("failed to write zero security types: %v", err)
				}
			},
			expectError: "0 security types",
		},
		{
			name: "no-supported-security-type",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 3}); err != nil {
					t.Fatalf("failed to write unsupported security types: %v", err)
				}
			},
			expectError: "no supported security type",
		},
		{
			name: "none-auth-rejected",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 1}); err != nil {
					t.Fatalf("failed to write None security type: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read selected security type: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0, 1}); err != nil {
					t.Fatalf("failed to write rejected auth result: %v", err)
				}
			},
			expectError: "none auth unexpectedly rejected",
		},
		{
			name: "short-vnc-challenge",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2}); err != nil {
					t.Fatalf("failed to write VNC auth security type: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read selected security type: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("short")); err != nil {
					t.Fatalf("failed to write short challenge: %v", err)
				}
			},
			expectError: "unexpected challenge length",
		},
		{
			name: "short-auth-result",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2}); err != nil {
					t.Fatalf("failed to write VNC auth security type: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read selected security type: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("0123456789ABCDEF")); err != nil {
					t.Fatalf("failed to write challenge: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read challenge response: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0}); err != nil {
					t.Fatalf("failed to write short auth result: %v", err)
				}
			},
			expectError: "short auth result",
		},
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Fatalf("upgrade failed: %v", err)
				}
				defer conn.Close()
				tc.handler(t, conn)
			}))
			defer server.Close()

			wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Fatalf("failed to dial websocket: %v", err)
			}
			defer conn.Close()

			if _, err := proxmoxpkg.PerformProxmoxVNCAuth(conn, "secret"); err == nil || !strings.Contains(err.Error(), tc.expectError) {
				t.Fatalf("expected %q error, got %v", tc.expectError, err)
			}
		})
	}
}

func TestPerformProxmoxVNCAuthTransportAndWriteBranches(t *testing.T) {
	type scenario struct {
		name        string
		handler     func(t *testing.T, conn *websocket.Conn)
		expectError string
	}

	scenarios := []scenario{
		{
			name: "read-rfb-version-failure",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				_ = conn.Close()
			},
			expectError: "read RFB version",
		},
		{
			name: "read-security-types-failure",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed RFB version: %v", err)
				}
				_ = conn.Close()
			},
			expectError: "read security types",
		},
		{
			name: "read-none-auth-result-failure",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed RFB version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 1}); err != nil {
					t.Fatalf("failed to write security types: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read None auth selection: %v", err)
				}
				_ = conn.Close()
			},
			expectError: "read None auth result",
		},
		{
			name: "read-vnc-challenge-failure",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed RFB version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2}); err != nil {
					t.Fatalf("failed to write VNC security types: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read VNC auth selection: %v", err)
				}
				_ = conn.Close()
			},
			expectError: "read VNC challenge",
		},
		{
			name: "read-vnc-auth-result-failure",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed RFB version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2}); err != nil {
					t.Fatalf("failed to write VNC security types: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read VNC auth selection: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("0123456789ABCDEF")); err != nil {
					t.Fatalf("failed to write challenge: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read VNC auth response: %v", err)
				}
				_ = conn.Close()
			},
			expectError: "read VNC auth result",
		},
		{
			name: "vnc-auth-result-rejected",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed RFB version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2}); err != nil {
					t.Fatalf("failed to write VNC security types: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read VNC auth selection: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("0123456789ABCDEF")); err != nil {
					t.Fatalf("failed to write challenge: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read VNC auth response: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0, 1}); err != nil {
					t.Fatalf("failed to write rejected VNC auth result: %v", err)
				}
			},
			expectError: "VNC authentication failed",
		},
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Fatalf("upgrade failed: %v", err)
				}
				defer conn.Close()
				tc.handler(t, conn)
			}))
			defer server.Close()

			wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Fatalf("failed to dial websocket: %v", err)
			}
			defer conn.Close()

			if _, err := proxmoxpkg.PerformProxmoxVNCAuth(conn, "secret"); err == nil || !strings.Contains(err.Error(), tc.expectError) {
				t.Fatalf("expected %q error, got %v", tc.expectError, err)
			}
		})
	}
}

func TestPerformProxmoxVNCAuthExplicitWriteFailures(t *testing.T) {
	t.Run("send RFB version failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		withProxmoxStreamHooks(t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == serverConn && messageType == websocket.BinaryMessage && strings.HasPrefix(string(payload), "RFB ") {
					return errors.New("forced version write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			nil,
			nil,
			nil,
		)

		go func() {
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n"))
		}()

		if _, err := proxmoxpkg.PerformProxmoxVNCAuth(serverConn, "secret"); err == nil || !strings.Contains(err.Error(), "send RFB version") {
			t.Fatalf("expected send RFB version failure, got %v", err)
		}
	})

	t.Run("send None selection failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		withProxmoxStreamHooks(t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == serverConn && messageType == websocket.BinaryMessage && len(payload) == 1 && payload[0] == 1 {
					return errors.New("forced none selection write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			nil,
			nil,
			nil,
		)

		go func() {
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n"))
			_, _, _ = clientConn.ReadMessage()
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte{1, 1})
			_, _, _ = clientConn.ReadMessage()
		}()

		if _, err := proxmoxpkg.PerformProxmoxVNCAuth(serverConn, "secret"); err == nil || !strings.Contains(err.Error(), "send None selection") {
			t.Fatalf("expected send None selection failure, got %v", err)
		}
	})

	t.Run("send VNC Auth selection failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		withProxmoxStreamHooks(t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == serverConn && messageType == websocket.BinaryMessage && len(payload) == 1 && payload[0] == 2 {
					return errors.New("forced vnc selection write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			nil,
			nil,
			nil,
		)

		go func() {
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n"))
			_, _, _ = clientConn.ReadMessage()
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte{1, 2})
			_, _, _ = clientConn.ReadMessage()
		}()

		if _, err := proxmoxpkg.PerformProxmoxVNCAuth(serverConn, "secret"); err == nil || !strings.Contains(err.Error(), "send VNC Auth selection") {
			t.Fatalf("expected send VNC Auth selection failure, got %v", err)
		}
	})

	t.Run("send VNC auth response failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		withProxmoxStreamHooks(t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == serverConn && messageType == websocket.BinaryMessage && len(payload) == 16 {
					return errors.New("forced vnc response write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			nil,
			nil,
			nil,
		)

		go func() {
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n"))
			_, _, _ = clientConn.ReadMessage()
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte{1, 2})
			_, _, _ = clientConn.ReadMessage()
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte("0123456789ABCDEF"))
		}()

		if _, err := proxmoxpkg.PerformProxmoxVNCAuth(serverConn, "secret"); err == nil || !strings.Contains(err.Error(), "send VNC auth response") {
			t.Fatalf("expected send VNC auth response failure, got %v", err)
		}
	})
}

func TestSendBrowserVNCNoAuth(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()

		if err := proxmoxpkg.SendBrowserVNCNoAuth(conn, "RFB 003.008"); err != nil {
			t.Fatalf("proxmoxpkg.SendBrowserVNCNoAuth failed: %v", err)
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()

	_, version, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read version: %v", err)
	}
	if string(version) != "RFB 003.008\n" {
		t.Fatalf("unexpected version payload: %q", string(version))
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, version); err != nil {
		t.Fatalf("failed to write echoed version: %v", err)
	}

	_, securityTypes, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read security types: %v", err)
	}
	if len(securityTypes) != 2 || securityTypes[0] != 1 || securityTypes[1] != 1 {
		t.Fatalf("unexpected security types payload: %v", securityTypes)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1}); err != nil {
		t.Fatalf("failed to write security selection: %v", err)
	}

	_, authResult, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read auth result: %v", err)
	}
	if len(authResult) != 4 || authResult[0] != 0 || authResult[1] != 0 || authResult[2] != 0 || authResult[3] != 0 {
		t.Fatalf("unexpected auth result payload: %v", authResult)
	}
}

func TestSendBrowserVNCNoAuthFailureBranches(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	errCh := make(chan error, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()
		errCh <- proxmoxpkg.SendBrowserVNCNoAuth(conn, "RFB 003.008")
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}

	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("failed to read initial RFB version: %v", err)
	}
	_ = conn.Close()

	select {
	case sendErr := <-errCh:
		if sendErr == nil || !strings.Contains(sendErr.Error(), "read browser RFB version") {
			t.Fatalf("expected browser version read failure, got %v", sendErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for proxmoxpkg.SendBrowserVNCNoAuth error")
	}

	errCh = make(chan error, 1)
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()
		errCh <- proxmoxpkg.SendBrowserVNCNoAuth(conn, "RFB 003.008")
	}))
	defer secondServer.Close()

	wsURL = "ws" + strings.TrimPrefix(secondServer.URL, "http")
	conn, _, err = websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial second websocket: %v", err)
	}
	_, version, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read second initial RFB version: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, version); err != nil {
		t.Fatalf("failed to echo browser version: %v", err)
	}
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("failed to read security types: %v", err)
	}
	_ = conn.Close()

	select {
	case sendErr := <-errCh:
		if sendErr == nil || !strings.Contains(sendErr.Error(), "read browser security selection") {
			t.Fatalf("expected security selection read failure, got %v", sendErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for security selection failure")
	}
}

func TestSendBrowserVNCNoAuthWriteFailureBranches(t *testing.T) {
	t.Run("write RFB version failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()
		_ = clientConn.Close()
		_ = serverConn.Close()

		if err := proxmoxpkg.SendBrowserVNCNoAuth(serverConn, "RFB 003.008"); err == nil || !strings.Contains(err.Error(), "send RFB version") {
			t.Fatalf("expected send RFB version write error, got %v", err)
		}
	})

	t.Run("write security types failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()

		errCh := make(chan error, 1)
		go func() {
			errCh <- proxmoxpkg.SendBrowserVNCNoAuth(serverConn, "RFB 003.008")
		}()

		_, version, err := clientConn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read version: %v", err)
		}
		if err := clientConn.WriteMessage(websocket.BinaryMessage, version); err != nil {
			t.Fatalf("failed to echo version: %v", err)
		}
		_ = clientConn.Close()

		select {
		case sendErr := <-errCh:
			if sendErr == nil {
				t.Fatalf("expected handshake failure after browser close")
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for send security types failure")
		}
	})
}

func TestSendBrowserVNCNoAuthDeterministicWriteFailures(t *testing.T) {
	t.Run("send security types failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		withProxmoxStreamHooks(t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == serverConn && messageType == websocket.BinaryMessage && len(payload) == 2 && payload[0] == 1 && payload[1] == 1 {
					return errors.New("forced security types write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			nil,
			nil,
			nil,
		)

		errCh := make(chan error, 1)
		go func() {
			errCh <- proxmoxpkg.SendBrowserVNCNoAuth(serverConn, "RFB 003.008")
		}()

		_, version, err := clientConn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read browser version: %v", err)
		}
		if err := clientConn.WriteMessage(websocket.BinaryMessage, version); err != nil {
			t.Fatalf("failed to echo browser version: %v", err)
		}

		select {
		case sendErr := <-errCh:
			if sendErr == nil || !strings.Contains(sendErr.Error(), "send security types") {
				t.Fatalf("expected send security types failure, got %v", sendErr)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for send security types failure")
		}
	})

	t.Run("send auth result failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		withProxmoxStreamHooks(t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == serverConn && messageType == websocket.BinaryMessage && len(payload) == 4 &&
					payload[0] == 0 && payload[1] == 0 && payload[2] == 0 && payload[3] == 0 {
					return errors.New("forced auth-result write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			nil,
			nil,
			nil,
		)

		errCh := make(chan error, 1)
		go func() {
			errCh <- proxmoxpkg.SendBrowserVNCNoAuth(serverConn, "RFB 003.008")
		}()

		_, version, err := clientConn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read browser version: %v", err)
		}
		if err := clientConn.WriteMessage(websocket.BinaryMessage, version); err != nil {
			t.Fatalf("failed to echo browser version: %v", err)
		}

		_, securityTypes, err := clientConn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read security types: %v", err)
		}
		if len(securityTypes) != 2 {
			t.Fatalf("expected security types payload")
		}
		if err := clientConn.WriteMessage(websocket.BinaryMessage, []byte{1}); err != nil {
			t.Fatalf("failed to write security selection: %v", err)
		}

		select {
		case sendErr := <-errCh:
			if sendErr == nil || !strings.Contains(sendErr.Error(), "send auth result") {
				t.Fatalf("expected send auth result failure, got %v", sendErr)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for send auth result failure")
		}
	})
}

func TestTryProxmoxTerminalStreamLoadRuntimeFailure(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
	rec := httptest.NewRecorder()

	err := sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind: "qemu",
		Node: "pve01",
		VMID: "101",
	})
	if err == nil || !strings.Contains(err.Error(), "load runtime") {
		t.Fatalf("expected load runtime error, got %v", err)
	}
}

func TestTryProxmoxTerminalStreamAuthRejected(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/termproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:terminal-ticket","user":"root@pam"}}`))
		case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("failed to upgrade proxmox websocket: %v", err)
			}
			defer conn.Close()

			_, authPayload, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("failed to read proxmox auth payload: %v", err)
			}
			if got := string(authPayload); got != "root@pam:PVEVNC:terminal-ticket\n" {
				t.Fatalf("unexpected proxmox auth payload: %q", got)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("ERR")); err != nil {
				t.Fatalf("failed to write auth reject: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
	rec := httptest.NewRecorder()

	err := sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})
	if err == nil || !strings.Contains(err.Error(), "auth rejected") {
		t.Fatalf("expected auth rejected error, got %v", err)
	}
}

func TestTryProxmoxTerminalStreamTicketAndDialFailures(t *testing.T) {
	sut := newTestAPIServer(t)

	ticketFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/nodes/pve01/qemu/101/termproxy" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"termproxy":"failed"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ticketFailure.Close()
	configureProxmoxStreamCollector(t, sut, ticketFailure.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
	rec := httptest.NewRecorder()
	err := sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})
	if err == nil || !strings.Contains(err.Error(), "open terminal ticket") {
		t.Fatalf("expected terminal ticket failure, got %v", err)
	}

	dialFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/nodes/pve01/qemu/101/termproxy" {
			_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:terminal-ticket","user":"root@pam"}}`))
			return
		}
		if r.URL.Path == "/api2/json/nodes/pve01/qemu/101/vncwebsocket" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("ws forbidden"))
			return
		}
		http.NotFound(w, r)
	}))
	defer dialFailure.Close()
	configureProxmoxStreamCollector(t, sut, dialFailure.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req = httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
	rec = httptest.NewRecorder()
	err = sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})
	if err == nil || !strings.Contains(err.Error(), "dial websocket") {
		t.Fatalf("expected websocket dial failure, got %v", err)
	}
}

func TestTryProxmoxTerminalStreamAuthTransportFailures(t *testing.T) {
	t.Run("auth read failure", func(t *testing.T) {
		upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/nodes/pve01/qemu/101/termproxy":
				_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:terminal-ticket","user":"root@pam"}}`))
			case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Fatalf("failed to upgrade proxmox websocket: %v", err)
				}
				defer conn.Close()
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read proxmox auth payload: %v", err)
				}
				_ = conn.Close()
			default:
				http.NotFound(w, r)
			}
		}))
		defer mock.Close()

		sut := newTestAPIServer(t)
		configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

		req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
		rec := httptest.NewRecorder()
		err := sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
			Kind:        "qemu",
			Node:        "pve01",
			VMID:        "101",
			CollectorID: "collector-proxmox-stream",
		})
		if err == nil || !strings.Contains(err.Error(), "auth read") {
			t.Fatalf("expected auth-read failure, got %v", err)
		}
	})
}

func TestTryProxmoxTerminalStreamAuthWriteAndDeadlineFailures(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

	t.Run("auth write failure", func(t *testing.T) {
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/nodes/pve01/qemu/101/termproxy":
				_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:terminal-ticket","user":"root@pam"}}`))
			case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Fatalf("failed to upgrade proxmox websocket: %v", err)
				}
				defer conn.Close()
				_, _, _ = conn.ReadMessage()
			default:
				http.NotFound(w, r)
			}
		}))
		defer mock.Close()

		sut := newTestAPIServer(t)
		configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		withProxmoxStreamHooks(t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if messageType == websocket.TextMessage && strings.Contains(string(payload), "PVEVNC:terminal-ticket") {
					return errors.New("forced auth write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			nil,
			nil,
			nil,
		)

		req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
		rec := httptest.NewRecorder()
		err := sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
			Kind:        "qemu",
			Node:        "pve01",
			VMID:        "101",
			CollectorID: "collector-proxmox-stream",
		})
		if err == nil || !strings.Contains(err.Error(), "send auth") {
			t.Fatalf("expected auth-write failure, got %v", err)
		}
	})

	t.Run("set read deadline failure", func(t *testing.T) {
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/nodes/pve01/qemu/101/termproxy":
				_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:terminal-ticket","user":"root@pam"}}`))
			case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Fatalf("failed to upgrade proxmox websocket: %v", err)
				}
				defer conn.Close()
				_, _, _ = conn.ReadMessage()
			default:
				http.NotFound(w, r)
			}
		}))
		defer mock.Close()

		sut := newTestAPIServer(t)
		configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

		withProxmoxStreamHooks(t,
			nil,
			nil,
			func(*websocket.Conn, time.Time) error {
				return errors.New("forced deadline failure")
			},
			nil,
		)

		req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
		rec := httptest.NewRecorder()
		err := sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
			Kind:        "qemu",
			Node:        "pve01",
			VMID:        "101",
			CollectorID: "collector-proxmox-stream",
		})
		if err == nil || !strings.Contains(err.Error(), "set read deadline") {
			t.Fatalf("expected read-deadline failure, got %v", err)
		}
	})
}

func TestTryProxmoxTerminalStreamWebSocketBridge(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	proxmoxInput := make(chan string, 1)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/termproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:terminal-ticket","user":"root@pam"}}`))
		case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
			if !strings.Contains(r.Header.Get("Authorization"), "PVEAPIToken=labtether@pve!stream=stream-secret") {
				t.Fatalf("missing token auth header: %q", r.Header.Get("Authorization"))
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("failed to upgrade proxmox websocket: %v", err)
			}
			defer conn.Close()

			_, authPayload, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("failed to read proxmox auth payload: %v", err)
			}
			if got := string(authPayload); got != "root@pam:PVEVNC:terminal-ticket\n" {
				t.Fatalf("unexpected proxmox auth payload: %q", got)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("OKready")); err != nil {
				t.Fatalf("failed to write auth OK payload: %v", err)
			}

			_, payload, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("failed to read bridged terminal payload: %v", err)
			}
			proxmoxInput <- string(payload)

			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("from-proxmox")); err != nil {
				t.Fatalf("failed to write proxmox output payload: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	handler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := sut.tryProxmoxTerminalStream(w, r, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
			Kind:        "qemu",
			Node:        "pve01",
			VMID:        "101",
			CollectorID: "collector-proxmox-stream",
		})
		if err != nil {
			t.Errorf("tryProxmoxTerminalStream returned error: %v", err)
		}
	}))
	defer handler.Close()

	wsURL := "ws" + strings.TrimPrefix(handler.URL, "http")
	browserConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial browser websocket: %v", err)
	}
	defer browserConn.Close()

	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err := browserConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read initial proxmox payload: %v", err)
	}
	if string(payload) != "ready" {
		t.Fatalf("unexpected initial proxmox payload: %q", string(payload))
	}

	if err := browserConn.WriteMessage(websocket.TextMessage, []byte("ls\n")); err != nil {
		t.Fatalf("failed to write browser terminal input: %v", err)
	}

	select {
	case got := <-proxmoxInput:
		if got != "0:3:ls\n" {
			t.Fatalf("unexpected proxmox terminal frame: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for proxmox terminal frame")
	}

	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err = browserConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read bridged proxmox output: %v", err)
	}
	if string(payload) != "from-proxmox" {
		t.Fatalf("unexpected bridged proxmox output: %q", string(payload))
	}
}

func TestHandleProxmoxDesktopStreamGuardsAndRuntimeFailure(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/desktop", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxDesktopStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind: "node",
		Node: "pve01",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported desktop kind, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	sut.handleProxmoxDesktopStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind: "qemu",
		Node: "pve01",
		VMID: "101",
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when runtime is unavailable, got %d", rec.Code)
	}
}

func TestHandleProxmoxDesktopStreamVNCAuthFailure(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/vncproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5901","ticket":"PVEVNC:desktop-ticket","user":"root@pam","password":"desktop-secret"}}`))
		case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("failed to upgrade proxmox websocket: %v", err)
			}
			defer conn.Close()

			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
				t.Fatalf("failed to send RFB version: %v", err)
			}
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("failed to read echoed RFB version: %v", err)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0}); err != nil {
				t.Fatalf("failed to send invalid security types payload: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/desktop", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxDesktopStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when VNC auth fails, got %d", rec.Code)
	}
}

func TestHandleProxmoxDesktopStreamProxyAndDialFailures(t *testing.T) {
	sut := newTestAPIServer(t)

	proxyFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/nodes/pve01/qemu/101/vncproxy" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"vncproxy":"failed"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer proxyFailure.Close()
	configureProxmoxStreamCollector(t, sut, proxyFailure.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/desktop", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxDesktopStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when vncproxy call fails, got %d", rec.Code)
	}

	dialFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/nodes/pve01/qemu/101/vncproxy" {
			_, _ = w.Write([]byte(`{"data":{"port":"5901","ticket":"PVEVNC:desktop-ticket","user":"root@pam","password":"desktop-secret"}}`))
			return
		}
		if r.URL.Path == "/api2/json/nodes/pve01/qemu/101/vncwebsocket" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("ws forbidden"))
			return
		}
		http.NotFound(w, r)
	}))
	defer dialFailure.Close()
	configureProxmoxStreamCollector(t, sut, dialFailure.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req = httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/desktop", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxDesktopStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when websocket dial fails, got %d", rec.Code)
	}
}

func TestHandleProxmoxDesktopStreamLXCProxyBranch(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/nodes/pve01/lxc/200/vncproxy" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"vncproxy":"failed"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/desktop", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxDesktopStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "lxc",
		Node:        "pve01",
		VMID:        "200",
		CollectorID: "collector-proxmox-stream",
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when LXC vncproxy call fails, got %d", rec.Code)
	}
}

func TestHandleProxmoxDesktopStreamBrowserHandshakeFailure(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/vncproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5901","ticket":"PVEVNC:desktop-ticket","user":"root@pam","password":"desktop-secret"}}`))
		case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("failed to upgrade proxmox websocket: %v", err)
			}
			defer conn.Close()

			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
				t.Fatalf("failed to write RFB version: %v", err)
			}
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("failed to read echoed version: %v", err)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 1}); err != nil {
				t.Fatalf("failed to write security types: %v", err)
			}
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("failed to read security selection: %v", err)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0, 0}); err != nil {
				t.Fatalf("failed to write auth result: %v", err)
			}

			// Browser-side handshake is expected to fail before bridge starts.
			_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			_, _, _ = conn.ReadMessage()
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	handler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handleProxmoxDesktopStream(w, r, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
			Kind:        "qemu",
			Node:        "pve01",
			VMID:        "101",
			CollectorID: "collector-proxmox-stream",
		})
	}))
	defer handler.Close()

	wsURL := "ws" + strings.TrimPrefix(handler.URL, "http")
	browserConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial desktop websocket: %v", err)
	}

	// Read the initial version then close without replying to force
	// proxmoxpkg.SendBrowserVNCNoAuth to fail in the handler.
	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := browserConn.ReadMessage(); err != nil {
		t.Fatalf("failed to read initial browser handshake payload: %v", err)
	}
	_ = browserConn.Close()
}

func TestHandleProxmoxDesktopStreamWebSocketBridge(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	upstreamInput := make(chan string, 1)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/vncproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5901","ticket":"PVEVNC:desktop-ticket","user":"root@pam","password":"desktop-secret"}}`))
		case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
			if !strings.Contains(r.Header.Get("Authorization"), "PVEAPIToken=labtether@pve!stream=stream-secret") {
				t.Fatalf("missing token auth header: %q", r.Header.Get("Authorization"))
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("failed to upgrade proxmox websocket: %v", err)
			}
			defer conn.Close()

			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
				t.Fatalf("failed to write RFB version: %v", err)
			}
			_, echoedVersion, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("failed to read echoed version: %v", err)
			}
			if string(echoedVersion) != "RFB 003.008\n" {
				t.Fatalf("unexpected echoed version: %q", string(echoedVersion))
			}

			if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 1}); err != nil {
				t.Fatalf("failed to write security types: %v", err)
			}
			_, selectedType, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("failed to read selected security type: %v", err)
			}
			if len(selectedType) != 1 || selectedType[0] != 1 {
				t.Fatalf("unexpected selected security type payload: %v", selectedType)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0, 0}); err != nil {
				t.Fatalf("failed to write security result: %v", err)
			}

			_, payload, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("failed to read bridged browser payload: %v", err)
			}
			upstreamInput <- string(payload)

			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("from-upstream")); err != nil {
				t.Fatalf("failed to write upstream payload: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	handler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handleProxmoxDesktopStream(w, r, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
			Kind:        "qemu",
			Node:        "pve01",
			VMID:        "101",
			CollectorID: "collector-proxmox-stream",
		})
	}))
	defer handler.Close()

	wsURL := "ws" + strings.TrimPrefix(handler.URL, "http")
	browserConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial desktop websocket: %v", err)
	}
	defer browserConn.Close()

	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, version, err := browserConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read browser RFB version: %v", err)
	}
	if string(version) != "RFB 003.008\n" {
		t.Fatalf("unexpected browser RFB version: %q", string(version))
	}
	if err := browserConn.WriteMessage(websocket.BinaryMessage, version); err != nil {
		t.Fatalf("failed to send browser RFB version: %v", err)
	}

	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, securityTypes, err := browserConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read browser security types: %v", err)
	}
	if len(securityTypes) != 2 || securityTypes[0] != 1 || securityTypes[1] != 1 {
		t.Fatalf("unexpected browser security types: %v", securityTypes)
	}
	if err := browserConn.WriteMessage(websocket.BinaryMessage, []byte{1}); err != nil {
		t.Fatalf("failed to send browser security selection: %v", err)
	}

	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, authResult, err := browserConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read browser auth result: %v", err)
	}
	if len(authResult) != 4 || authResult[0] != 0 || authResult[1] != 0 || authResult[2] != 0 || authResult[3] != 0 {
		t.Fatalf("unexpected browser auth result: %v", authResult)
	}

	if err := browserConn.WriteMessage(websocket.BinaryMessage, []byte("browser-frame")); err != nil {
		t.Fatalf("failed to send browser frame: %v", err)
	}

	select {
	case got := <-upstreamInput:
		if got != "browser-frame" {
			t.Fatalf("unexpected upstream payload: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for bridged browser payload")
	}

	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, upstreamPayload, err := browserConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read bridged upstream payload: %v", err)
	}
	if string(upstreamPayload) != "from-upstream" {
		t.Fatalf("unexpected bridged upstream payload: %q", string(upstreamPayload))
	}
}

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

func TestTryProxmoxTerminalStreamBrowserUpgradeFailure(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/termproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:terminal-ticket","user":"root@pam"}}`))
		case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("failed to upgrade proxmox websocket: %v", err)
			}
			defer conn.Close()

			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("failed to read auth payload: %v", err)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("OK")); err != nil {
				t.Fatalf("failed to write auth OK payload: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
	rec := httptest.NewRecorder()
	err := sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})
	if err != nil {
		t.Fatalf("expected nil when browser websocket upgrade fails post-auth, got %v", err)
	}
}

func TestHandleProxmoxDesktopStreamBrowserUpgradeFailure(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/vncproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5901","ticket":"PVEVNC:desktop-ticket","user":"root@pam","password":"desktop-secret"}}`))
		case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("failed to upgrade proxmox websocket: %v", err)
			}
			defer conn.Close()

			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
				t.Fatalf("failed to write RFB version: %v", err)
			}
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("failed to read echoed version: %v", err)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 1}); err != nil {
				t.Fatalf("failed to write security types: %v", err)
			}
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("failed to read security selection: %v", err)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0, 0}); err != nil {
				t.Fatalf("failed to write security result: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/desktop", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxDesktopStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})

	// Upgrade happens after successful upstream auth; a plain HTTP request should
	// fail websocket upgrade and write a 400 response.
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when browser upgrade fails post-auth, got %d", rec.Code)
	}
}

func configureProxmoxStreamCollector(t *testing.T, sut *apiServer, baseURL, collectorID, credentialID string) {
	t.Helper()

	createProxmoxCredentialProfile(
		t,
		sut,
		credentialID,
		"labtether@pve!stream",
		"stream-secret",
		baseURL,
	)

	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            collectorID,
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      baseURL,
					"token_id":      "labtether@pve!stream",
					"credential_id": credentialID,
					"skip_verify":   true,
				},
			},
		},
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

func TestBridgeWebSocketPairAndBridgeProxmoxTerminal(t *testing.T) {
	browserServerConn, browserClientConn, browserCleanup := newWebSocketPair(t)
	defer browserCleanup()
	upstreamServerConn, upstreamClientConn, upstreamCleanup := newWebSocketPair(t)
	defer upstreamCleanup()

	go proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)

	if err := browserClientConn.WriteMessage(websocket.TextMessage, []byte("hello-upstream")); err != nil {
		t.Fatalf("failed to write browser->upstream message: %v", err)
	}
	_ = upstreamClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err := upstreamClientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read upstream message: %v", err)
	}
	if string(payload) != "hello-upstream" {
		t.Fatalf("unexpected upstream payload: %q", string(payload))
	}

	if err := upstreamClientConn.WriteMessage(websocket.BinaryMessage, []byte("hello-browser")); err != nil {
		t.Fatalf("failed to write upstream->browser message: %v", err)
	}
	_ = browserClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err = browserClientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read browser message: %v", err)
	}
	if string(payload) != "hello-browser" {
		t.Fatalf("unexpected browser payload: %q", string(payload))
	}

	bridgeBrowserServerConn, bridgeBrowserClientConn, bridgeBrowserCleanup := newWebSocketPair(t)
	defer bridgeBrowserCleanup()
	bridgeUpstreamServerConn, bridgeUpstreamClientConn, bridgeUpstreamCleanup := newWebSocketPair(t)
	defer bridgeUpstreamCleanup()

	go proxmoxpkg.BridgeProxmoxTerminal(bridgeBrowserServerConn, bridgeUpstreamServerConn)

	if err := bridgeUpstreamClientConn.WriteMessage(websocket.BinaryMessage, []byte("proxmox-output")); err != nil {
		t.Fatalf("failed to write proxmox output: %v", err)
	}
	_ = bridgeBrowserClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err = bridgeBrowserClientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read browser terminal output: %v", err)
	}
	if string(payload) != "proxmox-output" {
		t.Fatalf("unexpected proxmox output payload: %q", string(payload))
	}

	if err := bridgeBrowserClientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":100,"rows":40}`)); err != nil {
		t.Fatalf("failed to write resize control message: %v", err)
	}
	_ = bridgeUpstreamClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err = bridgeUpstreamClientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read translated resize message: %v", err)
	}
	if string(payload) != "1:100:40:" {
		t.Fatalf("unexpected translated resize payload: %q", string(payload))
	}

	if err := bridgeBrowserClientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"input","data":"ls\n"}`)); err != nil {
		t.Fatalf("failed to write input control message: %v", err)
	}
	_ = bridgeUpstreamClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err = bridgeUpstreamClientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read translated input message: %v", err)
	}
	if string(payload) != "0:3:ls\n" {
		t.Fatalf("unexpected translated input payload: %q", string(payload))
	}
}

func TestBridgeProxmoxTerminalKeepalivePing(t *testing.T) {
	bridgeBrowserServerConn, bridgeBrowserClientConn, bridgeBrowserCleanup := newWebSocketPair(t)
	defer bridgeBrowserCleanup()
	bridgeUpstreamServerConn, bridgeUpstreamClientConn, bridgeUpstreamCleanup := newWebSocketPair(t)
	defer bridgeUpstreamCleanup()

	setProxmoxTerminalKeepaliveIntervalForTest(t, 20*time.Millisecond)

	go proxmoxpkg.BridgeProxmoxTerminal(bridgeBrowserServerConn, bridgeUpstreamServerConn)

	_ = bridgeUpstreamClientConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, payload, err := bridgeUpstreamClientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read keepalive ping: %v", err)
	}
	if string(payload) != "2" {
		t.Fatalf("expected keepalive ping payload '2', got %q", string(payload))
	}

	_ = bridgeBrowserClientConn.Close()
}

func TestBridgeProxmoxTerminalSkipsNilTranslatedPayloads(t *testing.T) {
	bridgeBrowserServerConn, bridgeBrowserClientConn, bridgeBrowserCleanup := newWebSocketPair(t)
	defer bridgeBrowserCleanup()
	bridgeUpstreamServerConn, bridgeUpstreamClientConn, bridgeUpstreamCleanup := newWebSocketPair(t)
	defer bridgeUpstreamCleanup()

	go proxmoxpkg.BridgeProxmoxTerminal(bridgeBrowserServerConn, bridgeUpstreamServerConn)

	if err := bridgeBrowserClientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"unknown"}`)); err != nil {
		t.Fatalf("failed to write unknown control message: %v", err)
	}

	// Unknown control messages translate to nil and should not be forwarded upstream.
	_ = bridgeUpstreamClientConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := bridgeUpstreamClientConn.ReadMessage(); err == nil {
		t.Fatalf("expected no upstream payload for unknown control message")
	}
}

func TestBridgeProxmoxTerminalAdditionalLifecycleBranches(t *testing.T) {
	t.Run("ignore non terminal message types and handle upstream write failure", func(t *testing.T) {
		bridgeBrowserServerConn, bridgeBrowserClientConn, bridgeBrowserCleanup := newWebSocketPair(t)
		defer bridgeBrowserCleanup()
		bridgeUpstreamServerConn, bridgeUpstreamClientConn, bridgeUpstreamCleanup := newWebSocketPair(t)
		defer bridgeUpstreamCleanup()

		go proxmoxpkg.BridgeProxmoxTerminal(bridgeBrowserServerConn, bridgeUpstreamServerConn)

		if err := bridgeBrowserClientConn.WriteMessage(websocket.PingMessage, []byte("keepalive")); err != nil {
			t.Fatalf("failed to write browser ping message: %v", err)
		}

		_ = bridgeUpstreamClientConn.Close()
		if err := bridgeBrowserClientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"input","data":"ls\n"}`)); err != nil {
			t.Fatalf("failed to write browser input message: %v", err)
		}
	})

	t.Run("upstream close triggers done branch", func(t *testing.T) {
		bridgeBrowserServerConn, bridgeBrowserClientConn, bridgeBrowserCleanup := newWebSocketPair(t)
		defer bridgeBrowserCleanup()
		bridgeUpstreamServerConn, bridgeUpstreamClientConn, bridgeUpstreamCleanup := newWebSocketPair(t)
		defer bridgeUpstreamCleanup()

		go proxmoxpkg.BridgeProxmoxTerminal(bridgeBrowserServerConn, bridgeUpstreamServerConn)

		if err := bridgeBrowserClientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"input","data":"pwd\n"}`)); err != nil {
			t.Fatalf("failed to write browser input message: %v", err)
		}
		_ = bridgeUpstreamClientConn.Close()

		// Allow the bridge loop to observe `done` on its next iteration.
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("browser write failure while forwarding upstream output", func(t *testing.T) {
		bridgeBrowserServerConn, bridgeBrowserClientConn, bridgeBrowserCleanup := newWebSocketPair(t)
		defer bridgeBrowserCleanup()
		bridgeUpstreamServerConn, bridgeUpstreamClientConn, bridgeUpstreamCleanup := newWebSocketPair(t)
		defer bridgeUpstreamCleanup()

		go proxmoxpkg.BridgeProxmoxTerminal(bridgeBrowserServerConn, bridgeUpstreamServerConn)
		_ = bridgeBrowserClientConn.Close()

		if err := bridgeUpstreamClientConn.WriteMessage(websocket.BinaryMessage, []byte("upstream-payload")); err != nil {
			t.Fatalf("failed to write upstream payload: %v", err)
		}
	})
}

func TestBridgeWebSocketPairAdditionalLifecycleBranches(t *testing.T) {
	t.Run("ignore non binary/text and handle upstream write failure", func(t *testing.T) {
		browserServerConn, browserClientConn, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, upstreamClientConn, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		go proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)

		if err := browserClientConn.WriteMessage(websocket.PingMessage, []byte("ignored")); err != nil {
			t.Fatalf("failed to write browser ping message: %v", err)
		}

		_ = upstreamClientConn.Close()
		if err := browserClientConn.WriteMessage(websocket.TextMessage, []byte("forward-me")); err != nil {
			t.Fatalf("failed to write browser payload after upstream close: %v", err)
		}
	})

	t.Run("upstream close triggers done branch", func(t *testing.T) {
		browserServerConn, browserClientConn, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, upstreamClientConn, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		go proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)

		if err := browserClientConn.WriteMessage(websocket.TextMessage, []byte("first-frame")); err != nil {
			t.Fatalf("failed to write first browser frame: %v", err)
		}
		_ = upstreamClientConn.Close()
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("browser write failure while forwarding upstream payload", func(t *testing.T) {
		browserServerConn, browserClientConn, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, upstreamClientConn, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		go proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)
		_ = browserClientConn.Close()

		if err := upstreamClientConn.WriteMessage(websocket.BinaryMessage, []byte("upstream-data")); err != nil {
			t.Fatalf("failed to write upstream data: %v", err)
		}
	})
}

func TestBridgeProxmoxTerminalDeterministicRemainingBranches(t *testing.T) {
	t.Run("upstream to browser write failure", func(t *testing.T) {
		browserServerConn, _, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, _, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		originalRead := proxmoxpkg.ProxmoxWSReadMessage
		upstreamReads := 0
		withProxmoxStreamHooks(
			t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == browserServerConn {
					return errors.New("forced browser write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			func(conn *websocket.Conn) (int, []byte, error) {
				if conn == upstreamServerConn {
					if upstreamReads == 0 {
						upstreamReads++
						return websocket.BinaryMessage, []byte("from-upstream"), nil
					}
					return 0, nil, io.EOF
				}
				if conn == browserServerConn {
					time.Sleep(40 * time.Millisecond)
					return 0, nil, io.EOF
				}
				return originalRead(conn)
			},
			nil,
			nil,
		)

		proxmoxpkg.BridgeProxmoxTerminal(browserServerConn, upstreamServerConn)
	})

	t.Run("keepalive write failure", func(t *testing.T) {
		browserServerConn, _, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, _, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		setProxmoxTerminalKeepaliveIntervalForTest(t, 10*time.Millisecond)

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		originalRead := proxmoxpkg.ProxmoxWSReadMessage
		withProxmoxStreamHooks(
			t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == upstreamServerConn && messageType == websocket.BinaryMessage && string(payload) == "2" {
					return errors.New("forced keepalive write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			func(conn *websocket.Conn) (int, []byte, error) {
				if conn == upstreamServerConn {
					time.Sleep(60 * time.Millisecond)
					return 0, nil, io.EOF
				}
				if conn == browserServerConn {
					time.Sleep(80 * time.Millisecond)
					return 0, nil, io.EOF
				}
				return originalRead(conn)
			},
			nil,
			nil,
		)

		proxmoxpkg.BridgeProxmoxTerminal(browserServerConn, upstreamServerConn)
	})

	t.Run("ignore non data frame", func(t *testing.T) {
		browserServerConn, _, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, _, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		originalRead := proxmoxpkg.ProxmoxWSReadMessage
		browserReads := 0
		withProxmoxStreamHooks(
			t,
			nil,
			func(conn *websocket.Conn) (int, []byte, error) {
				if conn == upstreamServerConn {
					time.Sleep(100 * time.Millisecond)
					return 0, nil, io.EOF
				}
				if conn == browserServerConn {
					browserReads++
					if browserReads == 1 {
						return websocket.PingMessage, []byte("ignored"), nil
					}
					return 0, nil, io.EOF
				}
				return originalRead(conn)
			},
			nil,
			nil,
		)

		proxmoxpkg.BridgeProxmoxTerminal(browserServerConn, upstreamServerConn)
	})

	t.Run("browser input write to proxmox failure", func(t *testing.T) {
		browserServerConn, _, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, _, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		originalRead := proxmoxpkg.ProxmoxWSReadMessage
		browserReads := 0
		withProxmoxStreamHooks(
			t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == upstreamServerConn && messageType == websocket.BinaryMessage {
					return errors.New("forced proxmox write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			func(conn *websocket.Conn) (int, []byte, error) {
				if conn == upstreamServerConn {
					time.Sleep(100 * time.Millisecond)
					return 0, nil, io.EOF
				}
				if conn == browserServerConn {
					browserReads++
					if browserReads == 1 {
						return websocket.TextMessage, []byte("ls\n"), nil
					}
					return 0, nil, io.EOF
				}
				return originalRead(conn)
			},
			nil,
			nil,
		)

		proxmoxpkg.BridgeProxmoxTerminal(browserServerConn, upstreamServerConn)
	})
}

func TestBridgeWebSocketPairDeterministicRemainingBranches(t *testing.T) {
	t.Run("upstream to browser write failure", func(t *testing.T) {
		browserServerConn, _, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, _, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		originalRead := proxmoxpkg.ProxmoxWSReadMessage
		upstreamReads := 0
		withProxmoxStreamHooks(
			t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == browserServerConn {
					return errors.New("forced browser write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			func(conn *websocket.Conn) (int, []byte, error) {
				if conn == upstreamServerConn {
					if upstreamReads == 0 {
						upstreamReads++
						return websocket.BinaryMessage, []byte("from-upstream"), nil
					}
					return 0, nil, io.EOF
				}
				if conn == browserServerConn {
					time.Sleep(40 * time.Millisecond)
					return 0, nil, io.EOF
				}
				return originalRead(conn)
			},
			nil,
			nil,
		)

		proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)
	})

	t.Run("done channel exits main loop", func(t *testing.T) {
		browserServerConn, _, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, _, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		releaseUpstream := make(chan struct{})
		browserReads := 0
		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		originalRead := proxmoxpkg.ProxmoxWSReadMessage
		withProxmoxStreamHooks(
			t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == upstreamServerConn && messageType == websocket.TextMessage && string(payload) == "first-frame" {
					close(releaseUpstream)
					time.Sleep(20 * time.Millisecond)
					return nil
				}
				return originalWrite(conn, messageType, payload)
			},
			func(conn *websocket.Conn) (int, []byte, error) {
				if conn == upstreamServerConn {
					<-releaseUpstream
					return 0, nil, io.EOF
				}
				if conn == browserServerConn {
					browserReads++
					if browserReads == 1 {
						return websocket.TextMessage, []byte("first-frame"), nil
					}
					t.Fatalf("browser ReadMessage should not execute after done channel closes")
				}
				return originalRead(conn)
			},
			nil,
			nil,
		)

		proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)
	})

	t.Run("ignore non data frame", func(t *testing.T) {
		browserServerConn, _, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, _, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		originalRead := proxmoxpkg.ProxmoxWSReadMessage
		browserReads := 0
		withProxmoxStreamHooks(
			t,
			nil,
			func(conn *websocket.Conn) (int, []byte, error) {
				if conn == upstreamServerConn {
					time.Sleep(100 * time.Millisecond)
					return 0, nil, io.EOF
				}
				if conn == browserServerConn {
					browserReads++
					if browserReads == 1 {
						return websocket.PingMessage, []byte("ignored"), nil
					}
					return 0, nil, io.EOF
				}
				return originalRead(conn)
			},
			nil,
			nil,
		)

		proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)
	})

	t.Run("upstream write failure", func(t *testing.T) {
		browserServerConn, _, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, _, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		originalRead := proxmoxpkg.ProxmoxWSReadMessage
		browserReads := 0
		withProxmoxStreamHooks(
			t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == upstreamServerConn {
					return errors.New("forced upstream write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			func(conn *websocket.Conn) (int, []byte, error) {
				if conn == upstreamServerConn {
					time.Sleep(100 * time.Millisecond)
					return 0, nil, io.EOF
				}
				if conn == browserServerConn {
					browserReads++
					if browserReads == 1 {
						return websocket.TextMessage, []byte("forward-me"), nil
					}
					return 0, nil, io.EOF
				}
				return originalRead(conn)
			},
			nil,
			nil,
		)

		proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)
	})
}

func withProxmoxStreamHooks(
	t *testing.T,
	write func(*websocket.Conn, int, []byte) error,
	read func(*websocket.Conn) (int, []byte, error),
	setReadDeadline func(*websocket.Conn, time.Time) error,
	newCipher func([]byte) (cipher.Block, error),
) {
	t.Helper()

	proxmoxpkg.ProxmoxStreamHooksMu.Lock()
	originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
	originalRead := proxmoxpkg.ProxmoxWSReadMessage
	originalSetReadDeadline := proxmoxpkg.ProxmoxWSSetReadDeadline
	originalNewCipher := proxmoxpkg.ProxmoxDESNewCipher

	if write != nil {
		proxmoxpkg.ProxmoxWSWriteMessage = write
	}
	if read != nil {
		proxmoxpkg.ProxmoxWSReadMessage = read
	}
	if setReadDeadline != nil {
		proxmoxpkg.ProxmoxWSSetReadDeadline = setReadDeadline
	}
	if newCipher != nil {
		proxmoxpkg.ProxmoxDESNewCipher = newCipher
	}
	proxmoxpkg.ProxmoxStreamHooksMu.Unlock()

	t.Cleanup(func() {
		proxmoxpkg.ProxmoxStreamHooksMu.Lock()
		proxmoxpkg.ProxmoxWSWriteMessage = originalWrite
		proxmoxpkg.ProxmoxWSReadMessage = originalRead
		proxmoxpkg.ProxmoxWSSetReadDeadline = originalSetReadDeadline
		proxmoxpkg.ProxmoxDESNewCipher = originalNewCipher
		proxmoxpkg.ProxmoxStreamHooksMu.Unlock()
	})
}

func setProxmoxTerminalKeepaliveIntervalForTest(t *testing.T, interval time.Duration) {
	t.Helper()

	proxmoxpkg.ProxmoxStreamHooksMu.Lock()
	original := proxmoxpkg.ProxmoxTerminalKeepaliveInterval
	proxmoxpkg.ProxmoxTerminalKeepaliveInterval = interval
	proxmoxpkg.ProxmoxStreamHooksMu.Unlock()

	t.Cleanup(func() {
		proxmoxpkg.ProxmoxStreamHooksMu.Lock()
		proxmoxpkg.ProxmoxTerminalKeepaliveInterval = original
		proxmoxpkg.ProxmoxStreamHooksMu.Unlock()
	})
}

func newWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	serverConnCh := make(chan *websocket.Conn, 1)
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("websocket upgrade failed: %v", err)
		}
		serverConnCh <- conn
		<-done
		_ = conn.Close()
	}))

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		close(done)
		server.Close()
		t.Fatalf("websocket dial failed: %v", err)
	}

	serverConn := <-serverConnCh
	cleanup := func() {
		_ = clientConn.Close()
		close(done)
		server.Close()
	}
	return serverConn, clientConn, cleanup
}

func testCAPEM(t *testing.T) string {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("failed to generate test CA key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "LabTether Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("failed to create test CA certificate: %v", err)
	}

	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	}))
}
