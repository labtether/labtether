package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
)

type trueNASRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type trueNASRPCError struct {
	Code    int
	Message string
}

func newTrueNASRPCServer(t *testing.T, handler func(method string, params []any) (any, *trueNASRPCError)) *httptest.Server {
	t.Helper()
	allowInsecureTransportForConnectorTests(t)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()

		var authReq trueNASRPCRequest
		if err := conn.ReadJSON(&authReq); err != nil {
			t.Fatalf("read auth request: %v", err)
		}
		if authReq.Method != "auth.login_with_api_key" {
			t.Fatalf("expected auth.login_with_api_key, got %s", authReq.Method)
		}
		if err := conn.WriteJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      authReq.ID,
			"result":  true,
		}); err != nil {
			t.Fatalf("write auth response: %v", err)
		}

		var req trueNASRPCRequest
		if err := conn.ReadJSON(&req); err != nil {
			t.Fatalf("read rpc request: %v", err)
		}
		params := make([]any, 0, 4)
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				t.Fatalf("decode request params: %v", err)
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
				},
			}); err != nil {
				t.Fatalf("write rpc error response: %v", err)
			}
			return
		}

		if err := conn.WriteJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		}); err != nil {
			t.Fatalf("write rpc result response: %v", err)
		}
	}))

	return server
}

func createTrueNASCredentialProfile(t *testing.T, sut *apiServer, credentialID, secret, baseURL string) {
	t.Helper()
	allowInsecureTransportForConnectorTests(t)

	secretCiphertext, err := sut.secretsManager.EncryptString(secret, credentialID)
	if err != nil {
		t.Fatalf("failed to encrypt truenas credential: %v", err)
	}
	_, err = sut.credentialStore.CreateCredentialProfile(credentials.Profile{
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

func configureTrueNASCollectors(t *testing.T, sut *apiServer, collectors ...hubcollector.Collector) {
	t.Helper()
	sut.hubCollectorStore = &stubHubCollectorStore{collectors: collectors}
}

func seedTrueNASAsset(t *testing.T, sut *apiServer, assetID, collectorID string) {
	t.Helper()
	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID,
		Type:    "nas",
		Name:    "omeganas",
		Source:  "truenas",
		Status:  "online",
		Metadata: map[string]string{
			"hostname":     "omeganas",
			"collector_id": collectorID,
		},
	})
	if err != nil {
		t.Fatalf("failed to seed truenas asset: %v", err)
	}
}

func TestHandleTrueNASAssetsGuards(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/truenas/assets/", nil)
	rec := httptest.NewRecorder()
	sut.handleTrueNASAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/truenas/assets/a/smart", nil)
	rec = httptest.NewRecorder()
	sut.handleTrueNASAssets(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for non-GET method, got %d", rec.Code)
	}
}

func TestHandleTrueNASAssetSMARTSuccess(t *testing.T) {
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "disk.query":
			return []map[string]any{
				{"name": "sda", "serial": "A1", "model": "NVMe", "type": "SSD", "size": 500_000_000_000, "smart_enabled": true, "smart_status": "OK"},
				{"name": "sdb", "serial": "B2", "model": "NVMe", "type": "SSD", "size": 500_000_000_000, "smart_enabled": true},
			}, nil
		case "disk.temperatures":
			return map[string]any{"sda": 38, "sdb": 62}, nil
		case "smart.test.results":
			return []map[string]any{
				{"disk": "sda", "type": "SHORT", "status": "SUCCESS", "created_at": "2026-02-23T00:00:00Z"},
				{"disk": "sdb", "type": "LONG", "status": "FAILED", "created_at": "2026-02-23T01:00:00Z"},
			}, nil
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	createTrueNASCredentialProfile(t, sut, "cred-truenas-1", "api-key-1", server.URL)
	configureTrueNASCollectors(t, sut, hubcollector.Collector{
		ID:            "collector-truenas-1",
		AssetID:       "truenas-cluster-1",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-1",
			"skip_verify":   true,
		},
	})
	seedTrueNASAsset(t, sut, "truenas-host-omeganas", "collector-truenas-1")

	req := httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-omeganas/smart", nil)
	rec := httptest.NewRecorder()
	sut.handleTrueNASAssets(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"critical":1`) {
		t.Fatalf("expected one critical disk in summary, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"sdb"`) {
		t.Fatalf("expected smart payload to include sdb disk, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"critical"`) {
		t.Fatalf("expected critical disk status in payload, got %s", rec.Body.String())
	}
}

func TestHandleTrueNASAssetSMARTUsesCollectorMetadata(t *testing.T) {
	var collectorOneCalls atomic.Int32

	collectorOne := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		collectorOneCalls.Add(1)
		return nil, &trueNASRPCError{Code: -32000, Message: "wrong collector"}
	})
	defer collectorOne.Close()

	collectorTwo := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "disk.query":
			return []map[string]any{{"name": "sda", "size": 1000, "smart_enabled": true}}, nil
		case "disk.temperatures":
			return map[string]any{"sda": 35}, nil
		case "smart.test.results":
			return []map[string]any{}, nil
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer collectorTwo.Close()

	sut := newTestAPIServer(t)
	createTrueNASCredentialProfile(t, sut, "cred-truenas-1", "api-key-1", collectorOne.URL)
	createTrueNASCredentialProfile(t, sut, "cred-truenas-2", "api-key-2", collectorTwo.URL)
	configureTrueNASCollectors(t, sut,
		hubcollector.Collector{
			ID:            "collector-truenas-1",
			AssetID:       "truenas-cluster-1",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config:        map[string]any{"base_url": collectorOne.URL, "credential_id": "cred-truenas-1", "skip_verify": true},
		},
		hubcollector.Collector{
			ID:            "collector-truenas-2",
			AssetID:       "truenas-cluster-2",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config:        map[string]any{"base_url": collectorTwo.URL, "credential_id": "cred-truenas-2", "skip_verify": true},
		},
	)
	seedTrueNASAsset(t, sut, "truenas-host-omeganas", "collector-truenas-2")

	req := httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-omeganas/smart", nil)
	rec := httptest.NewRecorder()
	sut.handleTrueNASAssets(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if collectorOneCalls.Load() != 0 {
		t.Fatalf("expected collector one to receive no calls, got %d", collectorOneCalls.Load())
	}
	if !strings.Contains(rec.Body.String(), `"collector_id":"collector-truenas-2"`) {
		t.Fatalf("expected smart payload to use collector-truenas-2, got %s", rec.Body.String())
	}
}

