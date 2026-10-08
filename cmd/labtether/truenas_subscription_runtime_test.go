package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/connectors/truenas"
	truenaspkg "github.com/labtether/labtether/internal/hubapi/truenas"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func isExpectedTrueNASSubscriptionDisconnect(expectedDisconnect <-chan struct{}, err error) bool {
	if expectedDisconnect == nil || err == nil {
		return false
	}
	select {
	case <-expectedDisconnect:
	default:
		return false
	}

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

func newTrueNASSubscriptionServer(
	t *testing.T,
	expectedDisconnect <-chan struct{},
	onSubscribe func(conn *websocket.Conn) error,
) *httptest.Server {
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
			if isExpectedTrueNASSubscriptionDisconnect(expectedDisconnect, err) {
				return
			}
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
			if isExpectedTrueNASSubscriptionDisconnect(expectedDisconnect, err) {
				return
			}
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

func TestIsExpectedTrueNASSubscriptionDisconnect(t *testing.T) {
	expectedDisconnect := make(chan struct{})
	abnormalClose := &websocket.CloseError{Code: websocket.CloseAbnormalClosure, Text: "unexpected EOF"}

	if isExpectedTrueNASSubscriptionDisconnect(expectedDisconnect, abnormalClose) {
		t.Fatal("must not ignore a disconnect before worker cancellation")
	}
	close(expectedDisconnect)
	if !isExpectedTrueNASSubscriptionDisconnect(expectedDisconnect, abnormalClose) {
		t.Fatal("expected cancellation-time disconnect to be ignored")
	}
	if isExpectedTrueNASSubscriptionDisconnect(expectedDisconnect, errors.New("malformed authentication payload")) {
		t.Fatal("must not ignore a protocol payload failure during cancellation")
	}
	protocolClose := &websocket.CloseError{Code: websocket.CloseProtocolError, Text: "invalid frame"}
	if isExpectedTrueNASSubscriptionDisconnect(expectedDisconnect, protocolClose) {
		t.Fatal("must not ignore a websocket protocol failure during cancellation")
	}
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
		ctx, cancel := context.WithCancel(context.Background())
		server := newTrueNASSubscriptionServer(t, ctx.Done(), func(conn *websocket.Conn) error {
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
		// The second completed subscription handshake proves that the first
		// retry timer fired and the 10ms backoff was capped at 15ms. Cancel from
		// that observable milestone instead of racing an 80ms wall-clock timeout
		// against a newly accepted websocket connection.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var subscribeCount atomic.Int32
		server := newTrueNASSubscriptionServer(t, ctx.Done(), func(conn *websocket.Conn) error {
			// Close immediately so Subscribe returns an error and enters backoff.
			if subscribeCount.Add(1) == 2 {
				cancel()
			}
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

		sut.runTrueNASSubscriptionWorker(ctx, collector, runtime)
		if got := subscribeCount.Load(); got < 2 {
			t.Fatalf("completed subscription handshakes = %d, want at least 2", got)
		}

		if len(sut.ensureTruenasDeps().TruenasSubs) != 0 {
			t.Fatalf("expected worker handle to be unregistered on exit")
		}
	})
}
