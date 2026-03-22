package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubcollector"
)

func TestSelectCollectorForPBSRuntime(t *testing.T) {
	collectors := []hubcollector.Collector{
		{ID: "docker-1", CollectorType: hubcollector.CollectorTypeDocker, Enabled: true},
		{ID: "pbs-1", CollectorType: hubcollector.CollectorTypePBS, Enabled: true},
		{ID: "pbs-2", CollectorType: hubcollector.CollectorTypePBS, Enabled: true},
	}

	if got := selectCollectorForPBSRuntime(collectors, "pbs-2"); got == nil || got.ID != "pbs-2" {
		t.Fatalf("expected explicit collector match, got %+v", got)
	}
	if got := selectCollectorForPBSRuntime(collectors, "missing"); got != nil {
		t.Fatalf("expected missing explicit collector to return nil, got %+v", got)
	}
	if got := selectCollectorForPBSRuntime(collectors, ""); got == nil || got.ID != "pbs-1" {
		t.Fatalf("expected first pbs collector, got %+v", got)
	}
	if got := selectCollectorForPBSRuntime([]hubcollector.Collector{
		{ID: "docker-only", CollectorType: hubcollector.CollectorTypeDocker, Enabled: true},
	}, ""); got != nil {
		t.Fatalf("expected nil when no pbs collector exists")
	}
}

