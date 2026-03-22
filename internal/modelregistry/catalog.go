package modelregistry

import "github.com/labtether/labtether/internal/model"

type CanonicalRegistry struct {
	Capabilities []model.CapabilitySpec      `json:"capabilities"`
	Operations   []model.OperationDescriptor `json:"operations"`
	Metrics      []model.MetricDescriptor    `json:"metrics"`
	Events       []model.EventDescriptor     `json:"events"`
	Templates    []model.TemplateDefinition  `json:"templates"`
}

func Snapshot() CanonicalRegistry {
	return CanonicalRegistry{
		Capabilities: CapabilityCatalog(),
		Operations:   OperationCatalog(),
		Metrics:      MetricCatalog(),
		Events:       EventCatalog(),
		Templates:    TemplateCatalog(),
	}
}
