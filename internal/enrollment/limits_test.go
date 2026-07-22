package enrollment

import "testing"

func TestBoundedLimitUsesSecureDefaultsAndHardMaximum(t *testing.T) {
	tests := []struct {
		name     string
		value    int
		fallback int
		hardMax  int
		want     int
	}{
		{name: "omitted", value: 0, fallback: 10, hardMax: 100, want: 10},
		{name: "negative", value: -1, fallback: 10, hardMax: 100, want: 10},
		{name: "configured", value: 25, fallback: 10, hardMax: 100, want: 25},
		{name: "hard clamp", value: 101, fallback: 10, hardMax: 100, want: 100},
		{name: "invalid fallback", value: 0, fallback: 101, hardMax: 100, want: 100},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := BoundedLimit(test.value, test.fallback, test.hardMax); got != test.want {
				t.Fatalf("BoundedLimit(%d, %d, %d)=%d, want %d", test.value, test.fallback, test.hardMax, got, test.want)
			}
		})
	}
}

func TestNormalizeRequestedTokenMaxUses(t *testing.T) {
	if got, err := NormalizeRequestedTokenMaxUses(0, 10); err != nil || got != DefaultTokenMaxUses {
		t.Fatalf("omitted max_uses got=%d err=%v", got, err)
	}
	if got, err := NormalizeRequestedTokenMaxUses(10, 10); err != nil || got != 10 {
		t.Fatalf("configured ceiling got=%d err=%v", got, err)
	}
	for _, value := range []int{-1, 11} {
		if _, err := NormalizeRequestedTokenMaxUses(value, 10); err == nil {
			t.Fatalf("max_uses=%d unexpectedly accepted", value)
		}
	}
	if _, err := NormalizeRequestedTokenMaxUses(HardTokenMaxUsesCeiling+1, HardTokenMaxUsesCeiling+100); err == nil {
		t.Fatal("value above the persistence hard maximum unexpectedly accepted")
	}
}

func TestValidateStoredTokenMaxUses(t *testing.T) {
	for _, value := range []int{1, HardTokenMaxUsesCeiling} {
		if err := ValidateStoredTokenMaxUses(value); err != nil {
			t.Fatalf("max_uses=%d rejected: %v", value, err)
		}
	}
	for _, value := range []int{0, -1, HardTokenMaxUsesCeiling + 1} {
		if err := ValidateStoredTokenMaxUses(value); err == nil {
			t.Fatalf("max_uses=%d unexpectedly accepted", value)
		}
	}
}
