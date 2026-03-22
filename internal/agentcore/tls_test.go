package agentcore

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildTLSConfig_Default(t *testing.T) {
	cfg := &RuntimeConfig{}
	tlsCfg := buildTLSConfig(cfg)
	if tlsCfg != nil {
		t.Fatalf("expected nil TLS config when no TLS settings, got %+v", tlsCfg)
	}
}

func TestBuildTLSConfig_Nil(t *testing.T) {
	tlsCfg := buildTLSConfig(nil)
	if tlsCfg != nil {
		t.Fatalf("expected nil TLS config for nil RuntimeConfig")
	}
}

func TestBuildTLSConfig_SkipVerify(t *testing.T) {
	cfg := &RuntimeConfig{TLSSkipVerify: true}
	tlsCfg := buildTLSConfig(cfg)
	if tlsCfg == nil {
		t.Fatalf("expected non-nil TLS config when SkipVerify=true")
	}
	if !tlsCfg.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify=true")
	}
}

func generateTestCACert(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestBuildTLSConfig_CAFile(t *testing.T) {
	tmpDir := t.TempDir()
	caFile := filepath.Join(tmpDir, "ca.crt")

	caPEM := generateTestCACert(t)
	if err := os.WriteFile(caFile, caPEM, 0600); err != nil {
		t.Fatalf("write CA file: %v", err)
	}

	cfg := &RuntimeConfig{TLSCAFile: caFile}
	tlsCfg := buildTLSConfig(cfg)
	if tlsCfg == nil {
		t.Fatalf("expected non-nil TLS config when CAFile is set")
	}
	if tlsCfg.RootCAs == nil {
		t.Fatalf("expected RootCAs to be populated")
	}
}

func TestBuildTLSConfig_CAFileMergesWithSystemPool(t *testing.T) {
	tmpDir := t.TempDir()
	caFile := filepath.Join(tmpDir, "ca.crt")

	caPEM := generateTestCACert(t)
	if err := os.WriteFile(caFile, caPEM, 0600); err != nil {
		t.Fatalf("write CA file: %v", err)
	}

	sentinelPool := x509.NewCertPool()
	basePEM := generateTestCACert(t)
	if !sentinelPool.AppendCertsFromPEM(basePEM) {
		t.Fatalf("append base cert")
	}

	originalLoadSystemCertPool := loadSystemCertPool
	t.Cleanup(func() {
		loadSystemCertPool = originalLoadSystemCertPool
	})
	baseSubjects := len(sentinelPool.Subjects())
	loadSystemCertPool = func() (*x509.CertPool, error) {
		return sentinelPool, nil
	}

	cfg := &RuntimeConfig{TLSCAFile: caFile}
	tlsCfg := buildTLSConfig(cfg)
	if tlsCfg == nil {
		t.Fatalf("expected non-nil TLS config when CAFile is set")
	}
	if tlsCfg.RootCAs == nil {
		t.Fatalf("expected RootCAs to be populated")
	}
	if got, wantMin := len(tlsCfg.RootCAs.Subjects()), baseSubjects+1; got < wantMin {
		t.Fatalf("expected merged pool to contain system subjects plus custom CA, got %d want at least %d", got, wantMin)
	}
}

func TestBuildTLSConfig_CAFileNotFound(t *testing.T) {
	cfg := &RuntimeConfig{TLSCAFile: "/nonexistent/ca.crt"}
	tlsCfg := buildTLSConfig(cfg)
	// Should return a config (non-nil) even if CA file fails to load
	if tlsCfg == nil {
		t.Fatalf("expected non-nil TLS config even when CA file is missing")
	}
	// RootCAs should be nil since file didn't load
	if tlsCfg.RootCAs != nil {
		t.Fatalf("expected RootCAs to be nil when CA file not found")
	}
}

func TestBuildTLSConfig_CAFileAndSkipVerify(t *testing.T) {
	cfg := &RuntimeConfig{
		TLSCAFile:     "/nonexistent/ca.crt",
		TLSSkipVerify: true,
	}
	tlsCfg := buildTLSConfig(cfg)
	if tlsCfg == nil {
		t.Fatalf("expected non-nil TLS config")
	}
	if !tlsCfg.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify=true")
	}
}
