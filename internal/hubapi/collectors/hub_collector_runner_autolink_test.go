package collectors_test

import (
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/dependencies"
	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
)

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
		ID:     "truenas-host-vaultnas",
		Type:   "nas",
		Name:   "vaultnas",
		Source: "truenas",
		Metadata: map[string]string{
			"ip": "10.0.0.25",
		},
	}

	identities := map[string]collectorspkg.CollectorIdentity{
		source.ID:        collectorspkg.CollectCollectorIdentity(source),
		proxmoxTarget.ID: collectorspkg.CollectCollectorIdentity(proxmoxTarget),
		trueNASTarget.ID: collectorspkg.CollectCollectorIdentity(trueNASTarget),
	}

	if targetID, reason, ok := collectorspkg.BestRunsOnIdentityTarget(source, []assets.Asset{proxmoxTarget, trueNASTarget}, identities); ok {
		t.Fatalf("expected non-priority match to remain ambiguous, got target=%q reason=%q", targetID, reason)
	}

	targetID, reason, ok := collectorspkg.BestRunsOnIdentityTargetWithPriority(source, []assets.Asset{proxmoxTarget, trueNASTarget}, identities, collectorspkg.DockerInfraTargetPriority)
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
	sut := hubdeps.New(t)
	deps := newStubDependencyStore()
	sut.DependencyStore = deps

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
			AssetID: "truenas-host-vaultnas",
			Type:    "nas",
			Name:    "vaultnas",
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
		if _, err := sut.AssetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatalf("failed to upsert test asset %q: %v", req.AssetID, err)
		}
	}

	if _, err := deps.CreateAssetDependency(dependencies.CreateDependencyRequest{
		SourceAssetID:    "truenas-host-vaultnas",
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

	if err := sut.CollectorsDeps().AutoLinkDockerHostsToInfra(); err != nil {
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
		if dep.SourceAssetID == "truenas-host-vaultnas" && dep.TargetAssetID == "docker-host-1" {
			t.Fatalf("reverse auto runs_on edge still present after relink")
		}
		if dep.SourceAssetID == "docker-host-1" {
			foundForward = true
			if dep.TargetAssetID != "truenas-host-vaultnas" {
				t.Fatalf("docker runs_on target = %q, want truenas-host-vaultnas", dep.TargetAssetID)
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
	sut := hubdeps.New(t)
	deps := newStubDependencyStore()
	sut.DependencyStore = deps

	// Create an agent host (like dockerhost1-northserver reported by agent).
	if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "dockerhost1-northserver",
		Type:    "host",
		Name:    "dockerhost1-northserver",
		Source:  "agent",
	}); err != nil {
		t.Fatalf("failed to upsert agent host: %v", err)
	}

	// Create a Docker container-host (auto-discovered, agent_id links to the agent).
	if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "docker-host-dockerhost1-northserver",
		Type:    "container-host",
		Name:    "docker-dockerhost1-northserver",
		Source:  "docker",
		Metadata: map[string]string{
			"agent_id": "dockerhost1-northserver",
		},
	}); err != nil {
		t.Fatalf("failed to upsert docker host: %v", err)
	}

	if err := sut.CollectorsDeps().AutoLinkDockerHostsToInfra(); err != nil {
		t.Fatalf("autoLinkDockerHostsToInfra() error = %v", err)
	}

	current, err := deps.ListAssetDependencies("docker-host-dockerhost1-northserver", 50)
	if err != nil {
		t.Fatalf("ListAssetDependencies() error = %v", err)
	}

	foundLink := false
	for _, dep := range current {
		if dep.SourceAssetID == "docker-host-dockerhost1-northserver" &&
			dep.TargetAssetID == "dockerhost1-northserver" &&
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
	sut := hubdeps.New(t)
	deps := newStubDependencyStore()
	sut.DependencyStore = deps

	// Agent host and Proxmox VM both share the same IP.
	if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
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
	if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
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
	if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
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

	if err := sut.CollectorsDeps().AutoLinkDockerHostsToInfra(); err != nil {
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
	sut := hubdeps.New(t)
	deps := newStubDependencyStore()
	sut.DependencyStore = deps

	// Create a Docker container-host.
	if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
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
		if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
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
	if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
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

	if err := sut.CollectorsDeps().AutoLinkDockerContainersToHosts(); err != nil {
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
	sut := hubdeps.New(t)
	deps := newStubDependencyStore()
	sut.DependencyStore = deps

	if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "docker-host-srv",
		Type:     "container-host",
		Name:     "docker-srv",
		Source:   "docker",
		Metadata: map[string]string{"agent_id": "srv"},
	}); err != nil {
		t.Fatalf("failed to upsert docker host: %v", err)
	}
	if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "docker-ct-srv-aaa",
		Type:     "docker-container",
		Name:     "app",
		Source:   "docker",
		Metadata: map[string]string{"agent_id": "srv"},
	}); err != nil {
		t.Fatalf("failed to upsert container: %v", err)
	}

	// Run twice — second run should not error on duplicate.
	if err := sut.CollectorsDeps().AutoLinkDockerContainersToHosts(); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	if err := sut.CollectorsDeps().AutoLinkDockerContainersToHosts(); err != nil {
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
