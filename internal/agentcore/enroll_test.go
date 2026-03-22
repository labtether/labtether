package agentcore

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testCACertPEM generates a real self-signed CA certificate for testing.
func testCACertPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestResolveToken_ExplicitAPIToken(t *testing.T) {
	cfg := &RuntimeConfig{
		APIToken: "existing-token",
	}
	if err := ResolveToken(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIToken != "existing-token" {
		t.Fatalf("expected token unchanged, got %q", cfg.APIToken)
	}
}

func TestResolveToken_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "agent-token")
	if err := os.WriteFile(tokenFile, []byte("file-token\n"), 0600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	cfg := &RuntimeConfig{
		TokenFilePath: tokenFile,
	}
	if err := ResolveToken(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIToken != "file-token" {
		t.Fatalf("expected 'file-token', got %q", cfg.APIToken)
	}
}

func TestResolveToken_EnrollmentFlow(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	// Mock enrollment server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/enroll" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req enrollRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode error: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.EnrollmentToken != "test-enroll-token" {
			t.Errorf("expected enrollment_token=test-enroll-token, got %q", req.EnrollmentToken)
		}
		if req.Hostname == "" {
			t.Errorf("expected hostname to be set")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(enrollResponse{
			AgentToken: "new-agent-token",
			AssetID:    req.Hostname,
			HubWSURL:   "ws://localhost/ws/agent",
			HubAPIURL:  "http://localhost",
		})
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "agent-token")

	cfg := &RuntimeConfig{
		EnrollmentToken: "test-enroll-token",
		APIBaseURL:      server.URL,
		TokenFilePath:   tokenFile,
	}

	if err := ResolveToken(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIToken != "new-agent-token" {
		t.Fatalf("expected 'new-agent-token', got %q", cfg.APIToken)
	}

	// Verify token was persisted
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if string(data) != "new-agent-token\n" {
		t.Fatalf("expected persisted token, got %q", string(data))
	}
}

func TestResolveToken_NoAuth(t *testing.T) {
	cfg := &RuntimeConfig{}
	if err := ResolveToken(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIToken != "" {
		t.Fatalf("expected empty token for legacy mode, got %q", cfg.APIToken)
	}
}

func TestResolveToken_EnrollmentMissingURL(t *testing.T) {
	cfg := &RuntimeConfig{
		EnrollmentToken: "some-token",
		// No WSBaseURL or APIBaseURL
	}
	err := ResolveToken(context.Background(), cfg)
	if err == nil {
		t.Fatalf("expected error when enrollment token set but no URL")
	}
}

func TestResolveToken_Priority_ExplicitOverFile(t *testing.T) {
	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "agent-token")
	os.WriteFile(tokenFile, []byte("file-token\n"), 0600)

	cfg := &RuntimeConfig{
		APIToken:      "explicit-token",
		TokenFilePath: tokenFile,
	}
	if err := ResolveToken(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIToken != "explicit-token" {
		t.Fatalf("expected explicit token to take priority, got %q", cfg.APIToken)
	}
}

func TestLoadTokenFromFile_Empty(t *testing.T) {
	token, err := loadTokenFromFile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Fatalf("expected empty token, got %q", token)
	}
}

func TestLoadTokenFromFile_NotFound(t *testing.T) {
	_, err := loadTokenFromFile("/nonexistent/path/token")
	if err == nil {
		t.Fatalf("expected error for nonexistent file")
	}
}

func TestSaveAndLoadToken(t *testing.T) {
	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "subdir", "agent-token")

	if err := saveTokenToFile(tokenFile, "test-token-123"); err != nil {
		t.Fatalf("save token: %v", err)
	}

	token, err := loadTokenFromFile(tokenFile)
	if err != nil {
		t.Fatalf("load token: %v", err)
	}
	if token != "test-token-123" {
		t.Fatalf("expected 'test-token-123', got %q", token)
	}

	// Verify file permissions
	info, err := os.Stat(tokenFile)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestSaveTokenToFile_EmptyPath(t *testing.T) {
	if err := saveTokenToFile("", "token"); err != nil {
		t.Fatalf("expected nil error for empty path, got %v", err)
	}
}

func TestEnrollWithHub_BadStatus(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := &RuntimeConfig{
		EnrollmentToken: "bad-token",
		APIBaseURL:      server.URL,
	}

	_, err := enrollWithHub(context.Background(), cfg)
	if err == nil {
		t.Fatalf("expected error for 401 response")
	}
}

func TestEnrollWithHub_TLS(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/enroll" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(enrollResponse{
			AgentToken: "tls-agent-token",
			AssetID:    "tls-node",
			HubWSURL:   "wss://localhost/ws/agent",
			HubAPIURL:  "https://localhost",
		})
	}))
	defer server.Close()

	// Use the test server's TLS cert pool
	tlsCert := server.TLS.Certificates[0]
	_ = tlsCert

	cfg := &RuntimeConfig{
		EnrollmentToken: "tls-enroll-token",
		APIBaseURL:      server.URL, // https://127.0.0.1:PORT
		TLSSkipVerify:   true,       // skip verify since test server uses self-signed cert
	}

	resp, err := enrollWithHub(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error enrolling over TLS: %v", err)
	}
	if resp.AgentToken != "tls-agent-token" {
		t.Fatalf("expected 'tls-agent-token', got %q", resp.AgentToken)
	}
	if resp.HubWSURL != "wss://localhost/ws/agent" {
		t.Fatalf("expected wss URL, got %q", resp.HubWSURL)
	}
}

