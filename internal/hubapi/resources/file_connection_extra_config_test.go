package resources

import (
	"strings"
	"testing"
)

func TestNormalizeFileConnectionExtraConfigRejectsSecretBearingAndUnknownFields(t *testing.T) {
	for _, tc := range []struct {
		protocol string
		config   map[string]any
	}{
		{"sftp", map[string]any{"password": "secret"}},
		{"smb", map[string]any{"token": "secret"}},
		{"webdav", map[string]any{"authorization": "Bearer secret"}},
		{"ftp", map[string]any{"ftp_tls": "true"}},
		{"smb", map[string]any{"smb_share": "share/path"}},
		{"webdav", map[string]any{"webdav_base_path": "https://attacker.invalid/path"}},
	} {
		if _, err := NormalizeFileConnectionExtraConfig(tc.protocol, tc.config); err == nil {
			t.Fatalf("expected rejection for %s %#v", tc.protocol, tc.config)
		}
	}
}

func TestNormalizeFileConnectionExtraConfigCanonicalizesSupportedFields(t *testing.T) {
	config, err := NormalizeFileConnectionExtraConfig("smb", map[string]any{
		"smb_share":  " QA$ ",
		"smb_domain": " LAB ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if config["smb_share"] != "QA$" || config["smb_domain"] != "LAB" {
		t.Fatalf("unexpected normalized config: %#v", config)
	}

	config, err = NormalizeFileConnectionExtraConfig("webdav", map[string]any{
		"webdav_tls":       true,
		"webdav_base_path": "remote.php/dav",
	})
	if err != nil {
		t.Fatal(err)
	}
	if config["webdav_base_path"] != "/remote.php/dav" {
		t.Fatalf("base path was not canonicalized: %#v", config)
	}
}

func TestNormalizeFileConnectionExtraConfigRequiresExplicitInsecureTransportOptIn(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "")
	if _, err := NormalizeFileConnectionExtraConfig("webdav", map[string]any{"webdav_tls_skip_verify": true}); err == nil {
		t.Fatal("TLS verification bypass was accepted without process-wide acknowledgement")
	}
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	if _, err := NormalizeFileConnectionExtraConfig("webdav", map[string]any{"webdav_tls_skip_verify": true}); err != nil {
		t.Fatalf("explicitly acknowledged skip verify rejected: %v", err)
	}
}

func TestSanitizeLegacyFileConnectionExtraConfigDropsSecretFields(t *testing.T) {
	config := sanitizeLegacyFileConnectionExtraConfig("ftp", map[string]any{
		"ftp_tls":  true,
		"password": strings.Repeat("secret", 3),
	})
	if _, exposed := config["password"]; exposed {
		t.Fatal("legacy secret-bearing extra config was returned")
	}
	if config["ftp_tls"] != true {
		t.Fatalf("supported legacy option was lost: %#v", config)
	}
}
