package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/certmgr"
)

func TestHandleCACert_ReturnsPEM(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.CACertPEM = []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ca.crt", nil)
	rec := httptest.NewRecorder()
	sut.handleCACert(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/x-pem-file" {
		t.Fatalf("expected Content-Type application/x-pem-file, got %q", ct)
	}

	cd := rec.Header().Get("Content-Disposition")
	if cd != `attachment; filename="labtether-ca.crt"` {
		t.Fatalf("expected Content-Disposition attachment header, got %q", cd)
	}

	body := rec.Body.String()
	if body != "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestHandleCACert_NoCert(t *testing.T) {
	sut := newTestAPIServer(t)
	// caCertPEM is nil by default

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ca.crt", nil)
	rec := httptest.NewRecorder()
	sut.handleCACert(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp["error"] == "" {
		t.Fatalf("expected error message in response")
	}
}

func TestHandleCACert_MethodNotAllowed(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.CACertPEM = []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ca.crt", nil)
	rec := httptest.NewRecorder()
	sut.handleCACert(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleTLSInfo(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	sut.tlsState.CACertPEM = []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tls/info", nil)
	rec := httptest.NewRecorder()
	sut.handleTLSInfo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode TLS info response: %v", err)
	}

	if resp["tls_enabled"] != true {
		t.Fatalf("expected tls_enabled=true, got %v", resp["tls_enabled"])
	}
	if resp["cert_type"] != "self-signed" {
		t.Fatalf("expected cert_type=self-signed, got %v", resp["cert_type"])
	}
	if resp["ca_available"] != true {
		t.Fatalf("expected ca_available=true, got %v", resp["ca_available"])
	}
}

func TestHandleTLSInfo_NoTLS(t *testing.T) {
	sut := newTestAPIServer(t)
	// tlsEnabled=false, caCertPEM=nil (defaults)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tls/info", nil)
	rec := httptest.NewRecorder()
	sut.handleTLSInfo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode TLS info response: %v", err)
	}

	if resp["tls_enabled"] != false {
		t.Fatalf("expected tls_enabled=false, got %v", resp["tls_enabled"])
	}
	if resp["cert_type"] != "none" {
		t.Fatalf("expected cert_type=none, got %v", resp["cert_type"])
	}
	if resp["ca_available"] != false {
		t.Fatalf("expected ca_available=false, got %v", resp["ca_available"])
	}
}

func TestHandleTLSInfo_ExternalCert(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	// caCertPEM is nil — external certs

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tls/info", nil)
	rec := httptest.NewRecorder()
	sut.handleTLSInfo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode TLS info response: %v", err)
	}

	if resp["tls_enabled"] != true {
		t.Fatalf("expected tls_enabled=true, got %v", resp["tls_enabled"])
	}
	if resp["cert_type"] != "external" {
		t.Fatalf("expected cert_type=external, got %v", resp["cert_type"])
	}
	if resp["ca_available"] != false {
		t.Fatalf("expected ca_available=false, got %v", resp["ca_available"])
	}
}

func TestHandleTLSInfo_WithRealCert(t *testing.T) {
	ca, err := certmgr.GenerateCA()
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	sut.tlsState.CACertPEM = certmgr.CertPEM(ca.Cert)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tls/info", nil)
	rec := httptest.NewRecorder()
	sut.handleTLSInfo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode TLS info response: %v", err)
	}

	fingerprint, ok := resp["ca_fingerprint_sha256"].(string)
	if !ok || len(fingerprint) != 64 {
		t.Fatalf("expected 64-char hex fingerprint, got %q", resp["ca_fingerprint_sha256"])
	}

	if resp["ca_expires"] == nil {
		t.Fatalf("expected ca_expires to be present")
	}
}

func TestHandleTLSInfo_MethodNotAllowed(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tls/info", nil)
	rec := httptest.NewRecorder()
	sut.handleTLSInfo(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleTLSInfo_IncludesPortFields(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	sut.tlsState.HttpsPort = 8443
	sut.tlsState.HttpPort = 8080
	sut.tlsState.CACertPEM = []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tls/info", nil)
	rec := httptest.NewRecorder()
	sut.handleTLSInfo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode TLS info response: %v", err)
	}

	httpsPort, ok := resp["https_port"].(float64)
	if !ok || int(httpsPort) != 8443 {
		t.Fatalf("expected https_port=8443, got %v", resp["https_port"])
	}

	httpPort, ok := resp["http_port"].(float64)
	if !ok || int(httpPort) != 8080 {
		t.Fatalf("expected http_port=8080, got %v", resp["http_port"])
	}
}

func TestHandleTLSInfo_DisabledIncludesPortFields(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.HttpPort = 8080

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tls/info", nil)
	rec := httptest.NewRecorder()
	sut.handleTLSInfo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode TLS info response: %v", err)
	}

	httpPort, ok := resp["http_port"].(float64)
	if !ok || int(httpPort) != 8080 {
		t.Fatalf("expected http_port=8080, got %v", resp["http_port"])
	}

	// https_port should be 0 when TLS is disabled
	httpsPort, _ := resp["https_port"].(float64)
	if int(httpsPort) != 0 {
		t.Fatalf("expected https_port=0 when TLS disabled, got %v", resp["https_port"])
	}
}
