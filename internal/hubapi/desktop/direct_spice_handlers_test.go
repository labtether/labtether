package desktop

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"
)

func TestValidateDirectSPICESecurityOptionsDefaultsToVerifiedTLS(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "")

	mode, caPEM, err := ValidateDirectSPICESecurityOptions("spice", "", "")
	if err != nil {
		t.Fatalf("validate default SPICE security: %v", err)
	}
	if mode != SPICESecurityTLS || caPEM != "" {
		t.Fatalf("default SPICE security = %q, %q; want tls and empty CA", mode, caPEM)
	}
}

func TestValidateDirectSPICECleartextRequiresBothOptIns(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "")
	if _, _, err := ValidateDirectSPICESecurityOptions("spice", "cleartext", ""); err == nil {
		t.Fatal("cleartext SPICE succeeded without process-wide opt-in")
	}

	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	mode, _, err := ValidateDirectSPICESecurityOptions("spice", "cleartext", "")
	if err != nil {
		t.Fatalf("cleartext SPICE with both opt-ins: %v", err)
	}
	if mode != SPICESecurityCleartext {
		t.Fatalf("mode = %q, want cleartext", mode)
	}
}

func TestValidateDirectSPICESecurityRejectsCrossProtocolAndInvalidCA(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	tests := []struct {
		name     string
		protocol string
		mode     string
		caPEM    string
	}{
		{name: "SPICE option on RDP", protocol: "rdp", mode: "tls"},
		{name: "unknown mode", protocol: "spice", mode: "opportunistic"},
		{name: "CA with cleartext", protocol: "spice", mode: "cleartext", caPEM: "certificate"},
		{name: "invalid CA", protocol: "spice", mode: "tls", caPEM: "not a certificate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := ValidateDirectSPICESecurityOptions(tc.protocol, tc.mode, tc.caPEM); err == nil {
				t.Fatal("unsafe SPICE security options were accepted")
			}
		})
	}
}

func TestDirectSPICETLSConfigVerifiesCAAndDNSIdentity(t *testing.T) {
	serverCertificate, caPEM := makeSPICETestCertificate(t, "spice.example.test")
	tlsConfig, err := NewDirectSPICETLSConfig("spice.example.test", caPEM)
	if err != nil {
		t.Fatalf("build TLS config: %v", err)
	}
	if tlsConfig.MinVersion != tls.VersionTLS12 || tlsConfig.InsecureSkipVerify {
		t.Fatalf("unsafe TLS config: %+v", tlsConfig)
	}
	if err := runSPICETestHandshake(t, serverCertificate, tlsConfig); err != nil {
		t.Fatalf("trusted SPICE TLS handshake: %v", err)
	}

	wrongName, err := NewDirectSPICETLSConfig("other.example.test", caPEM)
	if err != nil {
		t.Fatalf("build wrong-name TLS config: %v", err)
	}
	if err := runSPICETestHandshake(t, serverCertificate, wrongName); err == nil {
		t.Fatal("SPICE TLS accepted a certificate for the wrong host")
	}

	_, wrongCA := makeSPICETestCertificate(t, "spice.example.test")
	wrongTrust, err := NewDirectSPICETLSConfig("spice.example.test", wrongCA)
	if err != nil {
		t.Fatalf("build wrong-CA TLS config: %v", err)
	}
	if err := runSPICETestHandshake(t, serverCertificate, wrongTrust); err == nil {
		t.Fatal("SPICE TLS accepted a certificate from an untrusted CA")
	}
}

func TestDirectSPICETLSConfigVerifiesIPIdentity(t *testing.T) {
	serverCertificate, caPEM := makeSPICETestCertificate(t, "192.0.2.80")
	tlsConfig, err := NewDirectSPICETLSConfig("192.0.2.80", caPEM)
	if err != nil {
		t.Fatalf("build IP TLS config: %v", err)
	}
	if err := runSPICETestHandshake(t, serverCertificate, tlsConfig); err != nil {
		t.Fatalf("trusted SPICE IP TLS handshake: %v", err)
	}
}

func TestDirectSPICETLSDoesNotFallBackToPlaintext(t *testing.T) {
	tlsConfig, err := NewDirectSPICETLSConfig("spice.example.test", "")
	if err != nil {
		t.Fatalf("build SPICE TLS config: %v", err)
	}
	clientRaw, serverRaw := net.Pipe()
	go func() {
		_, _ = serverRaw.Write([]byte("plain SPICE endpoint"))
		_ = serverRaw.Close()
	}()

	conn, err := secureDirectSPICEConnection(t.Context(), clientRaw, tlsConfig)
	if err == nil {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("TLS SPICE silently fell back to a plaintext endpoint")
	}
}

func makeSPICETestCertificate(t *testing.T, host string) (tls.Certificate, string) {
	t.Helper()
	now := time.Now()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "LabTether SPICE test CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(host); ip != nil {
		serverTemplate.IPAddresses = []net.IP{ip}
	} else {
		serverTemplate.DNSNames = []string{host}
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCertificate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server certificate: %v", err)
	}
	serverCertificate := tls.Certificate{
		Certificate: [][]byte{serverDER, caDER},
		PrivateKey:  serverKey,
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	return serverCertificate, string(caPEM)
}

func runSPICETestHandshake(t *testing.T, certificate tls.Certificate, clientConfig *tls.Config) error {
	t.Helper()
	clientRaw, serverRaw := net.Pipe()
	server := tls.Server(serverRaw, &tls.Config{ // #nosec G402 -- generated test certificate and TLS floor are explicit.
		Certificates: []tls.Certificate{certificate},
		MinVersion:   tls.VersionTLS12,
	})
	client := tls.Client(clientRaw, clientConfig)
	serverResult := make(chan error, 1)
	go func() {
		serverResult <- server.Handshake()
	}()
	clientErr := client.Handshake()
	_ = client.Close()
	_ = server.Close()
	select {
	case <-serverResult:
	case <-time.After(time.Second):
		t.Fatal("server TLS handshake did not finish")
	}
	return clientErr
}
