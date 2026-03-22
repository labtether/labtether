// Package certmgr provides certificate authority and TLS certificate
// generation for LabTether's built-in TLS infrastructure.
package certmgr

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"
)

// KeyPair holds a parsed certificate, its private key, and the raw DER bytes.
type KeyPair struct {
	Cert    *x509.Certificate
	Key     *ecdsa.PrivateKey
	CertDER []byte
}

// randomSerial generates a 128-bit random serial number for certificates.
func randomSerial() (*big.Int, error) {
	// 128 bits = 16 bytes
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}
	return serial, nil
}

// GenerateCA creates a self-signed ECDSA P-256 root CA certificate.
// The certificate has a 10-year validity period with a 5-minute NotBefore
// backdate to accommodate clock skew between systems.
func GenerateCA() (*KeyPair, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "LabTether CA",
			Organization: []string{"LabTether"},
		},
		NotBefore:             now.Add(-5 * time.Minute), // backdate for clock skew
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create CA certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse CA certificate: %w", err)
	}

	return &KeyPair{
		Cert:    cert,
		Key:     key,
		CertDER: der,
	}, nil
}

// GenerateServerCert creates a server certificate signed by the given CA.
// The certificate has a 1-year validity period with ECDSA P-256 keys.
// dnsNames and ipStrings are added as Subject Alternative Names (SANs).
func GenerateServerCert(ca *KeyPair, dnsNames []string, ipStrings []string) (*KeyPair, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate server key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	var ips []net.IP
	for _, s := range ipStrings {
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address: %q", s)
		}
		ips = append(ips, ip)
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "LabTether Hub",
			Organization: []string{"LabTether"},
		},
		NotBefore:   now.Add(-5 * time.Minute),
		NotAfter:    now.Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    dnsNames,
		IPAddresses: ips,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, &key.PublicKey, ca.Key)
	if err != nil {
		return nil, fmt.Errorf("create server certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse server certificate: %w", err)
	}

	return &KeyPair{
		Cert:    cert,
		Key:     key,
		CertDER: der,
	}, nil
}

// SaveKeyPair writes the certificate and private key to PEM files.
// The certificate file is written with permissions 0644 and the key file
// with permissions 0600 to protect the private key.
func SaveKeyPair(kp *KeyPair, certPath, keyPath string) error {
	// Encode certificate PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: kp.CertDER,
	})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil { // #nosec G306 -- certificate file is intentionally world-readable trust material.
		return fmt.Errorf("write cert file: %w", err)
	}

	// Encode private key PEM
	keyDER, err := x509.MarshalECPrivateKey(kp.Key)
	if err != nil {
		return fmt.Errorf("marshal EC private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("write key file: %w", err)
	}

	return nil
}

// LoadKeyPair reads and parses a certificate and private key from PEM files.
func LoadKeyPair(certPath, keyPath string) (*KeyPair, error) {
	// Read and parse certificate
	certPEM, err := os.ReadFile(certPath) // #nosec G304 -- path comes from trusted runtime configuration and is validated by startup wiring.
	if err != nil {
		return nil, fmt.Errorf("read cert file: %w", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("failed to decode certificate PEM from %s", certPath)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	// Read and parse private key
	keyPEMData, err := os.ReadFile(keyPath) // #nosec G304 -- path comes from trusted runtime configuration and is validated by startup wiring.
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}
	keyBlock, _ := pem.Decode(keyPEMData)
	if keyBlock == nil {
		return nil, fmt.Errorf("failed to decode key PEM from %s", keyPath)
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse EC private key: %w", err)
	}

	return &KeyPair{
		Cert:    cert,
		Key:     key,
		CertDER: block.Bytes,
	}, nil
}

// NeedsRenewal returns true if the certificate expires within the given
// time window from now.
func NeedsRenewal(cert *x509.Certificate, window time.Duration) bool {
	return time.Until(cert.NotAfter) < window
}

// CertPEM returns the PEM-encoded bytes of a certificate.
func CertPEM(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
}

// CollectLocalSANs gathers DNS names and IP addresses suitable for use as
// Subject Alternative Names on a certificate for the local machine. It always
// includes localhost, 127.0.0.1, and ::1, plus the machine's hostname,
// hostname.local, and all non-loopback interface IP addresses.
func CollectLocalSANs() (dnsNames []string, ips []string) {
	dnsNames = []string{"localhost"}
	ips = []string{"127.0.0.1", "::1"}

	// Add hostname
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		dnsNames = append(dnsNames, hostname)
		dnsNames = append(dnsNames, hostname+".local")
	}

	// Add non-loopback interface IPs
	ifaces, err := net.Interfaces()
	if err != nil {
		return dnsNames, ips
	}
	for _, iface := range ifaces {
		// Skip down interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			ips = append(ips, ip.String())
		}
	}

	return dnsNames, ips
}
