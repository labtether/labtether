package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/connectors/truenas"
	truenaspkg "github.com/labtether/labtether/internal/hubapi/truenas"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
)

type errorHubCollectorStore struct {
	collectors []hubcollector.Collector
	listErr    error
	getErr     error
}

func (s *errorHubCollectorStore) CreateHubCollector(req hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *errorHubCollectorStore) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	if s.getErr != nil {
		return hubcollector.Collector{}, false, s.getErr
	}
	for _, collector := range s.collectors {
		if collector.ID == id {
			return collector, true, nil
		}
	}
	return hubcollector.Collector{}, false, nil
}

func (s *errorHubCollectorStore) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := make([]hubcollector.Collector, 0, len(s.collectors))
	for _, collector := range s.collectors {
		if enabledOnly && !collector.Enabled {
			continue
		}
		out = append(out, collector)
	}
	return out, nil
}

func (s *errorHubCollectorStore) UpdateHubCollector(id string, req hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *errorHubCollectorStore) DeleteHubCollector(id string) error {
	return fmt.Errorf("not implemented")
}

func (s *errorHubCollectorStore) UpdateHubCollectorStatus(id, status, lastError string, collectedAt time.Time) error {
	return nil
}

func newTrueNASSubscriptionServer(t *testing.T, onSubscribe func(conn *websocket.Conn) error) *httptest.Server {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	errCh := make(chan error, 1)
	var errOnce sync.Once
	var active sync.WaitGroup
	reportErr := func(format string, args ...any) {
		errOnce.Do(func() {
			errCh <- fmt.Errorf(format, args...)
		})
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		active.Add(1)
		defer active.Done()

		if r.URL.Path != "/api/current" {
			http.NotFound(w, r)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			reportErr("upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		var authReq map[string]any
		if err := conn.ReadJSON(&authReq); err != nil {
			reportErr("read auth request: %v", err)
			return
		}
		if strings.TrimSpace(collectorAnyString(authReq["method"])) != "auth.login_with_api_key" {
			reportErr("expected auth.login_with_api_key, got %#v", authReq["method"])
			return
		}
		if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": authReq["id"], "result": true}); err != nil {
			reportErr("write auth response: %v", err)
			return
		}

		var subscribeReq map[string]any
		if err := conn.ReadJSON(&subscribeReq); err != nil {
			reportErr("read subscribe request: %v", err)
			return
		}
		if strings.TrimSpace(collectorAnyString(subscribeReq["method"])) != "core.subscribe" {
			reportErr("expected core.subscribe, got %#v", subscribeReq["method"])
			return
		}
		if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": subscribeReq["id"], "result": "sub-1"}); err != nil {
			reportErr("write subscribe response: %v", err)
			return
		}

		if onSubscribe != nil {
			if err := onSubscribe(conn); err != nil {
				reportErr("onSubscribe: %v", err)
			}
		}
	}))

	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()

		done := make(chan struct{})
		go func() {
			active.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("timed out waiting for TrueNAS subscription server handlers to exit")
		}

		select {
		case err := <-errCh:
			t.Errorf("TrueNAS subscription test server error: %v", err)
		default:
		}
	})

	return server
}

func TestSelectCollectorForTrueNASRuntime(t *testing.T) {
	collectors := []hubcollector.Collector{
		{ID: "docker-1", CollectorType: hubcollector.CollectorTypeDocker, Enabled: true},
		{ID: "truenas-1", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: true},
		{ID: "truenas-2", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: true},
	}

	if got := selectCollectorForTrueNASRuntime(collectors, "truenas-2"); got == nil || got.ID != "truenas-2" {
		t.Fatalf("expected explicit collector match, got %+v", got)
	}
	if got := selectCollectorForTrueNASRuntime(collectors, "missing"); got != nil {
		t.Fatalf("expected missing explicit collector to return nil, got %+v", got)
	}
	if got := selectCollectorForTrueNASRuntime(collectors, ""); got == nil || got.ID != "truenas-1" {
		t.Fatalf("expected first truenas collector, got %+v", got)
	}
	if got := selectCollectorForTrueNASRuntime([]hubcollector.Collector{{ID: "docker", CollectorType: hubcollector.CollectorTypeDocker}}, ""); got != nil {
		t.Fatalf("expected nil when no truenas collector exists")
	}
}

