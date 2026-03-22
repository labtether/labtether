package modelregistry

import (
	"sort"
	"strings"

	"github.com/labtether/labtether/internal/model"
)

var operationCatalog = []model.OperationDescriptor{
	{ID: "workload.start", DisplayName: "Start Workload", CapabilityRequired: "workload.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true, IsIdempotent: true},
	{ID: "workload.stop", DisplayName: "Stop Workload", CapabilityRequired: "workload.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true, IsIdempotent: true},
	{ID: "workload.shutdown", DisplayName: "Shutdown Workload", CapabilityRequired: "workload.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "workload.reboot", DisplayName: "Reboot Workload", CapabilityRequired: "workload.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "workload.suspend", DisplayName: "Suspend Workload", CapabilityRequired: "workload.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "workload.resume", DisplayName: "Resume Workload", CapabilityRequired: "workload.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true},
	{ID: "workload.force_stop", DisplayName: "Force Stop Workload", CapabilityRequired: "workload.action", SafetyLevel: model.OperationSafetyDestructive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "workload.migrate", DisplayName: "Migrate Workload", CapabilityRequired: "workload.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "workload.clone", DisplayName: "Clone Workload", CapabilityRequired: "workload.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true},
	{ID: "workload.clone_from_template", DisplayName: "Clone From Template", CapabilityRequired: "workload.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true},
	{ID: "workload.disk_resize", DisplayName: "Resize Workload Disk", CapabilityRequired: "workload.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "snapshot.create", DisplayName: "Create Snapshot", CapabilityRequired: "snapshot.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true},
	{ID: "snapshot.delete", DisplayName: "Delete Snapshot", CapabilityRequired: "snapshot.action", SafetyLevel: model.OperationSafetyDestructive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "snapshot.rollback", DisplayName: "Rollback Snapshot", CapabilityRequired: "snapshot.action", SafetyLevel: model.OperationSafetyDestructive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "backup.start", DisplayName: "Start Backup", CapabilityRequired: "backup.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true},
	{ID: "storage.pool.scrub", DisplayName: "Scrub Storage Pool", CapabilityRequired: "system.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true},
	{ID: "storage.datastore.verify", DisplayName: "Verify Datastore", CapabilityRequired: "backup.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true},
	{ID: "storage.datastore.prune", DisplayName: "Prune Datastore", CapabilityRequired: "backup.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "storage.datastore.gc", DisplayName: "Datastore Garbage Collection", CapabilityRequired: "backup.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "disk.smart_test", DisplayName: "Run SMART Test", CapabilityRequired: "system.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true},
	{ID: "service.start", DisplayName: "Start Service", CapabilityRequired: "service.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true, IsIdempotent: true},
	{ID: "service.stop", DisplayName: "Stop Service", CapabilityRequired: "service.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true, IsIdempotent: true},
	{ID: "service.restart", DisplayName: "Restart Service", CapabilityRequired: "service.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "app.start", DisplayName: "Start App", CapabilityRequired: "app.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true, IsIdempotent: true},
	{ID: "app.stop", DisplayName: "Stop App", CapabilityRequired: "app.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true, IsIdempotent: true},
	{ID: "app.restart", DisplayName: "Restart App", CapabilityRequired: "app.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "container.pause", DisplayName: "Pause Container", CapabilityRequired: "system.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "container.unpause", DisplayName: "Unpause Container", CapabilityRequired: "system.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true},
	{ID: "container.kill", DisplayName: "Kill Container", CapabilityRequired: "system.action", SafetyLevel: model.OperationSafetyDestructive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "container.remove", DisplayName: "Remove Container", CapabilityRequired: "system.action", SafetyLevel: model.OperationSafetyDestructive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "container.restart", DisplayName: "Restart Container", CapabilityRequired: "system.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "image.pull", DisplayName: "Pull Image", CapabilityRequired: "image.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true},
	{ID: "image.remove", DisplayName: "Remove Image", CapabilityRequired: "image.action", SafetyLevel: model.OperationSafetyDestructive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "stack.up", DisplayName: "Stack Up", CapabilityRequired: "stack.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true},
	{ID: "stack.down", DisplayName: "Stack Down", CapabilityRequired: "stack.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "stack.restart", DisplayName: "Restart Stack", CapabilityRequired: "stack.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "stack.pull", DisplayName: "Pull Stack Images", CapabilityRequired: "stack.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true},
	{ID: "stack.start", DisplayName: "Start Stack", CapabilityRequired: "stack.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true, IsIdempotent: true},
	{ID: "stack.stop", DisplayName: "Stop Stack", CapabilityRequired: "stack.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true, IsIdempotent: true},
	{ID: "stack.redeploy", DisplayName: "Redeploy Stack", CapabilityRequired: "stack.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "stack.remove", DisplayName: "Remove Stack", CapabilityRequired: "stack.action", SafetyLevel: model.OperationSafetyDestructive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "system.reboot", DisplayName: "Reboot System", CapabilityRequired: "system.action", SafetyLevel: model.OperationSafetyDisruptive, SupportsDryRun: true, RequiresTarget: true},
	{ID: "automation.entity.toggle", DisplayName: "Toggle Entity", CapabilityRequired: "system.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true},
	{ID: "automation.service.call", DisplayName: "Call Automation Service", CapabilityRequired: "system.action", SafetyLevel: model.OperationSafetySafe, SupportsDryRun: true, RequiresTarget: true},
}

func OperationCatalog() []model.OperationDescriptor {
	if len(operationCatalog) == 0 {
		return nil
	}
	out := make([]model.OperationDescriptor, len(operationCatalog))
	copy(out, operationCatalog)
	for idx := range out {
		out[idx].Parameters = cloneOperationParameters(out[idx].Parameters)
	}
	return out
}

func OperationIDsForCapabilities(capabilityIDs []string, targetKind string) []string {
	if len(capabilityIDs) == 0 {
		return nil
	}
	capSet := make(map[string]struct{}, len(capabilityIDs))
	for _, capabilityID := range capabilityIDs {
		normalized := strings.ToLower(strings.TrimSpace(capabilityID))
		if normalized == "" {
			continue
		}
		capSet[normalized] = struct{}{}
	}
	if len(capSet) == 0 {
		return nil
	}

	targetKind = strings.ToLower(strings.TrimSpace(targetKind))
	out := make([]string, 0, 12)
	for _, descriptor := range operationCatalog {
		required := strings.ToLower(strings.TrimSpace(descriptor.CapabilityRequired))
		if required != "" {
			if _, ok := capSet[required]; !ok {
				continue
			}
		}
		target := strings.ToLower(strings.TrimSpace(descriptor.TargetKind))
		if target != "" && targetKind != "" && target != targetKind {
			continue
		}
		out = append(out, descriptor.ID)
	}
	sort.Strings(out)
	return dedupeStrings(out)
}

func cloneOperationParameters(input []model.OperationParameter) []model.OperationParameter {
	if len(input) == 0 {
		return nil
	}
	out := make([]model.OperationParameter, len(input))
	copy(out, input)
	for idx := range out {
		if len(out[idx].Schema) > 0 {
			out[idx].Schema = cloneAnyMap(out[idx].Schema)
		}
	}
	return out
}
