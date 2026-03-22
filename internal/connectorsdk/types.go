package connectorsdk

import "context"

type Connector interface {
	ID() string
	DisplayName() string
	Capabilities() Capabilities
	Discover(ctx context.Context) ([]Asset, error)
	TestConnection(ctx context.Context) (Health, error)
	Actions() []ActionDescriptor
	ExecuteAction(ctx context.Context, actionID string, req ActionRequest) (ActionResult, error)
}

type Capabilities struct {
	DiscoverAssets bool `json:"discover_assets"`
	CollectMetrics bool `json:"collect_metrics"`
	CollectEvents  bool `json:"collect_events"`
	ExecuteActions bool `json:"execute_actions"`
}

type Asset struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Name         string            `json:"name"`
	Source       string            `json:"source"`
	Kind         string            `json:"kind,omitempty"`
	Class        string            `json:"class,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Attributes   map[string]any    `json:"attributes,omitempty"`
	ProviderData map[string]any    `json:"provider_data,omitempty"`
}

type Health struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type ActionDescriptor struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	CanonicalID    string            `json:"canonical_id,omitempty"`
	Description    string            `json:"description,omitempty"`
	RequiresTarget bool              `json:"requires_target"`
	SupportsDryRun bool              `json:"supports_dry_run"`
	Parameters     []ActionParameter `json:"parameters,omitempty"`
}

type ActionParameter struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

type ActionRequest struct {
	TargetID string            `json:"target_id,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
	DryRun   bool              `json:"dry_run,omitempty"`
}

type ActionResult struct {
	Status   string            `json:"status"`
	Message  string            `json:"message"`
	Output   string            `json:"output,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