func TestLoadTrueNASRuntimeBranches(t *testing.T) {
	t.Run("hub collector store unavailable", func(t *testing.T) {
		sut := newTestAPIServer(t)
		if _, err := sut.loadTrueNASRuntime(""); err == nil || !strings.Contains(err.Error(), "hub collector store unavailable") {
			t.Fatalf("expected missing store error, got %v", err)
		}
	})

	t.Run("credential store unavailable", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.credentialStore = nil
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{{
				ID:            "collector-truenas-1",
				CollectorType: hubcollector.CollectorTypeTrueNAS,
				Enabled:       true,
			}},
		}
		if _, err := sut.loadTrueNASRuntime(""); err == nil || !strings.Contains(err.Error(), "credential store unavailable") {
			t.Fatalf("expected credential store unavailable, got %v", err)
		}
	})

	t.Run("list collectors failure", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.hubCollectorStore = &errorHubCollectorStore{listErr: errors.New("boom")}
		if _, err := sut.loadTrueNASRuntime(""); err == nil || !strings.Contains(err.Error(), "failed to list hub collectors") {
			t.Fatalf("expected list hub collectors error, got %v", err)
		}
	})

	t.Run("no active truenas collector", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{ID: "collector-docker-1", CollectorType: hubcollector.CollectorTypeDocker, Enabled: true},
			},
		}
		if _, err := sut.loadTrueNASRuntime(""); err == nil || !strings.Contains(err.Error(), "no active truenas collector configured") {
			t.Fatalf("expected no active collector error, got %v", err)
		}
	})

	t.Run("incomplete collector config", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{ID: "collector-truenas-1", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: true, Config: map[string]any{"base_url": "https://tn.local"}},
			},
		}
		if _, err := sut.loadTrueNASRuntime(""); err == nil || !strings.Contains(err.Error(), "config is incomplete") {
			t.Fatalf("expected incomplete config error, got %v", err)
		}
	})

	t.Run("credential profile missing", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{
					ID:            "collector-truenas-1",
					CollectorType: hubcollector.CollectorTypeTrueNAS,
					Enabled:       true,
					Config:        map[string]any{"base_url": "https://tn.local", "credential_id": "missing-credential"},
				},
			},
		}
		if _, err := sut.loadTrueNASRuntime(""); err == nil || !strings.Contains(err.Error(), "credential profile not found") {
			t.Fatalf("expected missing credential error, got %v", err)
		}
	})

	t.Run("decrypt failure", func(t *testing.T) {
		sut := newTestAPIServer(t)
		const credentialID = "cred-truenas-bad-cipher"
		_, err := sut.credentialStore.CreateCredentialProfile(credentials.Profile{
			ID:               credentialID,
			Name:             "bad cipher",
			Kind:             credentials.KindTrueNASAPIKey,
			Status:           "active",
			SecretCiphertext: "not-valid-ciphertext",
			Metadata:         map[string]string{"base_url": "https://tn.local"},
			CreatedAt:        time.Now().UTC(),
			UpdatedAt:        time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("create profile: %v", err)
		}
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{
					ID:            "collector-truenas-1",
					CollectorType: hubcollector.CollectorTypeTrueNAS,
					Enabled:       true,
					Config:        map[string]any{"base_url": "https://tn.local", "credential_id": credentialID},
				},
			},
		}
		if _, err := sut.loadTrueNASRuntime(""); err == nil || !strings.Contains(err.Error(), "failed to decrypt truenas credential") {
			t.Fatalf("expected decrypt error, got %v", err)
		}
	})

	t.Run("cache hit and defaults", func(t *testing.T) {
		sut := newTestAPIServer(t)
		createTrueNASCredentialProfile(t, sut, "cred-truenas-cache", "api-key-cache", "https://tn.local")
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{
					ID:            "collector-truenas-cache",
					CollectorType: hubcollector.CollectorTypeTrueNAS,
					Enabled:       true,
					Config: map[string]any{
						"base_url":      "https://tn.local",
						"credential_id": "cred-truenas-cache",
					},
				},
			},
		}

		first, err := sut.loadTrueNASRuntime("collector-truenas-cache")
		if err != nil {
			t.Fatalf("first loadTrueNASRuntime() error = %v", err)
		}
		second, err := sut.loadTrueNASRuntime("collector-truenas-cache")
		if err != nil {
			t.Fatalf("second loadTrueNASRuntime() error = %v", err)
		}
		if first != second {
			t.Fatalf("expected runtime cache hit")
		}
		if first.SkipVerify {
			t.Fatalf("expected skipVerify default false")
		}
		if first.Timeout != 15*time.Second {
			t.Fatalf("expected default timeout 15s, got %s", first.Timeout)
		}
	})
}

