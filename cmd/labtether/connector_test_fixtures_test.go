package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubcollector"
)

// Connector test fixtures that cmd tests still use after the Proxmox tests
// moved to internal/hubapi/proxmox. The stub store stays here because about
// 50 cmd struct literals set its unexported collectors field; testutil has
// the exported copy (StubHubCollectorStore) for package tests.

type stubHubCollectorStore struct {
	collectors []hubcollector.Collector
}

func (s *stubHubCollectorStore) CreateHubCollector(req hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *stubHubCollectorStore) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	for _, collector := range s.collectors {
		if collector.ID == id {
			return collector, true, nil
		}
	}
	return hubcollector.Collector{}, false, nil
}

func (s *stubHubCollectorStore) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	result := make([]hubcollector.Collector, 0, len(s.collectors))
	for _, collector := range s.collectors {
		if enabledOnly && !collector.Enabled {
			continue
		}
		result = append(result, collector)
	}
	return result, nil
}

func (s *stubHubCollectorStore) UpdateHubCollector(id string, req hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *stubHubCollectorStore) DeleteHubCollector(id string) error {
	return fmt.Errorf("not implemented")
}

func (s *stubHubCollectorStore) UpdateHubCollectorStatus(id, status, lastError string, collectedAt time.Time) error {
	return nil
}

func createProxmoxCredentialProfile(t *testing.T, sut *apiServer, credentialID, username, secret, baseURL string) {
	t.Helper()
	testutil.CreateProxmoxCredentialProfile(t, sut.credentialStore, sut.secretsManager, credentialID, username, secret, baseURL)
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

func newWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()
	return testutil.NewWebSocketPair(t)
}