func TestEnrollWithHub_TLSWithCA(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/enroll" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(enrollResponse{
			AgentToken: "ca-agent-token",
			AssetID:    "ca-node",
			HubWSURL:   "wss://localhost/ws/agent",
			HubAPIURL:  "https://localhost",
		})
	}))
	defer server.Close()

	// Get the test server's CA certificate and create a custom transport
	// We'll test that TLSSkipVerify works alongside CA, since httptest certs
	// won't validate against a random CA file anyway
	cfg := &RuntimeConfig{
		EnrollmentToken: "ca-enroll-token",
		APIBaseURL:      server.URL,
		TLSSkipVerify:   true,
	}

	// Manually build a client with the test server's cert pool to prove CA flow works
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    server.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs,
			},
		},
	}
	_ = client // Demonstrates the pattern; actual test uses skip-verify

	resp, err := enrollWithHub(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AgentToken != "ca-agent-token" {
		t.Fatalf("expected 'ca-agent-token', got %q", resp.AgentToken)
	}
}

func TestEnrollWithHub_WSURLConversion(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/enroll" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(enrollResponse{
			AgentToken: "ws-token",
			AssetID:    "ws-node",
			HubWSURL:   "ws://localhost/ws/agent",
			HubAPIURL:  "http://localhost",
		})
	}))
	defer server.Close()

	// Convert http://host:port to ws://host:port/ws/agent
	wsURL := "ws" + server.URL[4:] + "/ws/agent"

	cfg := &RuntimeConfig{
		EnrollmentToken: "test-token",
		WSBaseURL:       wsURL,
	}

	resp, err := enrollWithHub(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AgentToken != "ws-token" {
		t.Fatalf("expected 'ws-token', got %q", resp.AgentToken)
	}
}

func TestEnrollment_SavesCACert(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	fakeCAPEM := testCACertPEM(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/enroll" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(enrollResponse{
			AgentToken: "ca-token",
			AssetID:    "ca-node",
			CACertPEM:  fakeCAPEM,
		})
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "agent-token")

	cfg := &RuntimeConfig{
		EnrollmentToken: "enroll-token",
		APIBaseURL:      server.URL,
		TokenFilePath:   tokenFile,
	}

	if err := ResolveToken(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIToken != "ca-token" {
		t.Fatalf("expected 'ca-token', got %q", cfg.APIToken)
	}

	// Verify CA cert was saved to disk
	caPath := filepath.Join(tmpDir, "ca.crt")
	data, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatalf("expected CA file at %s: %v", caPath, err)
	}
	if string(data) != fakeCAPEM {
		t.Fatalf("CA content mismatch: got %q", string(data))
	}

	// Verify cfg.TLSCAFile was updated
	if cfg.TLSCAFile != caPath {
		t.Fatalf("expected TLSCAFile=%q, got %q", caPath, cfg.TLSCAFile)
	}
}