func TestTrueNASSubscriptionWorkerHelpers(t *testing.T) {
	t.Run("ensure short-circuits for invalid runtime", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.ensureTrueNASSubscriptionWorker(context.Background(), hubcollector.Collector{ID: "collector"}, nil)
		sut.ensureTrueNASSubscriptionWorker(context.Background(), hubcollector.Collector{ID: "collector"}, &truenasRuntime{})
		if len(sut.ensureTruenasDeps().TruenasSubs) != 0 {
			t.Fatalf("expected no worker handles for invalid runtime")
		}
	})

	t.Run("ensure cancels replaced config", func(t *testing.T) {
		sut := newTestAPIServer(t)
		var canceled atomic.Int32
		sut.ensureTruenasDeps().TruenasSubs = map[string]truenasSubscriptionHandle{
			"collector-truenas-1": {
				ConfigKey: "old-key",
				Cancel:    func() { canceled.Add(1) },
			},
		}
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{ID: "collector-truenas-1", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: true},
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		sut.ensureTrueNASSubscriptionWorker(ctx, hubcollector.Collector{ID: "collector-truenas-1"}, &truenasRuntime{
			Client:    &truenas.Client{BaseURL: "http://127.0.0.1:1", APIKey: "api-key", Timeout: 50 * time.Millisecond},
			ConfigKey: "new-key",
		})
		if canceled.Load() != 1 {
			t.Fatalf("expected previous worker cancellation")
		}
	})

	t.Run("unregister guard paths", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.ensureTruenasDeps().TruenasSubs = map[string]truenasSubscriptionHandle{
			"collector-truenas-1": {ConfigKey: "config-a", Cancel: func() {}},
		}
		sut.unregisterTrueNASSubscriptionWorker("", "")
		sut.unregisterTrueNASSubscriptionWorker("collector-missing", "")
		sut.unregisterTrueNASSubscriptionWorker("collector-truenas-1", "config-b")
		if len(sut.ensureTruenasDeps().TruenasSubs) != 1 {
			t.Fatalf("expected mismatched configKey to preserve handle")
		}
		sut.unregisterTrueNASSubscriptionWorker("collector-truenas-1", "config-a")
		if len(sut.ensureTruenasDeps().TruenasSubs) != 0 {
			t.Fatalf("expected matching configKey to remove handle")
		}
	})

	t.Run("collector active checks", func(t *testing.T) {
		sut := newTestAPIServer(t)
		if sut.isTrueNASCollectorActive("") {
			t.Fatalf("expected empty collector id inactive")
		}

		sut.hubCollectorStore = nil
		if sut.isTrueNASCollectorActive("collector-truenas-1") {
			t.Fatalf("expected nil store inactive")
		}

		sut.hubCollectorStore = &errorHubCollectorStore{getErr: errors.New("transient")}
		if !sut.isTrueNASCollectorActive("collector-truenas-1") {
			t.Fatalf("expected transient get error to keep worker alive")
		}

		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{ID: "collector-truenas-1", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: false},
			},
		}
		if sut.isTrueNASCollectorActive("collector-truenas-1") {
			t.Fatalf("expected disabled collector inactive")
		}

		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{ID: "collector-docker-1", CollectorType: hubcollector.CollectorTypeDocker, Enabled: true},
			},
		}
		if sut.isTrueNASCollectorActive("collector-docker-1") {
			t.Fatalf("expected non-truenas collector inactive")
		}

		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{ID: "collector-truenas-2", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: true},
			},
		}
		if !sut.isTrueNASCollectorActive("collector-truenas-2") {
			t.Fatalf("expected enabled truenas collector active")
		}
		if sut.isTrueNASCollectorActive("collector-missing") {
			t.Fatalf("expected unknown collector inactive")
		}
	})

	t.Run("ensure collector id and config key fallbacks", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.ensureTrueNASSubscriptionWorker(context.Background(), hubcollector.Collector{}, &truenasRuntime{
			Client: &truenas.Client{},
		})
		if len(sut.ensureTruenasDeps().TruenasSubs) != 0 {
			t.Fatalf("expected empty collector id to short-circuit")
		}

		sut.ensureTruenasDeps().TruenasSubs = map[string]truenasSubscriptionHandle{
			"collector-truenas-same": {ConfigKey: "https://tn.local", Cancel: func() { t.Fatalf("cancel should not be called for same config") }},
		}
		sut.ensureTrueNASSubscriptionWorker(context.Background(), hubcollector.Collector{ID: "collector-truenas-same"}, &truenasRuntime{
			Client:    &truenas.Client{},
			BaseURL:   "https://tn.local",
			ConfigKey: "",
		})
		if len(sut.ensureTruenasDeps().TruenasSubs) != 1 {
			t.Fatalf("expected same config handle to remain unchanged")
		}
	})
}

