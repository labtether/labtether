package collectors

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// An expired certificate, one made for another address and one from an
// unknown issuer need different fixes, so each must get its own reason.
// Before, all of them read "not trusted". The failures here are the ones
// crypto/x509 really returns, wrapped as crypto/tls wraps them, so a change
// in Go's wording fails this test instead of falling to another reason.
func TestConnectorTestFailureNamesTheCertificateProblem(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "pve.example.test"},
		DNSNames:              []string{"pve.example.test"},
		NotBefore:             start,
		NotAfter:              start.AddDate(0, 1, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	trusted := x509.NewCertPool()
	trusted.AddCert(cert)
	inside := start.AddDate(0, 0, 10)
	const wrongAddress = "The certificate of Proxmox does not match that address. Use the name or IP address the certificate was made for."
	cases := []struct {
		name string
		opts x509.VerifyOptions
		want string
	}{
		{"expired", x509.VerifyOptions{Roots: trusted, DNSName: "pve.example.test", CurrentTime: start.AddDate(0, 2, 0)},
			"The certificate of Proxmox has expired. Renew it on Proxmox."},
		{"not valid yet", x509.VerifyOptions{Roots: trusted, DNSName: "pve.example.test", CurrentTime: start.AddDate(0, -1, 0)},
			"The certificate of Proxmox is not valid yet. Check the date and time on Proxmox and on the Hub."},
		{"another name", x509.VerifyOptions{Roots: trusted, DNSName: "nas.example.test", CurrentTime: inside}, wrongAddress},
		{"an IP address the certificate does not hold", x509.VerifyOptions{Roots: trusted, DNSName: "192.0.2.10", CurrentTime: inside}, wrongAddress},
		{"unknown issuer", x509.VerifyOptions{Roots: x509.NewCertPool(), DNSName: "pve.example.test", CurrentTime: inside},
			"The certificate of Proxmox is not trusted. Add its CA certificate or turn on Skip TLS verification."},
	}
	for _, tc := range cases {
		_, verifyErr := cert.Verify(tc.opts)
		if verifyErr == nil {
			t.Fatalf("%s: the certificate verified", tc.name)
		}
		failure := `Get "https://192.0.2.10:8006/api2/json/version": tls: failed to verify certificate: ` + verifyErr.Error()
		rec := httptest.NewRecorder()
		writeConnectorTestFailure(rec, "Proxmox", failure)
		body := rec.Body.String()
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, tc.want) {
			t.Fatalf("%s: %q gave %d %s, want 422 %q", tc.name, failure, rec.Code, body, tc.want)
		}
		for _, internal := range []string{"192.0.2.10", "example.test", "x509", "2026"} {
			if strings.Contains(body, internal) {
				t.Fatalf("%s: answer carries %q: %s", tc.name, internal, body)
			}
		}
	}
}
