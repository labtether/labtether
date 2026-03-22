package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/dependencies"
	"github.com/labtether/labtether/internal/incidents"
)

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

func TestBestRunsOnIdentityTargetWithPriorityPrefersTrueNAS(t *testing.T) {
	source := assets.Asset{
		ID:     "docker-host-1",
		Type:   "container-host",
		Name:   "docker-host-1",
		Source: "docker",
		Metadata: map[string]string{
			"endpoint_ip": "10.0.0.25",
		},
	}
	proxmoxTarget := assets.Asset{
		ID:     "proxmox-vm-101",
		Type:   "vm",
		Name:   "proxmox-vm-101",
		Source: "proxmox",
		Metadata: map[string]string{
			"node_ip": "10.0.0.25",
		},
	}
	trueNASTarget := assets.Asset{
		ID:     "truenas-host-omeganas",
		Type:   "nas",
		Name:   "omeganas",
		Source: "truenas",
		Metadata: map[string]string{
			"ip": "10.0.0.25",
		},
	}

	identities := map[string]collectorIdentity{
		source.ID:        collectCollectorIdentity(source),
		proxmoxTarget.ID: collectCollectorIdentity(proxmoxTarget),
		trueNASTarget.ID: collectCollectorIdentity(trueNASTarget),
	}

	if targetID, reason, ok := bestRunsOnIdentityTarget(source, []assets.Asset{proxmoxTarget, trueNASTarget}, identities); ok {
		t.Fatalf("expected non-priority match to remain ambiguous, got target=%q reason=%q", targetID, reason)
	}

	targetID, reason, ok := bestRunsOnIdentityTargetWithPriority(source, []assets.Asset{proxmoxTarget, trueNASTarget}, identities, dockerInfraTargetPriority)
	if !ok {
		t.Fatalf("expected priority-based identity match")
	}
	if targetID != trueNASTarget.ID {
		t.Fatalf("bestRunsOnIdentityTargetWithPriority() target = %q, want %q", targetID, trueNASTarget.ID)
	}
	if reason != "ip" {
		t.Fatalf("bestRunsOnIdentityTargetWithPriority() reason = %q, want ip", reason)
	}
}

func TestAutoLinkDockerHostsToInfraRemovesReverseAutoEdge(t *testing.T) {
	sut := newTestAPIServer(t)
	deps := newStubDependencyStore()
	sut.dependencyStore = deps

	assetsToUpsert := []assets.HeartbeatRequest{
		{
			AssetID: "docker-host-1",
			Type:    "container-host",
			Name:    "docker-host-1",
			Source:  "docker",
			Metadata: map[string]string{
				"endpoint_ip": "10.0.0.25",
			},
		},
		{
			AssetID: "truenas-host-omeganas",
			Type:    "nas",
			Name:    "omeganas",
			Source:  "truenas",
			Metadata: map[string]string{
				"collector_endpoint_ip": "10.0.0.25",
			},
		},
		{
			AssetID: "proxmox-vm-101",
			Type:    "vm",
			Name:    "proxmox-vm-101",
			Source:  "proxmox",
			Metadata: map[string]string{
				"node_ip": "10.0.0.25",
			},
		},
	}
	for _, req := range assetsToUpsert {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatalf("failed to upsert test asset %q: %v", req.AssetID, err)
		}
	}

	if _, err := deps.CreateAssetDependency(dependencies.CreateDependencyRequest{
		SourceAssetID:    "truenas-host-omeganas",
		TargetAssetID:    "docker-host-1",
		RelationshipType: dependencies.RelationshipRunsOn,
		Direction:        dependencies.DirectionDownstream,
		Criticality:      dependencies.CriticalityMedium,
		Metadata: map[string]string{
			"binding": "auto",
		},
	}); err != nil {
		t.Fatalf("failed to seed reverse auto dependency: %v", err)
	}

	if err := sut.autoLinkDockerHostsToInfra(); err != nil {
		t.Fatalf("autoLinkDockerHostsToInfra() error = %v", err)
	}

	current, err := deps.ListAssetDependencies("docker-host-1", 50)
	if err != nil {
		t.Fatalf("ListAssetDependencies() error = %v", err)
	}

	foundForward := false
	for _, dep := range current {
		if dep.RelationshipType != dependencies.RelationshipRunsOn {
			continue
		}
		if dep.SourceAssetID == "truenas-host-omeganas" && dep.TargetAssetID == "docker-host-1" {
			t.Fatalf("reverse auto runs_on edge still present after relink")
		}
		if dep.SourceAssetID == "docker-host-1" {
			foundForward = true
			if dep.TargetAssetID != "truenas-host-omeganas" {
				t.Fatalf("docker runs_on target = %q, want truenas-host-omeganas", dep.TargetAssetID)
			}
			if dep.Metadata["binding"] != "auto" {
				t.Fatalf("expected auto binding metadata, got %q", dep.Metadata["binding"])
			}
			if dep.Metadata["source"] != "hub_collector_identity" {
				t.Fatalf("expected auto-link source metadata, got %q", dep.Metadata["source"])
			}
		}
	}

	if !foundForward {
		t.Fatalf("expected forward docker runs_on dependency to be created")
	}
}

