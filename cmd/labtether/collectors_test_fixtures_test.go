package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/dependencies"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/incidents"
)

// Fixtures that cmd tests still use after the first collectors tests
// moved to internal/hubapi/collectors, copied verbatim from the files
// that moved. The moved tests keep their own copies (or call testutil).

// From connector_handlers_portainer_test.go.

func allowInsecureTransportForConnectorTests(t *testing.T) {
	t.Helper()
	testutil.AllowInsecureTransportForConnectorTests(t)
}

type mockPortainerConnector struct {
	discoverFn func(ctx context.Context) ([]connectorsdk.Asset, error)
	testFn     func(ctx context.Context) (connectorsdk.Health, error)
	actionsFn  func() []connectorsdk.ActionDescriptor
	executeFn  func(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error)
}

func (m *mockPortainerConnector) ID() string { return "portainer" }

func (m *mockPortainerConnector) DisplayName() string { return "Portainer (mock)" }

func (m *mockPortainerConnector) Capabilities() connectorsdk.Capabilities {
	return connectorsdk.Capabilities{
		DiscoverAssets: true,
		CollectMetrics: true,
		CollectEvents:  true,
		ExecuteActions: true,
	}
}

func (m *mockPortainerConnector) Discover(ctx context.Context) ([]connectorsdk.Asset, error) {
	if m.discoverFn != nil {
		return m.discoverFn(ctx)
	}
	return nil, nil
}

func (m *mockPortainerConnector) TestConnection(ctx context.Context) (connectorsdk.Health, error) {
	if m.testFn != nil {
		return m.testFn(ctx)
	}
	return connectorsdk.Health{Status: "ok", Message: "ok"}, nil
}

func (m *mockPortainerConnector) Actions() []connectorsdk.ActionDescriptor {
	if m.actionsFn != nil {
		return m.actionsFn()
	}
	return nil
}

func (m *mockPortainerConnector) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, actionID, req)
	}
	return connectorsdk.ActionResult{Status: "ok", Message: "ok"}, nil
}

func seedPortainerCredentialProfile(t *testing.T, sut *apiServer, username, secret, baseURL string) string {
	t.Helper()

	const credentialID = "cred-portainer-test"
	secretCiphertext, err := sut.secretsManager.EncryptString(secret, credentialID)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}

	_, err = sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               credentialID,
		Name:             "Portainer Test Credential",
		Kind:             credentials.KindPortainerAPIKey,
		Username:         username,
		SecretCiphertext: secretCiphertext,
		Metadata: map[string]string{
			"base_url": baseURL,
		},
	})
	if err != nil {
		t.Fatalf("create credential profile: %v", err)
	}

	return credentialID
}

func assertErrorBodyContains(t *testing.T, body []byte, want string) {
	t.Helper()
	testutil.AssertErrorBodyContains(t, body, want)
}

// From connector_handlers_homeassistant_test.go.

func createHomeAssistantCredentialProfile(t *testing.T, sut *apiServer, credentialID, secret, baseURL string) {
	t.Helper()

	secretCiphertext, err := sut.secretsManager.EncryptString(secret, credentialID)
	if err != nil {
		t.Fatalf("failed to encrypt home assistant credential: %v", err)
	}
	_, err = sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               credentialID,
		Name:             "homeassistant " + credentialID,
		Kind:             credentials.KindHomeAssistantToken,
		Status:           "active",
		SecretCiphertext: secretCiphertext,
		Metadata:         map[string]string{"base_url": baseURL},
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to store home assistant credential profile: %v", err)
	}
}

// From connector_handlers_pbs_test.go.

func createPBSCredentialProfile(t *testing.T, sut *apiServer, credentialID, username, secret, baseURL string) {
	t.Helper()
	testutil.CreatePBSCredentialProfile(t, sut.credentialStore, sut.secretsManager, credentialID, username, secret, baseURL)
}

// From hub_collector_runner_autolink_test.go.

type stubDependencyStore struct {
	mu    sync.Mutex
	deps  map[string]dependencies.Dependency
	next  int
	links map[string]incidents.IncidentAsset
}

