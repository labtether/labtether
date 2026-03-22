package persistence

import "time"

// WebServiceManual is a user-managed web service entry attached to a host asset.
type WebServiceManual struct {
	ID          string            `json:"id"`
	HostAssetID string            `json:"host_asset_id"`
	Name        string            `json:"name"`
	Category    string            `json:"category"`
	URL         string            `json:"url"`
	IconKey     string            `json:"icon_key"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// WebServiceOverride stores user overrides for discovered services.
type WebServiceOverride struct {
	HostAssetID      string    `json:"host_asset_id"`
	ServiceID        string    `json:"service_id"`
	NameOverride     string    `json:"name_override,omitempty"`
	CategoryOverride string    `json:"category_override,omitempty"`
	URLOverride      string    `json:"url_override,omitempty"`
	IconKeyOverride  string    `json:"icon_key_override,omitempty"`
	TagsOverride     string    `json:"tags_override,omitempty"`
	Hidden           bool      `json:"hidden"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// WebServiceStore provides persistence for manual services and service overrides.
type WebServiceStore interface {
	ListManualWebServices(hostAssetID string) ([]WebServiceManual, error)
	GetManualWebService(id string) (WebServiceManual, bool, error)
	SaveManualWebService(service WebServiceManual) (WebServiceManual, error)
	DeleteManualWebService(id string) error
	PromoteManualServicesToStandalone(hostAssetID string) error

	ListWebServiceOverrides(hostAssetID string) ([]WebServiceOverride, error)
	SaveWebServiceOverride(override WebServiceOverride) (WebServiceOverride, error)
	DeleteWebServiceOverride(hostAssetID, serviceID string) error
}
