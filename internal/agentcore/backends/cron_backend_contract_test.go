package backends

import "testing"

func TestNewCronBackend(t *testing.T) {
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
			backend := NewCronBackend(tc.goos)
			switch tc.want {
			case "linux":
				if _, ok := backend.(LinuxCronBackend); !ok {
					t.Fatalf("expected LinuxCronBackend, got %T", backend)
				}
			case "darwin":
				if _, ok := backend.(DarwinCronBackend); !ok {
					t.Fatalf("expected DarwinCronBackend, got %T", backend)
				}
			case "windows":
				if _, ok := backend.(WindowsCronBackend); !ok {
					t.Fatalf("expected WindowsCronBackend, got %T", backend)
				}
			case "unsupported":
				if _, ok := backend.(UnsupportedCronBackend); !ok {
					t.Fatalf("expected UnsupportedCronBackend, got %T", backend)
				}
			}
		})
	}
}
