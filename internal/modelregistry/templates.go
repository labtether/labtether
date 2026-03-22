package modelregistry

import (
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/model"
)

var templateCatalog = []model.TemplateDefinition{
	{
		ID: "template.docker.container",
		AppliesTo: model.TemplateAppliesTo{
			Kinds:   []string{"docker-container"},
			Sources: []string{"docker"},
		},
		Sections: []model.TemplateSection{
			{ID: "overview", Title: "Overview"},
			{ID: "logs", Title: "Logs"},
			{ID: "stats", Title: "Stats"},
			{ID: "inspect", Title: "Inspect"},
		},
		Priority: 300,
		Version:  "v1",
	},
	{
		ID: "template.docker.host",
		AppliesTo: model.TemplateAppliesTo{
			Kinds:   []string{"container-host"},
			Sources: []string{"docker", "portainer"},
		},
		Sections: []model.TemplateSection{
			{ID: "overview", Title: "Overview"},
			{ID: "containers", Title: "Containers"},
			{ID: "images", Title: "Images"},
			{ID: "stacks", Title: "Stacks"},
			{ID: "terminal", Title: "Terminal"},
		},
		Priority: 280,
		Version:  "v1",
	},
	{
		ID: "template.proxmox.node",
		AppliesTo: model.TemplateAppliesTo{
			Kinds:   []string{"hypervisor-node"},
			Sources: []string{"proxmox"},
		},
		Sections: []model.TemplateSection{
			{ID: "overview", Title: "Overview"},
			{ID: "proxmox", Title: "Proxmox"},
			{ID: "telemetry", Title: "Metrics"},
			{ID: "logs", Title: "Logs"},
			{ID: "actions", Title: "Actions"},
			{ID: "terminal", Title: "Terminal"},
			{ID: "desktop", Title: "Remote Access"},
			{ID: "settings", Title: "Settings"},
		},
		ActionGroups: []model.TemplateActionGroup{
			{
				ID:    "lifecycle",
				Title: "Lifecycle",
				Actions: []model.TemplateActionBinding{
					{OperationID: "system.reboot", RequiredCapability: "system.action", ConfirmMode: "confirm", Dangerous: true},
				},
			},
		},
		Priority: 320,
		Version:  "v1",
	},
	{
		ID: "template.proxmox.workload",
		AppliesTo: model.TemplateAppliesTo{
			Kinds:   []string{"vm", "container"},
			Sources: []string{"proxmox"},
		},
		Sections: []model.TemplateSection{
			{ID: "overview", Title: "Overview"},
			{ID: "proxmox", Title: "Proxmox"},
			{ID: "telemetry", Title: "Metrics"},
			{ID: "logs", Title: "Logs"},
			{ID: "actions", Title: "Actions"},
			{ID: "terminal", Title: "Terminal"},
			{ID: "desktop", Title: "Remote Access"},
		},
		ActionGroups: []model.TemplateActionGroup{
			{
				ID:    "workload",
				Title: "Workload",
				Actions: []model.TemplateActionBinding{
					{OperationID: "workload.start", RequiredCapability: "workload.action"},
					{OperationID: "workload.stop", RequiredCapability: "workload.action", ConfirmMode: "confirm"},
					{OperationID: "workload.reboot", RequiredCapability: "workload.action", ConfirmMode: "confirm"},
				},
			},
		},
		Priority: 310,
		Version:  "v1",
	},
	{
		ID: "template.truenas.controller",
		AppliesTo: model.TemplateAppliesTo{
			Kinds:   []string{"storage-controller"},
			Sources: []string{"truenas"},
		},
		Sections: []model.TemplateSection{
			{ID: "overview", Title: "Overview"},
			{ID: "truenas", Title: "TrueNAS"},
			{ID: "telemetry", Title: "Metrics"},
			{ID: "logs", Title: "Logs"},
			{ID: "actions", Title: "Actions"},
			{ID: "terminal", Title: "Terminal"},
		},
		Priority: 260,
		Version:  "v1",
	},
	{
		ID: "template.pbs.controller",
		AppliesTo: model.TemplateAppliesTo{
			Kinds:   []string{"storage-controller"},
			Sources: []string{"pbs"},
		},
		Sections: []model.TemplateSection{
			{ID: "overview", Title: "Overview"},
			{ID: "pbs", Title: "PBS"},
			{ID: "telemetry", Title: "Metrics"},
			{ID: "logs", Title: "Logs"},
			{ID: "actions", Title: "Actions"},
			{ID: "terminal", Title: "Terminal"},
		},
		Priority: 260,
		Version:  "v1",
	},
	{
		ID: "template.pbs.datastore",
		AppliesTo: model.TemplateAppliesTo{
			Kinds:   []string{"datastore"},
			Sources: []string{"pbs"},
		},
		Sections: []model.TemplateSection{
			{ID: "overview", Title: "Overview"},
			{ID: "pbs", Title: "PBS"},
			{ID: "telemetry", Title: "Metrics"},
			{ID: "logs", Title: "Logs"},
			{ID: "actions", Title: "Actions"},
		},
		Priority: 250,
		Version:  "v1",
	},
	{
		ID: "template.homeassistant.entity",
		AppliesTo: model.TemplateAppliesTo{
			Kinds:   []string{"ha-entity"},
			Sources: []string{"home-assistant"},
		},
		Sections: []model.TemplateSection{
			{ID: "overview", Title: "Overview"},
			{ID: "telemetry", Title: "Metrics"},
			{ID: "logs", Title: "Logs"},
			{ID: "actions", Title: "Actions"},
		},
		Priority: 240,
		Version:  "v1",
	},
	{
		ID: "template.compute.default",
		AppliesTo: model.TemplateAppliesTo{
			Classes: []model.ResourceClass{model.ResourceClassCompute},
		},
		Sections: []model.TemplateSection{
			{ID: "overview", Title: "Overview"},
			{ID: "telemetry", Title: "Metrics"},
			{ID: "logs", Title: "Logs"},
			{ID: "actions", Title: "Actions"},
			{ID: "terminal", Title: "Terminal"},
		},
		Priority: 100,
		Version:  "v1",
	},
	{
		ID: "template.storage.default",
		AppliesTo: model.TemplateAppliesTo{
			Classes: []model.ResourceClass{model.ResourceClassStorage},
		},
		Sections: []model.TemplateSection{
			{ID: "overview", Title: "Overview"},
			{ID: "telemetry", Title: "Metrics"},
			{ID: "logs", Title: "Logs"},
			{ID: "actions", Title: "Actions"},
		},
		Priority: 100,
		Version:  "v1",
	},
	{
		ID:        "template.other.default",
		AppliesTo: model.TemplateAppliesTo{},
		Sections: []model.TemplateSection{
			{ID: "overview", Title: "Overview"},
			{ID: "telemetry", Title: "Metrics"},
			{ID: "logs", Title: "Logs"},
			{ID: "actions", Title: "Actions"},
		},
		Priority: 1,
		Version:  "v1",
	},
}

