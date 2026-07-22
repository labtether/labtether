package assetid

import "testing"

func TestScopeCollectorAssetIDIsStableUniqueAndReversible(t *testing.T) {
	t.Parallel()

	const nativeID = "proxmox-vm-101"
	first := ScopeCollectorAssetID(nativeID, "collector-proxmox-delta")
	second := ScopeCollectorAssetID(nativeID, "collector-proxmox-gamma")
	if first == second {
		t.Fatalf("different collectors produced the same asset ID: %q", first)
	}
	if got := ScopeCollectorAssetID(nativeID, "collector-proxmox-delta"); got != first {
		t.Fatalf("scope was not deterministic: got %q want %q", got, first)
	}
	if got := NativeCollectorAssetID(first); got != nativeID {
		t.Fatalf("native ID = %q, want %q", got, nativeID)
	}
	if got := ScopeCollectorAssetID(first, "collector-proxmox-delta"); got != first {
		t.Fatalf("scoping was not idempotent: got %q want %q", got, first)
	}
	if scope, ok := CollectorScopeFromAssetID(first); !ok || scope != CollectorScope("collector-proxmox-delta") {
		t.Fatalf("scope = %q, ok=%v", scope, ok)
	}
}

func TestNativeCollectorAssetIDRejectsLookalikeSuffix(t *testing.T) {
	t.Parallel()
	const lookalike = "asset--ltc-not-a-valid-scope"
	if got := NativeCollectorAssetID(lookalike); got != lookalike {
		t.Fatalf("lookalike suffix was stripped: %q", got)
	}
}