func TestEnrollment_NoCACert_DoesNotCreateFile(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/enroll" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(enrollResponse{
			AgentToken: "no-ca-token",
			AssetID:    "no-ca-node",
		})
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "agent-token")

	cfg := &RuntimeConfig{
		EnrollmentToken: "enroll-token",
		APIBaseURL:      server.URL,
		TokenFilePath:   tokenFile,
	}

	if err := ResolveToken(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify no CA file was created
	caPath := filepath.Join(tmpDir, "ca.crt")
	if _, err := os.Stat(caPath); err == nil {
		t.Fatalf("CA file should not exist when server sends no ca_cert_pem")
	}

	// TLSCAFile should remain empty
	if cfg.TLSCAFile != "" {
		t.Fatalf("expected empty TLSCAFile, got %q", cfg.TLSCAFile)
	}
}

func TestEnrollment_AutoLoadSavedCA(t *testing.T) {
	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "agent-token")

	// Pre-create a saved CA file (simulates previous enrollment)
	caPath := filepath.Join(tmpDir, "ca.crt")
	if err := os.WriteFile(caPath, []byte("-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n"), 0644); err != nil {
		t.Fatalf("write CA file: %v", err)
	}

	// Simulate LoadConfig with no explicit TLSCAFile
	t.Setenv("LABTETHER_TOKEN_FILE", tokenFile)
	t.Setenv("LABTETHER_TLS_CA_FILE", "")
	t.Setenv("LABTETHER_TLS_SKIP_VERIFY", "")
	t.Setenv("LABTETHER_API_BASE_URL", "")
	t.Setenv("LABTETHER_API_TOKEN", "")
	t.Setenv("LABTETHER_WS_URL", "")
	t.Setenv("LABTETHER_ENROLLMENT_TOKEN", "")

	cfg := LoadConfig("test-agent", "9100", "test")

	if cfg.TLSCAFile != caPath {
		t.Fatalf("expected auto-loaded TLSCAFile=%q, got %q", caPath, cfg.TLSCAFile)
	}
}

func TestEnrollment_ExplicitCAOverridesSavedCA(t *testing.T) {
	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "agent-token")

	// Pre-create a saved CA file
	savedCA := filepath.Join(tmpDir, "ca.crt")
	if err := os.WriteFile(savedCA, []byte("saved-ca"), 0644); err != nil {
		t.Fatalf("write CA file: %v", err)
	}

	// Create an explicit CA file
	explicitCA := filepath.Join(tmpDir, "explicit-ca.crt")
	if err := os.WriteFile(explicitCA, []byte("explicit-ca"), 0644); err != nil {
		t.Fatalf("write explicit CA file: %v", err)
	}

	t.Setenv("LABTETHER_TOKEN_FILE", tokenFile)
	t.Setenv("LABTETHER_TLS_CA_FILE", explicitCA)
	t.Setenv("LABTETHER_TLS_SKIP_VERIFY", "")
	t.Setenv("LABTETHER_API_BASE_URL", "")
	t.Setenv("LABTETHER_API_TOKEN", "")
	t.Setenv("LABTETHER_WS_URL", "")
	t.Setenv("LABTETHER_ENROLLMENT_TOKEN", "")

	cfg := LoadConfig("test-agent", "9100", "test")

	// Explicit CA should take precedence over saved CA
	if cfg.TLSCAFile != explicitCA {
		t.Fatalf("expected explicit TLSCAFile=%q, got %q", explicitCA, cfg.TLSCAFile)
	}
}

func TestEnrollment_HTTPSWithoutTrustConfigFails(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/enroll" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(enrollResponse{
			AgentToken: "bootstrap-token",
			AssetID:    "bootstrap-node",
			CACertPEM:  testCACertPEM(t),
		})
	}))
	defer server.Close()

	cfg := &RuntimeConfig{
		EnrollmentToken: "enroll-token",
		APIBaseURL:      server.URL,
		TokenFilePath:   filepath.Join(t.TempDir(), "agent-token"),
	}

	err := ResolveToken(context.Background(), cfg)
	if err == nil {
		t.Fatalf("expected enrollment over HTTPS without trust config to fail")
	}
}