func TestTrueNASSubscriptionEventIngestionAndMessages(t *testing.T) {
	sut := newTestAPIServer(t)
	collector := hubcollector.Collector{ID: "collector-truenas-1", AssetID: "truenas-cluster-1"}

	sut.ingestTrueNASSubscriptionEvent(collector, collector.ID, truenas.SubscriptionEvent{
		Collection:  "alert.list",
		MessageType: "removed",
		Fields: map[string]any{
			"hostname":  "OmegaNAS",
			"uuid":      "alert-uuid-1",
			"formatted": "Pool degraded",
			"level":     "info",
			"datetime":  "2026-02-23T00:00:00Z",
			"klass":     "PoolStatus",
			"source":    "middlewared",
		},
	})

	events, err := sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected subscription log event")
	}
	first := events[0]
	if first.AssetID != "truenas-host-omeganas" {
		t.Fatalf("asset id = %q, want truenas-host-omeganas", first.AssetID)
	}
	if first.Level != "warn" {
		t.Fatalf("level = %q, want warn", first.Level)
	}
	if !strings.Contains(first.Message, "Pool degraded") {
		t.Fatalf("message = %q, want pool degraded content", first.Message)
	}
	if first.Fields["hostname"] != "OmegaNAS" || first.Fields["uuid"] != "alert-uuid-1" {
		t.Fatalf("expected propagated fields, got %#v", first.Fields)
	}

	sut.ingestTrueNASSubscriptionEvent(collector, collector.ID, truenas.SubscriptionEvent{
		Collection:  "service.query",
		MessageType: "failed",
		EventID:     "evt-2",
		Fields: map[string]any{
			"message": "ssh stopped",
			"level":   "info",
		},
	})
	events, err = sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected at least two subscription events")
	}
	if events[0].Level != "error" {
		t.Fatalf("expected failed event to map to error level, got %q", events[0].Level)
	}

	sut.ingestTrueNASSubscriptionEvent(collector, collector.ID, truenas.SubscriptionEvent{
		Collection:  "service.query",
		MessageType: "",
		Fields:      map[string]any{"name": "ssh"},
	})
	events, err = sut.logStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(365 * 24 * time.Hour),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	if len(events) == 0 || events[0].ID == "" {
		t.Fatalf("expected generated subscription log id for key fallback")
	}
}

