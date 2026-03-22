package backends

import (
	"reflect"
	"testing"
)

func TestBuildLinuxPackageActionCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		manager  string
		action   string
		packages []string
		want     []PackageActionCommand
		wantErr  bool
	}{
		{
			name:     "apt install refreshes metadata first",
			manager:  "apt-get",
			action:   "install",
			packages: []string{"gstreamer1.0-tools"},
			want: []PackageActionCommand{
				{Name: "apt-get", Args: []string{"update"}},
				{Name: "apt-get", Args: []string{"-y", "install", "gstreamer1.0-tools"}},
			},
		},
		{
			name:    "apt upgrade refreshes metadata first",
			manager: "apt-get",
			action:  "upgrade",
			want: []PackageActionCommand{
				{Name: "apt-get", Args: []string{"update"}},
				{Name: "apt-get", Args: []string{"-y", "upgrade"}},
			},
		},
		{
			name:     "apt remove does not refresh metadata first",
			manager:  "apt-get",
			action:   "remove",
			packages: []string{"xdotool"},
			want: []PackageActionCommand{
				{Name: "apt-get", Args: []string{"-y", "remove", "xdotool"}},
			},
		},
		{
			name:     "yum install stays single command",
			manager:  "yum",
			action:   "install",
			packages: []string{"xdotool"},
			want: []PackageActionCommand{
				{Name: "yum", Args: []string{"-y", "install", "xdotool"}},
			},
		},
		{
			name:    "unsupported manager returns error",
			manager: "pkg",
			action:  "install",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := BuildLinuxPackageActionCommands(tc.manager, tc.action, tc.packages)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("commands mismatch:\n got: %#v\nwant: %#v", got, tc.want)
			}
		})
	}
}