var capabilityTabHints = map[string][]string{
	"telemetry.read": {"telemetry"},
	"logs.read":      {"logs"},
	"logs.query":     {"logs"},
	"logs.stream":    {"logs"},
	"events.read":    {"logs"},
	"events.stream":  {"logs"},
	"terminal.open":  {"terminal"},
	"process.list":   {"processes"},
	"service.list":   {"services"},
	"package.list":   {"packages"},
	"network.list":   {"interfaces"},
	"cron.list":      {"cron"},
	"files.list":     {"files"},
	"users.list":     {"users"},
}

func TemplateCatalog() []model.TemplateDefinition {
	if len(templateCatalog) == 0 {
		return nil
	}
	out := make([]model.TemplateDefinition, len(templateCatalog))
	copy(out, templateCatalog)
	for idx := range out {
		out[idx].AppliesTo.Kinds = cloneStrings(out[idx].AppliesTo.Kinds)
		out[idx].AppliesTo.Traits = cloneStrings(out[idx].AppliesTo.Traits)
		out[idx].AppliesTo.Sources = cloneStrings(out[idx].AppliesTo.Sources)
		if len(out[idx].AppliesTo.Classes) > 0 {
			classes := make([]model.ResourceClass, len(out[idx].AppliesTo.Classes))
			copy(classes, out[idx].AppliesTo.Classes)
			out[idx].AppliesTo.Classes = classes
		}
		out[idx].Sections = cloneTemplateSections(out[idx].Sections)
		out[idx].ActionGroups = cloneTemplateActionGroups(out[idx].ActionGroups)
	}
	return out
}

