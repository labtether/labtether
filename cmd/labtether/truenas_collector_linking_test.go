package main

import (
	"github.com/labtether/labtether/internal/assets"
	"testing"
)

func TestTrueNASAutoLinkWrappers(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	deps := newStubDependencyStore()
	sut.dependencyStore = deps

	assetsToUpsert := []assets.HeartbeatRequest{
		{
			AssetID: "portainer-host-1",
			Type:    "container-host",
			Name:    "portainer-host",
			Source:  "portainer",
			Metadata: map[string]string{
				"endpoint_ip": "10.0.0.25",
			},
		},
		{
			AssetID: "truenas-host-omeganas",
			Type:    "nas",
			Name:    "OmegaNAS",
			Source:  "truenas",
			Metadata: map[string]string{
				"collector_endpoint_ip": "10.0.0.25",
				"hostname":              "omeganas",
			},
		},
		{
			AssetID: "proxmox-vm-101",
			Type:    "vm",
			Name:    "OmegaNAS",
			Source:  "proxmox",
			Metadata: map[string]string{
				"node_ip": "10.0.0.25",
			},
		},
	}
	for _, req := range assetsToUpsert {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatalf("failed to upsert %s: %v", req.AssetID, err)
		}
	}

	if err := sut.autoLinkPortainerHostsToTrueNASHosts(); err != nil {
		t.Fatalf("autoLinkPortainerHostsToTrueNASHosts() error = %v", err)
	}
	if err := sut.autoLinkTrueNASHostsToProxmoxGuests(); err != nil {
		t.Fatalf("autoLinkTrueNASHostsToProxmoxGuests() error = %v", err)
	}

	portainerDeps, err := deps.ListAssetDependencies("portainer-host-1", 20)
	if err != nil {
		t.Fatalf("ListAssetDependencies(portainer) error = %v", err)
	}
	foundPortainerLink := false
	for _, dep := range portainerDeps {
		if dep.SourceAssetID == "portainer-host-1" && dep.TargetAssetID == "truenas-host-omeganas" {
			foundPortainerLink = true
			break
		}
	}
	if !foundPortainerLink {
		t.Fatalf("expected portainer -> truenas runs_on link")
	}

	truenasDeps, err := deps.ListAssetDependencies("truenas-host-omeganas", 20)
	if err != nil {
		t.Fatalf("ListAssetDependencies(truenas) error = %v", err)
	}
	foundTrueNASLink := false
	for _, dep := range truenasDeps {
		if dep.SourceAssetID == "truenas-host-omeganas" && dep.TargetAssetID == "proxmox-vm-101" {
			foundTrueNASLink = true
			break
		}
	}
	if !foundTrueNASLink {
		t.Fatalf("expected truenas -> proxmox runs_on link")
	}
}
