package operations

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

func testCertificateFor(t *testing.T, dnsName string) *tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// Uploading a certificate for an outside name used to replace the certificate
// for every name, so the bundled console, which reaches the Hub as
// "labtether", failed its name check and lost the Hub.
func TestUploadedCertificateKeepsBuiltInForNamesItDoesNotCover(t *testing.T) {
	uploaded := testCertificateFor(t, "hub.example.com")
	builtIn := testCertificateFor(t, "labtether")
	provider := certificateForServerName(
		func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return uploaded, nil },
		func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return builtIn, nil },
	)

	for serverName, want := range map[string]*tls.Certificate{
		"labtether":       builtIn,
		"hub.example.com": uploaded,
		"":                uploaded,
		"nas01.lan":       uploaded,
	} {
		got, err := provider(&tls.ClientHelloInfo{ServerName: serverName})
		if err != nil {
			t.Fatalf("server name %q: %v", serverName, err)
		}
		if got != want {
			t.Fatalf("server name %q got the wrong certificate", serverName)
		}
	}
}
