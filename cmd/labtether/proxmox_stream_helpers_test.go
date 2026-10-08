package main

import (
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"github.com/gorilla/websocket"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubcollector"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type listErrorHubCollectorStore struct {
	stubHubCollectorStore
	listErr error
}

func (s *listErrorHubCollectorStore) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.stubHubCollectorStore.ListHubCollectors(limit, enabledOnly)
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

func configureProxmoxStreamCollector(t *testing.T, sut *apiServer, baseURL, collectorID, credentialID string) {
	t.Helper()

	createProxmoxCredentialProfile(
		t,
		sut,
		credentialID,
		"labtether@pve!stream",
		"stream-secret",
		baseURL,
	)

	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
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

func withProxmoxStreamHooks(
	t *testing.T,
	write func(*websocket.Conn, int, []byte) error,
	read func(*websocket.Conn) (int, []byte, error),
	setReadDeadline func(*websocket.Conn, time.Time) error,
	newCipher func([]byte) (cipher.Block, error),
) {
	t.Helper()

	proxmoxpkg.ProxmoxStreamHooksMu.Lock()
	originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
	originalRead := proxmoxpkg.ProxmoxWSReadMessage
	originalSetReadDeadline := proxmoxpkg.ProxmoxWSSetReadDeadline
	originalNewCipher := proxmoxpkg.ProxmoxDESNewCipher

	if write != nil {
		proxmoxpkg.ProxmoxWSWriteMessage = write
	}
	if read != nil {
		proxmoxpkg.ProxmoxWSReadMessage = read
	}
	if setReadDeadline != nil {
		proxmoxpkg.ProxmoxWSSetReadDeadline = setReadDeadline
	}
	if newCipher != nil {
		proxmoxpkg.ProxmoxDESNewCipher = newCipher
	}
	proxmoxpkg.ProxmoxStreamHooksMu.Unlock()

	t.Cleanup(func() {
		proxmoxpkg.ProxmoxStreamHooksMu.Lock()
		proxmoxpkg.ProxmoxWSWriteMessage = originalWrite
		proxmoxpkg.ProxmoxWSReadMessage = originalRead
		proxmoxpkg.ProxmoxWSSetReadDeadline = originalSetReadDeadline
		proxmoxpkg.ProxmoxDESNewCipher = originalNewCipher
		proxmoxpkg.ProxmoxStreamHooksMu.Unlock()
	})
}

func setProxmoxTerminalKeepaliveIntervalForTest(t *testing.T, interval time.Duration) {
	t.Helper()

	proxmoxpkg.ProxmoxStreamHooksMu.Lock()
	original := proxmoxpkg.ProxmoxTerminalKeepaliveInterval
	proxmoxpkg.ProxmoxTerminalKeepaliveInterval = interval
	proxmoxpkg.ProxmoxStreamHooksMu.Unlock()

	t.Cleanup(func() {
		proxmoxpkg.ProxmoxStreamHooksMu.Lock()
		proxmoxpkg.ProxmoxTerminalKeepaliveInterval = original
		proxmoxpkg.ProxmoxStreamHooksMu.Unlock()
	})
}

func newWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	serverConnCh := make(chan *websocket.Conn, 1)
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("websocket upgrade failed: %v", err)
		}
		serverConnCh <- conn
		<-done
		_ = conn.Close()
	}))

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		close(done)
		server.Close()
		t.Fatalf("websocket dial failed: %v", err)
	}

	serverConn := <-serverConnCh
	cleanup := func() {
		_ = clientConn.Close()
		close(done)
		server.Close()
	}
	return serverConn, clientConn, cleanup
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
