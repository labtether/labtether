package operations

import (
	"crypto/tls"

	"github.com/labtether/labtether/internal/certmgr"
)

// TLSState bundles all TLS runtime configuration that was previously scattered
// as flat fields on apiServer. It is defined here (in the operations package)
// so that internal/hubapi/admin can accept a *TLSState without creating an
// import cycle back to cmd/labtether.
type TLSState struct {
	Enabled               bool
	Mode                  string
	Source                string
	CertFile              string
	KeyFile               string
	HttpsPort             int
	HttpPort              int
	DefaultMode           string
	DefaultSource         string
	DefaultCertFile       string
	DefaultKeyFile        string
	DefaultCAPEM          []byte
	DefaultGetCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error)
	CACertPEM             []byte                // PEM-encoded CA certificate (for enrollment distribution)
	CertReloader          *certmgr.CertReloader // non-nil when TLS mode is "auto" with built-in certs
	// TailscaleCertReloader holds the Tailscale cert reloader when TLS mode is
	// "auto" with Tailscale certs. The concrete type lives in cmd/labtether and
	// is stored as any to avoid an import cycle. Callers that need method access
	// (e.g. apiv2_advanced.go) perform a type assertion at the call site.
	TailscaleCertReloader any
	CertSwitcher          *HubCertificateSwitcher
}
