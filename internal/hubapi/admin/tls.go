package admin

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"net/http"

	opspkg "github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/servicehttp"
)

// HandleCACert serves the hub's CA certificate as a PEM file download.
// GET /api/v1/ca.crt — unauthenticated.
func (d *Deps) HandleCACert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if d.TLSState == nil || len(d.TLSState.CACertPEM) == 0 {
		servicehttp.WriteError(w, http.StatusNotFound, "no CA certificate available (TLS may be disabled or using external certs)")
		return
	}

	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="labtether-ca.crt"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(d.TLSState.CACertPEM)
}

// HandleTLSInfo returns TLS configuration status as JSON.
// GET /api/v1/tls/info — unauthenticated.
func (d *Deps) HandleTLSInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if d.TLSState == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "tls state unavailable")
		return
	}

	caAvailable := len(d.TLSState.CACertPEM) > 0

	resp := map[string]any{
		"tls_enabled":  d.TLSState.Enabled,
		"tls_mode":     d.TLSState.Mode,
		"tls_source":   d.TLSState.Source,
		"cert_type":    d.currentTLSCertType(),
		"ca_available": caAvailable,
	}
	resp["https_port"] = d.TLSState.HttpsPort
	resp["http_port"] = d.TLSState.HttpPort

	if activeMeta, err := d.activeTLSCertificateMetadata(); err == nil {
		if activeMeta.SubjectCommonName != "" {
			resp["cert_subject_common_name"] = activeMeta.SubjectCommonName
		}
		if activeMeta.SubjectSummary != "" {
			resp["cert_subject_summary"] = activeMeta.SubjectSummary
		}
		if activeMeta.IssuerSummary != "" {
			resp["cert_issuer_summary"] = activeMeta.IssuerSummary
		}
		if !activeMeta.ExpiresAt.IsZero() {
			resp["cert_expires"] = activeMeta.ExpiresAt
		}
		if activeMeta.FingerprintSHA256 != "" {
			resp["cert_fingerprint_sha256"] = activeMeta.FingerprintSHA256
		}
		if len(activeMeta.DNSNames) > 0 {
			resp["cert_dns_names"] = activeMeta.DNSNames
		}
	}

	// If we have a CA cert, parse it to extract fingerprint and expiry.
	if caAvailable {
		block, _ := pem.Decode(d.TLSState.CACertPEM)
		if block != nil {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil {
				fingerprint := sha256.Sum256(cert.Raw)
				resp["ca_fingerprint_sha256"] = hex.EncodeToString(fingerprint[:])
				resp["ca_expires"] = cert.NotAfter
			}
		}
	}

	servicehttp.WriteJSON(w, http.StatusOK, resp)
}

// currentTLSCertType returns a human-readable label for the active TLS cert.
func (d *Deps) currentTLSCertType() string {
	if d.TLSState == nil {
		return "none"
	}
	switch {
	case !d.TLSState.Enabled:
		return "none"
	case d.TLSState.Source == opspkg.TLSSourceUIUploaded:
		return "uploaded"
	case d.TLSState.Source == opspkg.TLSSourceTailscale:
		return "tailscale"
	case d.TLSState.Source == opspkg.TLSSourceBuiltIn, len(d.TLSState.CACertPEM) > 0:
		return "self-signed"
	default:
		return "external"
	}
}

// activeTLSCertificateMetadata reads and parses the metadata of the currently
// active TLS certificate from disk.
func (d *Deps) activeTLSCertificateMetadata() (opspkg.TLSCertificateMetadata, error) {
	if d.TLSState == nil || !d.TLSState.Enabled || d.TLSState.CertFile == "" {
		return opspkg.TLSCertificateMetadata{}, nil
	}
	return opspkg.TLSCertificateMetadataFromFile(d.TLSState.CertFile)
}

// CurrentTLSCertType is the exported equivalent of currentTLSCertType, used by
// cmd/labtether/admin_bridge.go so the method is accessible as a forwarding
// stub on apiServer.
func (d *Deps) CurrentTLSCertType() string {
	return d.currentTLSCertType()
}

// ActiveTLSCertificateMetadata is the exported equivalent used by the bridge.
func (d *Deps) ActiveTLSCertificateMetadata() (opspkg.TLSCertificateMetadata, error) {
	return d.activeTLSCertificateMetadata()
}