func ResolveTemplateBinding(resource model.Resource, capabilityIDs []string, at time.Time) model.TemplateBinding {
	normalizedCaps := normalizeCapabilityIDs(capabilityIDs)
	selected := pickTemplate(resource)

	tabs := make([]string, 0, 12)
	operations := make([]string, 0, 12)
	templateID := "template.other.default"
	if selected != nil {
		templateID = selected.ID
		for _, section := range selected.Sections {
			sectionID := strings.TrimSpace(section.ID)
			if sectionID == "" {
				continue
			}
			tabs = append(tabs, sectionID)
		}
		for _, actionGroup := range selected.ActionGroups {
			for _, action := range actionGroup.Actions {
				requiredCapability := strings.ToLower(strings.TrimSpace(action.RequiredCapability))
				if requiredCapability != "" {
					if _, ok := normalizedCaps[requiredCapability]; !ok {
						continue
					}
				}
				if strings.TrimSpace(action.OperationID) != "" {
					operations = append(operations, strings.TrimSpace(action.OperationID))
				}
			}
		}
	}

	for capabilityID, tabIDs := range capabilityTabHints {
		if _, ok := normalizedCaps[capabilityID]; !ok {
			continue
		}
		tabs = append(tabs, tabIDs...)
	}

	source := normalizeSource(resource.Source)
	switch source {
	case "proxmox":
		tabs = append(tabs, "proxmox")
	case "truenas":
		tabs = append(tabs, "truenas")
	case "pbs":
		tabs = append(tabs, "pbs")
	}

	if len(tabs) == 0 {
		tabs = append(tabs, defaultTabsForClass(resource.Class)...)
	}

	capabilityList := sortedCapabilityIDs(normalizedCaps)
	operations = append(operations, OperationIDsForCapabilities(capabilityList, resource.Kind)...)

	return model.TemplateBinding{
		ResourceID: resource.ID,
		TemplateID: templateID,
		Tabs:       normalizeTabs(tabs),
		Operations: dedupeStrings(operations),
		UpdatedAt:  at.UTC(),
	}
}

func pickTemplate(resource model.Resource) *model.TemplateDefinition {
	catalog := TemplateCatalog()
	if len(catalog) == 0 {
		return nil
	}

	matches := make([]model.TemplateDefinition, 0, 4)
	for _, definition := range catalog {
		if templateMatchesResource(definition, resource) {
			matches = append(matches, definition)
		}
	}
	if len(matches) == 0 {
		return nil
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Priority == matches[j].Priority {
			return matches[i].ID < matches[j].ID
		}
		return matches[i].Priority > matches[j].Priority
	})

	selected := matches[0]
	return &selected
}

