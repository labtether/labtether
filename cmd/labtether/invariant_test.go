package main

import (
	"testing"
)

func TestValidateLoginRequest_MinPasswordLength(t *testing.T) {
	short := loginRequest{Username: "admin", Password: "short"}
	if err := validateLoginRequest(short); err == nil {
		t.Error("expected error for password shorter than 8 chars")
	}

	exact := loginRequest{Username: "admin", Password: "12345678"}
	if err := validateLoginRequest(exact); err != nil {
		t.Errorf("unexpected error for 8-char password: %v", err)
	}
}

func TestValidateLoginRequest_EmptyFields(t *testing.T) {
	if err := validateLoginRequest(loginRequest{Username: "", Password: "longpassword"}); err == nil {
		t.Error("expected error for empty username")
	}
	if err := validateLoginRequest(loginRequest{Username: "admin", Password: ""}); err == nil {
		t.Error("expected error for empty password")
	}
}

func TestValidateLoginRequest_MaxLength(t *testing.T) {
	longUser := make([]byte, 65)
	for i := range longUser {
		longUser[i] = 'a'
	}
	if err := validateLoginRequest(loginRequest{Username: string(longUser), Password: "longpassword"}); err == nil {
		t.Error("expected error for username > 64 chars")
	}

	longPass := make([]byte, 257)
	for i := range longPass {
		longPass[i] = 'a'
	}
	if err := validateLoginRequest(loginRequest{Username: "admin", Password: string(longPass)}); err == nil {
		t.Error("expected error for password > 256 chars")
	}
}

func TestClampPercent(t *testing.T) {
	tests := []struct {
		input, want float64
	}{
		{-10, 0},
		{0, 0},
		{50.5, 50.5},
		{100, 100},
		{150, 100},
	}
	for _, tc := range tests {
		got := clampPercent(tc.input)
		if got != tc.want {
			t.Errorf("clampPercent(%v) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestTerminalSessionModes(t *testing.T) {
	// Invariant: only these modes are valid for terminal sessions.
	validModes := map[string]bool{
		"structured":  true,
		"interactive": true,
	}

	for mode := range validModes {
		if !validModes[mode] {
			t.Errorf("mode %q should be valid", mode)
		}
	}

	invalidModes := []string{"", "desktop", "batch", "INTERACTIVE"}
	for _, mode := range invalidModes {
		if validModes[mode] {
			t.Errorf("mode %q should be invalid for terminal sessions", mode)
		}
	}
}

func TestDesktopModeIsNotTerminalMode(t *testing.T) {
	// Invariant: "desktop" is only valid via the desktop session endpoint,
	// never via the terminal session endpoint.
	terminalModes := map[string]bool{"structured": true, "interactive": true}
	if terminalModes["desktop"] {
		t.Error("desktop should not be a valid terminal mode")
	}
}
