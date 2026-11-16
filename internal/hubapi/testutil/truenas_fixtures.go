package testutil

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

type trueNASRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// TrueNASRPCError is the JSON-RPC error a NewTrueNASRPCServer handler returns.
type TrueNASRPCError struct {
	Code    int
	Message string
	Data    map[string]any
}

// IsExpectedTrueNASRPCTestDisconnect recognizes transport shutdown from the
// collector's best-effort subscription worker during test teardown. Primary
// RPC failures still fail their caller assertions, while malformed protocol
// traffic and non-transport errors remain visible here.
func IsExpectedTrueNASRPCTestDisconnect(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, websocket.ErrCloseSent) ||
		websocket.IsCloseError(
			err,
			websocket.CloseNormalClosure,
			websocket.CloseGoingAway,
			websocket.CloseAbnormalClosure,
		)
}

// NewTrueNASRPCServer is a fake TrueNAS JSON-RPC WebSocket API: it accepts
// the API key login, then answers one call with handler's result or error.
func NewTrueNASRPCServer(t *testing.T, handler func(method string, params []any) (any, *TrueNASRPCError)) *httptest.Server {
	t.Helper()
	AllowInsecureTransportForConnectorTests(t)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			if !IsExpectedTrueNASRPCTestDisconnect(err) {
				t.Errorf("upgrade failed: %v", err)
			}
			return
		}
		defer conn.Close()

		var authReq trueNASRPCRequest
		if err := conn.ReadJSON(&authReq); err != nil {
			if !IsExpectedTrueNASRPCTestDisconnect(err) {
				t.Errorf("read auth request: %v", err)
			}
			return
		}
		if authReq.Method != "auth.login_with_api_key" {
			t.Errorf("expected auth.login_with_api_key, got %s", authReq.Method)
			return
		}
		if err := conn.WriteJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      authReq.ID,
			"result":  true,
		}); err != nil {
			if !IsExpectedTrueNASRPCTestDisconnect(err) {
				t.Errorf("write auth response: %v", err)
			}
			return
		}

		var req trueNASRPCRequest
		if err := conn.ReadJSON(&req); err != nil {
			if !IsExpectedTrueNASRPCTestDisconnect(err) {
				t.Errorf("read rpc request: %v", err)
			}
			return
		}
		params := make([]any, 0, 4)
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				t.Errorf("decode request params: %v", err)
				return
			}
		}

		result, rpcErr := handler(req.Method, params)
		if rpcErr != nil {
			if err := conn.WriteJSON(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error": map[string]any{
					"code":    rpcErr.Code,
					"message": rpcErr.Message,
					"data":    rpcErr.Data,
				},
			}); err != nil {
				if !IsExpectedTrueNASRPCTestDisconnect(err) {
					t.Errorf("write rpc error response: %v", err)
				}
			}
			return
		}

		if err := conn.WriteJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		}); err != nil {
			if !IsExpectedTrueNASRPCTestDisconnect(err) {
				t.Errorf("write rpc result response: %v", err)
			}
		}
	}))

	return server
}

// CreateTrueNASCredentialProfile stores an encrypted TrueNAS API key
// profile, as the TrueNAS settings page does.
func CreateTrueNASCredentialProfile(t *testing.T, store persistence.CredentialStore, manager *secrets.Manager, credentialID, secret, baseURL string) {
	t.Helper()
	AllowInsecureTransportForConnectorTests(t)

	secretCiphertext, err := manager.EncryptString(secret, credentialID)
	if err != nil {
		t.Fatalf("failed to encrypt truenas credential: %v", err)
	}
	_, err = store.CreateCredentialProfile(credentials.Profile{
		ID:               credentialID,
		Name:             "truenas " + credentialID,
		Kind:             credentials.KindTrueNASAPIKey,
		Status:           "active",
		SecretCiphertext: secretCiphertext,
		Metadata:         map[string]string{"base_url": baseURL},
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to store truenas credential profile: %v", err)
	}
}

// SeedTrueNASAsset stores an online TrueNAS NAS asset owned by collectorID.
func SeedTrueNASAsset(t *testing.T, store persistence.AssetStore, assetID, collectorID string) {
	t.Helper()
	_, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID,
		Type:    "nas",
		Name:    "vaultnas",
		Source:  "truenas",
		Status:  "online",
		Metadata: map[string]string{
			"hostname":     "vaultnas",
			"collector_id": collectorID,
		},
	})
	if err != nil {
		t.Fatalf("failed to seed truenas asset: %v", err)
	}
}
