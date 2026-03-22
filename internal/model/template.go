package model

import "time"

type TemplateDefinition struct {
	ID           string                `json:"id"`
	AppliesTo    TemplateAppliesTo     `json:"applies_to"`
	Sections     []TemplateSection     `json:"sections,omitempty"`
	ActionGroups []TemplateActionGroup `json:"action_groups,omitempty"`
	Priority     int                   `json:"priority,omitempty"`
	Version      string                `json:"version,omitempty"`
}

type TemplateAppliesTo struct {
	Classes []ResourceClass `json:"classes,omitempty"`
	Kinds   []string        `json:"kinds,omitempty"`
	Traits  []string        `json:"traits,omitempty"`
	Sources []string        `json:"sources,omitempty"`
}

type TemplateSection struct {
	ID     string                 `json:"id"`
	Title  string                 `json:"title"`
	Fields []TemplateFieldBinding `json:"fields,omitempty"`
}

type TemplateFieldBinding struct {
	Label          string   `json:"label"`
	Path           string   `json:"path"`
	FallbackPaths  []string `json:"fallback_paths,omitempty"`
	Format         string   `json:"format,omitempty"`
	VisibilityRule string   `json:"visibility_rule,omitempty"`
}

type TemplateActionGroup struct {
	ID      string                  `json:"id"`
	Title   string                  `json:"title"`
	Actions []TemplateActionBinding `json:"actions,omitempty"`
}

type TemplateActionBinding struct {
	OperationID        string `json:"operation_id"`
	RequiredCapability string `json:"required_capability,omitempty"`
	ConfirmMode        string `json:"confirm_mode,omitempty"`
	Dangerous          bool   `json:"dangerous,omitempty"`
}

type TemplateBinding struct {
	ResourceID string    `json:"resource_id"`
	TemplateID string    `json:"template_id"`
	Tabs       []string  `json:"tabs,omitempty"`
	Operations []string  `json:"operations,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}
