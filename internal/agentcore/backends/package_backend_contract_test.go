package backends

import (
	"runtime"
	"testing"
)

func TestNewPackageBackend(t *testing.T) {
	tests := []struct {
		name string
		goos string
		want string
	}{
		{name: "linux", goos: "linux", want: "linux"},
		{name: "darwin", goos: "darwin", want: "darwin"},
		{name: "windows", goos: "windows", want: "windows"},
		{name: "unsupported", goos: "freebsd", want: "unsupported"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			backend := NewPackageBackend(tc.goos)
			switch tc.want {
			case "linux":
				if _, ok := backend.(LinuxPackageBackend); !ok {
					t.Fatalf("expected LinuxPackageBackend, got %T", backend)
				}
			case "darwin":
				if runtime.GOOS == "darwin" {
					if _, ok := backend.(UnsupportedPackageBackend); ok {
						t.Fatalf("expected darwin backend on darwin host, got %T", backend)
					}
					return
				}
				unsupported, ok := backend.(UnsupportedPackageBackend)
				if !ok {
					t.Fatalf("expected UnsupportedPackageBackend for darwin target on %s host, got %T", runtime.GOOS, backend)
				}
				if unsupported.OS != "darwin" {
					t.Fatalf("expected unsupported backend os marker darwin, got %q", unsupported.OS)
				}
			case "windows":
				wb, ok := backend.(WindowsPackageBackend)
				if !ok {
					t.Fatalf("expected WindowsPackageBackend, got %T", backend)
				}
				if wb.backend != "winget" {
					t.Fatalf("expected default backend winget, got %q", wb.backend)
				}
			case "unsupported":
				if _, ok := backend.(UnsupportedPackageBackend); !ok {
					t.Fatalf("expected UnsupportedPackageBackend, got %T", backend)
				}
			}
		})
	}
}
