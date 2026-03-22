package certmgr

import (
	"bytes"
	"context"
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProvision_FreshDirectory(t *testing.T) {
	dir := t.TempDir()
	certsDir := filepath.Join(dir, "certs")

	result, err := Provision(certsDir)
	if err != nil {
		t.Fatalf("Provision() error: %v", err)
	}

	// Verify all paths are non-empty
	if result.CACertPath == "" {
		t.Error("expected non-empty CACertPath")
	}
	if result.ServerCertPath == "" {
		t.Error("expected non-empty ServerCertPath")
	}
	if result.ServerKeyPath == "" {
		t.Error("expected non-empty ServerKeyPath")
	}

	// Verify CACertPEM is non-empty
	if len(result.CACertPEM) == 0 {
		t.Error("expected non-empty CACertPEM")
	}

	// Verify all 4 files exist on disk
	for _, name := range []string{"ca.crt", "ca.key", "server.crt", "server.key"} {
		path := filepath.Join(certsDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", name)
		}
	}
}

func TestProvision_ExistingCerts(t *testing.T) {
	dir := t.TempDir()
	certsDir := filepath.Join(dir, "certs")

	// First provision — generates everything fresh
	result1, err := Provision(certsDir)
	if err != nil {
		t.Fatalf("first Provision() error: %v", err)
	}

	// Second provision — should load existing certs
	result2, err := Provision(certsDir)
	if err != nil {
		t.Fatalf("second Provision() error: %v", err)
	}

	// CA PEM should match between calls (same CA was loaded, not regenerated)
	if !bytes.Equal(result1.CACertPEM, result2.CACertPEM) {
		t.Error("expected CA PEM to match between first and second provision calls")
	}

	// Paths should be the same
	if result1.CACertPath != result2.CACertPath {
		t.Error("expected CACertPath to match between calls")
	}
	if result1.ServerCertPath != result2.ServerCertPath {
		t.Error("expected ServerCertPath to match between calls")
	}
	if result1.ServerKeyPath != result2.ServerKeyPath {
		t.Error("expected ServerKeyPath to match between calls")
	}
}

func TestProvision_MissingCAKey_Errors(t *testing.T) {
	dir := t.TempDir()
	certsDir := filepath.Join(dir, "certs")

	// First provision — generates everything
	_, err := Provision(certsDir)
	if err != nil {
		t.Fatalf("Provision() error: %v", err)
	}

	// Delete the CA key to simulate corruption
	caKeyPath := filepath.Join(certsDir, "ca.key")
	if err := os.Remove(caKeyPath); err != nil {
		t.Fatalf("failed to remove ca.key: %v", err)
	}

	// Second provision should fail because ca.crt exists but ca.key is missing
	_, err = Provision(certsDir)
	if err == nil {
		t.Fatal("expected error when ca.key is missing but ca.crt exists")
	}
}

func TestProvision_MissingServerKey_Errors(t *testing.T) {
	dir := t.TempDir()
	certsDir := filepath.Join(dir, "certs")

	// First provision — generates everything
	_, err := Provision(certsDir)
	if err != nil {
		t.Fatalf("Provision() error: %v", err)
	}

	// Delete the server key to simulate corruption
	serverKeyPath := filepath.Join(certsDir, "server.key")
	if err := os.Remove(serverKeyPath); err != nil {
		t.Fatalf("failed to remove server.key: %v", err)
	}

	// Second provision should fail because server.crt exists but server.key is missing
	_, err = Provision(certsDir)
	if err == nil {
		t.Fatal("expected error when server.key is missing but server.crt exists")
	}
}

// TestCertReloader_GetCertificate verifies that Provision returns a functional
// CertReloader whose GetCertificate callback returns a valid, non-nil certificate.
func TestCertReloader_GetCertificate(t *testing.T) {
	dir := t.TempDir()
	certsDir := filepath.Join(dir, "certs")

	result, err := Provision(certsDir)
	if err != nil {
		t.Fatalf("Provision() error: %v", err)
	}

	if result.Reloader == nil {
		t.Fatal("expected non-nil Reloader in ProvisionResult")
	}

	cert, err := result.Reloader.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatalf("GetCertificate() error: %v", err)
	}
	if cert == nil {
		t.Fatal("GetCertificate() returned nil cert")
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("GetCertificate() returned cert with no DER bytes")
	}
}

