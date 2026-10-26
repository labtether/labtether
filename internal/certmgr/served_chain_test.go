package certmgr

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"path/filepath"
	"testing"
)

// The iOS app pins the Hub CA on first use when the server presents it. The
// auto-TLS listener must serve leaf + Hub CA, and the CA must stay the same
// across leaf renewals (which use a new leaf key), or every renewal would look
// like a changed certificate.
func TestAutoTLSServesHubCAChainStableAcrossRenewal(t *testing.T) {
	certsDir := filepath.Join(t.TempDir(), "certs")
	result, err := Provision(certsDir)
	if err != nil {
		t.Fatalf("Provision() error: %v", err)
	}
	ca, err := LoadKeyPair(result.CACertPath, filepath.Join(certsDir, "ca.key"))
	if err != nil {
		t.Fatal(err)
	}

	served, err := result.Reloader.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatal(err)
	}
	assertServedChain(t, served, ca)
	firstLeaf := append([]byte(nil), served.Certificate[0]...)

	if err := result.Reloader.ForceRenew(); err != nil {
		t.Fatalf("ForceRenew() error: %v", err)
	}
	renewed, err := result.Reloader.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(firstLeaf, renewed.Certificate[0]) {
		t.Fatal("renewal did not issue a new leaf")
	}
	assertServedChain(t, renewed, ca)

	// A restart reloads the same CA from disk rather than minting a new one.
	again, err := Provision(certsDir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := again.Reloader.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatal(err)
	}
	assertServedChain(t, reloaded, ca)
}

func assertServedChain(t *testing.T, served *tls.Certificate, ca *KeyPair) {
	t.Helper()
	if len(served.Certificate) != 2 {
		t.Fatalf("served chain has %d certificates, want leaf + Hub CA", len(served.Certificate))
	}
	if !bytes.Equal(served.Certificate[1], ca.CertDER) {
		t.Fatal("second certificate in the served chain is not the Hub CA")
	}
	leaf, err := x509.ParseCertificate(served.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots}); err != nil {
		t.Fatalf("served leaf does not chain to the Hub CA: %v", err)
	}
}
