package demo

import (
	"testing"
	"time"
)

func TestGenerateCPU_KnownAsset(t *testing.T) {
	now := time.Now()
	for i := range 100 {
		v := GenerateCPU("asset-proxmox-1", now.Add(time.Duration(i)*time.Second))
		if v < 5 || v > 85 {
			t.Fatalf("iteration %d: GenerateCPU(asset-proxmox-1) = %f, want [5,85]", i, v)
		}
	}
}

func TestGenerateCPU_UnknownAsset(t *testing.T) {
	now := time.Now()
	for i := range 100 {
		v := GenerateCPU("asset-nonexistent", now.Add(time.Duration(i)*time.Second))
		if v < 5 || v > 85 {
			t.Fatalf("iteration %d: GenerateCPU(asset-nonexistent) = %f, want [5,85]", i, v)
		}
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		name     string
		v        float64
		min, max float64
		want     float64
	}{
		{"below min", -5, 0, 100, 0},
		{"above max", 150, 0, 100, 100},
		{"in range", 50, 0, 100, 50},
		{"at min boundary", 0, 0, 100, 0},
		{"at max boundary", 100, 0, 100, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := clamp(tc.v, tc.min, tc.max)
			if got != tc.want {
				t.Errorf("clamp(%f, %f, %f) = %f, want %f", tc.v, tc.min, tc.max, got, tc.want)
			}
		})
	}
}

func TestRandomDuration(t *testing.T) {
	min := 2 * time.Minute
	max := 5 * time.Minute
	for i := range 100 {
		d := randomDuration(min, max)
		if d < min || d > max {
			t.Fatalf("iteration %d: randomDuration(%v, %v) = %v, want [%v,%v]", i, min, max, d, min, max)
		}
	}
}

func TestOnlineAssetIDsCount(t *testing.T) {
	ids := OnlineAssetIDs()
	if len(ids) != 8 {
		t.Fatalf("OnlineAssetIDs() returned %d entries, want 8", len(ids))
	}
}
