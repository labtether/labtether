package modelmap

import (
	"testing"

	"github.com/labtether/labtether/internal/connectorsdk"
)

func TestDeriveAssetCanonical(t *testing.T) {
	t.Parallel()

	class, kind, attrs := DeriveAssetCanonical("docker", "docker-container", map[string]string{
		"container_id":   "abc123",
		"cpu_percent":    "12.5",
		"memory_percent": "33.1",
	})

	if class != "compute" {
		t.Fatalf("class = %q, want compute", class)
	}
	if kind != "docker-container" {
		t.Fatalf("kind = %q, want docker-container", kind)
	}
	if attrs == nil {
		t.Fatalf("expected derived attributes")
	}
	if got, ok := attrs["cpu_used_percent"].(float64); !ok || got != 12.5 {
		t.Fatalf("cpu_used_percent = %#v, want 12.5", attrs["cpu_used_percent"])
	}
	if got := attrs["container_id"]; got != "abc123" {
		t.Fatalf("container_id = %#v, want abc123", got)
	}
}

func TestCanonicalizeConnectorAssetSetsClassKind(t *testing.T) {
	t.Parallel()

	asset := CanonicalizeConnectorAsset("docker", connectorsdk.Asset{
		ID:       "docker-ct-host-abc123",
		Type:     "docker-container",
		Name:     "nginx",
		Source:   "",
		Metadata: map[string]string{"state": "running"},
	})

	if asset.Source != "docker" {
		t.Fatalf("asset.Source = %q, want docker", asset.Source)
	}
	if asset.Class != "compute" {
		t.Fatalf("asset.Class = %q, want compute", asset.Class)
	}
	if asset.Kind != "docker-container" {
		t.Fatalf("asset.Kind = %q, want docker-container", asset.Kind)
	}
}

func TestCanonicalResourceKindSourceAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		source    string
		assetType string
		metadata  map[string]string
		wantKind  string
		wantClass string
	}{
		{
			name:      "pbs datastore alias",
			source:    "pbs",
			assetType: "storage-pool",
			metadata:  map[string]string{"store": "backup"},
			wantKind:  "datastore",
			wantClass: "storage",
		},
		{
			name:      "truenas nas alias",
			source:    "truenas",
			assetType: "nas",
			metadata:  map[string]string{"hostname": "omega"},
			wantKind:  "storage-controller",
			wantClass: "storage",
		},
		{
			name:      "home assistant entity hint",
			source:    "home-assistant",
			assetType: "",
			metadata:  map[string]string{"entity_id": "switch.rack_fan"},
			wantKind:  "ha-entity",
			wantClass: "service",
		},
		{
			name:      "proxmox vm passthrough",
			source:    "proxmox",
			assetType: "vm",
			metadata:  map[string]string{"vmid": "100"},
			wantKind:  "vm",
			wantClass: "compute",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			class, kind, _ := DeriveAssetCanonical(tc.source, tc.assetType, tc.metadata)
			if kind != tc.wantKind {
				t.Fatalf("kind = %q, want %q", kind, tc.wantKind)
			}
			if class != tc.wantClass {
				t.Fatalf("class = %q, want %q", class, tc.wantClass)
			}
		})
	}
}

func TestCanonicalizeConnectorAssetStoresRawTypeWhenKindAliased(t *testing.T) {
	t.Parallel()

	asset := CanonicalizeConnectorAsset("truenas", connectorsdk.Asset{
		ID:       "truenas-host-omega",
		Type:     "nas",
		Name:     "OmegaNAS",
		Metadata: map[string]string{"hostname": "OmegaNAS"},
	})

	if asset.Kind != "storage-controller" {
		t.Fatalf("asset.Kind = %q, want storage-controller", asset.Kind)
	}
	if asset.ProviderData == nil {
		t.Fatalf("expected provider_data to be set")
	}
	if rawType, _ := asset.ProviderData["raw_type"].(string); rawType != "nas" {
		t.Fatalf("provider_data.raw_type = %#v, want nas", asset.ProviderData["raw_type"])
	}
}
