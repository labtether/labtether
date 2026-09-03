package protocols

import (
	"encoding/json"
	"testing"
)

func TestDecodeRDPConfigUsesSecureCertificateDefault(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage("null"), json.RawMessage("{}")} {
		cfg, err := DecodeRDPConfig(raw)
		if err != nil {
			t.Fatalf("DecodeRDPConfig(%q) returned error: %v", raw, err)
		}
		if cfg.IgnoreCertificate {
			t.Fatalf("DecodeRDPConfig(%q) disabled certificate checks by default", raw)
		}
	}
}

func TestDecodeRDPConfigReadsExplicitOptionsAndRejectsUnknownFields(t *testing.T) {
	cfg, err := DecodeRDPConfig(json.RawMessage(`{"domain":"EXAMPLE","nla_enabled":true,"certificate_fingerprints":"sha256:AA:BB"}`))
	if err != nil {
		t.Fatalf("DecodeRDPConfig returned error: %v", err)
	}
	if cfg.Domain != "EXAMPLE" || !cfg.NLAEnabled || cfg.CertificateFingerprints != "sha256:AA:BB" {
		t.Fatalf("unexpected decoded config: %+v", cfg)
	}

	if _, err := DecodeRDPConfig(json.RawMessage(`{"ignore_cert":true}`)); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}

func TestDecodeRDPConfigRejectsMisleadingSecurityCombinations(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"ignore_certificate":true,"certificate_fingerprints":"sha256:AA"}`),
		json.RawMessage(`{"allow_legacy_security":true,"certificate_fingerprints":"sha256:AA"}`),
		json.RawMessage(`{"allow_legacy_security":true,"nla_enabled":true}`),
		json.RawMessage(`{"allow_legacy_security":true,"ignore_certificate":true}`),
		json.RawMessage("{\"certificate_fingerprints\":\"sha256:AA\\nBB\"}"),
	} {
		if _, err := DecodeRDPConfig(raw); err == nil {
			t.Fatalf("expected invalid RDP config to be rejected: %s", raw)
		}
	}
}