func newStubDependencyStore() *stubDependencyStore {
	return &stubDependencyStore{
		deps:  make(map[string]dependencies.Dependency),
		links: make(map[string]incidents.IncidentAsset),
	}
}

func (s *stubDependencyStore) CreateAssetDependency(req dependencies.CreateDependencyRequest) (dependencies.Dependency, error) {
	sourceID := strings.TrimSpace(req.SourceAssetID)
	targetID := strings.TrimSpace(req.TargetAssetID)
	relType := strings.TrimSpace(req.RelationshipType)
	if sourceID == targetID {
		return dependencies.Dependency{}, dependencies.ErrSelfReference
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, dep := range s.deps {
		if dep.SourceAssetID == sourceID && dep.TargetAssetID == targetID && dep.RelationshipType == relType {
			return dependencies.Dependency{}, dependencies.ErrDuplicateDependency
		}
	}

	s.next++
	now := time.Now().UTC()
	dep := dependencies.Dependency{
		ID:               fmt.Sprintf("dep-%d", s.next),
		SourceAssetID:    sourceID,
		TargetAssetID:    targetID,
		RelationshipType: relType,
		Direction:        dependencies.NormalizeDirection(req.Direction),
		Criticality:      dependencies.NormalizeCriticality(req.Criticality),
		Metadata:         cloneMap(req.Metadata),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if dep.Direction == "" {
		dep.Direction = dependencies.DirectionDownstream
	}
	if dep.Criticality == "" {
		dep.Criticality = dependencies.CriticalityMedium
	}
	s.deps[dep.ID] = dep
	return dep, nil
}

func (s *stubDependencyStore) ListAssetDependencies(assetID string, limit int) ([]dependencies.Dependency, error) {
	if limit <= 0 {
		limit = 50
	}
	assetID = strings.TrimSpace(assetID)

	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]dependencies.Dependency, 0, len(s.deps))
	for _, dep := range s.deps {
		if dep.SourceAssetID == assetID || dep.TargetAssetID == assetID {
			copied := dep
			copied.Metadata = cloneMap(dep.Metadata)
			out = append(out, copied)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *stubDependencyStore) GetAssetDependency(id string) (dependencies.Dependency, bool, error) {
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()

	dep, ok := s.deps[id]
	if !ok {
		return dependencies.Dependency{}, false, nil
	}
	dep.Metadata = cloneMap(dep.Metadata)
	return dep, true, nil
}

func (s *stubDependencyStore) DeleteAssetDependency(id string) error {
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.deps[id]; !ok {
		return dependencies.ErrDependencyNotFound
	}
	delete(s.deps, id)
	return nil
}

func (s *stubDependencyStore) BlastRadius(assetID string, maxDepth int) ([]dependencies.ImpactNode, error) {
	return nil, nil
}

func (s *stubDependencyStore) UpstreamCauses(assetID string, maxDepth int) ([]dependencies.ImpactNode, error) {
	return nil, nil
}

func (s *stubDependencyStore) LinkIncidentAsset(incidentID string, req incidents.LinkAssetRequest) (incidents.IncidentAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.next++
	link := incidents.IncidentAsset{
		ID:         fmt.Sprintf("ia-%d", s.next),
		IncidentID: strings.TrimSpace(incidentID),
		AssetID:    strings.TrimSpace(req.AssetID),
		Role:       incidents.NormalizeAssetRole(req.Role),
		CreatedAt:  time.Now().UTC(),
	}
	s.links[link.ID] = link
	return link, nil
}

func (s *stubDependencyStore) ListIncidentAssets(incidentID string, limit int) ([]incidents.IncidentAsset, error) {
	if limit <= 0 {
		limit = 50
	}
	incidentID = strings.TrimSpace(incidentID)

	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]incidents.IncidentAsset, 0, len(s.links))
	for _, link := range s.links {
		if link.IncidentID == incidentID {
			out = append(out, link)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *stubDependencyStore) UnlinkIncidentAsset(incidentID, linkID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.links, strings.TrimSpace(linkID))
	return nil
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
