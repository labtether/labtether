package truenas_test

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	truenaspkg "github.com/labtether/labtether/internal/hubapi/truenas"
	"github.com/labtether/labtether/internal/hubcollector"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSelectCollectorForTrueNASRuntime(t *testing.T) {
	collectors := []hubcollector.Collector{
		{ID: "docker-1", CollectorType: hubcollector.CollectorTypeDocker, Enabled: true},
		{ID: "truenas-1", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: true},
		{ID: "truenas-2", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: true},
	}

	if got := truenaspkg.SelectCollectorForTrueNASRuntime(collectors, "truenas-2"); got == nil || got.ID != "truenas-2" {
		t.Fatalf("expected explicit collector match, got %+v", got)
	}
	if got := truenaspkg.SelectCollectorForTrueNASRuntime(collectors, "missing"); got != nil {
		t.Fatalf("expected missing explicit collector to return nil, got %+v", got)
	}
	if got := truenaspkg.SelectCollectorForTrueNASRuntime(collectors, ""); got == nil || got.ID != "truenas-1" {
		t.Fatalf("expected first truenas collector, got %+v", got)
	}
	if got := truenaspkg.SelectCollectorForTrueNASRuntime([]hubcollector.Collector{{ID: "docker", CollectorType: hubcollector.CollectorTypeDocker}}, ""); got != nil {
		t.Fatalf("expected nil when no truenas collector exists")
	}
}

