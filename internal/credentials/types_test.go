package credentials

import (
	"encoding/json"
	"testing"
	"time"
)

// --- Kind constants ---

func TestKindConstants(t *testing.T) {
	// Guard against typos or accidental renames; callers serialise these to the DB.
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"KindSSHPassword", KindSSHPassword, "ssh_password"},
		{"KindSSHPrivateKey", KindSSHPrivateKey, "ssh_private_key"},
		{"KindHubSSHIdentity", KindHubSSHIdentity, "hub_ssh_identity"},
		{"KindVNCPassword", KindVNCPassword, "vnc_password"},
		{"KindProxmoxAPIToken", KindProxmoxAPIToken, "proxmox_api_token"},
		{"KindProxmoxPassword", KindProxmoxPassword, "proxmox_password"},
		{"KindPBSAPIToken", KindPBSAPIToken, "pbs_api_token"},
		{"KindPortainerAPIKey", KindPortainerAPIKey, "portainer_api_key"},
		{"KindTrueNASAPIKey", KindTrueNASAPIKey, "truenas_api_key"},
		{"KindHomeAssistantToken", KindHomeAssistantToken, "homeassistant_token"},
		{"KindTelnetPassword", KindTelnetPassword, "telnet_password"},
		{"KindRDPPassword", KindRDPPassword, "rdp_password"},
		{"KindFTPPassword", KindFTPPassword, "ftp_password"},
		{"KindSMBCredentials", KindSMBCredentials, "smb_credentials"},
		{"KindWebDAVCredentials", KindWebDAVCredentials, "webdav_credentials"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestKindConstantsAreUnique(t *testing.T) {
	all := []string{
		KindSSHPassword,
		KindSSHPrivateKey,
		KindHubSSHIdentity,
		KindVNCPassword,
		KindProxmoxAPIToken,
		KindProxmoxPassword,
		KindPBSAPIToken,
		KindPortainerAPIKey,
		KindTrueNASAPIKey,
		KindHomeAssistantToken,
		KindTelnetPassword,
		KindRDPPassword,
		KindFTPPassword,
		KindSMBCredentials,
		KindWebDAVCredentials,
	}
	seen := make(map[string]bool, len(all))
	for _, k := range all {
		if seen[k] {
			t.Fatalf("duplicate kind constant value %q", k)
		}
		seen[k] = true
	}
}

// --- Profile JSON serialisation ---

func TestProfileJSONOmitsSecretFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	p := Profile{
		ID:                   "cred_abc",
		Name:                 "My Key",
		Kind:                 KindSSHPrivateKey,
		Username:             "root",
		Description:          "test profile",
		Status:               "active",
		CreatedAt:            now,
		UpdatedAt:            now,
		SecretCiphertext:     "enc:supersecret",
		PassphraseCiphertext: "enc:passphrase",
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := raw["secret_ciphertext"]; ok {
		t.Fatal("SecretCiphertext must not appear in JSON output (tagged json:\"-\")")
	}
	if _, ok := raw["passphrase_ciphertext"]; ok {
		t.Fatal("PassphraseCiphertext must not appear in JSON output (tagged json:\"-\")")
	}
}

func TestProfileJSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	rotated := now.Add(-24 * time.Hour)
	lastUsed := now.Add(-1 * time.Hour)
	expires := now.Add(30 * 24 * time.Hour)

	original := Profile{
		ID:          "cred_xyz",
		Name:        "Proxmox Token",
		Kind:        KindProxmoxAPIToken,
		Username:    "admin@pam",
		Description: "main node",
		Status:      "active",
		Metadata:    map[string]string{"node": "pve1"},
		CreatedAt:   now,
		UpdatedAt:   now,
		RotatedAt:   &rotated,
		LastUsedAt:  &lastUsed,
		ExpiresAt:   &expires,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded Profile
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID: got %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name: got %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Kind != original.Kind {
		t.Errorf("Kind: got %q, want %q", decoded.Kind, original.Kind)
	}
	if decoded.Username != original.Username {
		t.Errorf("Username: got %q, want %q", decoded.Username, original.Username)
	}
	if decoded.Description != original.Description {
		t.Errorf("Description: got %q, want %q", decoded.Description, original.Description)
	}
	if decoded.Status != original.Status {
		t.Errorf("Status: got %q, want %q", decoded.Status, original.Status)
	}
	if decoded.Metadata["node"] != "pve1" {
		t.Errorf("Metadata[node]: got %q, want pve1", decoded.Metadata["node"])
	}
	if !decoded.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", decoded.CreatedAt, original.CreatedAt)
	}
	if decoded.RotatedAt == nil || !decoded.RotatedAt.Equal(rotated) {
		t.Errorf("RotatedAt: got %v, want %v", decoded.RotatedAt, rotated)
	}
	if decoded.LastUsedAt == nil || !decoded.LastUsedAt.Equal(lastUsed) {
		t.Errorf("LastUsedAt: got %v, want %v", decoded.LastUsedAt, lastUsed)
	}
	if decoded.ExpiresAt == nil || !decoded.ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt: got %v, want %v", decoded.ExpiresAt, expires)
	}
	// Secret fields must remain empty after a JSON round-trip (they are tagged json:"-").
	if decoded.SecretCiphertext != "" {
		t.Errorf("SecretCiphertext should be empty after JSON round-trip, got %q", decoded.SecretCiphertext)
	}
	if decoded.PassphraseCiphertext != "" {
		t.Errorf("PassphraseCiphertext should be empty after JSON round-trip, got %q", decoded.PassphraseCiphertext)
	}
}

