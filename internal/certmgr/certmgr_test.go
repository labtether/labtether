package certmgr

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateCA(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error: %v", err)
	}
	if ca.Cert == nil {
		t.Fatal("expected non-nil Cert")
	}
	if ca.Key == nil {
		t.Fatal("expected non-nil Key")
	}
	if len(ca.CertDER) == 0 {
		t.Fatal("expected non-empty CertDER")
	}

	cert := ca.Cert

	// Verify IsCA
	if !cert.IsCA {
		t.Error("expected IsCA=true")
	}
	if !cert.BasicConstraintsValid {
		t.Error("expected BasicConstraintsValid=true")
	}

	// Verify subject CN and Org
	if cert.Subject.CommonName != "LabTether CA" {
		t.Errorf("expected CN=%q, got %q", "LabTether CA", cert.Subject.CommonName)
	}
	if len(cert.Subject.Organization) == 0 || cert.Subject.Organization[0] != "LabTether" {
		t.Errorf("expected Org=%q, got %v", "LabTether", cert.Subject.Organization)
	}

	// Verify KeyUsage includes CertSign and CRLSign
	if cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Error("expected KeyUsageCertSign")
	}
	if cert.KeyUsage&x509.KeyUsageCRLSign == 0 {
		t.Error("expected KeyUsageCRLSign")
	}

	// Verify ~10-year validity
	validity := cert.NotAfter.Sub(cert.NotBefore)
	expectedValidity := 10 * 365 * 24 * time.Hour
	tolerance := 48 * time.Hour // allow ±2 days for leap years
	if validity < expectedValidity-tolerance || validity > expectedValidity+tolerance {
		t.Errorf("expected ~10yr validity, got %v", validity)
	}

	// Verify NotBefore is backdated (should be before now)
	if cert.NotBefore.After(time.Now()) {
		t.Error("expected NotBefore to be backdated (before now)")
	}

	// Verify self-signed (issuer == subject)
	if cert.Issuer.CommonName != cert.Subject.CommonName {
		t.Errorf("expected self-signed CA: issuer CN=%q, subject CN=%q",
			cert.Issuer.CommonName, cert.Subject.CommonName)
	}
}

func TestGenerateServerCert(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error: %v", err)
	}

	dnsNames := []string{"node1.lab.local", "node2.lab.local"}
	ipStrings := []string{"10.0.0.1", "192.168.1.100"}

	server, err := GenerateServerCert(ca, dnsNames, ipStrings)
	if err != nil {
		t.Fatalf("GenerateServerCert() error: %v", err)
	}
	if server.Cert == nil {
		t.Fatal("expected non-nil server Cert")
	}
	if server.Key == nil {
		t.Fatal("expected non-nil server Key")
	}

	cert := server.Cert

	// Verify not a CA
	if cert.IsCA {
		t.Error("server cert should not be a CA")
	}

	// Verify ExtKeyUsage includes ServerAuth
	foundServerAuth := false
	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth {
			foundServerAuth = true
			break
		}
	}
	if !foundServerAuth {
		t.Error("expected ExtKeyUsageServerAuth")
	}

	// Verify DNS SANs
	for _, dns := range dnsNames {
		found := false
		for _, san := range cert.DNSNames {
			if san == dns {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected DNS SAN %q in cert, got %v", dns, cert.DNSNames)
		}
	}

	// Verify IP SANs
	for _, ipStr := range ipStrings {
		expected := net.ParseIP(ipStr)
		found := false
		for _, ip := range cert.IPAddresses {
			if ip.Equal(expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected IP SAN %q in cert, got %v", ipStr, cert.IPAddresses)
		}
	}

	// Verify ~1-year validity
	validity := cert.NotAfter.Sub(cert.NotBefore)
	expectedValidity := 365 * 24 * time.Hour
	tolerance := 48 * time.Hour
	if validity < expectedValidity-tolerance || validity > expectedValidity+tolerance {
		t.Errorf("expected ~1yr validity, got %v", validity)
	}

	// Verify signed by CA (verify chain)
	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)
	opts := x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if _, err := cert.Verify(opts); err != nil {
		t.Errorf("server cert failed CA verification: %v", err)
	}
}