func TestAutoLinkDockerHostsToAgentHost(t *testing.T) {
	sut := newTestAPIServer(t)
	deps := newStubDependencyStore()
	sut.dependencyStore = deps

	// Create an agent host (like containervm-deltaserver reported by agent).
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "containervm-deltaserver",
		Type:    "host",
		Name:    "containervm-deltaserver",
		Source:  "agent",
	}); err != nil {
		t.Fatalf("failed to upsert agent host: %v", err)
	}

	// Create a Docker container-host (auto-discovered, agent_id links to the agent).
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "docker-host-containervm-deltaserver",
		Type:    "container-host",
		Name:    "docker-containervm-deltaserver",
		Source:  "docker",
		Metadata: map[string]string{
			"agent_id": "containervm-deltaserver",
		},
	}); err != nil {
		t.Fatalf("failed to upsert docker host: %v", err)
	}

	if err := sut.autoLinkDockerHostsToInfra(); err != nil {
		t.Fatalf("autoLinkDockerHostsToInfra() error = %v", err)
	}

	current, err := deps.ListAssetDependencies("docker-host-containervm-deltaserver", 50)
	if err != nil {
		t.Fatalf("ListAssetDependencies() error = %v", err)
	}

	foundLink := false
	for _, dep := range current {
		if dep.SourceAssetID == "docker-host-containervm-deltaserver" &&
			dep.TargetAssetID == "containervm-deltaserver" &&
			dep.RelationshipType == dependencies.RelationshipRunsOn {
			foundLink = true
			if dep.Metadata["binding"] != "auto" {
				t.Fatalf("expected auto binding, got %q", dep.Metadata["binding"])
			}
			if dep.Metadata["match_reason"] != "agent_id" {
				t.Fatalf("expected match_reason=agent_id, got %q", dep.Metadata["match_reason"])
			}
		}
	}

	if !foundLink {
		t.Fatalf("expected docker-host runs_on agent host dependency, none found")
	}
}

func TestAutoLinkDockerPrefersAgentHostOverIdentityMatch(t *testing.T) {
	sut := newTestAPIServer(t)
	deps := newStubDependencyStore()
	sut.dependencyStore = deps

	// Agent host and Proxmox VM both share the same IP.
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "my-vm",
		Type:    "host",
		Name:    "my-vm",
		Source:  "agent",
		Metadata: map[string]string{
			"ip": "10.0.0.50",
		},
	}); err != nil {
		t.Fatalf("failed to upsert agent host: %v", err)
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-vm-200",
		Type:    "vm",
		Name:    "proxmox-vm-200",
		Source:  "proxmox",
		Metadata: map[string]string{
			"node_ip": "10.0.0.50",
		},
	}); err != nil {
		t.Fatalf("failed to upsert proxmox vm: %v", err)
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "docker-host-my-vm",
		Type:    "container-host",
		Name:    "docker-my-vm",
		Source:  "docker",
		Metadata: map[string]string{
			"agent_id": "my-vm",
		},
	}); err != nil {
		t.Fatalf("failed to upsert docker host: %v", err)
	}

	if err := sut.autoLinkDockerHostsToInfra(); err != nil {
		t.Fatalf("autoLinkDockerHostsToInfra() error = %v", err)
	}

	current, err := deps.ListAssetDependencies("docker-host-my-vm", 50)
	if err != nil {
		t.Fatalf("ListAssetDependencies() error = %v", err)
	}

	for _, dep := range current {
		if dep.SourceAssetID == "docker-host-my-vm" &&
			dep.RelationshipType == dependencies.RelationshipRunsOn {
			// agent_id strongest path should pick agent host, not proxmox.
			if dep.TargetAssetID != "my-vm" {
				t.Fatalf("expected docker runs_on target=my-vm (agent host), got %q", dep.TargetAssetID)
			}
			return
		}
	}
	t.Fatalf("expected docker-host runs_on dependency, none found")
}

