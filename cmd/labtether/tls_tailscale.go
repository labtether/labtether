package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// provisionTailscaleCert attempts to obtain a publicly trusted TLS certificate
// via `tailscale cert`. Requires Tailscale to be installed, logged in, and able
// to provision ACME certs for the machine's MagicDNS name.
func provisionTailscaleCert(certsDir string) (certPath, keyPath, domain string, err error) {
	path, err := resolveTailscaleBinaryPath()
	if err != nil {
		return "", "", "", fmt.Errorf("tailscale binary not found: %w", err)
	}

	statusOut, err := tailscaleRunner(4*time.Second, path, "status", "--json")
	if err != nil {
		return "", "", "", fmt.Errorf("tailscale status: %w", err)
	}
	status := parseTailscaleStatusSnapshot(statusOut)
	if !status.LoggedIn || status.DNSName == "" {
		return "", "", "", fmt.Errorf("tailscale not logged in or no DNS name available")
	}

	domain = strings.TrimSuffix(status.DNSName, ".")

	tsDir := filepath.Join(certsDir, "tailscale")
	if err := os.MkdirAll(tsDir, 0700); err != nil {
		return "", "", "", fmt.Errorf("create tailscale cert directory: %w", err)
	}

	certPath = filepath.Join(tsDir, "server.crt")
	keyPath = filepath.Join(tsDir, "server.key")

	output, err := tailscaleRunner(30*time.Second, path, "cert",
		"--cert-file", certPath,
		"--key-file", keyPath,
		domain)
	if err != nil {
		return "", "", "", fmt.Errorf("tailscale cert for %s: %s: %w",
			domain, strings.TrimSpace(string(output)), err)
	}

	// Verify the cert is loadable before returning.
	if _, loadErr := tls.LoadX509KeyPair(certPath, keyPath); loadErr != nil {
		return "", "", "", fmt.Errorf("tailscale cert verification: %w", loadErr)
	}

	return certPath, keyPath, domain, nil
}

// tailscaleCertReloader manages a Tailscale-provisioned TLS certificate with
// periodic renewal. It satisfies the same GetCertificate callback signature
// used by Go's tls.Config.
type tailscaleCertReloader struct {
	mu       sync.RWMutex
	cert     *tls.Certificate
	certPath string
	keyPath  string
	domain   string
	certsDir string
}

func newTailscaleCertReloader(certPath, keyPath, domain, certsDir string) *tailscaleCertReloader {
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		log.Printf("labtether: tailscale cert initial load: %v", err)
		return &tailscaleCertReloader{
			certPath: certPath,
			keyPath:  keyPath,
			domain:   domain,
			certsDir: certsDir,
		}
	}
	return &tailscaleCertReloader{
		cert:     &pair,
		certPath: certPath,
		keyPath:  keyPath,
		domain:   domain,
		certsDir: certsDir,
	}
}

func (r *tailscaleCertReloader) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.cert == nil {
		return nil, fmt.Errorf("tailscale certificate not loaded")
	}
	return r.cert, nil
}

// Run periodically calls `tailscale cert` to renew the certificate.
// Tailscale handles ACME renewal internally; this just re-fetches and reloads.
func (r *tailscaleCertReloader) Run(ctx context.Context) {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.renew(); err != nil {
				log.Printf("labtether: tailscale cert renewal: %v", err)
			}
		}
	}
}

func (r *tailscaleCertReloader) renew() error {
	_, _, _, err := provisionTailscaleCert(r.certsDir)
	if err != nil {
		return err
	}
	pair, err := tls.LoadX509KeyPair(r.certPath, r.keyPath)
	if err != nil {
		return fmt.Errorf("reload tailscale cert: %w", err)
	}
	r.mu.Lock()
	r.cert = &pair
	r.mu.Unlock()
	log.Printf("labtether: tailscale cert renewed for %s", r.domain)
	return nil
}
