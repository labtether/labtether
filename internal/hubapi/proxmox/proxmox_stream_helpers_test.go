package proxmox_test

import (
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"github.com/labtether/labtether/internal/hubcollector"
	"math/big"
	"testing"
	"time"
)

type listErrorHubCollectorStore struct {
	testutil.StubHubCollectorStore
	listErr error
}

func (s *listErrorHubCollectorStore) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.StubHubCollectorStore.ListHubCollectors(limit, enabledOnly)
}

func TestTranslateBrowserToProxmoxTerm(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		want    string
		nilOut  bool
	}{
		{
			name:    "resize",
			payload: []byte(`{"type":"resize","cols":120,"rows":40}`),
			want:    "1:120:40:",
		},
		{
			name:    "ping",
			payload: []byte(`{"type":"ping"}`),
			want:    "2",
		},
		{
			name:    "input",
			payload: []byte(`{"type":"input","data":"ls\n"}`),
			want:    "0:3:ls\n",
		},
		{
			name:    "resize-invalid",
			payload: []byte(`{"type":"resize","cols":0,"rows":40}`),
			nilOut:  true,
		},
		{
			name:    "input-empty",
			payload: []byte(`{"type":"input","data":""}`),
			nilOut:  true,
		},
		{
			name:    "unknown-control",
			payload: []byte(`{"type":"unknown"}`),
			nilOut:  true,
		},
		{
			name:    "raw",
			payload: []byte("pwd\n"),
			want:    "0:4:pwd\n",
		},
		{
			name:    "empty",
			payload: []byte(""),
			nilOut:  true,
		},
	}

	for _, tc := range cases {
		got := proxmoxpkg.TranslateBrowserToProxmoxTerm(tc.payload)
		if tc.nilOut {
			if got != nil {
				t.Fatalf("%s: expected nil output, got %q", tc.name, string(got))
			}
			continue
		}
		if string(got) != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, string(got))
		}
	}
}

func TestProxmoxVNCUtilityFunctions(t *testing.T) {
	if got := proxmoxpkg.VNCReverseBits(0x12); got != 0x48 {
		t.Fatalf("expected bit reverse 0x12 -> 0x48, got %#x", got)
	}
	key := proxmoxpkg.VNCDESKey("password")
	if key[0] != proxmoxpkg.VNCReverseBits('p') {
		t.Fatalf("unexpected DES key first byte: %#x", key[0])
	}
	response := proxmoxpkg.VNCEncryptChallenge([]byte("0123456789ABCDEF"), "password")
	if len(response) != 16 {
		t.Fatalf("expected 16-byte encrypted challenge, got %d", len(response))
	}
	second := proxmoxpkg.VNCEncryptChallenge([]byte("0123456789ABCDEF"), "password")
	if string(response) != string(second) {
		t.Fatalf("expected deterministic encryption output")
	}

	withProxmoxStreamHooks(
		t,
		nil,
		nil,
		nil,
		func([]byte) (cipher.Block, error) {
			return nil, errors.New("forced cipher construction failure")
		},
	)
	fallback := proxmoxpkg.VNCEncryptChallenge([]byte("0123456789ABCDEF"), "password")
	if len(fallback) != 16 {
		t.Fatalf("expected 16-byte fallback challenge, got %d", len(fallback))
	}
	for i, b := range fallback {
		if b != 0 {
			t.Fatalf("expected zeroed fallback response byte at %d, got %d", i, b)
		}
	}
}

func configureProxmoxStreamCollector(t *testing.T, sut *hubdeps.Hub, baseURL, collectorID, credentialID string) {
	t.Helper()

	createProxmoxCredentialProfile(
		t,
		sut,
		credentialID,
		"labtether@pve!stream",
		"stream-secret",
		baseURL,
	)

	sut.HubCollectorStore = &testutil.StubHubCollectorStore{
		Collectors: []hubcollector.Collector{
			{
				ID:            collectorID,
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      baseURL,
					"token_id":      "labtether@pve!stream",
					"credential_id": credentialID,
					"skip_verify":   true,
				},
			},
		},
	}
}

func testCAPEM(t *testing.T) string {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("failed to generate test CA key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "LabTether Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("failed to create test CA certificate: %v", err)
	}

	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	}))
}
