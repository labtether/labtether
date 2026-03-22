package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labtether/labtether/internal/certmgr"
)

type testTLSKeyPair struct {
	CACertPEM []byte
	CertPEM   []byte
	KeyPEM    []byte
	CertPath  string
	KeyPath   string
}

func writeTestTLSKeyPair(t *testing.T, dnsName string) testTLSKeyPair {
	t.Helper()

	ca, err := certmgr.GenerateCA()
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}
	server, err := certmgr.GenerateServerCert(ca, []string{dnsName}, nil)
	if err != nil {
		t.Fatalf("generate server certificate: %v", err)
	}

	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")
	if err := certmgr.SaveKeyPair(server, certPath, keyPath); err != nil {
		t.Fatalf("save key pair: %v", err)
	}

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read cert PEM: %v", err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read key PEM: %v", err)
	}

	return testTLSKeyPair{
		CACertPEM: certmgr.CertPEM(ca.Cert),
		CertPEM:   certPEM,
		KeyPEM:    keyPEM,
		CertPath:  certPath,
		KeyPath:   keyPath,
	}
}

func TestHandleTLSSettingsUpdateAppliesUploadedCertificateLive(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.dataDir = t.TempDir()
	sut.tlsState.Enabled = true
	sut.tlsState.CertSwitcher = &hubCertificateSwitcher{}

	uploaded := writeTestTLSKeyPair(t, "upload.example.ts.net")
	body, err := json.Marshal(map[string]string{
		"cert_pem": string(uploaded.CertPEM),
		"key_pem":  string(uploaded.KeyPEM),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, tlsSettingsRoute, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	sut.handleTLSSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp tlsSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RestartRequired {
		t.Fatalf("expected live apply without restart")
	}
	if resp.TLSSource != tlsSourceUIUploaded {
		t.Fatalf("expected tls_source=%q, got %q", tlsSourceUIUploaded, resp.TLSSource)
	}
	if resp.CertType != "uploaded" {
		t.Fatalf("expected cert_type=uploaded, got %q", resp.CertType)
	}
	if !resp.UploadedOverridePresent {
		t.Fatalf("expected uploaded override to be present")
	}
	if resp.ActiveCertificate.SubjectSummary == "" {
		t.Fatalf("expected active certificate metadata in response")
	}

	if sut.tlsState.Source != tlsSourceUIUploaded {
		t.Fatalf("expected server tls source to switch to uploaded, got %q", sut.tlsState.Source)
	}
	if sut.tlsState.CACertPEM != nil {
		t.Fatalf("expected built-in CA PEM to be cleared after live upload")
	}

	certPath := filepath.Join(sut.dataDir, tlsUploadedCertRelativePath)
	keyPath := filepath.Join(sut.dataDir, tlsUploadedKeyRelativePath)
	if _, err := os.Stat(certPath); err != nil {
		t.Fatalf("expected uploaded cert file to exist: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("expected uploaded key file to exist: %v", err)
	}

	values, err := sut.runtimeStore.ListRuntimeSettingOverrides()
	if err != nil {
		t.Fatalf("list runtime overrides: %v", err)
	}
	if values[tlsOverrideCertPEMKey] != string(bytes.TrimSpace(uploaded.CertPEM)) {
		t.Fatalf("expected uploaded cert PEM to be persisted")
	}
	if values[tlsOverrideKeyCipherKey] == "" {
		t.Fatalf("expected encrypted key to be persisted")
	}
	if values[tlsOverrideKeyCipherKey] == string(bytes.TrimSpace(uploaded.KeyPEM)) {
		t.Fatalf("expected stored key to be encrypted, not plaintext")
	}
	decryptedKey, err := sut.secretsManager.DecryptString(values[tlsOverrideKeyCipherKey], tlsOverrideAAD)
	if err != nil {
		t.Fatalf("decrypt persisted key: %v", err)
	}
	if decryptedKey != string(bytes.TrimSpace(uploaded.KeyPEM)) {
		t.Fatalf("expected decrypted key PEM to match uploaded key")
	}
}

func TestHandleTLSSettingsClearRestoresStartupTLSProviderLive(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.dataDir = t.TempDir()
	sut.tlsState.Enabled = true

	defaultPair := writeTestTLSKeyPair(t, "default.local")
	defaultProvider, err := newStaticHubCertificateProvider(defaultPair.CertPath, defaultPair.KeyPath)
	if err != nil {
		t.Fatalf("load default provider: %v", err)
	}

	sut.tlsState.Mode = "auto"
	sut.tlsState.Source = tlsSourceBuiltIn
	sut.tlsState.CertFile = defaultPair.CertPath
	sut.tlsState.KeyFile = defaultPair.KeyPath
	sut.tlsState.CACertPEM = append([]byte(nil), defaultPair.CACertPEM...)
	sut.tlsState.CertSwitcher = &hubCertificateSwitcher{}
	sut.tlsState.CertSwitcher.SetProvider(defaultProvider.GetCertificate)
	sut.tlsState.DefaultMode = "auto"
	sut.tlsState.DefaultSource = tlsSourceBuiltIn
	sut.tlsState.DefaultCertFile = defaultPair.CertPath
	sut.tlsState.DefaultKeyFile = defaultPair.KeyPath
	sut.tlsState.DefaultCAPEM = append([]byte(nil), defaultPair.CACertPEM...)
	sut.tlsState.DefaultGetCertificate = defaultProvider.GetCertificate

	uploaded := writeTestTLSKeyPair(t, "upload.example.ts.net")
	updateBody, err := json.Marshal(map[string]string{
		"cert_pem": string(uploaded.CertPEM),
		"key_pem":  string(uploaded.KeyPEM),
	})
	if err != nil {
		t.Fatalf("marshal update request: %v", err)
	}

	updateReq := httptest.NewRequest(http.MethodPost, tlsSettingsRoute, bytes.NewReader(updateBody))
	updateRec := httptest.NewRecorder()
	sut.handleTLSSettings(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected upload to succeed, got %d: %s", updateRec.Code, updateRec.Body.String())
	}

	clearReq := httptest.NewRequest(http.MethodDelete, tlsSettingsRoute, nil)
	clearRec := httptest.NewRecorder()
	sut.handleTLSSettings(clearRec, clearReq)
	if clearRec.Code != http.StatusOK {
		t.Fatalf("expected clear to succeed, got %d: %s", clearRec.Code, clearRec.Body.String())
	}

	var resp tlsSettingsResponse
	if err := json.Unmarshal(clearRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode clear response: %v", err)
	}
	if resp.RestartRequired {
		t.Fatalf("expected live restore without restart")
	}
	if resp.TLSSource != tlsSourceBuiltIn {
		t.Fatalf("expected tls_source=%q, got %q", tlsSourceBuiltIn, resp.TLSSource)
	}
	if resp.CertType != "self-signed" {
		t.Fatalf("expected cert_type=self-signed after restore, got %q", resp.CertType)
	}

	if sut.tlsState.Source != tlsSourceBuiltIn {
		t.Fatalf("expected server tls source restored to %q, got %q", tlsSourceBuiltIn, sut.tlsState.Source)
	}
	if sut.tlsState.CertFile != defaultPair.CertPath {
		t.Fatalf("expected active cert file restored to default path, got %q", sut.tlsState.CertFile)
	}
	if !bytes.Equal(sut.tlsState.CACertPEM, defaultPair.CACertPEM) {
		t.Fatalf("expected built-in CA PEM to be restored")
	}
	if _, err := sut.tlsState.CertSwitcher.GetCertificate(nil); err != nil {
		t.Fatalf("expected restored TLS provider to serve a certificate: %v", err)
	}

	values, err := sut.runtimeStore.ListRuntimeSettingOverrides()
	if err != nil {
		t.Fatalf("list runtime overrides: %v", err)
	}
	if values[tlsOverrideCertPEMKey] != "" || values[tlsOverrideKeyCipherKey] != "" || values[tlsOverrideUpdatedAtKey] != "" {
		t.Fatalf("expected uploaded TLS override settings to be removed, got %#v", values)
	}
}
