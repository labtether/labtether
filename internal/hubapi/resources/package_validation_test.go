package resources

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeAndValidatePackageTokens(t *testing.T) {
	t.Parallel()

	got, err := normalizeAndValidatePackageTokens([]string{
		" curl ",
		"curl",
		"@scope/package@1.2.3",
		"homebrew/cask/google-chrome",
		"libssl3:amd64=2:3.0.2-1~deb12u1",
		"Microsoft.PowerShell",
	})
	if err != nil {
		t.Fatalf("normalize valid packages: %v", err)
	}
	want := []string{
		"curl",
		"@scope/package@1.2.3",
		"homebrew/cask/google-chrome",
		"libssl3:amd64=2:3.0.2-1~deb12u1",
		"Microsoft.PowerShell",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("packages = %#v, want %#v", got, want)
	}
}

func TestNormalizeAndValidatePackageTokensRejectsUnsafeValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "whitespace", value: "   "},
		{name: "apt option", value: "-o"},
		{name: "pacman option", value: "--config"},
		{name: "brew option", value: "--formula"},
		{name: "windows option", value: "--source"},
		{name: "newline", value: "bad\nname"},
		{name: "nul", value: "bad\x00name"},
		{name: "invalid character", value: "bad;name"},
		{name: "path traversal", value: "foo/../../bar"},
		{name: "overlong", value: strings.Repeat("a", maxPackageActionNameBytes+1)},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := normalizeAndValidatePackageTokens([]string{test.value}); err == nil {
				t.Fatalf("unsafe package %q was accepted", test.value)
			}
		})
	}
}

func TestNormalizeAndValidatePackageTokensRejectsExcessiveCount(t *testing.T) {
	t.Parallel()

	packages := make([]string, maxPackageActionCount+1)
	for index := range packages {
		packages[index] = "curl"
	}
	if _, err := normalizeAndValidatePackageTokens(packages); err == nil {
		t.Fatal("expected excessive package count error")
	}
}
