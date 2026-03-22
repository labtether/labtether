package backends

import (
	"runtime"
	"testing"
)

func TestNewLogBackend(t *testing.T) {
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
			backend := NewLogBackend(tc.goos)
			switch tc.want {
			case "linux":
				if _, ok := backend.(LinuxLogBackend); !ok {
					t.Fatalf("expected LinuxLogBackend, got %T", backend)
				}
			case "darwin":
				if runtime.GOOS == "darwin" {
					if _, ok := backend.(UnsupportedLogBackend); ok {
						t.Fatalf("expected darwin backend on darwin host, got %T", backend)
					}
					return
				}
				unsupported, ok := backend.(UnsupportedLogBackend)
				if !ok {
					t.Fatalf("expected UnsupportedLogBackend for darwin target on %s host, got %T", runtime.GOOS, backend)
				}
				if unsupported.OS != "darwin" {
					t.Fatalf("expected unsupported backend os marker darwin, got %q", unsupported.OS)
				}
			case "windows":
				if _, ok := backend.(WindowsLogBackend); !ok {
					t.Fatalf("expected WindowsLogBackend, got %T", backend)
				}
			case "unsupported":
				if _, ok := backend.(UnsupportedLogBackend); !ok {
					t.Fatalf("expected UnsupportedLogBackend, got %T", backend)
				}
			}
		})
	}
}
