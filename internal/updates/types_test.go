package updates

import (
	"reflect"
	"testing"
)

func TestNormalizeExecutableScopes(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []string
		wantErr bool
	}{
		{name: "defaults to implemented scope", want: []string{ScopeOSPackages}},
		{name: "normalizes and deduplicates", input: []string{" OS_PACKAGES ", "os_packages"}, want: []string{ScopeOSPackages}},
		{name: "rejects roadmap scope", input: []string{ScopeDockerImage}, wantErr: true},
		{name: "rejects unknown scope", input: []string{"kernel_magic"}, wantErr: true},
		{name: "rejects empty entry", input: []string{" "}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeExecutableScopes(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("NormalizeExecutableScopes(%q) unexpectedly succeeded with %q", test.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeExecutableScopes(%q): %v", test.input, err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("NormalizeExecutableScopes(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestNormalizeTargets(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []string
		wantErr bool
	}{
		{name: "requires target", wantErr: true},
		{name: "normalizes and deduplicates", input: []string{" asset-a ", "asset-b", "asset-a"}, want: []string{"asset-a", "asset-b"}},
		{name: "rejects blank target", input: []string{"asset-a", " "}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeTargets(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("NormalizeTargets(%q) unexpectedly succeeded with %q", test.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeTargets(%q): %v", test.input, err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("NormalizeTargets(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestDefaultScopesAreExecutable(t *testing.T) {
	if _, err := NormalizeExecutableScopes(DefaultScopes); err != nil {
		t.Fatalf("DefaultScopes must remain executable: %v", err)
	}
	if len(DefaultScopes) != 1 || DefaultScopes[0] != ScopeOSPackages {
		t.Fatalf("DefaultScopes = %q, want only %q", DefaultScopes, ScopeOSPackages)
	}
}
