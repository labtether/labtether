package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
