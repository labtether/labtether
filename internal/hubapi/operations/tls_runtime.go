package operations

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/secrets"
)

const (
	TLSSourceDisabled           = "disabled"
	TLSSourceBuiltIn            = "built_in"
	TLSSourceTailscale          = "tailscale"
	TLSSourceDeploymentExternal = "deployment_external"
	TLSSourceUIUploaded         = "ui_uploaded"

	TLSTrustModePublicTLS   = "public_tls"
	TLSTrustModeCustomTLS   = "custom_tls"
	TLSTrustModeLabtetherCA = "labtether_ca"
	TLSTrustModePlainHTTP   = "plain_http"

	TLSBootstrapStrategyInstall  = "install_script"
	TLSBootstrapStrategyPinnedCA = "pinned_ca_bootstrap"

	TLSOverrideCertPEMKey       = "tls.override.cert_pem"
	TLSOverrideKeyCipherKey     = "tls.override.key_ciphertext"
	TLSOverrideUpdatedAtKey     = "tls.override.updated_at"
	TLSOverrideAAD              = "runtime.tls.override.key"
	TLSUploadedCertRelativePath = "certs/uploaded/server.crt"
	TLSUploadedKeyRelativePath  = "certs/uploaded/server.key"
)

// HubCertificateSwitcher allows runtime switching of TLS certificate providers.
type HubCertificateSwitcher struct {
	mu       sync.RWMutex
	provider func(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

func (s *HubCertificateSwitcher) SetProvider(provider func(*tls.ClientHelloInfo) (*tls.Certificate, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.provider = provider
}

func (s *HubCertificateSwitcher) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	s.mu.RLock()
	provider := s.provider
	s.mu.RUnlock()
	if provider == nil {
		return nil, errors.New("tls certificate provider is not configured")
	}
	return provider(hello)
}

// StaticHubCertificateProvider serves a fixed TLS certificate.
type StaticHubCertificateProvider struct {
	cert *tls.Certificate
}

// NewStaticHubCertificateProvider loads a TLS key pair from files.
func NewStaticHubCertificateProvider(certFile, keyFile string) (*StaticHubCertificateProvider, error) {
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS key pair: %w", err)
	}
	return &StaticHubCertificateProvider{cert: &pair}, nil
}

func (p *StaticHubCertificateProvider) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if p == nil || p.cert == nil {
		return nil, errors.New("static tls certificate is not configured")
	}
	return p.cert, nil
}

// TLSCertificateMetadata holds parsed metadata from a TLS certificate.
type TLSCertificateMetadata struct {
	SubjectCommonName string    `json:"subject_common_name,omitempty"`
	SubjectSummary    string    `json:"subject_summary,omitempty"`
	IssuerSummary     string    `json:"issuer_summary,omitempty"`
	ExpiresAt         time.Time `json:"expires_at,omitempty"`
	FingerprintSHA256 string    `json:"fingerprint_sha256,omitempty"`
	DNSNames          []string  `json:"dns_names,omitempty"`
}

// TLSOverrideMaterial holds uploaded TLS certificate and key material.
type TLSOverrideMaterial struct {
	CertPEM   string
	KeyPEM    string
	UpdatedAt time.Time
}

// SummarizePKIXName returns a human-readable summary of a PKIX name.
func SummarizePKIXName(name pkix.Name) string {
	if name.String() != "" {
		return name.String()
	}
	if len(name.Organization) > 0 {
		return strings.Join(name.Organization, ", ")
	}
	if len(name.OrganizationalUnit) > 0 {
		return strings.Join(name.OrganizationalUnit, ", ")
	}
	if len(name.Country) > 0 {
		return strings.Join(name.Country, ", ")
	}
	return ""
}

// TLSCertificateMetadataFromPEM parses certificate metadata from PEM data.
func TLSCertificateMetadataFromPEM(certPEM string) (TLSCertificateMetadata, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(certPEM)))
	if block == nil || block.Type != "CERTIFICATE" {
		return TLSCertificateMetadata{}, errors.New("PEM data does not contain a CERTIFICATE block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return TLSCertificateMetadata{}, err
	}
	return TLSCertificateMetadataFromCert(cert), nil
}

// TLSCertificateMetadataFromFile reads a certificate file and parses its metadata.
func TLSCertificateMetadataFromFile(certFile string) (TLSCertificateMetadata, error) {
	data, err := os.ReadFile(certFile) // #nosec G304 -- Certificate path is operator-configured runtime state, not user-supplied input.
	if err != nil {
		return TLSCertificateMetadata{}, err
	}
	return TLSCertificateMetadataFromPEM(string(data))
}