func templateMatchesResource(definition model.TemplateDefinition, resource model.Resource) bool {
	appliesTo := definition.AppliesTo
	resourceSource := normalizeSource(resource.Source)
	resourceKind := strings.ToLower(strings.TrimSpace(resource.Kind))

	if len(appliesTo.Sources) > 0 {
		matched := false
		for _, source := range appliesTo.Sources {
			if normalizeSource(source) == resourceSource {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(appliesTo.Kinds) > 0 {
		matched := false
		for _, kind := range appliesTo.Kinds {
			if strings.EqualFold(strings.TrimSpace(kind), resourceKind) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(appliesTo.Classes) > 0 {
		matched := false
		for _, class := range appliesTo.Classes {
			if class == resource.Class {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(appliesTo.Traits) > 0 {
		traits := make(map[string]struct{}, len(resource.Traits))
		for _, trait := range resource.Traits {
			normalized := strings.ToLower(strings.TrimSpace(trait))
			if normalized == "" {
				continue
			}
			traits[normalized] = struct{}{}
		}
		for _, requiredTrait := range appliesTo.Traits {
			normalized := strings.ToLower(strings.TrimSpace(requiredTrait))
			if normalized == "" {
				continue
			}
			if _, ok := traits[normalized]; !ok {
				return false
			}
		}
	}

	return true
}

func normalizeTabs(values []string) []string {
	if len(values) == 0 {
		return []string{"overview"}
	}
	cleaned := make([]string, 0, len(values)+1)
	seen := make(map[string]struct{}, len(values)+1)
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		cleaned = append(cleaned, normalized)
	}
	if len(cleaned) == 0 {
		return []string{"overview"}
	}

	containsOverview := false
	for _, value := range cleaned {
		if value == "overview" {
			containsOverview = true
			break
		}
	}
	if !containsOverview {
		cleaned = append([]string{"overview"}, cleaned...)
		return cleaned
	}
	if cleaned[0] == "overview" {
		return cleaned
	}

	out := make([]string, 0, len(cleaned))
	out = append(out, "overview")
	for _, value := range cleaned {
		if value == "overview" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func defaultTabsForClass(class model.ResourceClass) []string {
	switch class {
	case model.ResourceClassCompute:
		return []string{"overview", "telemetry", "logs", "actions", "terminal"}
	case model.ResourceClassStorage:
		return []string{"overview", "telemetry", "logs", "actions"}
	case model.ResourceClassNetwork:
		return []string{"overview", "telemetry", "logs", "actions", "interfaces"}
	default:
		return []string{"overview", "telemetry", "logs", "actions"}
	}
}

func normalizeSource(source string) string {
	normalized := strings.ToLower(strings.TrimSpace(source))
	switch normalized {
	case "homeassistant":
		return "home-assistant"
	default:
		return normalized
	}
}

func normalizeCapabilityIDs(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		out[normalized] = struct{}{}
	}
	return out
}

func sortedCapabilityIDs(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func cloneTemplateSections(input []model.TemplateSection) []model.TemplateSection {
	if len(input) == 0 {
		return nil
	}
	out := make([]model.TemplateSection, len(input))
	copy(out, input)
	for idx := range out {
		out[idx].Fields = cloneTemplateFieldBindings(out[idx].Fields)
	}
	return out
}

func cloneTemplateFieldBindings(input []model.TemplateFieldBinding) []model.TemplateFieldBinding {
	if len(input) == 0 {
		return nil
	}
	out := make([]model.TemplateFieldBinding, len(input))
	copy(out, input)
	for idx := range out {
		out[idx].FallbackPaths = cloneStrings(out[idx].FallbackPaths)
	}
	return out
}

func cloneTemplateActionGroups(input []model.TemplateActionGroup) []model.TemplateActionGroup {
	if len(input) == 0 {
		return nil
	}
	out := make([]model.TemplateActionGroup, len(input))
	copy(out, input)
	for idx := range out {
		if len(out[idx].Actions) == 0 {
			continue
		}
		actions := make([]model.TemplateActionBinding, len(out[idx].Actions))
		copy(actions, out[idx].Actions)
		out[idx].Actions = actions
	}
	return out
}
