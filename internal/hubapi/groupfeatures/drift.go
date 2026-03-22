package groupfeatures

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupprofiles"
)

// RunGroupDriftChecker starts the background group drift checker loop.
// It ticks every 30 minutes and records drift check results via GroupProfileStore.
func (d *Deps) RunGroupDriftChecker(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	log.Printf("group drift checker started (interval=30m)")

	for {
		select {
		case <-ctx.Done():
			log.Printf("group drift checker stopped")
			return
		case <-ticker.C:
			d.checkGroupDrift(ctx)
		}
	}
}

func (d *Deps) checkGroupDrift(ctx context.Context) {
	if d.GroupStore == nil || d.GroupProfileStore == nil {
		return
	}

	groupList, err := d.GroupStore.ListGroups()
	if err != nil {
		log.Printf("group drift checker: failed to list groups: %v", err)
		return
	}
	if len(groupList) == 0 {
		return
	}

	assetsByGroup := d.groupDriftAssetsByGroup()
	profilesByID := make(map[string]groupprofiles.Profile, len(groupList))
	missingProfiles := make(map[string]struct{}, 8)

	for _, groupEntry := range groupList {
		select {
		case <-ctx.Done():
			return
		default:
		}

		groupID := strings.TrimSpace(groupEntry.ID)
		if groupID == "" {
			continue
		}

		assignment, ok, err := d.GroupProfileStore.GetGroupProfileAssignment(groupID)
		if err != nil || !ok {
			continue
		}

		profileID := strings.TrimSpace(assignment.ProfileID)
		if profileID == "" {
			continue
		}

		profile, profileOK := profilesByID[profileID]
		if !profileOK {
			if _, missing := missingProfiles[profileID]; missing {
				continue
			}
			loadedProfile, found, loadErr := d.GroupProfileStore.GetGroupProfile(profileID)
			if loadErr != nil {
				log.Printf("group drift checker: failed to load profile %s: %v", profileID, loadErr)
				continue
			}
			if !found {
				missingProfiles[profileID] = struct{}{}
				continue
			}
			profilesByID[profileID] = loadedProfile
			profile = loadedProfile
		}

		driftStatus, driftDetails := ComputeGroupDrift(profile, assetsByGroup[groupID])

		check := groupprofiles.DriftCheck{
			GroupID:      groupID,
			ProfileID:    profile.ID,
			Status:       driftStatus,
			DriftDetails: driftDetails,
			CheckedAt:    time.Now().UTC(),
		}

		if _, err := d.GroupProfileStore.RecordDriftCheck(check); err != nil {
			log.Printf("group drift checker: failed to record drift for group %s: %v", groupID, err)
		}
	}
}

func (d *Deps) groupDriftAssetsByGroup() map[string][]assets.Asset {
	assetsByGroup := map[string][]assets.Asset{}
	if d.AssetStore == nil {
		return assetsByGroup
	}

	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		log.Printf("group drift checker: failed to list assets: %v", err)
		return assetsByGroup
	}

	assetsByGroup = make(map[string][]assets.Asset, len(assetList))
	for _, assetEntry := range assetList {
		groupID := strings.TrimSpace(assetEntry.GroupID)
		if groupID == "" {
			continue
		}
		assetsByGroup[groupID] = append(assetsByGroup[groupID], assetEntry)
	}
	return assetsByGroup
}

// ComputeGroupDrift compares a group's actual state against its profile expectations.
func ComputeGroupDrift(profile groupprofiles.Profile, groupAssets []assets.Asset) (string, map[string]any) {
	details := map[string]any{
		"checked_fields": 0,
		"drifted_fields": 0,
	}

	checkedFields := 0
	driftedFields := 0
	driftReasons := []string{}

	// Check expected_asset_count
	if raw, ok := profile.Config["expected_asset_count"]; ok {
		checkedFields++
		expected := toIntFromAny(raw)
		if expected > 0 && len(groupAssets) != expected {
			driftedFields++
			driftReasons = append(driftReasons, fmt.Sprintf("expected %d assets, found %d", expected, len(groupAssets)))
		}
	}

	// Check required_platforms
	if raw, ok := profile.Config["required_platforms"]; ok {
		checkedFields++
		var requiredPlatforms []string
		switch v := raw.(type) {
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					requiredPlatforms = append(requiredPlatforms, s)
				}
			}
		case []string:
			requiredPlatforms = v
		}

		if len(requiredPlatforms) > 0 {
			platformSet := map[string]struct{}{}
			for _, a := range groupAssets {
				platform := strings.ToLower(strings.TrimSpace(a.Platform))
				if platform != "" {
					platformSet[platform] = struct{}{}
				}
			}
			for _, required := range requiredPlatforms {
				requiredPlatform := strings.ToLower(strings.TrimSpace(required))
				if requiredPlatform == "" {
					continue
				}
				if _, ok := platformSet[requiredPlatform]; !ok {
					driftedFields++
					driftReasons = append(driftReasons, fmt.Sprintf("required platform %q not found", strings.TrimSpace(required)))
				}
			}
		}
	}

	// Check min_online_percent
	if raw, ok := profile.Config["min_online_percent"]; ok {
		checkedFields++
		minPct := toFloat64FromAny(raw)
		if minPct > 0 && len(groupAssets) > 0 {
			onlineCount := 0
			for _, a := range groupAssets {
				if strings.EqualFold(strings.TrimSpace(a.Status), "online") {
					onlineCount++
				}
			}
			actualPct := float64(onlineCount) / float64(len(groupAssets)) * 100.0
			if actualPct < minPct {
				driftedFields++
				driftReasons = append(driftReasons, fmt.Sprintf("online %.1f%% < required %.1f%%", actualPct, minPct))
			}
		}
	}

	details["checked_fields"] = checkedFields
	details["drifted_fields"] = driftedFields
	if len(driftReasons) > 0 {
		details["reasons"] = driftReasons
	}

	if driftedFields > 0 {
		return groupprofiles.DriftStatusDrifted, details
	}
	return groupprofiles.DriftStatusCompliant, details
}

func toIntFromAny(v any) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case string:
		n, _ := strconv.Atoi(val)
		return n
	default:
		return 0
	}
}

func toFloat64FromAny(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}
