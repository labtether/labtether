package main

import (
	"errors"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// builtInCertificateSANHosts returns the externally advertised HTTPS hostname
// or IP so the built-in server certificate is valid for the exact URL shown to
// users and agents. Invalid and non-HTTPS external URLs are never trusted as
// certificate inputs.
func builtInCertificateSANHosts(rawExternalURL string) []string {
	sanitized, ok := sanitizeExternalBaseURL(strings.TrimSpace(rawExternalURL))
	if !ok {
		return nil
	}
	parsed, err := url.Parse(sanitized)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") {
		return nil
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return nil
	}
	return []string{host}
}

func shouldUseTailscaleCertificate(tailscaleDomain string, externalHosts []string) bool {
	if len(externalHosts) == 0 {
		return true
	}
	return strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(tailscaleDomain), "."), externalHosts[0])
}

// shareCACert copies the public CA cert into a share directory for sidecar
// services (agent/console) when available. In non-container local dev runs,
// the default path is often absent; skip quietly in that case.
func shareCACert(caCertPEM []byte, defaultShareDir string) {
	shareDir, explicit := os.LookupEnv("LABTETHER_CA_SHARE_DIR")
	if explicit {
		shareDir = strings.TrimSpace(shareDir)
		if shareDir == "" {
			log.Printf("labtether: warning: LABTETHER_CA_SHARE_DIR is empty; skipping CA share copy")
			return
		}
	} else {
		shareDir = strings.TrimSpace(defaultShareDir)
		if shareDir == "" {
			return
		}
		info, err := os.Stat(shareDir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return
			}
			log.Printf("labtether: warning: could not stat CA share dir %s: %v", shareDir, err)
			return
		}
		if !info.IsDir() {
			log.Printf("labtether: warning: CA share path %s is not a directory; skipping", shareDir)
			return
		}
	}

	if err := os.MkdirAll(shareDir, 0750); err != nil {
		log.Printf("labtether: warning: could not create CA share dir %s: %v", shareDir, err)
		return
	}

	caPath := filepath.Join(shareDir, "ca.crt")
	if err := os.WriteFile(caPath, caCertPEM, 0644); err != nil { // #nosec G306 -- CA certificate is public trust material and intended to be readable by sidecars.
		log.Printf("labtether: warning: could not write CA to %s: %v", caPath, err)
	}
}