func TestProfileJSONOmitsOptionalFieldsWhenEmpty(t *testing.T) {
	p := Profile{
		ID:        "cred_min",
		Name:      "Minimal",
		Kind:      KindSSHPassword,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	for _, field := range []string{"username", "description", "status", "metadata", "rotated_at", "last_used_at", "expires_at"} {
		if _, ok := raw[field]; ok {
			t.Errorf("field %q should be omitted when empty/nil, but it appears in JSON", field)
		}
	}
}

func TestProfileNilPointerFieldsRoundTrip(t *testing.T) {
	p := Profile{
		ID:        "cred_nil",
		Name:      "No Expiry",
		Kind:      KindVNCPassword,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded Profile
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.RotatedAt != nil {
		t.Errorf("RotatedAt should be nil, got %v", decoded.RotatedAt)
	}
	if decoded.LastUsedAt != nil {
		t.Errorf("LastUsedAt should be nil, got %v", decoded.LastUsedAt)
	}
	if decoded.ExpiresAt != nil {
		t.Errorf("ExpiresAt should be nil, got %v", decoded.ExpiresAt)
	}
}

// --- CreateProfileRequest JSON ---

func TestCreateProfileRequestJSONRoundTrip(t *testing.T) {
	expires := time.Now().UTC().Add(90 * 24 * time.Hour).Truncate(time.Second)
	req := CreateProfileRequest{
		Name:        "My SSH Key",
		Kind:        KindSSHPrivateKey,
		Username:    "deploy",
		Description: "CI deploy key",
		Secret:      "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----",
		Passphrase:  "hunter2",
		Metadata:    map[string]string{"env": "prod"},
		ExpiresAt:   &expires,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded CreateProfileRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Name != req.Name {
		t.Errorf("Name: got %q, want %q", decoded.Name, req.Name)
	}
	if decoded.Kind != req.Kind {
		t.Errorf("Kind: got %q, want %q", decoded.Kind, req.Kind)
	}
	if decoded.Secret != req.Secret {
		t.Errorf("Secret: got %q, want %q", decoded.Secret, req.Secret)
	}
	if decoded.Passphrase != req.Passphrase {
		t.Errorf("Passphrase: got %q, want %q", decoded.Passphrase, req.Passphrase)
	}
	if decoded.Metadata["env"] != "prod" {
		t.Errorf("Metadata[env]: got %q, want prod", decoded.Metadata["env"])
	}
	if decoded.ExpiresAt == nil || !decoded.ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt: got %v, want %v", decoded.ExpiresAt, expires)
	}
}

func TestCreateProfileRequestMissingOptionalFields(t *testing.T) {
	req := CreateProfileRequest{
		Name:   "Minimal",
		Kind:   KindRDPPassword,
		Secret: "s3cr3t",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded CreateProfileRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Username != "" {
		t.Errorf("Username should be empty, got %q", decoded.Username)
	}
	if decoded.Passphrase != "" {
		t.Errorf("Passphrase should be empty, got %q", decoded.Passphrase)
	}
	if decoded.ExpiresAt != nil {
		t.Errorf("ExpiresAt should be nil, got %v", decoded.ExpiresAt)
	}
}

// --- RotateProfileRequest JSON ---

func TestRotateProfileRequestJSONRoundTrip(t *testing.T) {
	expires := time.Now().UTC().Add(7 * 24 * time.Hour).Truncate(time.Second)
	req := RotateProfileRequest{
		Secret:      "newpassword",
		Passphrase:  "newphrase",
		Reason:      "quarterly rotation",
		ExpiresAt:   &expires,
		KeepEnabled: true,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded RotateProfileRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Secret != req.Secret {
		t.Errorf("Secret: got %q, want %q", decoded.Secret, req.Secret)
	}
	if decoded.Passphrase != req.Passphrase {
		t.Errorf("Passphrase: got %q, want %q", decoded.Passphrase, req.Passphrase)
	}
	if decoded.Reason != req.Reason {
		t.Errorf("Reason: got %q, want %q", decoded.Reason, req.Reason)
	}
	if decoded.ExpiresAt == nil || !decoded.ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt: got %v, want %v", decoded.ExpiresAt, expires)
	}
	if !decoded.KeepEnabled {
		t.Errorf("KeepEnabled: got false, want true")
	}
}

// --- AssetTerminalConfig JSON ---

func TestAssetTerminalConfigJSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	cfg := AssetTerminalConfig{
		AssetID:             "asset_01",
		Host:                "192.168.1.10",
		Port:                2222,
		Username:            "admin",
		StrictHostKey:       true,
		HostKey:             "ecdsa-sha2-nistp256 AAAA...",
		CredentialProfileID: "cred_01",
		UpdatedAt:           now,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded AssetTerminalConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.AssetID != cfg.AssetID {
		t.Errorf("AssetID: got %q, want %q", decoded.AssetID, cfg.AssetID)
	}
	if decoded.Host != cfg.Host {
		t.Errorf("Host: got %q, want %q", decoded.Host, cfg.Host)
	}
	if decoded.Port != cfg.Port {
		t.Errorf("Port: got %d, want %d", decoded.Port, cfg.Port)
	}
	if decoded.Username != cfg.Username {
		t.Errorf("Username: got %q, want %q", decoded.Username, cfg.Username)
	}
	if decoded.StrictHostKey != cfg.StrictHostKey {
		t.Errorf("StrictHostKey: got %v, want %v", decoded.StrictHostKey, cfg.StrictHostKey)
	}
	if decoded.HostKey != cfg.HostKey {
		t.Errorf("HostKey: got %q, want %q", decoded.HostKey, cfg.HostKey)
	}
	if decoded.CredentialProfileID != cfg.CredentialProfileID {
		t.Errorf("CredentialProfileID: got %q, want %q", decoded.CredentialProfileID, cfg.CredentialProfileID)
	}
	if !decoded.UpdatedAt.Equal(cfg.UpdatedAt) {
		t.Errorf("UpdatedAt: got %v, want %v", decoded.UpdatedAt, cfg.UpdatedAt)
	}
}

func TestAssetTerminalConfigJSONOmitsOptionalFields(t *testing.T) {
	cfg := AssetTerminalConfig{
		AssetID:   "asset_02",
		Host:      "10.0.0.1",
		Port:      22,
		UpdatedAt: time.Now().UTC(),
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	for _, field := range []string{"username", "host_key", "credential_profile_id"} {
		if _, ok := raw[field]; ok {
			t.Errorf("field %q should be omitted when empty, but appears in JSON", field)
		}
	}
}

// --- UpdateAssetTerminalConfigRequest JSON ---

func TestUpdateAssetTerminalConfigRequestJSONRoundTrip(t *testing.T) {
	strictKey := true
	req := UpdateAssetTerminalConfigRequest{
		Host:                "10.10.10.10",
		Port:                22,
		Username:            "root",
		StrictHostKey:       &strictKey,
		HostKey:             "ssh-ed25519 AAAA...",
		CredentialProfileID: "cred_99",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded UpdateAssetTerminalConfigRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Host != req.Host {
		t.Errorf("Host: got %q, want %q", decoded.Host, req.Host)
	}
	if decoded.Port != req.Port {
		t.Errorf("Port: got %d, want %d", decoded.Port, req.Port)
	}
	if decoded.StrictHostKey == nil || *decoded.StrictHostKey != true {
		t.Errorf("StrictHostKey: got %v, want *true", decoded.StrictHostKey)
	}
	if decoded.CredentialProfileID != req.CredentialProfileID {
		t.Errorf("CredentialProfileID: got %q, want %q", decoded.CredentialProfileID, req.CredentialProfileID)
	}
}

func TestUpdateAssetTerminalConfigRequestNilStrictHostKey(t *testing.T) {
	// When StrictHostKey is nil (omitempty), the field should not appear in JSON.
	req := UpdateAssetTerminalConfigRequest{
		Host: "10.0.0.5",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := raw["strict_host_key"]; ok {
		t.Error("strict_host_key should be omitted when nil")
	}
}

// --- AssetDesktopConfig JSON ---

func TestAssetDesktopConfigJSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	cfg := AssetDesktopConfig{
		AssetID:             "asset_desktop_01",
		VNCPort:             5900,
		CredentialProfileID: "cred_vnc",
		UpdatedAt:           now,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded AssetDesktopConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.AssetID != cfg.AssetID {
		t.Errorf("AssetID: got %q, want %q", decoded.AssetID, cfg.AssetID)
	}
	if decoded.VNCPort != cfg.VNCPort {
		t.Errorf("VNCPort: got %d, want %d", decoded.VNCPort, cfg.VNCPort)
	}
	if decoded.CredentialProfileID != cfg.CredentialProfileID {
		t.Errorf("CredentialProfileID: got %q, want %q", decoded.CredentialProfileID, cfg.CredentialProfileID)
	}
	if !decoded.UpdatedAt.Equal(cfg.UpdatedAt) {
		t.Errorf("UpdatedAt: got %v, want %v", decoded.UpdatedAt, cfg.UpdatedAt)
	}
}

func TestAssetDesktopConfigJSONOmitsCredentialProfileIDWhenEmpty(t *testing.T) {
	cfg := AssetDesktopConfig{
		AssetID:   "asset_desktop_02",
		VNCPort:   5901,
		UpdatedAt: time.Now().UTC(),
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := raw["credential_profile_id"]; ok {
		t.Error("credential_profile_id should be omitted when empty")
	}
}

// --- Profile zero-value sanity ---

func TestProfileZeroValue(t *testing.T) {
	var p Profile
	if p.ID != "" {
		t.Errorf("expected empty ID, got %q", p.ID)
	}
	if p.RotatedAt != nil {
		t.Errorf("expected nil RotatedAt, got %v", p.RotatedAt)
	}
	if p.LastUsedAt != nil {
		t.Errorf("expected nil LastUsedAt, got %v", p.LastUsedAt)
	}
	if p.ExpiresAt != nil {
		t.Errorf("expected nil ExpiresAt, got %v", p.ExpiresAt)
	}
	if p.SecretCiphertext != "" {
		t.Errorf("expected empty SecretCiphertext, got %q", p.SecretCiphertext)
	}
	if p.PassphraseCiphertext != "" {
		t.Errorf("expected empty PassphraseCiphertext, got %q", p.PassphraseCiphertext)
	}
}