// TestCertReloader_RenewsExpiredCert verifies that maybeRenew detects a cert
// within the renewal window and replaces it with a fresh one. The test injects
// a near-expiry leaf (NotAfter = now+1d, well inside the 30-day renewal window)
// directly into the reloader, then calls maybeRenew synchronously and confirms
// the certificate was replaced with a fresh one.
func TestCertReloader_RenewsExpiredCert(t *testing.T) {
	dir := t.TempDir()
	certsDir := filepath.Join(dir, "certs")

	// Provision a full set of certs to get a valid CA on disk.
	result, err := Provision(certsDir)
	if err != nil {
		t.Fatalf("Provision() error: %v", err)
	}

	// Load the CA key pair so we can generate a near-expiry server cert.
	caCertPath := filepath.Join(certsDir, "ca.crt")
	caKeyPath := filepath.Join(certsDir, "ca.key")
	ca, err := LoadKeyPair(caCertPath, caKeyPath)
	if err != nil {
		t.Fatalf("LoadKeyPair(CA) error: %v", err)
	}

	// Generate a fresh server cert, then mutate its NotAfter to be only 1 day
	// away so it falls inside the 30-day renewal window.
	dnsNames, ipStrings := CollectLocalSANs()
	nearExpiryKP, err := GenerateServerCert(ca, dnsNames, ipStrings)
	if err != nil {
		t.Fatalf("GenerateServerCert() error: %v", err)
	}
	nearExpiryKP.Cert.NotAfter = time.Now().Add(24 * time.Hour)

	// Overwrite the server cert files with the near-expiry cert so that
	// maybeRenew can save the renewed cert to the same paths.
	serverCertPath := filepath.Join(certsDir, "server.crt")
	serverKeyPath := filepath.Join(certsDir, "server.key")
	if err := SaveKeyPair(nearExpiryKP, serverCertPath, serverKeyPath); err != nil {
		t.Fatalf("SaveKeyPair(nearExpiry) error: %v", err)
	}

	// Build a tls.Certificate from the near-expiry files.
	tlsNearExpiry, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		t.Fatalf("tls.LoadX509KeyPair(nearExpiry) error: %v", err)
	}

	// Inject the near-expiry cert (with manipulated Leaf) into the reloader.
	result.Reloader.mu.Lock()
	result.Reloader.cert = &tlsNearExpiry
	result.Reloader.cert.Leaf = nearExpiryKP.Cert // expose manipulated NotAfter
	result.Reloader.mu.Unlock()

	// Capture the original DER bytes before renewal for comparison.
	origDER := make([]byte, len(tlsNearExpiry.Certificate[0]))
	copy(origDER, tlsNearExpiry.Certificate[0])

	// Trigger renewal synchronously — should detect expiry and renew.
	if err := result.Reloader.maybeRenew(); err != nil {
		t.Fatalf("maybeRenew() error: %v", err)
	}

	// The reloader should now hold a different certificate.
	renewed, err := result.Reloader.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatalf("GetCertificate() after renewal error: %v", err)
	}
	if len(renewed.Certificate) == 0 {
		t.Fatal("renewed cert has no DER bytes")
	}
	if bytes.Equal(origDER, renewed.Certificate[0]) {
		t.Error("expected renewed cert DER to differ from near-expiry cert DER")
	}

	// The renewed cert should have ample validity remaining.
	leaf, err := parseCertLeaf(renewed)
	if err != nil {
		t.Fatalf("parseCertLeaf(renewed) error: %v", err)
	}
	if NeedsRenewal(leaf, renewalWindow) {
		t.Errorf("renewed cert should not need renewal; NotAfter=%s", leaf.NotAfter)
	}
}

// TestCertReloader_Run_CancelsCleanly verifies that Run returns promptly when
// its context is cancelled, ensuring no goroutine leak.
func TestCertReloader_Run_CancelsCleanly(t *testing.T) {
	dir := t.TempDir()
	certsDir := filepath.Join(dir, "certs")

	result, err := Provision(certsDir)
	if err != nil {
		t.Fatalf("Provision() error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		result.Reloader.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Run returned as expected.
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}
