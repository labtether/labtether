package certmgr

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const renewalWindow = 30 * 24 * time.Hour

// checkInterval is how often the background loop inspects the server cert.
// Defined as a const so tests can verify the loop behaviour without waiting 24h.
const checkInterval = 24 * time.Hour

// ProvisionResult contains the paths and PEM data produced by Provision.
type ProvisionResult struct {
	CACertPath     string
	ServerCertPath string
	ServerKeyPath  string
	CACertPEM      []byte
	Reloader       *CertReloader
}

// CertReloader holds the active TLS certificate and renews it in the background.
// It is safe for concurrent use: GetCertificate and Run may be called from
// multiple goroutines simultaneously.
type CertReloader struct {
	ca       *KeyPair
	certsDir string

	mu   sync.RWMutex
	cert *tls.Certificate
}

// newCertReloader builds a CertReloader from an already-provisioned server cert.
func newCertReloader(ca *KeyPair, certsDir string, serverCertPath, serverKeyPath string) (*CertReloader, error) {
	tlsCert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		return nil, fmt.Errorf("certmgr: load TLS key pair for reloader: %w", err)
	}
	return &CertReloader{
		ca:       ca,
		certsDir: certsDir,
		cert:     &tlsCert,
	}, nil
}

// GetCertificate implements the tls.Config.GetCertificate callback.
// It returns the current server certificate under a read lock so that
// in-flight renewal never races with TLS handshakes.
func (r *CertReloader) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cert, nil
}

// Run starts the background renewal loop. It checks every checkInterval
// whether the server certificate is within the 30-day renewal window and,
// if so, generates and persists a new certificate then atomically swaps it
// into the reloader. Run returns when ctx is cancelled.
func (r *CertReloader) Run(ctx context.Context) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.maybeRenew(); err != nil {
				log.Printf("certmgr: cert renewal error: %v", err)
			}
		}
	}
}

// maybeRenew checks whether the cert needs renewal and, if so, performs it.
func (r *CertReloader) maybeRenew() error {
	r.mu.RLock()
	currentCert := r.cert
	r.mu.RUnlock()

	// Parse the leaf so we can inspect NotAfter.
	leaf := currentCert.Leaf
	if leaf == nil {
		// Leaf may be nil if the cert was loaded without parsing — load it now.
		var err error
		leaf, err = parseCertLeaf(currentCert)
		if err != nil {
			return fmt.Errorf("parse current cert leaf: %w", err)
		}
	}

	if !NeedsRenewal(leaf, renewalWindow) {
		return nil
	}

	return r.renewNow()
}

// ForceRenew generates and installs a new server certificate immediately,
// regardless of the current certificate's expiry. It is safe to call
// concurrently with ongoing TLS handshakes.
func (r *CertReloader) ForceRenew() error {
	return r.renewNow()
}

// renewNow generates a new server cert, persists it, and atomically swaps it
// into the reloader. It is the shared implementation for maybeRenew and
// ForceRenew.
func (r *CertReloader) renewNow() error {
	// Generate a new server cert.
	serverCertPath := filepath.Join(r.certsDir, "server.crt")
	serverKeyPath := filepath.Join(r.certsDir, "server.key")

	dnsNames, ipStrings := CollectLocalSANs()
	serverKP, err := GenerateServerCert(r.ca, dnsNames, ipStrings)
	if err != nil {
		return fmt.Errorf("generate server cert: %w", err)
	}
	if err := SaveKeyPair(serverKP, serverCertPath, serverKeyPath); err != nil {
		return fmt.Errorf("save server cert: %w", err)
	}

	// Load the new cert as a tls.Certificate.
	tlsCert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		return fmt.Errorf("load renewed TLS key pair: %w", err)
	}

	// Atomically swap in the new cert.
	r.mu.Lock()
	r.cert = &tlsCert
	r.mu.Unlock()

	log.Printf("certmgr: renewed server cert, new expiry: %s", serverKP.Cert.NotAfter.Format(time.RFC3339))
	return nil
}

