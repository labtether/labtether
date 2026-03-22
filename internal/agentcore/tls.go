package agentcore

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

var loadSystemCertPool = x509.SystemCertPool

// buildTLSConfig creates a *tls.Config from agent TLS settings.
// Returns nil when no TLS options are configured (plain HTTP mode).
func buildTLSConfig(cfg *RuntimeConfig) *tls.Config {
	if cfg == nil {
		return nil
	}
	if cfg.TLSCAFile == "" && !cfg.TLSSkipVerify {
		return nil
	}

	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		// #nosec G402 -- operator opt-in for local/dev self-signed deployments.
		InsecureSkipVerify: cfg.TLSSkipVerify, //nolint:gosec // #nosec G402 -- operator opt-in for dev/self-signed
	}

	if cfg.TLSCAFile != "" {
		caCert, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agent: warning: failed to read TLS CA file %s: %v\n", cfg.TLSCAFile, err)
			return tlsCfg
		}
		pool, err := loadSystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(caCert) {
			fmt.Fprintf(os.Stderr, "agent: warning: no valid certs found in %s\n", cfg.TLSCAFile)
			return tlsCfg
		}
		tlsCfg.RootCAs = pool
	}

	return tlsCfg
}
