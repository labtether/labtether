package main

import (
	"strings"
	"testing"
)

func TestSanitizeConnectorErrorMessage(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		secrets       []string
		expectContain string
		expectAbsent  string
	}{
		{
			name:          "redacts token secret kv",
			input:         `pbs api returned 502: {"token_secret":"super-secret"}`,
			expectContain: redactedConnectorSecret,
			expectAbsent:  "super-secret",
		},
		{
			name:          "redacts authorization header",
			input:         "authorization: Bearer abc.def.ghi",
			expectContain: redactedConnectorSecret,
			expectAbsent:  "abc.def.ghi",
		},
		{
			name:          "redacts pve api token suffix",
			input:         "PVEAPIToken=labtether@pve!agent=top-secret",
			expectContain: redactedConnectorSecret,
			expectAbsent:  "top-secret",
		},
		{
			name:          "redacts url credentials",
			input:         "dial https://admin:passw0rd@host.local failed",
			expectContain: redactedConnectorSecret,
			expectAbsent:  "passw0rd",
		},
		{
			name:          "redacts explicit secrets",
			input:         "upstream echoed token value my-inline-secret",
			secrets:       []string{"my-inline-secret"},
			expectContain: redactedConnectorSecret,
			expectAbsent:  "my-inline-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitized := sanitizeConnectorErrorMessage(tt.input, tt.secrets...)
			if !strings.Contains(sanitized, tt.expectContain) {
				t.Fatalf("expected sanitized output to contain %q, got %q", tt.expectContain, sanitized)
			}
			if strings.Contains(sanitized, tt.expectAbsent) {
				t.Fatalf("expected sanitized output to exclude %q, got %q", tt.expectAbsent, sanitized)
			}
		})
	}
}
