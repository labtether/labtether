package model

import (
	"strings"
	"time"
)

type ResourceClass string

const (
	ResourceClassCompute  ResourceClass = "compute"
	ResourceClassStorage  ResourceClass = "storage"
	ResourceClassNetwork  ResourceClass = "network"
	ResourceClassService  ResourceClass = "service"
	ResourceClassPower    ResourceClass = "power"
	ResourceClassSecurity ResourceClass = "security"
	ResourceClassOther    ResourceClass = "other"
)

type ResourceStatus string

const (
	ResourceStatusOnline   ResourceStatus = "online"
	ResourceStatusDegraded ResourceStatus = "degraded"
	ResourceStatusOffline  ResourceStatus = "offline"
	ResourceStatusStale    ResourceStatus = "stale"
	ResourceStatusUnknown  ResourceStatus = "unknown"
)

type ExternalRef struct {
	ProviderInstanceID string `json:"provider_instance_id"`
	ExternalID         string `json:"external_id"`
	ExternalType       string `json:"external_type,omitempty"`
	ExternalParentID   string `json:"external_parent_id,omitempty"`
	RawLocator         string `json:"raw_locator,omitempty"`
}

type Resource struct {
	ID                 string            `json:"id"`
	Class              ResourceClass     `json:"class"`
	Kind               string            `json:"kind"`
	Name               string            `json:"name"`
	Source             string            `json:"source"`
	ProviderInstanceID string            `json:"provider_instance_id,omitempty"`
	GroupID            string            `json:"group_id,omitempty"`
	Platform           string            `json:"platform,omitempty"`
	Status             ResourceStatus    `json:"status"`
	ParentResourceID   string            `json:"parent_resource_id,omitempty"`
	Labels             map[string]string `json:"labels,omitempty"`
	Annotations        map[string]string `json:"annotations,omitempty"`
	Traits             []string          `json:"traits,omitempty"`
	Attributes         map[string]any    `json:"attributes,omitempty"`
	ProviderData       map[string]any    `json:"provider_data,omitempty"`
	ExternalRefs       []ExternalRef     `json:"external_refs,omitempty"`
	FirstSeenAt        time.Time         `json:"first_seen_at"`
	LastSeenAt         time.Time         `json:"last_seen_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

var resourceClassByKind = map[string]ResourceClass{
	"host":               ResourceClassCompute,
	"hypervisor-node":    ResourceClassCompute,
	"vm":                 ResourceClassCompute,
	"container":          ResourceClassCompute,
	"container-host":     ResourceClassCompute,
	"docker-container":   ResourceClassCompute,
	"app":                ResourceClassCompute,
	"pod":                ResourceClassCompute,
	"deployment":         ResourceClassCompute,
	"stack":              ResourceClassCompute,
	"compose-stack":      ResourceClassCompute,
	"storage-controller": ResourceClassStorage,
	"storage-pool":       ResourceClassStorage,
	"datastore":          ResourceClassStorage,
	"dataset":            ResourceClassStorage,
	"disk":               ResourceClassStorage,
	"volume":             ResourceClassStorage,
	"share-smb":          ResourceClassStorage,
	"share-nfs":          ResourceClassStorage,
	"snapshot":           ResourceClassStorage,
	"interface":          ResourceClassNetwork,
	"gateway":            ResourceClassNetwork,
	"switch":             ResourceClassNetwork,
	"wap":                ResourceClassNetwork,
	"firewall-rule":      ResourceClassNetwork,
	"vlan":               ResourceClassNetwork,
	"route":              ResourceClassNetwork,
	"wan-link":           ResourceClassNetwork,
	"service":            ResourceClassService,
	"ha-entity":          ResourceClassService,
	"backup-task":        ResourceClassService,
	"replication-task":   ResourceClassService,
	"cloud-sync-task":    ResourceClassService,
	"cron-task":          ResourceClassService,
	"ups":                ResourceClassPower,
	"pdu":                ResourceClassPower,
	"ipmi-sensor":        ResourceClassPower,
}

func NormalizeResourceKind(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func ResourceClassForKind(kind string) ResourceClass {
	normalized := NormalizeResourceKind(kind)
	if normalized == "" {
		return ResourceClassOther
	}
	if strings.HasPrefix(normalized, "x.") {
		return ResourceClassOther
	}
	if class, ok := resourceClassByKind[normalized]; ok {
		return class
	}
	return ResourceClassOther
}
