package collectors

import (
	"github.com/labtether/labtether/internal/assets"
)

type CollectorIdentity struct {
	IPs       map[string]struct{}
	Hostnames map[string]struct{}
}

func BestRunsOnIdentityTarget(
	source assets.Asset,
	targets []assets.Asset,
	identities map[string]CollectorIdentity,
) (string, string, bool) {
	return BestRunsOnIdentityTargetWithPriority(source, targets, identities, nil)
}

func BestRunsOnIdentityTargetWithPriority(
	source assets.Asset,
	targets []assets.Asset,
	identities map[string]CollectorIdentity,
	priority func(candidate assets.Asset) int,
) (string, string, bool) {
	sourceIdentity := identities[source.ID]

	bestTargetID := ""
	bestReason := ""
	bestScore := 0
	bestPriority := 0
	tied := false

	for _, candidate := range targets {
		if candidate.ID == source.ID {
			continue
		}
		if source.GroupID != "" && candidate.GroupID != "" && source.GroupID != candidate.GroupID {
			continue
		}

		score, reason := identityMatchScore(sourceIdentity, identities[candidate.ID])
		if score <= 0 {
			continue
		}

		candidatePriority := 0
		if priority != nil {
			candidatePriority = priority(candidate)
		}

		if score > bestScore {
			bestScore = score
			bestTargetID = candidate.ID
			bestReason = reason
			bestPriority = candidatePriority
			tied = false
			continue
		}
		if score == bestScore {
			if candidatePriority < bestPriority {
				bestTargetID = candidate.ID
				bestReason = reason
				bestPriority = candidatePriority
				tied = false
				continue
			}
			if candidatePriority == bestPriority {
				tied = true
			}
		}
	}

	if bestTargetID == "" || tied {
		return "", "", false
	}
	return bestTargetID, bestReason, true
}

func identityMatchScore(source, candidate CollectorIdentity) (int, string) {
	ipMatches := OverlapIdentity(source.IPs, candidate.IPs)
	hostMatches := OverlapIdentity(source.Hostnames, candidate.Hostnames)
	if ipMatches > 0 {
		return (ipMatches * 100) + (hostMatches * 10), "ip"
	}
	if hostMatches > 0 {
		return hostMatches * 10, "hostname"
	}
	return 0, ""
}

func OverlapIdentity(left, right map[string]struct{}) int {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	count := 0
	for value := range left {
		if _, ok := right[value]; ok {
			count++
		}
	}
	return count
}
