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

func TestNormalizeFTPRequiresExplicitCleartextOptIn(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")

	secure, err := NormalizeFileConnectionExtraConfig("ftp", nil)
	if err != nil {
		t.Fatalf("secure FTP default rejected: %v", err)
	}
	if secure["ftp_tls"] != true {
		t.Fatalf("FTP did not default to verified FTPS: %#v", secure)
	}

	for _, config := range []map[string]any{
		{"ftp_tls": false},
		{"ftp_tls": false, "ftp_allow_cleartext": true},
		{"ftp_tls": true, "ftp_allow_cleartext": true},
	} {
		if _, err := NormalizeFileConnectionExtraConfig("ftp", config); err == nil {
			t.Fatalf("unsafe or misleading FTP config was accepted: %#v", config)
		}
	}

	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	if _, err := NormalizeFileConnectionExtraConfig("ftp", map[string]any{"ftp_tls": false}); err == nil {
		t.Fatal("global opt-in alone enabled plain FTP")
	}
	cleartext, err := NormalizeFileConnectionExtraConfig("ftp", map[string]any{
		"ftp_tls":             false,
		"ftp_allow_cleartext": true,
	})
	if err != nil {
		t.Fatalf("double-gated plain FTP rejected: %v", err)
	}
	if cleartext["ftp_tls"] != false || cleartext["ftp_allow_cleartext"] != true {
		t.Fatalf("plain FTP options were not preserved: %#v", cleartext)
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

func TestPreserveExistingSFTPHostKeyRequiresExplicitReplacement(t *testing.T) {
	current := map[string]any{"host_key": "trusted-key"}
	requested := map[string]any{}
	merged := preserveExistingSFTPHostKey("sftp", current, requested)
	if merged["host_key"] != "trusted-key" {
		t.Fatalf("partial update cleared the trusted host key: %#v", merged)
	}
	if _, mutated := requested["host_key"]; mutated {
		t.Fatal("host key preservation mutated the request map")
	}

	replaced := preserveExistingSFTPHostKey("sftp", current, map[string]any{"host_key": "replacement-key"})
	if replaced["host_key"] != "replacement-key" {
		t.Fatalf("explicit host key replacement was ignored: %#v", replaced)
	}
}

func TestSanitizeLegacyFTPDoesNotApproveCleartext(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	config := sanitizeLegacyFileConnectionExtraConfig("ftp", map[string]any{
		"ftp_tls": false,
	})
	if config["ftp_tls"] != true {
		t.Fatalf("legacy plain FTP was not reset to the secure default: %#v", config)
	}
}

func TestNormalizeWebDAVRequiresExplicitCleartextOptIn(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	secure, err := NormalizeFileConnectionExtraConfig("webdav", nil)
	if err != nil || secure["webdav_tls"] != true {
		t.Fatalf("WebDAV did not default to verified HTTPS: %#v, %v", secure, err)
	}
	for _, config := range []map[string]any{
		{"webdav_tls": false},
		{"webdav_tls": false, "webdav_allow_cleartext": true},
		{"webdav_tls": true, "webdav_allow_cleartext": true},
		{"webdav_allow_cleartext": "true"},
	} {
		if _, err := NormalizeFileConnectionExtraConfig("webdav", config); err == nil {
			t.Fatalf("unsafe WebDAV config accepted: %#v", config)
		}
	}
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	if _, err := NormalizeFileConnectionExtraConfig("webdav", map[string]any{"webdav_tls": false}); err == nil {
		t.Fatal("global opt-in alone enabled plain WebDAV")
	}
	plain, err := NormalizeFileConnectionExtraConfig("webdav", map[string]any{
		"webdav_tls": false, "webdav_allow_cleartext": true,
	})
	if err != nil || plain["webdav_tls"] != false || plain["webdav_allow_cleartext"] != true {
		t.Fatalf("double-gated WebDAV config rejected: %#v, %v", plain, err)
	}
}

func TestSanitizeLegacyWebDAVDoesNotApproveCleartext(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	config := sanitizeLegacyFileConnectionExtraConfig("webdav", map[string]any{
		"webdav_tls": false, "webdav_base_path": "/dav", "password": "test-secret",
	})
	if config["webdav_tls"] != true || config["webdav_base_path"] != "/dav" {
		t.Fatalf("legacy config lost path or retained insecure TLS: %#v", config)
	}
	if _, present := config["password"]; present {
		t.Fatal("legacy password escaped sanitation")
	}
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	config = sanitizeLegacyFileConnectionExtraConfig("webdav", map[string]any{
		"webdav_tls": false, "webdav_allow_cleartext": true, "password": "test-secret",
	})
	if config["webdav_tls"] != false || config["webdav_allow_cleartext"] != true {
		t.Fatalf("approved cleartext options were lost: %#v", config)
	}
}