func TestTrueNASSubscriptionFieldHelpers(t *testing.T) {
	normalized := normalizeTrueNASSubscriptionFields(map[string]any{
		" hostname ": " OmegaNAS ",
		"":           "ignored",
		"empty":      "",
		"id":         123,
	})
	if normalized["hostname"] != "OmegaNAS" {
		t.Fatalf("normalized hostname = %q", normalized["hostname"])
	}
	if normalized["id"] != "123" {
		t.Fatalf("normalized id = %q", normalized["id"])
	}
	if _, exists := normalized[""]; exists {
		t.Fatalf("expected blank key to be filtered")
	}
	if len(normalizeTrueNASSubscriptionFields(nil)) != 0 {
		t.Fatalf("expected empty map for nil fields")
	}

	target := map[string]string{}
	copySubscriptionField(nil, normalized, "hostname")
	copySubscriptionField(target, nil, "hostname")
	copySubscriptionField(target, normalized, "missing")
	if len(target) != 0 {
		t.Fatalf("expected no copied fields yet")
	}
	copySubscriptionField(target, normalized, "hostname")
	if target["hostname"] != "OmegaNAS" {
		t.Fatalf("expected copied hostname, got %#v", target)
	}

	alertMsg := trueNASSubscriptionMessage(truenas.SubscriptionEvent{Collection: "alert.list"}, map[string]string{"formatted": "Disk warning"})
	if alertMsg != "Disk warning" {
		t.Fatalf("alert message = %q, want Disk warning", alertMsg)
	}
	if got := trueNASSubscriptionMessage(truenas.SubscriptionEvent{}, map[string]string{"message": "explicit"}); got != "explicit" {
		t.Fatalf("message field fallback = %q, want explicit", got)
	}
	if got := trueNASSubscriptionMessage(truenas.SubscriptionEvent{}, map[string]string{"name": "svc"}); got != "truenas event: svc" {
		t.Fatalf("name fallback = %q, want truenas event: svc", got)
	}
	if got := trueNASSubscriptionMessage(truenas.SubscriptionEvent{}, map[string]string{}); got != "truenas subscription event" {
		t.Fatalf("default fallback = %q, want truenas subscription event", got)
	}
}

func TestRunTrueNASSubscriptionWorkerInactiveCollector(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.hubCollectorStore = &errorHubCollectorStore{
		collectors: []hubcollector.Collector{
			{ID: "collector-truenas-1", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: false},
		},
	}
	sut.ensureTruenasDeps().TruenasSubs = map[string]truenasSubscriptionHandle{
		"collector-truenas-1": {ConfigKey: "cfg-1", Cancel: func() {}},
	}

	sut.runTrueNASSubscriptionWorker(context.Background(), hubcollector.Collector{
		ID:      "collector-truenas-1",
		AssetID: "truenas-cluster-1",
	}, &truenasRuntime{
		Client:      &truenas.Client{},
		CollectorID: "collector-truenas-1",
		ConfigKey:   "cfg-1",
	})

	if len(sut.ensureTruenasDeps().TruenasSubs) != 0 {
		t.Fatalf("expected worker handle to be unregistered on exit")
	}
}

