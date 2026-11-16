package testutil

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

// StubHubCollectorStore is a read-only hub collector store over a fixed
// list. It is the exported copy of cmd/labtether's stubHubCollectorStore;
// cmd keeps its own for the tests that still build struct literals with the
// unexported field.
type StubHubCollectorStore struct {
	Collectors []hubcollector.Collector
}

func (s *StubHubCollectorStore) CreateHubCollector(hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *StubHubCollectorStore) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	for _, collector := range s.Collectors {
		if collector.ID == id {
			return collector, true, nil
		}
	}
	return hubcollector.Collector{}, false, nil
}

func (s *StubHubCollectorStore) ListHubCollectors(_ int, enabledOnly bool) ([]hubcollector.Collector, error) {
	result := make([]hubcollector.Collector, 0, len(s.Collectors))
	for _, collector := range s.Collectors {
		if enabledOnly && !collector.Enabled {
			continue
		}
		result = append(result, collector)
	}
	return result, nil
}

func (s *StubHubCollectorStore) UpdateHubCollector(string, hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *StubHubCollectorStore) DeleteHubCollector(string) error {
	return fmt.Errorf("not implemented")
}

func (s *StubHubCollectorStore) UpdateHubCollectorStatus(string, string, string, time.Time) error {
	return nil
}

// AllowInsecureTransportForConnectorTests lets connector clients reach
// plain-HTTP loopback test servers for the rest of the test.
func AllowInsecureTransportForConnectorTests(t *testing.T) {
	t.Helper()
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
}

// CreateProxmoxCredentialProfile stores an encrypted Proxmox API token
// profile, as the Proxmox settings page does.
func CreateProxmoxCredentialProfile(t *testing.T, store persistence.CredentialStore, manager *secrets.Manager, credentialID, username, secret, baseURL string) {
	t.Helper()
	AllowInsecureTransportForConnectorTests(t)

	secretCiphertext, err := manager.EncryptString(secret, credentialID)
	if err != nil {
		t.Fatalf("failed to encrypt credential %s: %v", credentialID, err)
	}
	_, err = store.CreateCredentialProfile(credentials.Profile{
		ID:               credentialID,
		Name:             "proxmox " + credentialID,
		Kind:             credentials.KindProxmoxAPIToken,
		Username:         username,
		Status:           "active",
		SecretCiphertext: secretCiphertext,
		Metadata:         map[string]string{"base_url": baseURL},
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to store credential profile %s: %v", credentialID, err)
	}
}

// NewWebSocketPair returns the server and client ends of one live WebSocket
// connection, and a function that closes both.
func NewWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
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