func TestLoadTrueNASRuntimeBranches(t *testing.T) {
	t.Run("hub collector store unavailable", func(t *testing.T) {
		sut := hubdeps.New(t)
		if _, err := sut.TrueNASDeps().LoadTrueNASRuntime(""); err == nil || !strings.Contains(err.Error(), "hub collector store unavailable") {
			t.Fatalf("expected missing store error, got %v", err)
		}
	})

	t.Run("credential store unavailable", func(t *testing.T) {
		sut := hubdeps.New(t)
		sut.CredentialStore = nil
		sut.HubCollectorStore = &testutil.ErrorHubCollectorStore{
			Collectors: []hubcollector.Collector{{
				ID:            "collector-truenas-1",
				CollectorType: hubcollector.CollectorTypeTrueNAS,
				Enabled:       true,
			}},
		}
		if _, err := sut.TrueNASDeps().LoadTrueNASRuntime(""); err == nil || !strings.Contains(err.Error(), "credential store unavailable") {
			t.Fatalf("expected credential store unavailable, got %v", err)
		}
	})

	t.Run("list collectors failure", func(t *testing.T) {
		sut := hubdeps.New(t)
		sut.HubCollectorStore = &testutil.ErrorHubCollectorStore{ListErr: errors.New("boom")}
		if _, err := sut.TrueNASDeps().LoadTrueNASRuntime(""); err == nil || !strings.Contains(err.Error(), "failed to list hub collectors") {
			t.Fatalf("expected list hub collectors error, got %v", err)
		}
	})

	t.Run("no active truenas collector", func(t *testing.T) {
		sut := hubdeps.New(t)
		sut.HubCollectorStore = &testutil.ErrorHubCollectorStore{
			Collectors: []hubcollector.Collector{
				{ID: "collector-docker-1", CollectorType: hubcollector.CollectorTypeDocker, Enabled: true},
			},
		}
		if _, err := sut.TrueNASDeps().LoadTrueNASRuntime(""); err == nil || !strings.Contains(err.Error(), "no active truenas collector configured") {
			t.Fatalf("expected no active collector error, got %v", err)
		}
	})

	t.Run("incomplete collector config", func(t *testing.T) {
		sut := hubdeps.New(t)
		sut.HubCollectorStore = &testutil.ErrorHubCollectorStore{
			Collectors: []hubcollector.Collector{
				{ID: "collector-truenas-1", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: true, Config: map[string]any{"base_url": "https://tn.local"}},
			},
		}
		if _, err := sut.TrueNASDeps().LoadTrueNASRuntime(""); err == nil || !strings.Contains(err.Error(), "config is incomplete") {
			t.Fatalf("expected incomplete config error, got %v", err)
		}
	})

	t.Run("credential profile missing", func(t *testing.T) {
		sut := hubdeps.New(t)
		sut.HubCollectorStore = &testutil.ErrorHubCollectorStore{
			Collectors: []hubcollector.Collector{
				{
					ID:            "collector-truenas-1",
					CollectorType: hubcollector.CollectorTypeTrueNAS,
					Enabled:       true,
					Config:        map[string]any{"base_url": "https://tn.local", "credential_id": "missing-credential"},
				},
			},
		}
		if _, err := sut.TrueNASDeps().LoadTrueNASRuntime(""); err == nil || !strings.Contains(err.Error(), "credential profile not found") {
			t.Fatalf("expected missing credential error, got %v", err)
		}
	})

	t.Run("decrypt failure", func(t *testing.T) {
		sut := hubdeps.New(t)
		const credentialID = "cred-truenas-bad-cipher"
		_, err := sut.CredentialStore.CreateCredentialProfile(credentials.Profile{
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
		sut.HubCollectorStore = &testutil.ErrorHubCollectorStore{
			Collectors: []hubcollector.Collector{
				{
					ID:            "collector-truenas-1",
					CollectorType: hubcollector.CollectorTypeTrueNAS,
					Enabled:       true,
					Config:        map[string]any{"base_url": "https://tn.local", "credential_id": credentialID},
				},
			},
		}
		if _, err := sut.TrueNASDeps().LoadTrueNASRuntime(""); err == nil || !strings.Contains(err.Error(), "failed to decrypt truenas credential") {
			t.Fatalf("expected decrypt error, got %v", err)
		}
	})

	t.Run("cache hit and defaults", func(t *testing.T) {
		sut := hubdeps.New(t)
		createTrueNASCredentialProfile(t, sut, "cred-truenas-cache", "api-key-cache", "https://tn.local")
		sut.HubCollectorStore = &testutil.ErrorHubCollectorStore{
			Collectors: []hubcollector.Collector{
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

		first, err := sut.TrueNASDeps().LoadTrueNASRuntime("collector-truenas-cache")
		if err != nil {
			t.Fatalf("first loadTrueNASRuntime() error = %v", err)
		}
		second, err := sut.TrueNASDeps().LoadTrueNASRuntime("collector-truenas-cache")
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
		sut := hubdeps.New(t)
		sut.TrueNASDeps().EnsureTrueNASSubscriptionWorker(context.Background(), hubcollector.Collector{ID: "collector"}, nil)
		sut.TrueNASDeps().EnsureTrueNASSubscriptionWorker(context.Background(), hubcollector.Collector{ID: "collector"}, &truenaspkg.TruenasRuntime{})
		if len(sut.TrueNASDeps().TruenasSubs) != 0 {
			t.Fatalf("expected no worker handles for invalid runtime")
		}
	})

	t.Run("ensure cancels replaced config", func(t *testing.T) {
		sut := hubdeps.New(t)
		var canceled atomic.Int32
		sut.TrueNASDeps().TruenasSubs = map[string]truenaspkg.TruenasSubscriptionHandle{
			"collector-truenas-1": {
				ConfigKey: "old-key",
				Cancel:    func() { canceled.Add(1) },
			},
		}
		sut.HubCollectorStore = &testutil.ErrorHubCollectorStore{
			Collectors: []hubcollector.Collector{
				{ID: "collector-truenas-1", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: true},
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		sut.TrueNASDeps().EnsureTrueNASSubscriptionWorker(ctx, hubcollector.Collector{ID: "collector-truenas-1"}, &truenaspkg.TruenasRuntime{
			Client:    &truenas.Client{BaseURL: "http://127.0.0.1:1", APIKey: "api-key", Timeout: 50 * time.Millisecond},
			ConfigKey: "new-key",
		})
		if canceled.Load() != 1 {
			t.Fatalf("expected previous worker cancellation")
		}
	})

	t.Run("unregister guard paths", func(t *testing.T) {
		sut := hubdeps.New(t)
		sut.TrueNASDeps().TruenasSubs = map[string]truenaspkg.TruenasSubscriptionHandle{
			"collector-truenas-1": {ConfigKey: "config-a", Cancel: func() {}},
		}
		sut.TrueNASDeps().UnregisterTrueNASSubscriptionWorker("", "")
		sut.TrueNASDeps().UnregisterTrueNASSubscriptionWorker("collector-missing", "")
		sut.TrueNASDeps().UnregisterTrueNASSubscriptionWorker("collector-truenas-1", "config-b")
		if len(sut.TrueNASDeps().TruenasSubs) != 1 {
			t.Fatalf("expected mismatched configKey to preserve handle")
		}
		sut.TrueNASDeps().UnregisterTrueNASSubscriptionWorker("collector-truenas-1", "config-a")
		if len(sut.TrueNASDeps().TruenasSubs) != 0 {
			t.Fatalf("expected matching configKey to remove handle")
		}
	})

	t.Run("collector active checks", func(t *testing.T) {
		sut := hubdeps.New(t)
		if sut.TrueNASDeps().IsTrueNASCollectorActive("") {
			t.Fatalf("expected empty collector id inactive")
		}

		sut.HubCollectorStore = nil
		if sut.TrueNASDeps().IsTrueNASCollectorActive("collector-truenas-1") {
			t.Fatalf("expected nil store inactive")
		}

		sut.HubCollectorStore = &testutil.ErrorHubCollectorStore{GetErr: errors.New("transient")}
		if !sut.TrueNASDeps().IsTrueNASCollectorActive("collector-truenas-1") {
			t.Fatalf("expected transient get error to keep worker alive")
		}

		sut.HubCollectorStore = &testutil.ErrorHubCollectorStore{
			Collectors: []hubcollector.Collector{
				{ID: "collector-truenas-1", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: false},
			},
		}
		if sut.TrueNASDeps().IsTrueNASCollectorActive("collector-truenas-1") {
			t.Fatalf("expected disabled collector inactive")
		}

		sut.HubCollectorStore = &testutil.ErrorHubCollectorStore{
			Collectors: []hubcollector.Collector{
				{ID: "collector-docker-1", CollectorType: hubcollector.CollectorTypeDocker, Enabled: true},
			},
		}
		if sut.TrueNASDeps().IsTrueNASCollectorActive("collector-docker-1") {
			t.Fatalf("expected non-truenas collector inactive")
		}

		sut.HubCollectorStore = &testutil.ErrorHubCollectorStore{
			Collectors: []hubcollector.Collector{
				{ID: "collector-truenas-2", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: true},
			},
		}
		if !sut.TrueNASDeps().IsTrueNASCollectorActive("collector-truenas-2") {
			t.Fatalf("expected enabled truenas collector active")
		}
		if sut.TrueNASDeps().IsTrueNASCollectorActive("collector-missing") {
			t.Fatalf("expected unknown collector inactive")
		}
	})

	t.Run("ensure collector id and config key fallbacks", func(t *testing.T) {
		sut := hubdeps.New(t)
		sut.TrueNASDeps().EnsureTrueNASSubscriptionWorker(context.Background(), hubcollector.Collector{}, &truenaspkg.TruenasRuntime{
			Client: &truenas.Client{},
		})
		if len(sut.TrueNASDeps().TruenasSubs) != 0 {
			t.Fatalf("expected empty collector id to short-circuit")
		}

		sut.TrueNASDeps().TruenasSubs = map[string]truenaspkg.TruenasSubscriptionHandle{
			"collector-truenas-same": {ConfigKey: "https://tn.local", Cancel: func() { t.Fatalf("cancel should not be called for same config") }},
		}
		sut.TrueNASDeps().EnsureTrueNASSubscriptionWorker(context.Background(), hubcollector.Collector{ID: "collector-truenas-same"}, &truenaspkg.TruenasRuntime{
			Client:    &truenas.Client{},
			BaseURL:   "https://tn.local",
			ConfigKey: "",
		})
		if len(sut.TrueNASDeps().TruenasSubs) != 1 {
			t.Fatalf("expected same config handle to remain unchanged")
		}
	})
}

func TestTrueNASSubscriptionFieldHelpers(t *testing.T) {
	normalized := truenaspkg.NormalizeTrueNASSubscriptionFields(map[string]any{
		" hostname ": " VaultNAS ",
		"":           "ignored",
		"empty":      "",
		"id":         123,
	})
	if normalized["hostname"] != "VaultNAS" {
		t.Fatalf("normalized hostname = %q", normalized["hostname"])
	}
	if normalized["id"] != "123" {
		t.Fatalf("normalized id = %q", normalized["id"])
	}
	if _, exists := normalized[""]; exists {
		t.Fatalf("expected blank key to be filtered")
	}
	if len(truenaspkg.NormalizeTrueNASSubscriptionFields(nil)) != 0 {
		t.Fatalf("expected empty map for nil fields")
	}

	target := map[string]string{}
	truenaspkg.CopySubscriptionField(nil, normalized, "hostname")
	truenaspkg.CopySubscriptionField(target, nil, "hostname")
	truenaspkg.CopySubscriptionField(target, normalized, "missing")
	if len(target) != 0 {
		t.Fatalf("expected no copied fields yet")
	}
	truenaspkg.CopySubscriptionField(target, normalized, "hostname")
	if target["hostname"] != "VaultNAS" {
		t.Fatalf("expected copied hostname, got %#v", target)
	}

	alertMsg := truenaspkg.TrueNASSubscriptionMessage(truenas.SubscriptionEvent{Collection: "alert.list"}, map[string]string{"formatted": "Disk warning"})
	if alertMsg != "Disk warning" {
		t.Fatalf("alert message = %q, want Disk warning", alertMsg)
	}
	if got := truenaspkg.TrueNASSubscriptionMessage(truenas.SubscriptionEvent{}, map[string]string{"message": "explicit"}); got != "explicit" {
		t.Fatalf("message field fallback = %q, want explicit", got)
	}
	if got := truenaspkg.TrueNASSubscriptionMessage(truenas.SubscriptionEvent{}, map[string]string{"name": "svc"}); got != "truenas event: svc" {
		t.Fatalf("name fallback = %q, want truenas event: svc", got)
	}
	if got := truenaspkg.TrueNASSubscriptionMessage(truenas.SubscriptionEvent{}, map[string]string{}); got != "truenas subscription event" {
		t.Fatalf("default fallback = %q, want truenas subscription event", got)
	}
}