func TestRunTrueNASSubscriptionWorkerAdditionalBranches(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	t.Run("subscription event callback is ingested", func(t *testing.T) {
		server := newTrueNASSubscriptionServer(t, func(conn *websocket.Conn) error {
			if err := conn.WriteJSON(map[string]any{
				"collection": "alert.list",
				"msg":        "added",
				"id":         "evt-1",
				"fields": map[string]any{
					"hostname":  "OmegaNAS",
					"formatted": "Pool warning",
					"level":     "info",
					"datetime":  "2026-02-23T00:00:00Z",
				},
			}); err != nil {
				return err
			}
			time.Sleep(20 * time.Millisecond)
			return nil
		})

		sut := newTestAPIServer(t)
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{ID: "collector-truenas-evt", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: true},
			},
		}

		collector := hubcollector.Collector{ID: "collector-truenas-evt", AssetID: "truenas-cluster-evt"}
		runtime := &truenasRuntime{
			Client:      &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second},
			CollectorID: collector.ID,
			ConfigKey:   "cfg-evt",
		}
		sut.ensureTruenasDeps().TruenasSubs = map[string]truenasSubscriptionHandle{
			collector.ID: {ConfigKey: "cfg-evt", Cancel: func() {}},
		}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			sut.runTrueNASSubscriptionWorker(ctx, collector, runtime)
			close(done)
		}()

		deadline := time.Now().Add(2 * time.Second)
		found := false
		for time.Now().Before(deadline) {
			events, err := sut.logStore.QueryEvents(logs.QueryRequest{
				Source: "truenas",
				From:   time.Unix(0, 0).UTC(),
				To:     time.Now().UTC().Add(365 * 24 * time.Hour),
				Limit:  20,
			})
			if err != nil {
				t.Fatalf("QueryEvents() error = %v", err)
			}
			for _, event := range events {
				if strings.Contains(event.Message, "Pool warning") {
					found = true
					break
				}
			}
			if found {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		if !found {
			t.Fatalf("expected ingested subscription event log")
		}

		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for worker shutdown")
		}
	})

	t.Run("backoff timer path and cap branch", func(t *testing.T) {
		server := newTrueNASSubscriptionServer(t, func(conn *websocket.Conn) error {
			// Close immediately so Subscribe returns an error and enters backoff.
			return conn.Close()
		})

		sut := newTestAPIServer(t)
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{ID: "collector-truenas-backoff", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: true},
			},
		}

		collector := hubcollector.Collector{ID: "collector-truenas-backoff", AssetID: "truenas-cluster-backoff"}
		runtime := &truenasRuntime{
			Client:      &truenas.Client{BaseURL: server.URL, APIKey: "api-key", Timeout: time.Second},
			CollectorID: collector.ID,
			ConfigKey:   "cfg-backoff",
		}
		sut.ensureTruenasDeps().TruenasSubs = map[string]truenasSubscriptionHandle{
			collector.ID: {ConfigKey: "cfg-backoff", Cancel: func() {}},
		}

		truenaspkg.SubscriptionBackoffMu.Lock()
		origInitial := truenaspkg.SubscriptionInitialBackoff
		origMax := truenaspkg.SubscriptionMaxBackoff
		truenaspkg.SubscriptionInitialBackoff = 10 * time.Millisecond
		truenaspkg.SubscriptionMaxBackoff = 15 * time.Millisecond
		truenaspkg.SubscriptionBackoffMu.Unlock()
		defer func() {
			truenaspkg.SubscriptionBackoffMu.Lock()
			truenaspkg.SubscriptionInitialBackoff = origInitial
			truenaspkg.SubscriptionMaxBackoff = origMax
			truenaspkg.SubscriptionBackoffMu.Unlock()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()
		sut.runTrueNASSubscriptionWorker(ctx, collector, runtime)

		if len(sut.ensureTruenasDeps().TruenasSubs) != 0 {
			t.Fatalf("expected worker handle to be unregistered on exit")
		}
	})
}
