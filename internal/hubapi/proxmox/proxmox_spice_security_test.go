package proxmox

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

func TestNewProxmoxSPICETLSConfigVerifiesChainAndHostSubject(t *testing.T) {
	caPEM, caCertificate, caKey := newSPICETestCA(t, "trusted root")
	leaf := newSPICETestLeaf(t, caCertificate, caKey, pkix.Name{
		CommonName:         "pve01",
		Organization:       []string{"Proxmox Virtual Environment"},
		OrganizationalUnit: []string{"PVE Cluster Node"},
	})
	hostSubject := "OU=PVE Cluster Node,O=Proxmox Virtual Environment,CN=pve01"

	config, err := NewProxmoxSPICETLSConfig(false, strings.ReplaceAll(caPEM, "\n", `\n`), hostSubject)
	if err != nil {
		t.Fatalf("build SPICE TLS config: %v", err)
	}
	if config.MinVersion != tls.VersionTLS12 || !config.InsecureSkipVerify || config.VerifyConnection == nil {
		t.Fatalf("SPICE TLS config does not enforce the expected verifier: %+v", config)
	}
	if err := config.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf, caCertificate}}); err != nil {
		t.Fatalf("trusted SPICE certificate was rejected: %v", err)
	}

	wrongSubjectConfig, err := NewProxmoxSPICETLSConfig(false, caPEM, "CN=pve02,O=Proxmox Virtual Environment,OU=PVE Cluster Node")
	if err != nil {
		t.Fatalf("build wrong-subject config: %v", err)
	}
	if err := wrongSubjectConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf, caCertificate}}); err == nil || !strings.Contains(err.Error(), "host-subject") {
		t.Fatalf("wrong SPICE host-subject error=%v, want mismatch", err)
	}

	wrongCAPEM, _, _ := newSPICETestCA(t, "wrong root")
	wrongCAConfig, err := NewProxmoxSPICETLSConfig(false, wrongCAPEM, hostSubject)
	if err != nil {
		t.Fatalf("build wrong-CA config: %v", err)
	}
	if err := wrongCAConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf, caCertificate}}); err == nil || !strings.Contains(err.Error(), "certificate chain") {
		t.Fatalf("wrong SPICE CA error=%v, want chain rejection", err)
	}
}

func TestNewProxmoxSPICETLSConfigRequiresHostSubject(t *testing.T) {
	if _, err := NewProxmoxSPICETLSConfig(false, "", ""); err == nil {
		t.Fatal("expected missing host-subject to be rejected")
	}
}

func TestProxmoxSPICEVerificationPolicyRejectsUntrustedAPICertificateData(t *testing.T) {
	if err := validateProxmoxSPICEVerificationPolicy(true, false); err == nil {
		t.Fatal("SPICE trusted certificate data obtained through an unverified API")
	}
	for _, policy := range []struct {
		apiSkipVerify   bool
		spiceSkipVerify bool
	}{
		{apiSkipVerify: false, spiceSkipVerify: false},
		{apiSkipVerify: false, spiceSkipVerify: true},
		{apiSkipVerify: true, spiceSkipVerify: true},
	} {
		if err := validateProxmoxSPICEVerificationPolicy(policy.apiSkipVerify, policy.spiceSkipVerify); err != nil {
			t.Fatalf("valid verification policy %+v was rejected: %v", policy, err)
		}
	}
}

func TestParseSPICEHostSubjectSupportsEscapedSeparators(t *testing.T) {
	attributes, err := parseSPICEHostSubject(`CN=pve01,O=Example\, Inc.,OU=Lab`)
	if err != nil {
		t.Fatalf("parse escaped host-subject: %v", err)
	}
	if got := attributes["O"]; len(got) != 1 || got[0] != "Example, Inc." {
		t.Fatalf("escaped organization=%v", got)
	}
}

func TestParseSPICEHostSubjectSupportsRealUnescapedCommaValues(t *testing.T) {
	attributes, err := parseSPICEHostSubject(`O=Example, Inc.,OU=PVE Cluster Node,CN=pve01`)
	if err != nil {
		t.Fatalf("parse Proxmox host-subject with comma value: %v", err)
	}
	if got := attributes["O"]; len(got) != 1 || got[0] != "Example, Inc." {
		t.Fatalf("organization with comma=%v", got)
	}
	if got := attributes["CN"]; len(got) != 1 || got[0] != "pve01" {
		t.Fatalf("common name=%v", got)
	}
}

func newSPICETestCA(t *testing.T, commonName string) (string, *x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate SPICE test CA key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create SPICE test CA: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse SPICE test CA: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), certificate, key
}

func newSPICETestLeaf(t *testing.T, caCertificate *x509.Certificate, caKey *rsa.PrivateKey, subject pkix.Name) *x509.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate SPICE test leaf key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      subject,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCertificate, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create SPICE test leaf: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse SPICE test leaf: %v", err)
	}
	return certificate
}
