package desktop

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/secrets"
	"github.com/labtether/labtether/internal/terminal"
)

func TestRDPGuacdParamsVerifyCertificatesByDefault(t *testing.T) {
	params := rdpGuacdParams(RDPConnectionTarget{})
	if got := params["ignore-cert"]; got != "false" {
		t.Fatalf("ignore-cert = %q, want false", got)
	}
	if got := params["security"]; got != "tls" {
		t.Fatalf("security = %q, want tls", got)
	}
}

func TestRDPGuacdParamsUseExplicitRDPOptions(t *testing.T) {
	params := rdpGuacdParams(RDPConnectionTarget{
		Domain:                  "EXAMPLE",
		NLAEnabled:              true,
		CertificateFingerprints: "sha256:AA:BB",
	})
	if got := params["domain"]; got != "EXAMPLE" {
		t.Fatalf("domain = %q, want EXAMPLE", got)
	}
	if got := params["security"]; got != "nla" {
		t.Fatalf("security = %q, want nla", got)
	}
	if got := params["ignore-cert"]; got != "false" {
		t.Fatalf("ignore-cert = %q, want false", got)
	}
	if got := params["cert-fingerprints"]; got != "sha256:AA:BB" {
		t.Fatalf("cert-fingerprints = %q, want configured pin", got)
	}
}

func TestRDPGuacdParamsRequireExplicitLegacyMode(t *testing.T) {
	params := rdpGuacdParams(RDPConnectionTarget{AllowLegacySecurity: true})
	if got := params["security"]; got != "rdp" {
		t.Fatalf("security = %q, want explicit legacy rdp", got)
	}
}

func TestResolveRDPTargetConsumesEnabledProtocolConfig(t *testing.T) {
	deps := &Deps{
		GetProtocolConfig: func(_ context.Context, assetID, protocol string) (*protocols.ProtocolConfig, error) {
			if assetID != "asset-1" || protocol != protocols.ProtocolRDP {
				t.Fatalf("unexpected lookup: asset=%q protocol=%q", assetID, protocol)
			}
			return &protocols.ProtocolConfig{
				AssetID:             assetID,
				Protocol:            protocol,
				Host:                "rdp.example.test",
				Port:                3390,
				Username:            "configured-user",
				CredentialProfileID: "profile-1",
				Enabled:             true,
				Config:              json.RawMessage(`{"domain":"EXAMPLE","nla_enabled":true,"certificate_fingerprints":"sha256:AA:BB"}`),
			}, nil
		},
	}

	target, err := deps.ResolveRDPTarget(context.Background(), terminal.Session{Target: "asset-1"})
	if err != nil {
		t.Fatalf("ResolveRDPTarget returned error: %v", err)
	}
	if target.Host != "rdp.example.test" || target.Port != 3390 || target.Username != "configured-user" {
		t.Fatalf("unexpected connection target: %+v", target)
	}
	if target.Domain != "EXAMPLE" || !target.NLAEnabled || target.IgnoreCertificate {
		t.Fatalf("protocol-specific options were not applied: %+v", target)
	}
	if target.CertificateFingerprints != "sha256:AA:BB" {
		t.Fatalf("certificate fingerprints were not applied: %+v", target)
	}
}

func TestResolveRDPTargetRejectsSavedInsecureOptionWithoutGlobalGate(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	deps := &Deps{
		GetProtocolConfig: func(context.Context, string, string) (*protocols.ProtocolConfig, error) {
			return &protocols.ProtocolConfig{
				Enabled: true,
				Config:  json.RawMessage(`{"ignore_certificate":true}`),
			}, nil
		},
	}

	_, err := deps.ResolveRDPTarget(context.Background(), terminal.Session{Target: "asset-1"})
	if err == nil || !strings.Contains(err.Error(), "insecure transport") {
		t.Fatalf("expected insecure transport gate error, got %v", err)
	}
}

