package proxmox

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const maxProxmoxSPICEHostSubjectBytes = 4096

func validateProxmoxSPICEVerificationPolicy(apiSkipVerify, spiceSkipVerify bool) error {
	if apiSkipVerify && !spiceSkipVerify {
		return errors.New("Proxmox SPICE is blocked because the API certificate is not verified; verify the API certificate or explicitly enable the SPICE certificate bypass")
	}
	return nil
}

// NewProxmoxSPICETLSConfig verifies a Proxmox SPICE certificate against both
// its CA chain and the exact host-subject supplied in the signed SPICE config.
// Proxmox cannot use normal hostname verification because its SPICE proxy
// ticket is deliberately placed in the connection host field.
func NewProxmoxSPICETLSConfig(skipVerify bool, caPEM, hostSubject string) (*tls.Config, error) {
	caPEM = strings.ReplaceAll(strings.TrimSpace(caPEM), `\n`, "\n")
	if skipVerify {
		return NewProxmoxTLSConfig(true, caPEM)
	}
	expectedSubject, err := parseSPICEHostSubject(hostSubject)
	if err != nil {
		return nil, err
	}

	var roots *x509.CertPool
	if caPEM != "" {
		roots = x509.NewCertPool()
		if ok := roots.AppendCertsFromPEM([]byte(caPEM)); !ok {
			return nil, errors.New("invalid proxmox SPICE CA PEM")
		}
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		// #nosec G402 -- VerifyConnection performs chain and exact subject verification.
		InsecureSkipVerify: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("proxmox SPICE server did not provide a certificate")
			}
			intermediates := x509.NewCertPool()
			for _, certificate := range state.PeerCertificates[1:] {
				intermediates.AddCert(certificate)
			}
			if _, err := state.PeerCertificates[0].Verify(x509.VerifyOptions{
				Roots:         roots,
				Intermediates: intermediates,
				KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			}); err != nil {
				return fmt.Errorf("verify proxmox SPICE certificate chain: %w", err)
			}
			actualSubject := certificateSubjectAttributes(state.PeerCertificates[0])
			if !equalSubjectAttributes(actualSubject, expectedSubject) {
				return errors.New("proxmox SPICE certificate subject does not match host-subject")
			}
			return nil
		},
	}, nil
}

func parseSPICEHostSubject(raw string) (map[string][]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("proxmox SPICE host-subject is required")
	}
	if len(raw) > maxProxmoxSPICEHostSubjectBytes {
		return nil, fmt.Errorf("proxmox SPICE host-subject too long (max %d bytes)", maxProxmoxSPICEHostSubjectBytes)
	}
	raw = strings.TrimPrefix(raw, "/")
	separator := byte(',')
	if !strings.Contains(raw, ",") && strings.Contains(raw, "/") {
		separator = '/'
	}

	parts, err := splitEscapedSubject(raw, separator)
	if err != nil {
		return nil, err
	}
	attributes := make(map[string][]string, len(parts))
	for _, part := range parts {
		keyValue, err := splitSubjectAttribute(part)
		if err != nil {
			return nil, err
		}
		key := strings.ToUpper(strings.TrimSpace(keyValue[0]))
		value := strings.TrimSpace(keyValue[1])
		if key == "" || value == "" {
			return nil, errors.New("invalid proxmox SPICE host-subject")
		}
		attributes[key] = append(attributes[key], value)
	}
	for key := range attributes {
		sort.Strings(attributes[key])
	}
	return attributes, nil
}

func splitEscapedSubject(raw string, separator byte) ([]string, error) {
	parts := make([]string, 0, 4)
	var current strings.Builder
	escaped := false
	for i := 0; i < len(raw); i++ {
		value := raw[i]
		if escaped {
			current.WriteByte(value)
			escaped = false
			continue
		}
		if value == '\\' {
			escaped = true
			continue
		}
		if value == separator && (separator != ',' || startsSubjectAttribute(raw[i+1:])) {
			if strings.TrimSpace(current.String()) == "" {
				return nil, errors.New("invalid proxmox SPICE host-subject")
			}
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(value)
	}
	if escaped || strings.TrimSpace(current.String()) == "" {
		return nil, errors.New("invalid proxmox SPICE host-subject")
	}
	parts = append(parts, current.String())
	return parts, nil
}

func startsSubjectAttribute(raw string) bool {
	raw = strings.TrimLeft(raw, " \t")
	separator := strings.IndexByte(raw, '=')
	if separator <= 0 {
		return false
	}
	for i := 0; i < separator; i++ {
		value := raw[i]
		if (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') ||
			(value >= '0' && value <= '9') || value == '.' || value == '-' || value == '_' {
			continue
		}
		return false
	}
	return true
}

func splitSubjectAttribute(raw string) ([2]string, error) {
	var result [2]string
	for i := 0; i < len(raw); i++ {
		if raw[i] == '=' {
			result[0], result[1] = raw[:i], raw[i+1:]
			return result, nil
		}
	}
	return result, errors.New("invalid proxmox SPICE host-subject")
}

func certificateSubjectAttributes(certificate *x509.Certificate) map[string][]string {
	attributes := make(map[string][]string, len(certificate.Subject.Names))
	for _, name := range certificate.Subject.Names {
		key := subjectOIDLabel(name.Type.String())
		attributes[key] = append(attributes[key], fmt.Sprint(name.Value))
	}
	for key := range attributes {
		sort.Strings(attributes[key])
	}
	return attributes
}

func subjectOIDLabel(oid string) string {
	switch oid {
	case "2.5.4.3":
		return "CN"
	case "2.5.4.5":
		return "SERIALNUMBER"
	case "2.5.4.6":
		return "C"
	case "2.5.4.7":
		return "L"
	case "2.5.4.8":
		return "ST"
	case "2.5.4.9":
		return "STREET"
	case "2.5.4.10":
		return "O"
	case "2.5.4.11":
		return "OU"
	case "2.5.4.17":
		return "POSTALCODE"
	case "1.2.840.113549.1.9.1":
		return "EMAILADDRESS"
	default:
		return oid
	}
}

func equalSubjectAttributes(left, right map[string][]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValues := range left {
		rightValues, ok := right[key]
		if !ok || len(leftValues) != len(rightValues) {
			return false
		}
		for i := range leftValues {
			if leftValues[i] != rightValues[i] {
				return false
			}
		}
	}
	return true
}