func TestLoadPBSRuntimeBranches(t *testing.T) {
	t.Run("hub collector store unavailable", func(t *testing.T) {
		sut := newTestAPIServer(t)
		if _, err := sut.loadPBSRuntime(""); err == nil || !strings.Contains(err.Error(), "hub collector store unavailable") {
			t.Fatalf("expected missing store error, got %v", err)
		}
	})

	t.Run("credential store unavailable", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.credentialStore = nil
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{{
				ID:            "collector-pbs-1",
				CollectorType: hubcollector.CollectorTypePBS,
				Enabled:       true,
			}},
		}
		if _, err := sut.loadPBSRuntime(""); err == nil || !strings.Contains(err.Error(), "credential store unavailable") {
			t.Fatalf("expected credential store unavailable error, got %v", err)
		}
	})

	t.Run("list collectors failure", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.hubCollectorStore = &errorHubCollectorStore{listErr: errors.New("boom")}
		if _, err := sut.loadPBSRuntime(""); err == nil || !strings.Contains(err.Error(), "failed to list hub collectors") {
			t.Fatalf("expected list hub collectors error, got %v", err)
		}
	})

	t.Run("no active pbs collector", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{ID: "collector-docker-1", CollectorType: hubcollector.CollectorTypeDocker, Enabled: true},
			},
		}
		if _, err := sut.loadPBSRuntime(""); err == nil || !strings.Contains(err.Error(), "no active pbs collector configured") {
			t.Fatalf("expected no active collector error, got %v", err)
		}
	})

	t.Run("collector id required when multiple active pbs collectors", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{ID: "collector-pbs-1", CollectorType: hubcollector.CollectorTypePBS, Enabled: true},
				{ID: "collector-pbs-2", CollectorType: hubcollector.CollectorTypePBS, Enabled: true},
			},
		}
		if _, err := sut.loadPBSRuntime(""); err == nil || !strings.Contains(err.Error(), "collector_id is required when multiple active pbs collectors are configured") {
			t.Fatalf("expected multi-collector collector_id validation error, got %v", err)
		}
	})

	t.Run("incomplete collector config", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{
					ID:            "collector-pbs-1",
					CollectorType: hubcollector.CollectorTypePBS,
					Enabled:       true,
					Config: map[string]any{
						"base_url": "https://pbs.local:8007",
					},
				},
			},
		}
		if _, err := sut.loadPBSRuntime(""); err == nil || !strings.Contains(err.Error(), "pbs collector config is incomplete") {
			t.Fatalf("expected incomplete config error, got %v", err)
		}
	})

	t.Run("credential profile missing", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{
					ID:            "collector-pbs-1",
					CollectorType: hubcollector.CollectorTypePBS,
					Enabled:       true,
					Config: map[string]any{
						"base_url":      "https://pbs.local:8007",
						"credential_id": "missing-credential",
					},
				},
			},
		}
		if _, err := sut.loadPBSRuntime(""); err == nil || !strings.Contains(err.Error(), "pbs credential profile not found") {
			t.Fatalf("expected missing credential error, got %v", err)
		}
	})

	t.Run("decrypt failure", func(t *testing.T) {
		sut := newTestAPIServer(t)
		const credentialID = "cred-pbs-bad-cipher"
		_, err := sut.credentialStore.CreateCredentialProfile(credentials.Profile{
			ID:               credentialID,
			Name:             "bad cipher",
			Kind:             credentials.KindPBSAPIToken,
			Username:         "root@pam!labtether",
			Status:           "active",
			SecretCiphertext: "not-valid-ciphertext",
			Metadata: map[string]string{
				"base_url": "https://pbs.local:8007",
			},
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("create profile: %v", err)
		}
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{
					ID:            "collector-pbs-1",
					CollectorType: hubcollector.CollectorTypePBS,
					Enabled:       true,
					Config: map[string]any{
						"base_url":      "https://pbs.local:8007",
						"credential_id": credentialID,
					},
				},
			},
		}
		if _, err := sut.loadPBSRuntime(""); err == nil || !strings.Contains(err.Error(), "failed to decrypt pbs credential") {
			t.Fatalf("expected decrypt error, got %v", err)
		}
	})

	t.Run("token id missing", func(t *testing.T) {
		sut := newTestAPIServer(t)
		createPBSCredentialProfileWithMetadata(t, sut, "cred-pbs-no-token", "", "secret-1", map[string]string{
			"base_url": "https://pbs.local:8007",
		})
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{
					ID:            "collector-pbs-1",
					CollectorType: hubcollector.CollectorTypePBS,
					Enabled:       true,
					Config: map[string]any{
						"base_url":      "https://pbs.local:8007",
						"credential_id": "cred-pbs-no-token",
					},
				},
			},
		}
		if _, err := sut.loadPBSRuntime(""); err == nil || !strings.Contains(err.Error(), "pbs token_id missing in collector config") {
			t.Fatalf("expected token id missing error, got %v", err)
		}
	})

	t.Run("invalid ca pem", func(t *testing.T) {
		sut := newTestAPIServer(t)
		createPBSCredentialProfile(t, sut, "cred-pbs-bad-capem", "root@pam!labtether", "secret-1", "https://pbs.local:8007")
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{
					ID:            "collector-pbs-1",
					CollectorType: hubcollector.CollectorTypePBS,
					Enabled:       true,
					Config: map[string]any{
						"base_url":      "https://pbs.local:8007",
						"credential_id": "cred-pbs-bad-capem",
						"token_id":      "root@pam!labtether",
						"ca_pem":        "not-a-valid-certificate",
					},
				},
			},
		}
		if _, err := sut.loadPBSRuntime(""); err == nil || !strings.Contains(err.Error(), "invalid PBS CA PEM") {
			t.Fatalf("expected invalid ca pem error, got %v", err)
		}
	})

	t.Run("cache hit and defaults", func(t *testing.T) {
		sut := newTestAPIServer(t)
		createPBSCredentialProfile(t, sut, "cred-pbs-cache", "root@pam!cache", "secret-cache", "https://pbs.local:8007")
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{
					ID:            "collector-pbs-cache",
					CollectorType: hubcollector.CollectorTypePBS,
					Enabled:       true,
					Config: map[string]any{
						"base_url":      "https://pbs.local:8007",
						"credential_id": "cred-pbs-cache",
						"token_id":      "root@pam!cache",
					},
				},
			},
		}

		first, err := sut.loadPBSRuntime("collector-pbs-cache")
		if err != nil {
			t.Fatalf("first loadPBSRuntime() error = %v", err)
		}
		second, err := sut.loadPBSRuntime("collector-pbs-cache")
		if err != nil {
			t.Fatalf("second loadPBSRuntime() error = %v", err)
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

	t.Run("token id fallback from credential metadata", func(t *testing.T) {
		sut := newTestAPIServer(t)
		createPBSCredentialProfileWithMetadata(t, sut, "cred-pbs-meta-token", "", "secret-meta", map[string]string{
			"base_url": "https://pbs.local:8007",
			"token_id": "metadata-token-id",
		})
		sut.hubCollectorStore = &errorHubCollectorStore{
			collectors: []hubcollector.Collector{
				{
					ID:            "collector-pbs-meta-token",
					CollectorType: hubcollector.CollectorTypePBS,
					Enabled:       true,
					Config: map[string]any{
						"base_url":      "https://pbs.local:8007",
						"credential_id": "cred-pbs-meta-token",
					},
				},
			},
		}

		runtime, err := sut.loadPBSRuntime("collector-pbs-meta-token")
		if err != nil {
			t.Fatalf("loadPBSRuntime() error = %v", err)
		}
		if runtime.TokenID != "metadata-token-id" {
			t.Fatalf("runtime token_id = %q, want metadata-token-id", runtime.TokenID)
		}
	})
}

func createPBSCredentialProfileWithMetadata(t *testing.T, sut *apiServer, credentialID, username, secret string, metadata map[string]string) {
	t.Helper()

	ciphertext, err := sut.secretsManager.EncryptString(secret, credentialID)
	if err != nil {
		t.Fatalf("encrypt pbs secret: %v", err)
	}
	_, err = sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               credentialID,
		Name:             "pbs " + credentialID,
		Kind:             credentials.KindPBSAPIToken,
		Username:         username,
		Status:           "active",
		SecretCiphertext: ciphertext,
		Metadata:         metadata,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create pbs credential profile: %v", err)
	}
}
