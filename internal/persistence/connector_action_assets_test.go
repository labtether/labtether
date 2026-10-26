package persistence

import (
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
)

func TestTrueNASActionLookupKeepsCollectorAndResourceBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, owner, scope, selected, kind, vmID string
		want                                     int
	}{
		{"exact VM", "nas-a", "nas-a", "nas-a", "vm", "42", 1},
		{"foreign owner", "nas-b", "nas-b", "nas-a", "vm", "42", 0},
		{"foreign unowned scope", "", "nas-b", "nas-a", "vm", "42", 0},
		{"matching unowned scope", "", "nas-a", "nas-a", "vm", "42", 1},
		{"unowned legacy remains visible", "", "", "nas-a", "vm", "42", 1},
		{"unknown collector retains ambiguity", "", "nas-b", "", "vm", "42", 1},
		{"trimmed owner", " nas-a ", "nas-a", "nas-a", "vm", "42", 1},
		{"different VM", "nas-a", "nas-a", "nas-a", "vm", "43", 0},
		{"NAS is not a VM", "nas-a", "nas-a", "nas-a", "nas", "42", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := NewMemoryAssetStore()
			_, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
				AssetID: assetid.ScopeCollectorAssetID("truenas-resource", tc.scope), Source: "truenas", Type: tc.kind,
				Metadata: map[string]string{"collector_id": tc.owner, "vm_id": tc.vmID},
			})
			if err != nil {
				t.Fatal(err)
			}
			matches, err := store.FindTrueNASActionAssets(tc.selected, "vm", "42")
			if err != nil || len(matches) != tc.want {
				t.Fatalf("matches=%d want=%d err=%v", len(matches), tc.want, err)
			}
		})
	}
}
