package platforms

import (
	"runtime"
	"strings"
)

const (
	Linux   = "linux"
	Windows = "windows"
	Darwin  = "darwin"
	FreeBSD = "freebsd"
)

var supportedPlatforms = []string{Linux, Windows, Darwin, FreeBSD}

var compactAliasMap = map[string]string{
	"linux":      Linux,
	"gnulinux":   Linux,
	"ubuntu":     Linux,
	"debian":     Linux,
	"raspbian":   Linux,
	"fedora":     Linux,
	"centos":     Linux,
	"redhat":     Linux,
	"rhel":       Linux,
	"rockylinux": Linux,
	"almalinux":  Linux,
	"alpine":     Linux,
	"arch":       Linux,
	"manjaro":    Linux,
	"opensuse":   Linux,
	"suse":       Linux,
	"gentoo":     Linux,
	"nixos":      Linux,

	"windows":   Windows,
	"win":       Windows,
	"win32":     Windows,
	"win64":     Windows,
	"windowsnt": Windows,

	"darwin": Darwin,
	"mac":    Darwin,
	"macos":  Darwin,
	"osx":    Darwin,
	"macosx": Darwin,
	"apple":  Darwin,

	"freebsd": FreeBSD,
}

var compactReplacer = strings.NewReplacer(
	" ", "",
	"-", "",
	"_", "",
	".", "",
	"/", "",
	"\\", "",
	",", "",
	"(", "",
	")", "",
	":", "",
	";", "",
	"'", "",
	"\"", "",
	"!", "",
)

// Supported returns canonical platform identifiers used across the codebase.
func Supported() []string {
	out := make([]string, len(supportedPlatforms))
	copy(out, supportedPlatforms)
	return out
}

// Current returns the canonical platform identifier for the current runtime OS.
func Current() string {
	return Normalize(runtime.GOOS)
}

// IsSupported returns true when the value resolves to a canonical platform.
func IsSupported(raw string) bool {
	switch Normalize(raw) {
	case Linux, Windows, Darwin, FreeBSD:
		return true
	default:
		return false
	}
}

// Resolve normalizes and returns the first non-empty platform value.
func Resolve(values ...string) string {
	for _, raw := range values {
		if normalized := Normalize(raw); normalized != "" {
			return normalized
		}
	}
	return ""
}

// Normalize maps platform labels and common aliases to canonical values.
func Normalize(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return ""
	}

	if canonical, ok := compactAliasMap[compactKey(normalized)]; ok {
		return canonical
	}

	switch {
	case strings.Contains(normalized, "linux"),
		containsAny(normalized,
			"ubuntu",
			"debian",
			"raspbian",
			"fedora",
			"centos",
			"red hat",
			"rhel",
			"rocky",
			"alma",
			"alpine",
			"arch",
			"manjaro",
			"suse",
			"nixos",
			"gentoo",
		):
		return Linux
	case strings.Contains(normalized, "windows"):
		return Windows
	case strings.Contains(normalized, "darwin"),
		strings.Contains(normalized, "mac"),
		strings.Contains(normalized, "os x"):
		return Darwin
	case strings.Contains(normalized, "freebsd"):
		return FreeBSD
	default:
		return normalized
	}
}

func compactKey(raw string) string {
	return compactReplacer.Replace(strings.ToLower(strings.TrimSpace(raw)))
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
