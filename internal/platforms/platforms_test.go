package platforms

import "testing"

func TestNormalize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "linux canonical", in: "linux", want: Linux},
		{name: "linux distro", in: "Ubuntu", want: Linux},
		{name: "linux distro with version", in: "Ubuntu 24.04 LTS", want: Linux},
		{name: "windows canonical", in: "windows", want: Windows},
		{name: "windows alias", in: "Windows NT", want: Windows},
		{name: "darwin canonical", in: "darwin", want: Darwin},
		{name: "darwin alias", in: "macOS Sonoma", want: Darwin},
		{name: "freebsd canonical", in: "freebsd", want: FreeBSD},
		{name: "freebsd with version", in: "FreeBSD 14.1", want: FreeBSD},
		{name: "unknown preserved", in: "openbsd", want: "openbsd"},
		{name: "empty", in: " ", want: ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Normalize(tc.in); got != tc.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()

	if got := Resolve("", "Ubuntu 24.04", "windows"); got != Linux {
		t.Fatalf("Resolve returned %q, want %q", got, Linux)
	}
	if got := Resolve("", " ", "Windows 11"); got != Windows {
		t.Fatalf("Resolve returned %q, want %q", got, Windows)
	}
	if got := Resolve("", " "); got != "" {
		t.Fatalf("Resolve returned %q, want empty string", got)
	}
}

func TestIsSupported(t *testing.T) {
	t.Parallel()

	if !IsSupported("macOS") {
		t.Fatalf("expected macOS alias to be supported")
	}
	if IsSupported("openbsd") {
		t.Fatalf("expected openbsd to be unsupported")
	}
}
