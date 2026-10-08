package modelregistry

import (
	"github.com/labtether/labtether/internal/model"
	"sort"
	"strings"
	"time"
)

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
