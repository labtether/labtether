package sysconfig

import "testing"

func TestNewNetworkBackend(t *testing.T) {
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
			backend := NewNetworkBackend(tc.goos)
			switch tc.want {
			case "linux":
				if _, ok := backend.(LinuxNetworkBackend); !ok {
					t.Fatalf("expected LinuxNetworkBackend, got %T", backend)
				}
			case "darwin":
				if _, ok := backend.(DarwinNetworkBackend); !ok {
					t.Fatalf("expected DarwinNetworkBackend, got %T", backend)
				}
			case "windows":
				if _, ok := backend.(WindowsNetworkBackend); !ok {
					t.Fatalf("expected WindowsNetworkBackend, got %T", backend)
				}
			case "unsupported":
				if _, ok := backend.(UnsupportedNetworkBackend); !ok {
					t.Fatalf("expected UnsupportedNetworkBackend, got %T", backend)
				}
			}
		})
	}
}
