package collectors

import (
	"log"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/dependencies"
)

func (d *Deps) autoLinkRunsOnByIdentity(
	sourceFilter func(asset assets.Asset) bool,
	targetFilter func(asset assets.Asset) bool,
) error {
	if d.AssetStore == nil || d.DependencyStore == nil {
		return nil
	}

	allAssets, err := d.AssetStore.ListAssets()
	if err != nil {
		return err
	}

	sources := make([]assets.Asset, 0, 8)
	targets := make([]assets.Asset, 0, 16)
	identities := make(map[string]CollectorIdentity, len(allAssets))
	for _, asset := range allAssets {
		if sourceFilter(asset) {
			sources = append(sources, asset)
		}
		if targetFilter(asset) {
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
		targetID, reason, ok := BestRunsOnIdentityTarget(source, targets, identities)
		if !ok {
			continue
		}
		if err := d.upsertAutoRunsOnDependency(source.ID, targetID, reason); err != nil {
			log.Printf("hub collector: failed to upsert auto runs_on %s -> %s: %v", source.ID, targetID, err)
		}
	}

	return nil
}

func (d *Deps) upsertAutoRunsOnDependency(sourceID, targetID, matchReason string) error {
	deps, err := d.DependencyStore.ListAssetDependencies(sourceID, 200)
	if err != nil {
		return err
	}

	for _, dep := range deps {
		if dep.SourceAssetID != sourceID || dep.RelationshipType != dependencies.RelationshipRunsOn {
			continue
		}

		binding := strings.ToLower(strings.TrimSpace(dep.Metadata["binding"]))
		if binding == "manual" {
			return nil
		}
		if dep.TargetAssetID == targetID {
			return d.removeAutoReverseRunsOn(targetID, sourceID)
		}
		if binding == "auto" {
			if err := d.DependencyStore.DeleteAssetDependency(dep.ID); err != nil && err != dependencies.ErrDependencyNotFound {
				return err
			}
		}
	}

	_, err = d.DependencyStore.CreateAssetDependency(dependencies.CreateDependencyRequest{
		SourceAssetID:    sourceID,
		TargetAssetID:    targetID,
		RelationshipType: dependencies.RelationshipRunsOn,
		Direction:        dependencies.DirectionDownstream,
		Criticality:      dependencies.CriticalityMedium,
		Metadata: map[string]string{
			"binding":      "auto",
			"source":       "hub_collector_identity",
			"match_reason": strings.TrimSpace(matchReason),
		},
	})
	if err == dependencies.ErrDuplicateDependency {
		return d.removeAutoReverseRunsOn(targetID, sourceID)
	}
	if err != nil {
		return err
	}
	return d.removeAutoReverseRunsOn(targetID, sourceID)
}

func (d *Deps) removeAutoReverseRunsOn(sourceID, targetID string) error {
	deps, err := d.DependencyStore.ListAssetDependencies(sourceID, 200)
	if err != nil {
		return err
	}

	for _, dep := range deps {
		if dep.SourceAssetID != sourceID || dep.TargetAssetID != targetID {
			continue
		}
		if dep.RelationshipType != dependencies.RelationshipRunsOn {
			continue
		}
		if strings.ToLower(strings.TrimSpace(dep.Metadata["binding"])) != "auto" {
			continue
		}
		if err := d.DependencyStore.DeleteAssetDependency(dep.ID); err != nil && err != dependencies.ErrDependencyNotFound {
			return err
		}
	}
	return nil
}