func TestSaveAndLoadKeyPair(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error: %v", err)
	}

	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "ca.crt")
	keyPath := filepath.Join(tmpDir, "ca.key")

	// Save
	if err := SaveKeyPair(ca, certPath, keyPath); err != nil {
		t.Fatalf("SaveKeyPair() error: %v", err)
	}

	// Verify cert file permissions (0644)
	certInfo, err := os.Stat(certPath)
	if err != nil {
		t.Fatalf("stat cert file: %v", err)
	}
	if perm := certInfo.Mode().Perm(); perm != 0644 {
		t.Errorf("expected cert perm 0644, got %04o", perm)
	}

	// Verify key file permissions (0600)
	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if perm := keyInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("expected key perm 0600, got %04o", perm)
	}

	// Load
	loaded, err := LoadKeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadKeyPair() error: %v", err)
	}

	// Verify roundtrip
	if loaded.Cert == nil {
		t.Fatal("loaded Cert is nil")
	}
	if loaded.Key == nil {
		t.Fatal("loaded Key is nil")
	}
	if loaded.Cert.Subject.CommonName != ca.Cert.Subject.CommonName {
		t.Errorf("expected CN=%q, got %q", ca.Cert.Subject.CommonName, loaded.Cert.Subject.CommonName)
	}
	if !loaded.Key.Equal(ca.Key) {
		t.Error("loaded key does not match original key")
	}
}

func TestNeedsRenewal(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error: %v", err)
	}

	// Fresh CA cert (10yr validity) should NOT need renewal with a 30-day window
	if NeedsRenewal(ca.Cert, 30*24*time.Hour) {
		t.Error("fresh CA cert should not need renewal within 30-day window")
	}

	// Fresh CA cert should NOT need renewal with a 1-year window
	if NeedsRenewal(ca.Cert, 365*24*time.Hour) {
		t.Error("fresh CA cert should not need renewal within 1-year window")
	}

	// But a cert expiring in a huge window should need renewal
	if !NeedsRenewal(ca.Cert, 11*365*24*time.Hour) {
		t.Error("cert should need renewal when window exceeds remaining validity")
	}
}

func TestCACertPEM(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error: %v", err)
	}

	pemBytes := CertPEM(ca.Cert)
	if len(pemBytes) == 0 {
		t.Fatal("expected non-empty PEM output")
	}

	// Verify it's a valid PEM block
	block, rest := pem.Decode(pemBytes)
	if block == nil {
		t.Fatal("failed to decode PEM block")
	}
	if block.Type != "CERTIFICATE" {
		t.Errorf("expected PEM type %q, got %q", "CERTIFICATE", block.Type)
	}
	if len(rest) != 0 {
		t.Error("unexpected trailing data after PEM block")
	}

	// Verify the PEM contains a parseable cert
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse cert from PEM: %v", err)
	}
	if parsed.Subject.CommonName != "LabTether CA" {
		t.Errorf("expected CN=%q, got %q", "LabTether CA", parsed.Subject.CommonName)
	}
}

func TestCollectSANs(t *testing.T) {
	dnsNames, ips := CollectLocalSANs()

	// Verify localhost is present in DNS names
	foundLocalhost := false
	for _, name := range dnsNames {
		if name == "localhost" {
			foundLocalhost = true
			break
		}
	}
	if !foundLocalhost {
		t.Errorf("expected 'localhost' in DNS names, got %v", dnsNames)
	}

	// Verify 127.0.0.1 is present in IPs
	foundLoopback4 := false
	for _, ip := range ips {
		if ip == "127.0.0.1" {
			foundLoopback4 = true
			break
		}
	}
	if !foundLoopback4 {
		t.Errorf("expected '127.0.0.1' in IPs, got %v", ips)
	}

	// Verify ::1 is present in IPs
	foundLoopback6 := false
	for _, ip := range ips {
		if ip == "::1" {
			foundLoopback6 = true
			break
		}
	}
	if !foundLoopback6 {
		t.Errorf("expected '::1' in IPs, got %v", ips)
	}
}