// parseCertLeaf parses the first certificate in the chain from the raw DER bytes.
func parseCertLeaf(tc *tls.Certificate) (*x509.Certificate, error) {
	if len(tc.Certificate) == 0 {
		return nil, fmt.Errorf("tls.Certificate has no certificate bytes")
	}
	return x509.ParseCertificate(tc.Certificate[0])
}

// Provision ensures a CA and server certificate exist in certsDir, creating
// or renewing them as needed. It returns paths to all artifacts and the CA
// certificate PEM bytes (for distribution to agents).
func Provision(certsDir string) (*ProvisionResult, error) {
	// 1. Create certsDir if it doesn't exist
	if err := os.MkdirAll(certsDir, 0700); err != nil {
		return nil, fmt.Errorf("certmgr: create certs directory: %w", err)
	}

	caCertPath := filepath.Join(certsDir, "ca.crt")
	caKeyPath := filepath.Join(certsDir, "ca.key")
	serverCertPath := filepath.Join(certsDir, "server.crt")
	serverKeyPath := filepath.Join(certsDir, "server.key")

	// 2. Provision CA
	var ca *KeyPair
	caCertExists := fileExists(caCertPath)
	caKeyExists := fileExists(caKeyPath)

	switch {
	case caCertExists && caKeyExists:
		// Both exist — load them
		var err error
		ca, err = LoadKeyPair(caCertPath, caKeyPath)
		if err != nil {
			return nil, fmt.Errorf("certmgr: load existing CA: %w", err)
		}
		log.Printf("certmgr: loaded existing CA from %s", certsDir)

	case caCertExists && !caKeyExists:
		// ca.crt exists but ca.key is missing — corruption
		return nil, fmt.Errorf("certmgr: ca.crt exists but ca.key is missing in %s (possible corruption)", certsDir)

	default:
		// Neither exists — generate new CA
		var err error
		ca, err = GenerateCA()
		if err != nil {
			return nil, fmt.Errorf("certmgr: generate CA: %w", err)
		}
		if err := SaveKeyPair(ca, caCertPath, caKeyPath); err != nil {
			return nil, fmt.Errorf("certmgr: save CA: %w", err)
		}
		log.Printf("certmgr: generated new CA in %s", certsDir)
	}

	// 3. Provision server certificate
	serverCertExists := fileExists(serverCertPath)
	serverKeyExists := fileExists(serverKeyPath)

	if serverCertExists && !serverKeyExists {
		return nil, fmt.Errorf("certmgr: server.crt exists but server.key is missing in %s (possible corruption)", certsDir)
	}

	needNewServerCert := true

	if serverCertExists && serverKeyExists {
		serverKP, err := LoadKeyPair(serverCertPath, serverKeyPath)
		if err != nil {
			log.Printf("certmgr: existing server cert is corrupt, regenerating: %v", err)
		} else if NeedsRenewal(serverKP.Cert, renewalWindow) {
			log.Printf("certmgr: server cert expiring within 30 days, regenerating")
		} else {
			log.Printf("certmgr: loaded existing server cert from %s", certsDir)
			needNewServerCert = false
		}
	}

	if needNewServerCert {
		dnsNames, ipStrings := CollectLocalSANs()
		serverKP, err := GenerateServerCert(ca, dnsNames, ipStrings)
		if err != nil {
			return nil, fmt.Errorf("certmgr: generate server cert: %w", err)
		}
		if err := SaveKeyPair(serverKP, serverCertPath, serverKeyPath); err != nil {
			return nil, fmt.Errorf("certmgr: save server cert: %w", err)
		}
		log.Printf("certmgr: generated new server cert in %s", certsDir)
	}

	reloader, err := newCertReloader(ca, certsDir, serverCertPath, serverKeyPath)
	if err != nil {
		return nil, fmt.Errorf("certmgr: create cert reloader: %w", err)
	}

	return &ProvisionResult{
		CACertPath:     caCertPath,
		ServerCertPath: serverCertPath,
		ServerKeyPath:  serverKeyPath,
		CACertPEM:      CertPEM(ca.Cert),
		Reloader:       reloader,
	}, nil
}

// fileExists returns true if the given path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
