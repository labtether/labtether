package backends

import "testing"

func TestNewServiceBackend(t *testing.T) {
	tests := []struct {
		name string
		goos string
		want string
	}{
		{name: "linux", goos: "linux", want: "linux"},
		{name: "darwin", goos: "darwin", want: "darwin"},
		{name: "windows", goos: "windows", want: "windows"},
		{name: "unsupported", goos: "plan9", want: "unsupported"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			backend := NewServiceBackend(tc.goos)
			switch tc.want {
			case "linux":
				if _, ok := backend.(LinuxServiceBackend); !ok {
					t.Fatalf("expected LinuxServiceBackend, got %T", backend)
				}
			case "darwin":
				if _, ok := backend.(DarwinServiceBackend); !ok {
					t.Fatalf("expected DarwinServiceBackend, got %T", backend)
				}
			case "windows":
				if _, ok := backend.(WindowsServiceBackend); !ok {
					t.Fatalf("expected WindowsServiceBackend, got %T", backend)
				}
			case "unsupported":
				if _, ok := backend.(UnsupportedServiceBackend); !ok {
					t.Fatalf("expected UnsupportedServiceBackend, got %T", backend)
				}
			}
		})
	}
}
