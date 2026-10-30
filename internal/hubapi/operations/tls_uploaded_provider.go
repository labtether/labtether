package operations

import (
	"crypto/tls"
	"crypto/x509"
)

// UploadedCertificateProvider wraps the provider for an uploaded certificate.
// For a TLS server name the uploaded certificate does not cover, it serves the
// Hub's built-in certificate when that one does. The bundled console reaches
// the Hub as https://labtether:8443 and checks that name against the built-in
// CA, so serving an uploaded certificate for an outside name there cut the
// console off from the Hub until the upload was removed.
func (s *TLSState) UploadedCertificateProvider(uploaded func(*tls.ClientHelloInfo) (*tls.Certificate, error)) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	var builtIn func(*tls.ClientHelloInfo) (*tls.Certificate, error)
	if s != nil && s.CertReloader != nil {
		builtIn = s.CertReloader.GetCertificate
	}
	return certificateForServerName(uploaded, builtIn)
}

func certificateForServerName(primary, fallback func(*tls.ClientHelloInfo) (*tls.Certificate, error)) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	if fallback == nil {
		return primary
	}
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		cert, err := primary(hello)
		if err != nil || hello == nil || hello.ServerName == "" || certificateCoversName(cert, hello.ServerName) {
			return cert, err
		}
		if other, otherErr := fallback(hello); otherErr == nil && certificateCoversName(other, hello.ServerName) {
			return other, nil
		}
		return cert, nil
	}
}

func certificateCoversName(cert *tls.Certificate, name string) bool {
	if cert == nil || len(cert.Certificate) == 0 {
		return false
	}
	leaf := cert.Leaf
	if leaf == nil {
		parsed, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return false
		}
		leaf = parsed
	}
	return leaf.VerifyHostname(name) == nil
}
