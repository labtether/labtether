package collectors

import (
	"log"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/dependencies"
)

func (d *Deps) AutoLinkDockerHostsToInfra() error {
	if d.AssetStore == nil || d.DependencyStore == nil {
		return nil
	}

	allAssets, err := d.AssetStore.ListAssets()
	if err != nil {
		return err
	}

	assetByID := make(map[string]assets.Asset, len(allAssets))
	sources := make([]assets.Asset, 0, 8)
	targets := make([]assets.Asset, 0, 24)
	identities := make(map[string]CollectorIdentity, len(allAssets))

	for _, asset := range allAssets {
		assetByID[asset.ID] = asset
		if asset.Source == "docker" && asset.Type == "container-host" {
			sources = append(sources, asset)
		}
		if isDockerInfraTarget(asset) {
			targets = append(targets, asset)
		}
	}
	if len(sources) == 0 || len(targets) == 0 {
		return nil
	}

	for _, asset := range sources {
		identities[asset.ID] = CollectCollectorIdentity(asset)
	}
	for _, asset := range targets {
		if _, exists := identities[asset.ID]; !exists {
			identities[asset.ID] = CollectCollectorIdentity(asset)
		}
	}

	for _, source := range sources {
		// Strongest path: agent_id directly references a known infra asset ID.
		if agentAssetID := strings.TrimSpace(source.Metadata["agent_id"]); agentAssetID != "" {
			if target, ok := assetByID[agentAssetID]; ok {
				if isDockerInfraTarget(target) {
					if err := d.upsertAutoRunsOnDependency(source.ID, target.ID, "agent_id"); err != nil {
						log.Printf("hub collector: failed to upsert docker agent_id runs_on %s -> %s: %v", source.ID, target.ID, err)
					}
					continue
				}
			}
		}

		targetID, reason, ok := BestRunsOnIdentityTargetWithPriority(source, targets, identities, DockerInfraTargetPriority)
		if !ok {
			continue
		}
		if err := d.upsertAutoRunsOnDependency(source.ID, targetID, reason); err != nil {
			log.Printf("hub collector: failed to upsert docker identity runs_on %s -> %s: %v", source.ID, targetID, err)
		}
	}

	return nil
}

// AutoLinkDockerContainersToHosts creates explicit hosted_on dependencies from
// Docker containers and compose stacks to their Docker container-host, matched
// by the agent_id metadata that the Docker connector sets on every child asset.
func (d *Deps) AutoLinkDockerContainersToHosts() error {
	if d.AssetStore == nil || d.DependencyStore == nil {
		return nil
	}

	allAssets, err := d.AssetStore.ListAssets()
	if err != nil {
		return err
	}

	// Index Docker container-hosts by their agent_id metadata.
	hostByAgentID := make(map[string]assets.Asset, 4)
	for _, asset := range allAssets {
		if asset.Source == "docker" && asset.Type == "container-host" {
			if agentID := strings.TrimSpace(asset.Metadata["agent_id"]); agentID != "" {
				hostByAgentID[agentID] = asset
			}
		}
	}
	if len(hostByAgentID) == 0 {
		return nil
	}

	for _, asset := range allAssets {
		if asset.Source != "docker" {
			continue
		}
		if asset.Type != "docker-container" && asset.Type != "compose-stack" {
			continue
		}
		agentID := strings.TrimSpace(asset.Metadata["agent_id"])
		if agentID == "" {
			continue
		}
		host, ok := hostByAgentID[agentID]
		if !ok {
			continue
		}

		_, err := d.DependencyStore.CreateAssetDependency(dependencies.CreateDependencyRequest{
			SourceAssetID:    asset.ID,
			TargetAssetID:    host.ID,
			RelationshipType: dependencies.RelationshipHostedOn,
			Direction:        dependencies.DirectionDownstream,
			Criticality:      dependencies.CriticalityLow,
			Metadata: map[string]string{
				"binding":      "auto",
				"source":       "hub_collector_docker",
				"match_reason": "agent_id",
			},
		})
		if err != nil && err != dependencies.ErrDuplicateDependency {
			log.Printf("hub collector docker: failed to link container %s to host %s: %v", asset.ID, host.ID, err)
		}
	}

	return nil
}

// isDockerInfraTarget returns true for assets that a Docker container-host
// can reasonably "run on": agent hosts, TrueNAS NAS nodes, and Proxmox guests.
func isDockerInfraTarget(asset assets.Asset) bool {
	if asset.Source == "agent" && asset.Type == "host" {
		return true
	}
	if asset.Source == "truenas" && asset.Type == "nas" {
		return true
	}
	if asset.Source == "proxmox" && (asset.Type == "vm" || asset.Type == "container") {
		return true
	}
	return false
}

func DockerInfraTargetPriority(candidate assets.Asset) int {
	// Agent host is highest priority: direct agent_id match is the
	// strongest signal that Docker runs on this machine.
	if candidate.Source == "agent" && candidate.Type == "host" {
		return 0
	}
	if candidate.Source == "truenas" && candidate.Type == "nas" {
		return 1
	}
	if candidate.Source == "proxmox" && (candidate.Type == "vm" || candidate.Type == "container") {
		return 2
	}
	return 10
}
