package collectors_test

import (
	"context"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"github.com/labtether/labtether/internal/hubcollector"
	"strings"
	"testing"
	"time"
)

func TestExecuteTrueNASCollectorErrorBranches(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	t.Run("credential store unavailable", func(t *testing.T) {
		sut := hubdeps.New(t)
		store := testutil.NewRecordingHubCollectorStore()
		store.StatusByID["collector-truenas-1"] = hubcollector.Collector{ID: "collector-truenas-1"}
		sut.HubCollectorStore = store
		sut.CredentialStore = nil

		sut.CollectorsDeps().ExecuteTrueNASCollector(context.Background(), hubcollector.Collector{
			ID:            "collector-truenas-1",
			AssetID:       "truenas-cluster-1",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
		})

		updated, ok, err := store.GetHubCollector("collector-truenas-1")
		if err != nil || !ok {
			t.Fatalf("failed to get collector status: ok=%v err=%v", ok, err)
		}
		if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "credential store unavailable") {
			t.Fatalf("unexpected collector status update: %+v", updated)
		}
	})

	t.Run("missing base url and credential id", func(t *testing.T) {
		sut := hubdeps.New(t)
		store := testutil.NewRecordingHubCollectorStore()
		sut.HubCollectorStore = store

		collector := hubcollector.Collector{
			ID:            "collector-truenas-2",
			AssetID:       "truenas-cluster-2",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config:        map[string]any{},
		}
		store.StatusByID[collector.ID] = collector
		sut.CollectorsDeps().ExecuteTrueNASCollector(context.Background(), collector)
		updated, _, _ := store.GetHubCollector(collector.ID)
		if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "missing base_url") {
			t.Fatalf("unexpected missing base_url status: %+v", updated)
		}

		collector.ID = "collector-truenas-3"
		collector.Config = map[string]any{"base_url": "https://tn.local"}
		store.StatusByID[collector.ID] = collector
		sut.CollectorsDeps().ExecuteTrueNASCollector(context.Background(), collector)
		updated, _, _ = store.GetHubCollector(collector.ID)
		if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "missing credential_id") {
			t.Fatalf("unexpected missing credential_id status: %+v", updated)
		}
	})

	t.Run("credential lookup and decrypt errors", func(t *testing.T) {
		sut := hubdeps.New(t)
		store := testutil.NewRecordingHubCollectorStore()
		sut.HubCollectorStore = store

		missingCollector := hubcollector.Collector{
			ID:            "collector-truenas-4",
			AssetID:       "truenas-cluster-4",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config:        map[string]any{"base_url": "https://tn.local", "credential_id": "missing"},
		}
		store.StatusByID[missingCollector.ID] = missingCollector
		sut.CollectorsDeps().ExecuteTrueNASCollector(context.Background(), missingCollector)
		updated, _, _ := store.GetHubCollector(missingCollector.ID)
		if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "credential not found") {
			t.Fatalf("unexpected missing credential status: %+v", updated)
		}

		const badCipherID = "cred-truenas-bad-cipher"
		_, err := sut.CredentialStore.CreateCredentialProfile(credentials.Profile{
			ID:               badCipherID,
			Name:             "bad cipher",
			Kind:             credentials.KindTrueNASAPIKey,
			Status:           "active",
			SecretCiphertext: "not-valid-ciphertext",
			Metadata:         map[string]string{"base_url": "https://tn.local"},
			CreatedAt:        time.Now().UTC(),
			UpdatedAt:        time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("create bad cipher profile: %v", err)
		}

		decryptCollector := hubcollector.Collector{
			ID:            "collector-truenas-5",
			AssetID:       "truenas-cluster-5",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config:        map[string]any{"base_url": "https://tn.local", "credential_id": badCipherID},
		}
		store.StatusByID[decryptCollector.ID] = decryptCollector
		sut.CollectorsDeps().ExecuteTrueNASCollector(context.Background(), decryptCollector)
		updated, _, _ = store.GetHubCollector(decryptCollector.ID)
		if updated.LastStatus != "error" || !strings.Contains(updated.LastError, "failed to decrypt credential") {
			t.Fatalf("unexpected decrypt error status: %+v", updated)
		}
	})
}

func TestExecuteTrueNASCollectorStatusErrorWhenHubCollectorStoreUnavailable(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	sut := hubdeps.New(t)
	sut.HubCollectorStore = nil
	sut.CollectorsDeps().ExecuteTrueNASCollector(context.Background(), hubcollector.Collector{
		ID:            "collector-truenas-no-store",
		AssetID:       "truenas-cluster-no-store",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
		Config:        map[string]any{},
	})
	// No panic is the assertion; status update is best-effort when store unavailable.
}
