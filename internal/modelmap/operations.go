package modelmap

import (
	"sort"
	"strings"

	"github.com/labtether/labtether/internal/connectorsdk"
)

var operationAliasByActionID = map[string]string{
	"vm.start":               "workload.start",
	"ct.start":               "workload.start",
	"container.start":        "workload.start",
	"vm.stop":                "workload.stop",
	"ct.stop":                "workload.stop",
	"container.stop":         "workload.stop",
	"vm.shutdown":            "workload.shutdown",
	"ct.shutdown":            "workload.shutdown",
	"vm.reboot":              "workload.reboot",
	"ct.reboot":              "workload.reboot",
	"vm.suspend":             "workload.suspend",
	"vm.resume":              "workload.resume",
	"vm.force_stop":          "workload.force_stop",
	"ct.force_stop":          "workload.force_stop",
	"vm.migrate":             "workload.migrate",
	"ct.migrate":             "workload.migrate",
	"vm.clone":               "workload.clone",
	"ct.clone":               "workload.clone",
	"vm.clone_from_template": "workload.clone_from_template",
	"ct.clone_from_template": "workload.clone_from_template",
	"vm.disk_resize":         "workload.disk_resize",
	"vm.backup":              "backup.start",
	"ct.backup":              "backup.start",
	"vm.snapshot":            "snapshot.create",
	"ct.snapshot":            "snapshot.create",
	"snapshot.create":        "snapshot.create",
	"vm.snapshot.delete":     "snapshot.delete",
	"ct.snapshot.delete":     "snapshot.delete",
	"snapshot.delete":        "snapshot.delete",
	"vm.snapshot.rollback":   "snapshot.rollback",
	"ct.snapshot.rollback":   "snapshot.rollback",
	"snapshot.rollback":      "snapshot.rollback",
	"pool.scrub":             "storage.pool.scrub",
	"datastore.verify":       "storage.datastore.verify",
	"datastore.prune":        "storage.datastore.prune",
	"datastore.gc":           "storage.datastore.gc",
	"smart.test":             "disk.smart_test",
	"service.start":          "service.start",
	"service.stop":           "service.stop",
	"service.restart":        "service.restart",
	"app.start":              "app.start",
	"app.stop":               "app.stop",
	"app.restart":            "app.restart",
	"container.pause":        "container.pause",
	"container.unpause":      "container.unpause",
	"container.kill":         "container.kill",
	"container.remove":       "container.remove",
	"container.restart":      "container.restart",
	"image.pull":             "image.pull",
	"image.remove":           "image.remove",
	"stack.up":               "stack.up",
	"stack.down":             "stack.down",
	"stack.restart":          "stack.restart",
	"stack.pull":             "stack.pull",
	"stack.start":            "stack.start",
	"stack.stop":             "stack.stop",
	"stack.redeploy":         "stack.redeploy",
	"stack.remove":           "stack.remove",
	"system.reboot":          "system.reboot",
	"entity.toggle":          "automation.entity.toggle",
	"service.call":           "automation.service.call",
}

func CanonicalOperationID(actionID string) string {
	normalized := strings.ToLower(strings.TrimSpace(actionID))
	if normalized == "" {
		return ""
	}
	if canonical, ok := operationAliasByActionID[normalized]; ok {
		return canonical
	}
	return normalized
}

func CanonicalizeActionDescriptors(actions []connectorsdk.ActionDescriptor) []connectorsdk.ActionDescriptor {
	if len(actions) == 0 {
		return nil
	}
	out := make([]connectorsdk.ActionDescriptor, 0, len(actions))
	for _, action := range actions {
		copied := action
		copied.Parameters = cloneActionParameters(action.Parameters)
		if strings.TrimSpace(copied.CanonicalID) == "" {
			copied.CanonicalID = CanonicalOperationID(copied.ID)
		}
		out = append(out, copied)
	}
	return out
}

func ResolveActionID(actionID, targetID string, actions []connectorsdk.ActionDescriptor) string {
	requested := strings.TrimSpace(actionID)
	if requested == "" {
		return ""
	}

	for _, action := range actions {
		if strings.EqualFold(strings.TrimSpace(action.ID), requested) {
			return action.ID
		}
	}

	descriptors := CanonicalizeActionDescriptors(actions)
	canonical := CanonicalOperationID(requested)
	candidates := make([]connectorsdk.ActionDescriptor, 0, len(descriptors))
	for _, descriptor := range descriptors {
		if strings.EqualFold(strings.TrimSpace(descriptor.CanonicalID), canonical) {
			candidates = append(candidates, descriptor)
		}
	}
	if len(candidates) == 0 {
		return requested
	}
	if len(candidates) == 1 {
		return candidates[0].ID
	}
	return pickBestActionForTarget(candidates, targetID)
}

func pickBestActionForTarget(candidates []connectorsdk.ActionDescriptor, targetID string) string {
	target := strings.ToLower(strings.TrimSpace(targetID))
	type scoredAction struct {
		id    string
		score int
	}
	scored := make([]scoredAction, 0, len(candidates))
	for _, candidate := range candidates {
		score := 0
		id := strings.ToLower(strings.TrimSpace(candidate.ID))
		switch {
		case strings.Contains(target, "-vm-") && strings.HasPrefix(id, "vm."):
			score += 10
		case strings.Contains(target, "-ct-") && strings.HasPrefix(id, "ct."):
			score += 10
		case (strings.Contains(target, "docker-ct-") || strings.Contains(target, "container")) && strings.HasPrefix(id, "container."):
			score += 10
		case strings.Contains(target, "stack") && strings.HasPrefix(id, "stack."):
			score += 10
		case strings.Contains(target, "-service-") && strings.HasPrefix(id, "service."):
			score += 10
		case strings.Contains(target, "-app-") && strings.HasPrefix(id, "app."):
			score += 10
		case strings.Contains(target, "pbs-datastore-") && strings.HasPrefix(id, "datastore."):
			score += 10
		case strings.Contains(target, "truenas-storage-pool-") && id == "pool.scrub":
			score += 10
		}
		scored = append(scored, scoredAction{id: candidate.ID, score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].id < scored[j].id
		}
		return scored[i].score > scored[j].score
	})
	return scored[0].id
}

func cloneActionParameters(input []connectorsdk.ActionParameter) []connectorsdk.ActionParameter {
	if len(input) == 0 {
		return nil
	}
	out := make([]connectorsdk.ActionParameter, len(input))
	copy(out, input)
	return out
}