func TestHandleTrueNASAssetFilesystemSuccess(t *testing.T) {
	server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
		switch method {
		case "filesystem.listdir":
			if len(params) == 0 || collectorAnyString(params[0]) != "/mnt" {
				t.Fatalf("expected filesystem.listdir first param to be /mnt, got %#v", params)
			}
			return []map[string]any{
				{"name": "photos", "path": "/mnt/photos", "type": "DIRECTORY", "size": 0},
				{"name": "notes.txt", "path": "/mnt/notes.txt", "type": "FILE", "size": 1024},
			}, nil
		default:
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	sut := newTestAPIServer(t)
	createTrueNASCredentialProfile(t, sut, "cred-truenas-1", "api-key-1", server.URL)
	configureTrueNASCollectors(t, sut, hubcollector.Collector{
		ID:            "collector-truenas-1",
		AssetID:       "truenas-cluster-1",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config: map[string]any{
			"base_url":      server.URL,
			"credential_id": "cred-truenas-1",
			"skip_verify":   true,
		},
	})
	seedTrueNASAsset(t, sut, "truenas-host-omeganas", "collector-truenas-1")

	req := httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-omeganas/filesystem?path=/mnt", nil)
	rec := httptest.NewRecorder()
	sut.handleTrueNASAssets(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"path":"/mnt"`) {
		t.Fatalf("expected filesystem response path to be /mnt, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"photos"`) || !strings.Contains(rec.Body.String(), `"is_directory":true`) {
		t.Fatalf("expected directory entry in payload, got %s", rec.Body.String())
	}
}

func TestHandleTrueNASAssetEventsFiltersByAsset(t *testing.T) {
	sut := newTestAPIServer(t)
	seedTrueNASAsset(t, sut, "truenas-host-omeganas", "collector-truenas-1")

	now := time.Now().UTC()
	_ = sut.logStore.AppendEvent(logs.Event{
		ID:        "event-1",
		AssetID:   "truenas-host-omeganas",
		Source:    "truenas",
		Level:     "info",
		Message:   "pool healthy",
		Timestamp: now,
	})
	_ = sut.logStore.AppendEvent(logs.Event{
		ID:        "event-2",
		AssetID:   "truenas-host-omeganas",
		Source:    "docker",
		Level:     "info",
		Message:   "not truenas",
		Timestamp: now,
	})
	_ = sut.logStore.AppendEvent(logs.Event{
		ID:        "event-3",
		AssetID:   "truenas-host-backup",
		Source:    "truenas",
		Level:     "info",
		Message:   "other asset",
		Timestamp: now,
	})

	req := httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-omeganas/events?limit=50&window=6h", nil)
	rec := httptest.NewRecorder()
	sut.handleTrueNASAssets(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"event-1"`) {
		t.Fatalf("expected event-1 in response, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"event-2"`) || strings.Contains(rec.Body.String(), `"event-3"`) {
		t.Fatalf("expected non-truenas and other-asset events to be filtered, got %s", rec.Body.String())
	}
}