func TestResolveRDPTargetUsesProtocolCredentialWithoutChangingSecretHandling(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	secretsManager, err := secrets.NewManagerFromEncodedKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("create secrets manager: %v", err)
	}
	const profileID = "profile-rdp"
	ciphertext, err := secretsManager.EncryptString("test-value", profileID)
	if err != nil {
		t.Fatalf("encrypt test credential: %v", err)
	}
	credentialStore := persistence.NewMemoryCredentialStore()
	if _, err := credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               profileID,
		Name:             "RDP test profile",
		Kind:             credentials.KindRDPPassword,
		Username:         "profile-user",
		SecretCiphertext: ciphertext,
	}); err != nil {
		t.Fatalf("create credential profile: %v", err)
	}

	deps := &Deps{
		CredentialStore: credentialStore,
		SecretsManager:  secretsManager,
		GetProtocolConfig: func(context.Context, string, string) (*protocols.ProtocolConfig, error) {
			return &protocols.ProtocolConfig{
				Enabled:             true,
				Username:            "configured-user",
				CredentialProfileID: profileID,
			}, nil
		},
	}
	target, err := deps.ResolveRDPTarget(context.Background(), terminal.Session{Target: "asset-1"})
	if err != nil {
		t.Fatalf("ResolveRDPTarget returned error: %v", err)
	}
	if target.Username != "configured-user" {
		t.Fatalf("username = %q, want protocol-configured username", target.Username)
	}
	if target.Password != "test-value" {
		t.Fatal("credential profile was not decrypted into the RDP target")
	}
}

func TestResolveRDPTargetDirectSessionUsesSecureDefaultAndDoubleGate(t *testing.T) {
	optionsMu := &sync.RWMutex{}
	options := map[string]DesktopSessionOptions{
		"secure": {
			Direct:     true,
			DirectHost: "192.0.2.10",
			DirectPort: 3389,
		},
		"insecure": {
			Direct:               true,
			DirectHost:           "192.0.2.11",
			DirectPort:           3389,
			RDPIgnoreCertificate: true,
		},
		"legacy": {
			Direct:                 true,
			DirectHost:             "192.0.2.12",
			DirectPort:             3389,
			RDPAllowLegacySecurity: true,
		},
		"pinned": {
			Direct:                     true,
			DirectHost:                 "192.0.2.13",
			DirectPort:                 3389,
			RDPCertificateFingerprints: "sha256:AA:BB",
		},
	}
	deps := &Deps{DesktopSessionMu: optionsMu, DesktopSessionOpts: &options}

	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	secure, err := deps.ResolveRDPTarget(context.Background(), terminal.Session{ID: "secure"})
	if err != nil {
		t.Fatalf("secure direct target returned error: %v", err)
	}
	if secure.IgnoreCertificate {
		t.Fatal("secure direct target disabled certificate checks")
	}
	if !secure.NLAEnabled || rdpGuacdParams(secure)["security"] != "nla" {
		t.Fatal("secure direct target did not require NLA")
	}
	if _, err := deps.ResolveRDPTarget(context.Background(), terminal.Session{ID: "insecure"}); err == nil {
		t.Fatal("expected local opt-in to fail while the global gate is disabled")
	}
	if _, err := deps.ResolveRDPTarget(context.Background(), terminal.Session{ID: "legacy"}); err == nil {
		t.Fatal("expected legacy security to fail while the global gate is disabled")
	}
	pinned, err := deps.ResolveRDPTarget(context.Background(), terminal.Session{ID: "pinned"})
	if err != nil {
		t.Fatalf("pinned direct target returned error: %v", err)
	}
	if pinned.CertificateFingerprints != "sha256:AA:BB" {
		t.Fatalf("pinned direct target lost its certificate fingerprint: %+v", pinned)
	}

	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	insecure, err := deps.ResolveRDPTarget(context.Background(), terminal.Session{ID: "insecure"})
	if err != nil {
		t.Fatalf("double-gated direct target returned error: %v", err)
	}
	if !insecure.IgnoreCertificate {
		t.Fatal("explicit double-gated option was not applied")
	}
	legacy, err := deps.ResolveRDPTarget(context.Background(), terminal.Session{ID: "legacy"})
	if err != nil {
		t.Fatalf("double-gated legacy target returned error: %v", err)
	}
	if !legacy.AllowLegacySecurity {
		t.Fatal("explicit double-gated legacy option was not applied")
	}
	if legacy.NLAEnabled || rdpGuacdParams(legacy)["security"] != "rdp" {
		t.Fatal("legacy direct target did not use only the explicit legacy mode")
	}
}
