package agentcore

import (
	"runtime/debug"
	"testing"
)

func TestDeriveAgentVersionFromBuildInfo(t *testing.T) {
	t.Run("uses semantic main version when available", func(t *testing.T) {
		got := deriveAgentVersionFromBuildInfo("v1.2.3", nil)
		if got != "v1.2.3" {
			t.Fatalf("deriveAgentVersionFromBuildInfo() = %q, want %q", got, "v1.2.3")
		}
	})

	t.Run("falls back to vcs revision", func(t *testing.T) {
		got := deriveAgentVersionFromBuildInfo("(devel)", []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
			{Key: "vcs.modified", Value: "false"},
		})
		if got != "git:0123456789ab" {
			t.Fatalf("deriveAgentVersionFromBuildInfo() = %q, want %q", got, "git:0123456789ab")
		}
	})

	t.Run("marks dirty vcs state", func(t *testing.T) {
		got := deriveAgentVersionFromBuildInfo("", []debug.BuildSetting{
			{Key: "vcs.revision", Value: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			{Key: "vcs.modified", Value: "true"},
		})
		if got != "git:aaaaaaaaaaaa-dirty" {
			t.Fatalf("deriveAgentVersionFromBuildInfo() = %q, want %q", got, "git:aaaaaaaaaaaa-dirty")
		}
	})

	t.Run("returns empty when no version data", func(t *testing.T) {
		got := deriveAgentVersionFromBuildInfo("(devel)", []debug.BuildSetting{})
		if got != "" {
			t.Fatalf("deriveAgentVersionFromBuildInfo() = %q, want empty", got)
		}
	})
}