func TestAutoLinkDockerContainersToHosts(t *testing.T) {
	sut := newTestAPIServer(t)
	deps := newStubDependencyStore()
	sut.dependencyStore = deps

	// Create a Docker container-host.
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "docker-host-myserver",
		Type:    "container-host",
		Name:    "docker-myserver",
		Source:  "docker",
		Metadata: map[string]string{
			"agent_id": "myserver",
		},
	}); err != nil {
		t.Fatalf("failed to upsert docker host: %v", err)
	}

	// Create Docker containers under that host.
	for _, ct := range []struct{ id, name string }{
		{"docker-ct-myserver-aaa", "nginx"},
		{"docker-ct-myserver-bbb", "postgres"},
	} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: ct.id,
			Type:    "docker-container",
			Name:    ct.name,
			Source:  "docker",
			Metadata: map[string]string{
				"agent_id": "myserver",
			},
		}); err != nil {
			t.Fatalf("failed to upsert container %s: %v", ct.id, err)
		}
	}

	// Create a compose stack under that host.
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "docker-stack-myserver-web",
		Type:    "compose-stack",
		Name:    "web",
		Source:  "docker",
		Metadata: map[string]string{
			"agent_id": "myserver",
		},
	}); err != nil {
		t.Fatalf("failed to upsert stack: %v", err)
	}

	if err := sut.autoLinkDockerContainersToHosts(); err != nil {
		t.Fatalf("autoLinkDockerContainersToHosts() error = %v", err)
	}

	// Verify all three children have hosted_on dependencies to the host.
	for _, childID := range []string{
		"docker-ct-myserver-aaa",
		"docker-ct-myserver-bbb",
		"docker-stack-myserver-web",
	} {
		childDeps, err := deps.ListAssetDependencies(childID, 50)
		if err != nil {
			t.Fatalf("ListAssetDependencies(%s) error = %v", childID, err)
		}
		found := false
		for _, dep := range childDeps {
			if dep.SourceAssetID == childID &&
				dep.TargetAssetID == "docker-host-myserver" &&
				dep.RelationshipType == dependencies.RelationshipHostedOn {
				found = true
				if dep.Metadata["binding"] != "auto" {
					t.Fatalf("%s: expected auto binding, got %q", childID, dep.Metadata["binding"])
				}
			}
		}
		if !found {
			t.Fatalf("expected %s hosted_on docker-host-myserver, not found", childID)
		}
	}
}

func TestAutoLinkDockerContainersSkipsDuplicates(t *testing.T) {
	sut := newTestAPIServer(t)
	deps := newStubDependencyStore()
	sut.dependencyStore = deps

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "docker-host-srv",
		Type:    "container-host",
		Name:    "docker-srv",
		Source:  "docker",
		Metadata: map[string]string{"agent_id": "srv"},
	}); err != nil {
		t.Fatalf("failed to upsert docker host: %v", err)
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "docker-ct-srv-aaa",
		Type:    "docker-container",
		Name:    "app",
		Source:  "docker",
		Metadata: map[string]string{"agent_id": "srv"},
	}); err != nil {
		t.Fatalf("failed to upsert container: %v", err)
	}

	// Run twice — second run should not error on duplicate.
	if err := sut.autoLinkDockerContainersToHosts(); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	if err := sut.autoLinkDockerContainersToHosts(); err != nil {
		t.Fatalf("second run error = %v", err)
	}

	allDeps, err := deps.ListAssetDependencies("docker-ct-srv-aaa", 50)
	if err != nil {
		t.Fatalf("ListAssetDependencies() error = %v", err)
	}

	hostedOnCount := 0
	for _, dep := range allDeps {
		if dep.RelationshipType == dependencies.RelationshipHostedOn {
			hostedOnCount++
		}
	}
	if hostedOnCount != 1 {
		t.Fatalf("expected exactly 1 hosted_on dependency, got %d", hostedOnCount)
	}
}
