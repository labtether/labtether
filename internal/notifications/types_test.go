package notifications

import (
	"testing"
	"time"
)

func TestRetryBackoffBounds(t *testing.T) {
	testCases := []struct {
		name    string
		attempt int
		want    time.Duration
	}{
		{name: "negative attempt clamps to base", attempt: -3, want: 30 * time.Second},
		{name: "zero attempt uses base", attempt: 0, want: 30 * time.Second},
		{name: "first retry doubles base", attempt: 1, want: 60 * time.Second},
		{name: "second retry doubles again", attempt: 2, want: 120 * time.Second},
		{name: "fifth retry caps at max", attempt: 5, want: 10 * time.Minute},
		{name: "large attempt caps at max", attempt: 999, want: 10 * time.Minute},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := RetryBackoff(tc.attempt)
			if got != tc.want {
				t.Fatalf("RetryBackoff(%d) = %v, want %v", tc.attempt, got, tc.want)
			}
		})
	}
}