// TLSCertificateMetadataFromCert extracts metadata from a parsed x509 certificate.
func TLSCertificateMetadataFromCert(cert *x509.Certificate) TLSCertificateMetadata {
	if cert == nil {
		return TLSCertificateMetadata{}
	}
	fingerprint := sha256.Sum256(cert.Raw)
	return TLSCertificateMetadata{
		SubjectCommonName: strings.TrimSpace(cert.Subject.CommonName),
		SubjectSummary:    SummarizePKIXName(cert.Subject),
		IssuerSummary:     SummarizePKIXName(cert.Issuer),
		ExpiresAt:         cert.NotAfter,
		FingerprintSHA256: hex.EncodeToString(fingerprint[:]),
		DNSNames:          append([]string(nil), cert.DNSNames...),
	}
}

// ValidateUploadedTLSPair validates a cert/key PEM pair and returns metadata.
func ValidateUploadedTLSPair(certPEM, keyPEM string) (TLSCertificateMetadata, error) {
	keyPair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return TLSCertificateMetadata{}, fmt.Errorf("certificate/key validation failed: %w", err)
	}
	if len(keyPair.Certificate) == 0 {
		return TLSCertificateMetadata{}, errors.New("uploaded certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return TLSCertificateMetadata{}, fmt.Errorf("parse certificate leaf: %w", err)
	}
	return TLSCertificateMetadataFromCert(leaf), nil
}

// MaterializeUploadedTLSFiles writes uploaded TLS cert/key PEM to disk.
func MaterializeUploadedTLSFiles(dataDir, certPEM, keyPEM string) (string, string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", "", errors.New("data directory is required")
	}
	certPath := filepath.Join(dataDir, TLSUploadedCertRelativePath)
	keyPath := filepath.Join(dataDir, TLSUploadedKeyRelativePath)
	if err := os.MkdirAll(filepath.Dir(certPath), 0700); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(certPath, []byte(strings.TrimSpace(certPEM)+"\n"), 0644); err != nil { // #nosec G306 -- certificate file is public trust material.
		return "", "", err
	}
	if err := os.WriteFile(keyPath, []byte(strings.TrimSpace(keyPEM)+"\n"), 0600); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}

// LoadPersistedTLSOverride reads a previously uploaded TLS override from the runtime store.
func LoadPersistedTLSOverride(store interface {
	ListRuntimeSettingOverrides() (map[string]string, error)
}, secretsManager *secrets.Manager) (TLSOverrideMaterial, bool, error) {
	if store == nil {
		return TLSOverrideMaterial{}, false, nil
	}
	values, err := store.ListRuntimeSettingOverrides()
	if err != nil {
		return TLSOverrideMaterial{}, false, err
	}
	certPEM := strings.TrimSpace(values[TLSOverrideCertPEMKey])
	keyCipher := strings.TrimSpace(values[TLSOverrideKeyCipherKey])
	if certPEM == "" || keyCipher == "" {
		return TLSOverrideMaterial{}, false, nil
	}
	if secretsManager == nil {
		return TLSOverrideMaterial{}, false, errors.New("secrets manager is required for persisted TLS override")
	}
	keyPEM, err := secretsManager.DecryptString(keyCipher, TLSOverrideAAD)
	if err != nil {
		return TLSOverrideMaterial{}, false, err
	}
	override := TLSOverrideMaterial{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}
	if rawUpdatedAt := strings.TrimSpace(values[TLSOverrideUpdatedAtKey]); rawUpdatedAt != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, rawUpdatedAt); parseErr == nil {
			override.UpdatedAt = parsed
		}
	}
	return override, true, nil
}

// BuildPinnedBootstrapURL constructs a bootstrap URL with the CA fingerprint pinned.
func BuildPinnedBootstrapURL(hubURL string, caCertPEM []byte) string {
	trimmedHubURL := strings.TrimRight(strings.TrimSpace(hubURL), "/")
	if trimmedHubURL == "" || len(caCertPEM) == 0 {
		return ""
	}
	block, _ := pem.Decode(caCertPEM)
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	fingerprint := sha256.Sum256(cert.Raw)
	return fmt.Sprintf("%s/api/v1/agent/bootstrap.sh?ca_fingerprint_sha256=%s", trimmedHubURL, hex.EncodeToString(fingerprint[:]))
}
